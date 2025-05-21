package pages

import (
	"errors"
	"fmt"
	"image/color"
	"path/filepath" // Para filepath.Base
	"strings"
	"time" // Para timeout em mensagens de status

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	// Para diálogos de arquivo nativos (exemplo, requer biblioteca externa ou implementação específica da plataforma)
	// "github.com/sqweek/dialog"

	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/auth"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core"
	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/services"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui/components"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui/theme"
)

// ImportTypeConfig define a configuração para cada tipo de importação.
type ImportTypeConfig struct {
	ID          services.FileType // Ex: services.FileTypeDireitos
	Title       string            // Título para exibição na UI (ex: "Movimento de Títulos - Direitos")
	AllowedExts []string          // Extensões de arquivo permitidas (ex: ".txt", ".csv")
	// Description string         // Descrição opcional sobre o formato do arquivo
}

// ImportSectionState armazena o estado para um tipo de importação específico (uma seção/card na UI).
type ImportSectionState struct {
	Config           ImportTypeConfig // Configuração do tipo de importação desta seção
	LastUpdateText   string           // Texto formatado da última atualização (ex: "Última atualização: 01/01/2024...")
	SelectedFilePath string           // Caminho completo do arquivo selecionado pelo usuário
	SelectedFileName string           // Apenas o nome do arquivo para exibição na UI

	SelectFileBtn widget.Clickable // Botão para abrir o diálogo de seleção de arquivo
	ImportBtn     widget.Clickable // Botão para iniciar a importação do arquivo selecionado

	IsImporting   bool        // True se este tipo específico estiver sendo importado no momento
	StatusMessage string      // Mensagem de status específica para esta seção (ex: "Importando...", "Sucesso!")
	MessageColor  color.NRGBA // Cor da StatusMessage (ex: verde para sucesso, vermelho para erro)
}

// ImportPage gerencia a UI para importação de arquivos de dados.
type ImportPage struct {
	router         *ui.Router
	cfg            *core.Config
	importService  services.ImportService
	permManager    *auth.PermissionManager
	sessionManager *auth.SessionManager

	// Cada tipo de importação terá sua própria seção e estado.
	// Usar um slice aqui para manter a ordem de exibição na UI.
	importSections []*ImportSectionState

	// Estado global da página (para carregamento inicial de status ou mensagens gerais)
	isLoadingGlobal     bool // True se estiver carregando o status de todas as importações
	statusMessageGlobal string
	messageColorGlobal  color.NRGBA

	refreshStatusBtn widget.Clickable // Botão para recarregar o status de todas as importações
	// closeBtn não é mais necessário aqui se for um módulo dentro de MainAppLayout

	spinner *components.LoadingSpinner // Spinner de carregamento global para a página

	firstLoadDone bool // Controla o carregamento inicial de dados
}

// NewImportPage cria uma nova instância da página de importação.
func NewImportPage(
	router *ui.Router,
	cfg *core.Config,
	importSvc services.ImportService,
	permMan *auth.PermissionManager,
	sessMan *auth.SessionManager,
) *ImportPage {
	p := &ImportPage{
		router:         router,
		cfg:            cfg,
		importService:  importSvc,
		permManager:    permMan,
		sessionManager: sessMan,
		spinner:        components.NewLoadingSpinner(theme.Colors.Primary),
	}

	// Define as configurações para cada tipo de importação que a página suportará.
	// A ordem aqui define a ordem de exibição na UI.
	supportedImportTypes := []ImportTypeConfig{
		{ID: services.FileTypeDireitos, Title: "Importar Títulos de Direitos", AllowedExts: []string{".txt", ".csv"}},
		{ID: services.FileTypeObrigacoes, Title: "Importar Títulos de Obrigações", AllowedExts: []string{".txt", ".csv"}},
		// Adicionar outros tipos de importação aqui conforme necessário.
	}

	p.importSections = make([]*ImportSectionState, 0, len(supportedImportTypes))
	for _, importCfg := range supportedImportTypes {
		p.importSections = append(p.importSections, &ImportSectionState{
			Config:         importCfg,
			LastUpdateText: "Última atualização: <i>Carregando...</i>", // Placeholder inicial
		})
	}
	return p
}

