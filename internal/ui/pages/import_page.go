package pages

import (
	"fmt"
	"image/color"
	"strings"

	// "os" // Para simular seleção de arquivo se necessário

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/seu_usuario/riograndense_gio/internal/auth"
	"github.com/seu_usuario/riograndense_gio/internal/core"
	appLogger "github.com/seu_usuario/riograndense_gio/internal/core/logger"
	"github.com/seu_usuario/riograndense_gio/internal/services"
	"github.com/seu_usuario/riograndense_gio/internal/ui"
	"github.com/seu_usuario/riograndense_gio/internal/ui/components"
	"github.com/seu_usuario/riograndense_gio/internal/ui/theme"
	// "github.com/sqweek/dialog" // Biblioteca externa para diálogos nativos (exemplo)
)

// ImportTypeConfig define a configuração para cada tipo de importação.
type ImportTypeConfig struct {
	ID          services.FileType // "DIREITOS", "OBRIGACOES"
	Title       string            // Título para a UI
	AllowedExts []string          // Extensões permitidas (ex: ".txt", ".csv")
}

// ImportPageState armazena o estado para um tipo de importação específico.
type ImportPageState struct {
	LastUpdateText   string // "Última atualização: ..."
	SelectedFilePath string // Caminho completo do arquivo selecionado
	SelectedFileName string // Apenas o nome do arquivo para exibição

	SelectFileBtn widget.Clickable
	ImportBtn     widget.Clickable

	IsImporting   bool   // Se este tipo específico está sendo importado
	StatusMessage string // Mensagem de status específica para este tipo
	MessageColor  color.NRGBA
}

// ImportPage gerencia a UI para importação de arquivos.
type ImportPage struct {
	router         *ui.Router
	cfg            *core.Config
	importService  services.ImportService
	permManager    *auth.PermissionManager
	sessionManager *auth.SessionManager

	importTypes []ImportTypeConfig
	pageStates  map[services.FileType]*ImportPageState // Estado para cada tipo de importação

	// Estado global da página
	isLoadingGlobal     bool // Para carregamento inicial de status
	statusMessageGlobal string
	messageColorGlobal  color.NRGBA

	closeBtn widget.Clickable
	spinner  *components.LoadingSpinner

	firstLoadDone bool
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
		spinner:        components.NewLoadingSpinner(),
		importTypes: []ImportTypeConfig{
			{ID: services.FileTypeDireitos, Title: "Movimento de Títulos - Direitos", AllowedExts: []string{".txt", ".csv"}},
			{ID: services.FileTypeObrigacoes, Title: "Movimento de Títulos - Obrigações", AllowedExts: []string{".txt", ".csv"}},
			// Adicionar outros tipos aqui
		},
		pageStates: make(map[services.FileType]*ImportPageState),
	}

	for _, itype := range p.importTypes {
		p.pageStates[itype.ID] = &ImportPageState{
			LastUpdateText: "Última atualização: <i>Carregando...</i>",
		}
	}
	return p
}

func (p *ImportPage) OnNavigatedTo(params interface{}) {
	appLogger.Info("Navegou para ImportPage")
	p.statusMessageGlobal = ""
	if !p.firstLoadDone {
		p.loadInitialStatus()
		p.firstLoadDone = true
	} else {
		// Se já carregou antes, apenas invalida para redesenhar
		p.router.GetAppWindow().Invalidate()
	}
}

func (p *ImportPage) OnNavigatedFrom() {
	appLogger.Info("Navegando para fora da ImportPage")
	// Parar spinners se estiverem ativos
	for _, state := range p.pageStates {
		state.IsImporting = false
	}
	p.isLoadingGlobal = false
	p.spinner.Stop(p.router.GetAppWindow().Context())
}