// OnNavigatedTo é chamado quando a página se torna ativa.
func (p *ImportPage) OnNavigatedTo(params interface{}) {
	appLogger.Info("Navegou para ImportPage")
	p.statusMessageGlobal = "" // Limpa mensagem global da página

	currentSession, errSess := p.sessionManager.GetCurrentSession()
	if errSess != nil || currentSession == nil {
		p.router.GetAppWindow().HandleLogout() // Força logout se sessão inválida
		return
	}
	// Verifica permissão para acessar a funcionalidade de importação.
	// PermImportExecute é para realizar a importação.
	// PermImportViewStatus é para ver o status.
	// Se não tiver PermImportViewStatus, pode não carregar os status.
	if err := p.permManager.CheckPermission(currentSession, auth.PermImportViewStatus, nil); err != nil {
		p.statusMessageGlobal = fmt.Sprintf("Acesso negado à visualização de status de importação: %v", err)
		p.messageColorGlobal = theme.Colors.Danger
		p.clearAllSectionStatus() // Limpa os status se não tem permissão para vê-los.
		p.router.GetAppWindow().Invalidate()
		return
	}


	if !p.firstLoadDone {
		p.loadAllImportStatuses(currentSession)
		p.firstLoadDone = true
	} else {
		// Recarrega os status ao revisitar a página para ter dados frescos.
		p.loadAllImportStatuses(currentSession)
	}
}

// OnNavigatedFrom é chamado quando o router navega para fora desta página.
func (p *ImportPage) OnNavigatedFrom() {
	appLogger.Info("Navegando para fora da ImportPage")
	// Para qualquer importação em andamento ou spinners.
	for _, section := range p.importSections {
		section.IsImporting = false // Cancela a flag de importação (a goroutine pode continuar, mas a UI não mostrará)
	}
	p.isLoadingGlobal = false
	p.spinner.Stop(p.router.GetAppWindow().Context())
}

// loadAllImportStatuses carrega o status de todas as importações definidas.
func (p *ImportPage) loadAllImportStatuses(currentSession *auth.SessionData) {
	if p.isLoadingGlobal {
		return
	}
	p.isLoadingGlobal = true
	p.statusMessageGlobal = "Carregando status das importações..."
	p.messageColorGlobal = theme.Colors.TextMuted
	p.spinner.Start(p.router.GetAppWindow().Context())
	p.router.GetAppWindow().Invalidate()

	go func(sess *auth.SessionData) {
		var overallErr error
		// Tenta carregar todos os status.
		allStatuses, err := s.importService.GetAllImportStatus(sess)
		if err != nil {
			overallErr = fmt.Errorf("falha ao carregar status de todas as importações: %w", err)
		}

		p.router.GetAppWindow().Execute(func() { // Atualiza UI na thread principal.
			p.isLoadingGlobal = false
			p.spinner.Stop(p.router.GetAppWindow().Context())
			if overallErr != nil {
				p.statusMessageGlobal = overallErr.Error()
				p.messageColorGlobal = theme.Colors.Danger
				appLogger.Errorf("Erro ao carregar todos os status para ImportPage: %v", overallErr)
				p.clearAllSectionStatus() // Limpa em caso de erro.
			} else {
				p.statusMessageGlobal = "Status das importações carregado."
				p.messageColorGlobal = theme.Colors.Success
				p.updateSectionsWithStatus(allStatuses)
			}
			p.router.GetAppWindow().Invalidate()
		})
	}(currentSession)
}

// updateSectionsWithStatus atualiza o texto de LastUpdateText para cada seção.
func (p *ImportPage) updateSectionsWithStatus(allStatuses []models.ImportMetadataPublic) {
	statusMap := make(map[services.FileType]models.ImportMetadataPublic)
	for _, status := range allStatuses {
		statusMap[services.FileType(strings.ToUpper(status.FileType))] = status
	}

	for _, section := range p.importSections {
		if status, found := statusMap[section.Config.ID]; found {
			updateTimeStr := "Nunca"
			if !status.LastUpdatedAt.IsZero() {
				updateTimeStr = status.LastUpdatedAt.Local().Format("02/01/2006 15:04:05")
			}
			fileNameStr := ""
			if status.OriginalFilename != nil && *status.OriginalFilename != "" {
				fileNameStr = fmt.Sprintf(" (Arquivo: %s)", *status.OriginalFilename)
			}
			countStr := ""
			if status.RecordCount != nil {
				countStr = fmt.Sprintf(" %d registros.", *status.RecordCount)
			}
			// Usar \n para quebras de linha em vez de HTML, pois Label não renderiza HTML.
			section.LastUpdateText = fmt.Sprintf("Última atualização: %s%s\n%s", updateTimeStr, countStr, fileNameStr)
		} else {
			section.LastUpdateText = "Última atualização: Nunca realizado."
		}
		// Limpa status de importação anterior da seção
		section.StatusMessage = ""
	}
}

// clearAllSectionStatus limpa o texto de status de todas as seções.
func (p *ImportPage) clearAllSectionStatus() {
	for _, section := range p.importSections {
		section.LastUpdateText = "Última atualização: Status indisponível."
		section.StatusMessage = ""
	}
}

// Layout é o método principal de desenho da página.
func (p *ImportPage) Layout(gtx layout.Context) layout.Dimensions {
	th := p.router.GetAppWindow().Theme()
	currentSession, _ := p.sessionManager.GetCurrentSession() // Para verificações de permissão

	// Processar cliques nos botões (Seleção de arquivo e Importação)
	for _, section := range p.importSections {
		// Captura variáveis de loop para uso em closures (importante!)
		currentSection := section

		if currentSection.SelectFileBtn.Clicked(gtx) {
			p.handleSelectFile(currentSection)
		}
		if currentSection.ImportBtn.Clicked(gtx) {
			p.handleImportFile(currentSection, currentSession)
		}
	}
	if p.refreshStatusBtn.Clicked(gtx) && !p.isLoadingGlobal {
		p.loadAllImportStatuses(currentSession)
	}


	return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceEnd}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { // Título da Página e Botão de Atualizar Status
			return layout.Flex{Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					title := material.H6(th, "Importação de Arquivos de Movimento")
					title.Font.Weight = font.Bold
					return layout.Inset{Bottom: theme.LargeVSpacer}.Layout(gtx, title.Layout)
				}),
				layout.Rigid(material.Button(th, &p.refreshStatusBtn, "Atualizar Status").Layout),
			)
		}),

		// Lista de cards de importação (um para cada tipo de arquivo)
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			list := layout.List{Axis: layout.Vertical}
			return list.Layout(gtx, len(p.importSections), func(gtx layout.Context, index int) layout.Dimensions {
				return layout.Inset{Bottom: theme.DefaultVSpacer}.Layout(gtx,
					p.layoutImportSectionCard(gtx, th, p.importSections[index], currentSession),
				)
			})
		}),

		// Mensagem de Status Global da Página
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.statusMessageGlobal != "" && !p.isLoadingGlobal {
				lbl := material.Body2(th, p.statusMessageGlobal)
				lbl.Color = p.messageColorGlobal
				return layout.Inset{Top: theme.DefaultVSpacer}.Layout(gtx, lbl.Layout)
			}
			return layout.Dimensions{}
		}),
		// Spinner Global (se isLoadingGlobal ou alguma seção estiver importando)
		// Deve ser sobreposto usando layout.Stack na AppWindow ou no topo deste layout.
		// Se (p.isLoadingGlobal || p.anySectionIsImporting()) { return p.spinner.Layout(gtx) }
	)
}