func (p *ImportPage) loadInitialStatus() {
	p.isLoadingGlobal = true
	p.statusMessageGlobal = "Carregando status das importações..."
	p.messageColorGlobal = theme.Colors.TextMuted
	p.spinner.Start(p.router.GetAppWindow().Context())
	p.router.GetAppWindow().Invalidate()

	go func() {
		var loadErr error
		currentSession, errSess := p.sessionManager.GetCurrentSession()
		if errSess != nil || currentSession == nil {
			loadErr = fmt.Errorf("sessão de usuário inválida: %v", errSess)
		} else {
			allStatus, err := p.importService.GetAllImportStatus(currentSession)
			if err != nil {
				loadErr = fmt.Errorf("falha ao carregar status das importações: %w", err)
			} else {
				p.router.GetAppWindow().Execute(func() { // Atualiza UI na thread principal
					for _, itype := range p.importTypes {
						found := false
						for _, status := range allStatus {
							if strings.EqualFold(string(itype.ID), status.FileType) {
								state := p.pageStates[itype.ID]
								updateTimeStr := "Nunca"
								if !status.LastUpdatedAt.IsZero() {
									updateTimeStr = status.LastUpdatedAt.Local().Format("02/01/2006 15:04:05")
								}
								fileNameStr := ""
								if status.OriginalFilename != nil && *status.OriginalFilename != "" {
									fileNameStr = fmt.Sprintf("(Arquivo: %s)", *status.OriginalFilename)
								}
								countStr := ""
								if status.RecordCount != nil {
									countStr = fmt.Sprintf(" %d registros.", *status.RecordCount)
								}
								state.LastUpdateText = fmt.Sprintf("Última atualização: <b>%s</b>%s %s", updateTimeStr, countStr, fileNameStr)
								found = true
								break
							}
						}
						if !found {
							p.pageStates[itype.ID].LastUpdateText = "Última atualização: <i>Nunca realizado</i>"
						}
					}
				})
			}
		}

		p.router.GetAppWindow().Execute(func() {
			p.isLoadingGlobal = false
			p.spinner.Stop(p.router.GetAppWindow().Context())
			if loadErr != nil {
				p.statusMessageGlobal = loadErr.Error()
				p.messageColorGlobal = theme.Colors.Danger
				appLogger.Errorf("Erro ao carregar status para ImportPage: %v", loadErr)
			} else {
				p.statusMessageGlobal = "Status das importações carregado."
				p.messageColorGlobal = theme.Colors.Success
			}
			p.router.GetAppWindow().Invalidate()
		})
	}()
}

func (p *ImportPage) Layout(gtx layout.Context) layout.Dimensions {
	th := p.router.GetAppWindow().Theme()

	// Processar clique no botão fechar
	if p.closeBtn.Clicked(gtx) {
		// Se esta página for um "diálogo" dentro de outra página (ex: MainAppLayout),
		// precisaria de um mecanismo para sinalizar o fechamento para o pai.
		// Se for uma página de nível superior, pode navegar para outra página.
		p.router.NavigateTo(p.router.PreviousPageID(), nil) // Supondo que o router tenha PreviousPageID()
	}

	// Gerenciar cliques dos botões de importação/seleção dentro dos cards
	for fileTypeID := range p.pageStates {
		state := p.pageStates[fileTypeID] // Captura a variável de estado para o closure
		ft := fileTypeID                  // Captura o fileTypeID para o closure

		if state.SelectFileBtn.Clicked(gtx) {
			p.handleSelectFile(ft)
		}
		if state.ImportBtn.Clicked(gtx) {
			p.handleImportFile(ft)
		}
	}

	return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceEnd}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			title := material.H6(th, "Importação de Arquivos Base")
			title.Font.Weight = font.Bold
			return layout.Inset{Bottom: theme.LargeVSpacer}.Layout(gtx, title.Layout)
		}),

		// Lista de cards de importação
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			list := layout.List{Axis: layout.Vertical}
			return list.Layout(gtx, len(p.importTypes), func(gtx layout.Context, index int) layout.Dimensions {
				importCfg := p.importTypes[index]
				return layout.Inset{Bottom: theme.DefaultVSpacer}.Layout(gtx,
					p.layoutImportCard(gtx, th, importCfg),
				)
			})
		}),

		// Status Global e Botão Fechar
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.End, Spacing: layout.SpaceSides}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if p.statusMessageGlobal != "" {
						lbl := material.Body2(th, p.statusMessageGlobal)
						lbl.Color = p.messageColorGlobal
						return layout.Inset{Bottom: theme.DefaultVSpacer}.Layout(gtx, lbl.Layout)
					}
					return layout.Dimensions{}
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					closeButton := material.Button(th, &p.closeBtn, "Fechar")
					// TODO: Estilo secundário para botão fechar
					return closeButton.Layout(gtx)
				}),
			)
		}),

		// Spinner Global (precisa de layout.Stack para sobrepor)
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.isLoadingGlobal || p.anyImportTypeIsImporting() {
				// Posicionar no centro da janela. AppWindow pode ter um spinner global.
				// return p.spinner.Layout(gtx)
			}
			return layout.Dimensions{}
		}),
	)
}