// layoutImportSectionCard desenha um card individual para um tipo de importação.
func (p *ImportPage) layoutImportSectionCard(gtx layout.Context, th *material.Theme, section *ImportSectionState, currentSession *auth.SessionData) layout.Dimensions {
	cardTitle := material.Subtitle1(th, section.Config.Title)
	cardTitle.Font.Weight = font.SemiBold

	// Verifica permissão para executar a importação (para habilitar/desabilitar botão Importar)
	canExecuteImport, _ := p.permManager.HasPermission(currentSession, auth.PermImportExecute, nil)

	return material.Card(th, theme.Colors.Surface, theme.ElevationSmall, layout.UniformInset(unit.Dp(16)),
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceSides}.Layout(gtx,
				layout.Rigid(cardTitle.Layout),
				layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { // LastUpdateText
					// Como Label não renderiza HTML, removemos as tags para exibição.
					// Uma solução mais elegante seria usar um widget de texto rico ou formatar no Go.
					plainText := strings.ReplaceAll(strings.ReplaceAll(section.LastUpdateText, "<b>", ""), "</b>", "")
					plainText = strings.ReplaceAll(strings.ReplaceAll(plainText, "<i>", ""), "</i>", "")
					return material.Body2(th, plainText).Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Height: theme.DefaultVSpacer}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { // Linha de Ação (Nome do Arquivo, Selecionar, Importar)
					return layout.Flex{Alignment: layout.Middle, Spacing: layout.SpaceEnd}.Layout(gtx,
						layout.Rigid(material.Body2(th, "Arquivo:").Layout),
						layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { // Nome do arquivo selecionado
							fileNameToDisplay := section.SelectedFileName
							if fileNameToDisplay == "" { fileNameToDisplay = "Nenhum arquivo selecionado" }
							
							label := material.Body2(th, fileNameToDisplay)
							if section.SelectedFileName == "" { label.Color = theme.Colors.TextMuted }

							// Adiciona uma borda para parecer um campo de texto readonly.
							border := widget.Border{Color: theme.Colors.Border, CornerRadius: theme.CornerRadius, Width: theme.BorderWidthDefault}
							return border.Layout(gtx, func(gtx C) D {
								return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(8), Top: unit.Dp(6), Bottom: unit.Dp(6)}.Layout(gtx, label.Layout)
							})
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
						layout.Rigid(func(gtx C) D { // Botão Selecionar Arquivo
							btn := material.Button(th, §ion.SelectFileBtn, "Selecionar...")
							if section.IsImporting || p.isLoadingGlobal || !canExecuteImport { // Desabilita se importando, carregando globalmente ou sem permissão
								btn.Style.TextColor = theme.Colors.TextMuted
								btn.Style.Background = theme.Colors.Grey300
							}
							return btn.Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions { // Botão Importar
							importButton := material.Button(th, §ion.ImportBtn, "Importar Arquivo")
							if section.SelectedFilePath == "" || section.IsImporting || p.isLoadingGlobal || !canExecuteImport {
								importButton.Style.TextColor = theme.Colors.TextMuted
								importButton.Style.Background = theme.Colors.Grey300
							} else {
								importButton.Background = theme.Colors.Primary
							}
							return importButton.Layout(gtx)
						}),
					)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { // Status específico da seção
					if section.StatusMessage != "" {
						lbl := material.Body2(th, section.StatusMessage)
						lbl.Color = section.MessageColor
						return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, lbl.Layout)
					}
					return layout.Dimensions{}
				}),
				layout.Rigid(func(gtx C)D { // Spinner local à seção
				    if section.IsImporting {
				        return layout.Center.Layout(gtx, p.spinner.Layout) // Reutiliza spinner global ou um específico da seção
				    }
				    return D{}
				}),
			)
		}).Layout(gtx)
}

// handleSelectFile abre o diálogo de seleção de arquivo para a seção especificada.
func (p *ImportPage) handleSelectFile(section *ImportSectionState) {
	if section.IsImporting || p.isLoadingGlobal { return } // Não permite selecionar se já estiver ocupado

	section.StatusMessage = ""      // Limpa mensagem de status anterior da seção
	p.statusMessageGlobal = ""      // Limpa mensagem global
	p.router.GetAppWindow().Invalidate()

	// Implementação do diálogo de seleção de arquivo é específica da plataforma
	// ou usa uma biblioteca externa.
	// Exemplo usando uma biblioteca hipotética `filedialog`:
	go func(sec *ImportSectionState) {
		// allowedExts := sec.Config.AllowedExts // Ex: []string{"*.txt", "*.csv"}
		// title := "Selecionar Arquivo para " + sec.Config.Title
		// filePath, err := filedialog.OpenFile(title, "", allowedExts)

		// Simulação para teste:
		time.Sleep(50 * time.Millisecond) // Simula pequena espera do diálogo
		filePath := "/simulado/caminho/para/" + strings.ToLower(string(sec.Config.ID)) + ".txt"
		var err error // = nil para sucesso simulado

		p.router.GetAppWindow().Execute(func() {
			if err != nil {
				// if errors.Is(err, filedialog.ErrCancelled) {
				// 	appLogger.Debugf("Seleção de arquivo cancelada para %s", sec.Config.Title)
				// 	return
				// }
				sec.StatusMessage = fmt.Sprintf("Erro ao selecionar arquivo: %v", err)
				sec.MessageColor = theme.Colors.Danger
				appLogger.Errorf("Erro no diálogo de arquivo para %s: %v", sec.Config.Title, err)
			} else if filePath != "" {
				sec.SelectedFilePath = filePath
				sec.SelectedFileName = filepath.Base(filePath) // Extrai apenas o nome do arquivo
				sec.StatusMessage = fmt.Sprintf("Arquivo '%s' selecionado.", sec.SelectedFileName)
				sec.MessageColor = theme.Colors.Info
			} else {
				// Nenhuma seleção ou diálogo cancelado
				sec.SelectedFilePath = ""
				sec.SelectedFileName = ""
				// Não mostrar mensagem se apenas cancelou
			}
			p.router.GetAppWindow().Invalidate()
		})
	}(section)
}