func (p *ImportPage) layoutImportCard(gtx layout.Context, th *material.Theme, importCfg ImportTypeConfig) layout.Dimensions {
	state, ok := p.pageStates[importCfg.ID]
	if !ok {
		return layout.Dimensions{}
	} // Não deveria acontecer

	cardTitle := material.Subtitle1(th, importCfg.Title)
	cardTitle.Font.Weight = font.SemiBold

	// Simula um QGroupBox ou Card
	return material.Card(th, theme.Colors.Background, theme.ElevationSmall, layout.UniformInset(unit.Dp(16)),
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceSides}.Layout(gtx,
				layout.Rigid(cardTitle.Layout),
				layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					// LastUpdateText pode conter HTML simples (<b>, <i>).
					// Gio material.Label não renderiza HTML. Você precisaria de um parser
					// ou um widget de texto rico customizado. Por agora, texto plano.
					plainText := strings.ReplaceAll(strings.ReplaceAll(state.LastUpdateText, "<b>", ""), "</b>", "")
					plainText = strings.ReplaceAll(plainText, "<i>", "")
					plainText = strings.ReplaceAll(plainText, "</i>", "")
					return material.Body2(th, plainText).Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Height: theme.DefaultVSpacer}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { // Linha de Ação
					return layout.Flex{Alignment: layout.Middle, Spacing: layout.SpaceEnd}.Layout(gtx,
						layout.Rigid(material.Body2(th, "Arquivo:").Layout),
						layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							// Simula um QLineEdit ReadOnly para nome do arquivo
							fileName := state.SelectedFileName
							if fileName == "" {
								fileName = "Nenhum arquivo selecionado"
							}
							lbl := material.Body2(th, fileName)
							if state.SelectedFileName == "" {
								lbl.Color = theme.Colors.TextMuted
							}

							// Adiciona uma borda para parecer um campo
							border := widget.Border{Color: theme.Colors.Border, CornerRadius: unit.Dp(4), Width: unit.Dp(1)}
							return border.Layout(gtx, func(gtx C) D {
								return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(8), Top: unit.Dp(6), Bottom: unit.Dp(6)}.Layout(gtx, lbl.Layout)
							})
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
						layout.Rigid(material.Button(th, &state.SelectFileBtn, "Selecionar...").Layout),
						layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							importButton := material.Button(th, &state.ImportBtn, "Importar")
							if state.SelectedFilePath == "" || state.IsImporting || p.isLoadingGlobal {
								// Desabilitar visualmente o botão
								importButton.Style.뭄Color = theme.Colors.TextMuted
								// Não processar clique se desabilitado (a lógica de clique já checa IsImporting)
							} else {
								importButton.Background = theme.Colors.Primary
							}
							return importButton.Layout(gtx)
						}),
					)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { // Status específico do card
					if state.StatusMessage != "" {
						lbl := material.Body2(th, state.StatusMessage)
						lbl.Color = state.MessageColor
						return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, lbl.Layout)
					}
					return layout.Dimensions{}
				}),
			)
		}).Layout(gtx)
}

func (p *ImportPage) handleSelectFile(fileTypeID services.FileType) {
	state := p.pageStates[fileTypeID]
	state.StatusMessage = ""
	p.statusMessageGlobal = "" // Limpa mensagens
	p.router.GetAppWindow().Invalidate()

	// --- Simulação de Seleção de Arquivo ---
	// Em uma aplicação real, você usaria um diálogo nativo.
	// Gio tem app.Window. 폴더 열기() mas é mais para diretórios.
	// Para arquivos, você pode precisar de uma biblioteca de diálogo nativo como github.com/sqweek/dialog
	// ou implementar a lógica na AppWindow que pode chamar código específico da plataforma.
	// Exemplo com sqweek/dialog:
	/*
		go func() { // Executa em goroutine para não bloquear UI
			filePath, err := dialog.File().Filter("Arquivos de Texto/CSV", "txt", "csv").Title("Selecionar Arquivo para " + string(fileTypeID)).Load()
			p.router.GetAppWindow().Execute(func(){ // Volta para a thread da UI
				if err != nil {
					if errors.Is(err, dialog.ErrCancelled) {
						appLogger.Debugf("Seleção de arquivo cancelada para %s", fileTypeID)
						return
					}
					state.StatusMessage = fmt.Sprintf("Erro ao selecionar arquivo: %v", err)
					state.MessageColor = theme.Colors.Danger
					appLogger.Errorf("Erro no diálogo de arquivo para %s: %v", fileTypeID, err)
					p.router.GetAppWindow().Invalidate()
					return
				}
				state.SelectedFilePath = filePath
				state.SelectedFileName = filepath.Base(filePath)
				p.router.GetAppWindow().Invalidate()
			})
		}()
	*/
	// --- Fim da Simulação/Exemplo com sqweek/dialog ---

	// Por agora, um placeholder:
	state.SelectedFilePath = "/caminho/para/simulado_" + strings.ToLower(string(fileTypeID)) + ".txt"
	state.SelectedFileName = "simulado_" + strings.ToLower(string(fileTypeID)) + ".txt"
	appLogger.Debugf("Arquivo 'selecionado' (simulado) para %s: %s", fileTypeID, state.SelectedFilePath)
	p.router.GetAppWindow().Invalidate() // Atualiza UI para mostrar nome do arquivo
}

func (p *ImportPage) handleImportFile(fileTypeID services.FileType) {
	state := p.pageStates[fileTypeID]
	if state.SelectedFilePath == "" || state.IsImporting || p.isLoadingGlobal {
		return
	}

	// TODO: Diálogo de confirmação (Importante!)

	state.IsImporting = true
	state.StatusMessage = "Importando arquivo..."
	state.MessageColor = theme.Colors.TextMuted
	p.spinner.Start(p.router.GetAppWindow().Context()) // Reutiliza spinner global, ou ter um por card
	p.router.GetAppWindow().Invalidate()

	go func(ft services.FileType, fp string) {
		var opResult map[string]interface{}
		var opErr error
		currentSession, _ := p.sessionManager.GetCurrentSession()

		opResult, opErr = p.importService.ImportFile(fp, ft, currentSession)

		p.router.GetAppWindow().Execute(func() {
			s := p.pageStates[ft] // Pega o estado mais recente
			s.IsImporting = false
			p.spinner.Stop(p.router.GetAppWindow().Context()) // Para o spinner global

			if opErr != nil {
				s.StatusMessage = fmt.Sprintf("Falha na importação: %v", opErr)
				s.MessageColor = theme.Colors.Danger
				appLogger.Errorf("Erro ao importar arquivo tipo %s (%s): %v", ft, fp, opErr)
			} else {
				processed := 0
				if proc, ok := opResult["records_processed"].(int); ok {
					processed = proc
				}
				s.StatusMessage = fmt.Sprintf("Importação concluída! %d registros processados.", processed)
				s.MessageColor = theme.Colors.Success
				appLogger.Infof("Arquivo tipo %s (%s) importado com sucesso. %d registros.", ft, fp, processed)
				p.updateSpecificStatus(ft) // Atualiza "Última atualização" para este tipo
			}
			// Limpar seleção de arquivo após tentativa
			s.SelectedFilePath = ""
			s.SelectedFileName = ""
			p.router.GetAppWindow().Invalidate()
		})
	}(fileTypeID, state.SelectedFilePath)
}

// updateSpecificStatus busca e atualiza o label de "Última atualização" para um tipo.
func (p *ImportPage) updateSpecificStatus(fileTypeID services.FileType) {
	state, ok := p.pageStates[fileTypeID]
	if !ok {
		return
	}

	go func(ft services.FileType) {
		var statusText string
		currentSession, _ := p.sessionManager.GetCurrentSession()
		status, err := p.importService.GetImportStatus(ft, currentSession)
		if err != nil {
			statusText = "Última atualização: <i>Erro ao buscar</i>"
			appLogger.Errorf("Erro ao buscar status de importação para %s após import: %v", ft, err)
		} else if status != nil {
			updateTimeStr := "Nunca"
			if !status.LastUpdatedAt.IsZero() {
				updateTimeStr = status.LastUpdatedAt.Local().Format("02/01/2006 15:04:05")
			}
			fileNameStr := ""
			if status.OriginalFilename != nil {
				fileNameStr = fmt.Sprintf("(Arquivo: %s)", *status.OriginalFilename)
			}
			countStr := ""
			if status.RecordCount != nil {
				countStr = fmt.Sprintf(" %d registros.", *status.RecordCount)
			}
			statusText = fmt.Sprintf("Última atualização: <b>%s</b>%s %s", updateTimeStr, countStr, fileNameStr)
		} else {
			statusText = "Última atualização: <i>Não encontrado</i>"
		}

		p.router.GetAppWindow().Execute(func() {
			s_ui := p.pageStates[ft]
			s_ui.LastUpdateText = statusText
			p.router.GetAppWindow().Invalidate()
		})
	}(fileTypeID)
}

func (p *ImportPage) anyImportTypeIsImporting() bool {
	for _, state := range p.pageStates {
		if state.IsImporting {
			return true
		}
	}
	return false
}