// handleImportFile inicia o processo de importação para o arquivo selecionado na seção.
func (p *ImportPage) handleImportFile(section *ImportSectionState, currentSession *auth.SessionData) {
	if section.SelectedFilePath == "" || section.IsImporting || p.isLoadingGlobal {
		return // Não faz nada se nenhum arquivo selecionado ou já importando/carregando
	}
	// Permissão já verificada para habilitar o botão, mas checar novamente é seguro
	if errPerm := p.permManager.CheckPermission(currentSession, auth.PermImportExecute, nil); errPerm != nil {
		section.StatusMessage = "Você não tem permissão para executar importações."
		section.MessageColor = theme.Colors.Danger
		p.router.GetAppWindow().Invalidate()
		return
	}


	// Idealmente, mostrar um diálogo de confirmação: "Tem certeza que deseja importar X, substituindo dados existentes?"
	// Por agora, prossegue diretamente.

	section.IsImporting = true
	section.StatusMessage = "Importando arquivo, por favor aguarde..."
	section.MessageColor = theme.Colors.TextMuted
	p.spinner.Start(p.router.GetAppWindow().Context()) // Spinner global ou local
	p.router.GetAppWindow().Invalidate()

	filePathToImport := section.SelectedFilePath // Copia para a goroutine

	go func(sec *ImportSectionState, fp string, sess *auth.SessionData) {
		var importResult map[string]interface{}
		var importErr error

		importResult, importErr = s.importService.ImportFile(fp, sec.Config.ID, sess)

		p.router.GetAppWindow().Execute(func() {
			sec.IsImporting = false // Atualiza o estado da seção específica
			// Parar o spinner global se nenhuma outra seção estiver importando E não houver carregamento global
			if !p.anySectionIsImporting() && !p.isLoadingGlobal {
				p.spinner.Stop(p.router.GetAppWindow().Context())
			}

			if importErr != nil {
				errMsg := fmt.Sprintf("Falha na importação: %v", importErr)
				// Tenta extrair mensagem mais amigável de ValidationError
				var valErr *appErrors.ValidationError
				if errors.As(importErr, &valErr) {
					errMsg = fmt.Sprintf("Falha na importação: %s", valErr.Message)
					if len(valErr.Fields) > 0 {
						errMsg += fmt.Sprintf(" (Detalhes: %v)", valErr.Fields)
					}
				}
				sec.StatusMessage = errMsg
				sec.MessageColor = theme.Colors.Danger
				appLogger.Errorf("Erro ao importar arquivo tipo %s (%s): %v", sec.Config.ID, fp, importErr)
			} else {
				processed := 0
				if proc, ok := importResult["records_processed"].(int); ok { processed = proc }
				skippedParse := 0
				if skipP, ok := importResult["records_skipped_parsing"].(int); ok { skippedParse = skipP }
				skippedRepo := 0
				if skipR, ok := importResult["records_skipped_repo"].(int); ok { skippedRepo = skipR }

				sec.StatusMessage = fmt.Sprintf("Importação concluída! %d registros processados. %d pulados (parsing), %d pulados (repositório).",
					processed, skippedParse, skippedRepo)
				sec.MessageColor = theme.Colors.Success
				appLogger.Infof("Arquivo tipo %s (%s) importado. Processados: %d, Pulados Parsing: %d, Pulados Repo: %d.",
					sec.Config.ID, fp, processed, skippedParse, skippedRepo)
				
				// Atualiza o "Última atualização" para esta seção.
				p.updateSpecificSectionStatus(sec, sess)
			}
			// Limpar seleção de arquivo após tentativa de importação.
			sec.SelectedFilePath = ""
			sec.SelectedFileName = ""
			p.router.GetAppWindow().Invalidate()
		})
	}(section, filePathToImport, currentSession)
}

// updateSpecificSectionStatus busca e atualiza o label de "Última atualização" para uma seção.
func (p *ImportPage) updateSpecificSectionStatus(section *ImportSectionState, currentSession *auth.SessionData) {
	fileTypeID := section.Config.ID
	go func(ft services.FileType, secState *ImportSectionState, sess *auth.SessionData) {
		var newStatusText string
		status, err := s.importService.GetImportStatus(ft, sess)
		if err != nil {
			newStatusText = "Última atualização: Erro ao buscar status."
			appLogger.Errorf("Erro ao buscar status de importação para %s após import: %v", ft, err)
		} else if status != nil {
			updateTimeStr := "Nunca"
			if !status.LastUpdatedAt.IsZero() {
				updateTimeStr = status.LastUpdatedAt.Local().Format("02/01/2006 15:04:05")
			}
			fileNameStr := ""
			if status.OriginalFilename != nil && *status.OriginalFilename != "" {
				fileNameStr = fmt.Sprintf(" (Arquivo: %s)", *status.OriginalFilename)
			}
			countStr := ""
			if status.RecordCount != nil {
				countStr = fmt.Sprintf(" %d registros.", *status.RecordCount)
			}
			newStatusText = fmt.Sprintf("Última atualização: %s%s\n%s", updateTimeStr, countStr, fileNameStr)
		} else {
			newStatusText = "Última atualização: Não encontrado."
		}

		p.router.GetAppWindow().Execute(func() {
			secState.LastUpdateText = newStatusText
			p.router.GetAppWindow().Invalidate()
		})
	}(fileTypeID, section, currentSession)
}

// anySectionIsImporting verifica se alguma das seções de importação está atualmente em processo.
func (p *ImportPage) anySectionIsImporting() bool {
	for _, section := range p.importSections {
		if section.IsImporting {
			return true
		}
	}
	return false
}