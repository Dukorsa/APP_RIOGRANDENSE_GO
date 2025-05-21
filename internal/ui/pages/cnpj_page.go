package pages

import (
	"errors"
	"fmt"
	"image/color"
	"sort"
	"strconv"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/auth"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core"
	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/models"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/services"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui/components"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/utils"
)

const (
	cnpjColIndexCNPJ      = 0
	cnpjColIndexNetworkID = 1
	cnpjColIndexRegDate   = 2
	cnpjColIndexStatus    = 3
	cnpjNumTableHeaders   = 4
)

// CNPJPage gerencia a interface para CNPJs.
type CNPJPage struct {
	router         *ui.Router
	cfg            *core.Config
	cnpjService    services.CNPJService
	networkService services.NetworkService
	permManager    *auth.PermissionManager
	sessionManager *auth.SessionManager

	// Estado da UI
	isLoading      bool
	cnpjs          []*models.CNPJPublic // Lista completa de CNPJs carregados do serviço
	filteredCNPJs  []*models.CNPJPublic // CNPJs após filtro e ordenação para exibição
	selectedCNPJID *uint64              // ID do CNPJ selecionado na lista para edição/exclusão
	statusMessage  string               // Mensagem de feedback global para a página
	messageColor   color.NRGBA

	// Widgets de formulário para adicionar/editar CNPJ
	cnpjInput      widget.Editor
	networkIDInput widget.Editor
	statusEnum     widget.Enum // Para o ComboBox de status Ativo/Inativo no modo de edição

	// Feedback para inputs do formulário
	cnpjInputFeedback      string
	networkIDInputFeedback string

	// Botões de ação do formulário
	saveBtn          widget.Clickable
	clearOrCancelBtn widget.Clickable

	// Botões de ação da lista (fora do formulário)
	deleteBtn      widget.Clickable
	refreshListBtn widget.Clickable

	// Para a lista/tabela de CNPJs
	cnpjList          layout.List
	cnpjClickables    []widget.Clickable // Um clickable por CNPJ na lista `filteredCNPJs`
	tableHeaderClicks [cnpjNumTableHeaders]widget.Clickable
	sortColumn        int
	sortAscending     bool

	spinner *components.LoadingSpinner

	firstLoadDone bool // Controla o carregamento inicial de dados
	isEditing     bool // True se um CNPJ existente estiver carregado no formulário para edição
}

// NewCNPJPage cria uma nova instância da página de gerenciamento de CNPJs.
func NewCNPJPage(
	router *ui.Router,
	cfg *core.Config,
	cnpjSvc services.CNPJService,
	netSvc services.NetworkService,
	permMan *auth.PermissionManager,
	sessMan *auth.SessionManager,
) *CNPJPage {
	p := &CNPJPage{
		router:         router,
		cfg:            cfg,
		cnpjService:    cnpjSvc,
		networkService: netSvc,
		permManager:    permMan,
		sessionManager: sessMan,
		cnpjList:       layout.List{Axis: layout.Vertical},
		spinner:        components.NewLoadingSpinner(theme.Colors.Primary),
		sortColumn:     cnpjColIndexRegDate, // Default: ordenar por Data de Cadastro
		sortAscending:  false,               // Mais recentes primeiro
	}
	p.cnpjInput.SingleLine = true
	p.cnpjInput.Hint = "XX.XXX.XXX/XXXX-XX"
	p.networkIDInput.SingleLine = true
	p.networkIDInput.Hint = "ID numérico da Rede"
	p.networkIDInput.Filter = "0123456789" // Permite apenas dígitos

	// Configuração do Enum para o status (usado no modo de edição)
	p.statusEnum.SetEnumValue("Ativo")   // Valor interno e label inicial
	p.statusEnum.SetEnumValue("Inativo") // Adiciona a outra opção
	p.statusEnum.Value = "Ativo"         // Define o valor padrão inicial

	return p
}

// OnNavigatedTo é chamado quando a página se torna ativa.
func (p *CNPJPage) OnNavigatedTo(params interface{}) {
	appLogger.Info("Navegou para CNPJPage")
	p.clearFormAndSelection(true) // Limpa formulário e seleção da lista
	p.statusMessage = ""

	currentSession, errSess := p.sessionManager.GetCurrentSession()
	if errSess != nil || currentSession == nil {
		p.router.GetAppWindow().HandleLogout() // Força logout se sessão inválida
		return
	}
	// Verifica permissão para visualizar CNPJs
	if err := p.permManager.CheckPermission(currentSession, auth.PermCNPJView, nil); err != nil {
		p.statusMessage = fmt.Sprintf("Acesso negado à página de CNPJs: %v", err)
		p.messageColor = theme.Colors.Danger
		p.cnpjs = []*models.CNPJPublic{}
		p.applyFiltersAndSort()
		p.router.GetAppWindow().Invalidate()
		return
	}

	if !p.firstLoadDone {
		p.loadCNPJs(currentSession)
		p.firstLoadDone = true
	} else {
		p.loadCNPJs(currentSession) // Recarrega para ter dados frescos
	}
}

// OnNavigatedFrom é chamado quando o router navega para fora desta página.
func (p *CNPJPage) OnNavigatedFrom() {
	appLogger.Info("Navegando para fora da CNPJPage")
	p.isLoading = false
	p.spinner.Stop(p.router.GetAppWindow().Context()) // Garante que o spinner pare
}

// loadCNPJs carrega a lista de CNPJs do serviço.
func (p *CNPJPage) loadCNPJs(currentSession *auth.SessionData) {
	if p.isLoading {
		return
	}
	p.isLoading = true
	p.statusMessage = "Carregando CNPJs..."
	p.messageColor = theme.Colors.TextMuted
	p.spinner.Start(p.router.GetAppWindow().Context())
	p.router.GetAppWindow().Invalidate()

	go func(sess *auth.SessionData) {
		var loadedCNPJs []*models.CNPJPublic
		var loadErr error

		// Permissão para visualizar já foi checada em OnNavigatedTo
		cnpjsList, err := s.cnpjService.GetAllCNPJs(true, sess) // true = include inactive
		if err != nil {
			loadErr = fmt.Errorf("falha ao carregar CNPJs: %w", err)
		} else {
			loadedCNPJs = cnpjsList
		}

		p.router.GetAppWindow().Execute(func() {
			p.isLoading = false
			p.spinner.Stop(p.router.GetAppWindow().Context())
			if loadErr != nil {
				p.statusMessage = loadErr.Error()
				p.messageColor = theme.Colors.Danger
				p.cnpjs = []*models.CNPJPublic{} // Limpa em caso de erro
				appLogger.Errorf("Erro ao carregar CNPJs para CNPJPage: %v", loadErr)
			} else {
				p.cnpjs = loadedCNPJs
				if len(p.cnpjs) > 0 {
					p.statusMessage = fmt.Sprintf("%d CNPJs carregados.", len(p.cnpjs))
					p.messageColor = theme.Colors.Success
				} else {
					p.statusMessage = "Nenhum CNPJ encontrado."
					p.messageColor = theme.Colors.Info
				}
				p.applyFiltersAndSort()
			}
			p.updateButtonStates(sess)
			p.router.GetAppWindow().Invalidate()
		})
	}(currentSession)
}

// applyFiltersAndSort aplica filtros (se houver) e ordena a lista de CNPJs.
func (p *CNPJPage) applyFiltersAndSort() {
	// Por enquanto, não há filtros de UI além de "includeInactive" (controlado no serviço).
	// Apenas a ordenação é aplicada aqui.
	tempFiltered := make([]*models.CNPJPublic, len(p.cnpjs))
	copy(tempFiltered, p.cnpjs)

	sort.SliceStable(tempFiltered, func(i, j int) bool {
		c1 := tempFiltered[i]
		c2 := tempFiltered[j]
		var less bool
		switch p.sortColumn {
		case cnpjColIndexCNPJ:
			less = c1.CNPJ < c2.CNPJ
		case cnpjColIndexNetworkID:
			less = c1.NetworkID < c2.NetworkID
		case cnpjColIndexRegDate:
			less = c1.RegistrationDate.Before(c2.RegistrationDate)
		case cnpjColIndexStatus:
			less = c1.Active && !c2.Active // Ativos primeiro
		default:
			less = c1.ID < c2.ID // Fallback para ordenação por ID
		}
		if !p.sortAscending {
			return !less
		}
		return less
	})

	p.filteredCNPJs = tempFiltered
	// Ajusta o tamanho do slice de clickables para corresponder aos CNPJs filtrados/ordenados.
	if len(p.filteredCNPJs) != len(p.cnpjClickables) {
		p.cnpjClickables = make([]widget.Clickable, len(p.filteredCNPJs))
	}
}

// Layout é o método principal de desenho da página.
func (p *CNPJPage) Layout(gtx layout.Context) layout.Dimensions {
	th := p.router.GetAppWindow().Theme()
	currentSession, _ := p.sessionManager.GetCurrentSession() // Para verificações de permissão

	// Processar eventos dos inputs do formulário
	if p.cnpjInput.Update(gtx) {
		p.formatAndValidateCNPJInputUI()
		p.statusMessage = "" // Limpa mensagem global ao digitar
		p.updateButtonStates(currentSession)
	}
	if p.networkIDInput.Update(gtx) {
		p.validateNetworkIDInputUI()
		p.statusMessage = ""
		p.updateButtonStates(currentSession)
	}
	if p.statusEnum.Update(gtx) { // Se o valor do ComboBox de status mudar
		p.updateButtonStates(currentSession)
		p.statusMessage = ""
	}

	// Processar cliques nos botões principais
	if p.saveBtn.Clicked(gtx) {
		p.handleSaveCNPJ(currentSession)
	}
	if p.clearOrCancelBtn.Clicked(gtx) {
		p.clearFormAndSelection(true) // Limpa formulário e seleção da lista
		p.updateButtonStates(currentSession)
	}
	if p.deleteBtn.Clicked(gtx) {
		p.handleDeleteCNPJ(currentSession)
	}
	if p.refreshListBtn.Clicked(gtx) {
		p.loadCNPJs(currentSession)
	}

	// Estrutura da página: Formulário no topo, Lista abaixo
	return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceEnd}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.layoutForm(gtx, th, currentSession)
		}),
		layout.Rigid(layout.Spacer{Height: theme.LargeVSpacer}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return p.layoutCNPJTable(gtx, th, currentSession)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { // Mensagem de Status Global
			if p.statusMessage != "" {
				lbl := material.Body2(th, p.statusMessage)
				lbl.Color = p.messageColor
				return layout.Inset{Top: theme.DefaultVSpacer}.Layout(gtx, lbl.Layout)
			}
			return layout.Dimensions{}
		}),
		// Spinner de carregamento (se `isLoading` for true, deve ser sobreposto,
		// geralmente por um layout.Stack na AppWindow ou no layout raiz desta página).
		// Se p.isLoading { return p.spinner.Layout(gtx) }
	)
}

// layoutForm desenha o formulário de entrada/edição de CNPJ.
func (p *CNPJPage) layoutForm(gtx layout.Context, th *material.Theme, currentSession *auth.SessionData) layout.Dimensions {
	title := "Novo CNPJ"
	if p.isEditing {
		title = "Editar CNPJ Selecionado"
	}
	// Verifica permissão para criar/editar. Botão Salvar será controlado por `updateButtonStates`.
	canEditForm, _ := p.permManager.HasPermission(currentSession, auth.PermCNPJCreate, nil) // PermCreate para adicionar
	if p.isEditing {
		canEditForm, _ = p.permManager.HasPermission(currentSession, auth.PermCNPJUpdate, nil) // PermUpdate para editar
	}

	return material.Card(th, theme.Colors.Surface, theme.ElevationSmall, layout.UniformInset(unit.Dp(16)),
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceSides}.Layout(gtx,
				layout.Rigid(material.Subtitle1(th, title).Layout),
				layout.Rigid(layout.Spacer{Height: theme.DefaultVSpacer}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { // CNPJ Input
					// O input de CNPJ é sempre editável se o formulário estiver ativo para novo,
					// mas não editável (readonly) se estiver editando um CNPJ existente.
					// A biblioteca Gio não tem um modo "readonly" direto para Editor.
					// Simula-se desabilitando interações ou mudando a aparência.
					// Aqui, vamos controlar a "editabilidade" permitindo ou não foco.
					cnpjEditorWidget := material.Editor(th, &p.cnpjInput, p.cnpjInput.Hint)
					if p.isEditing { // Se editando, CNPJ não pode ser alterado.
						// Visualmente, poderia ter um fundo diferente ou texto mais claro.
						// Para impedir edição, não adicionar eventos ou usar um Label.
						// Usar um Label para exibir o CNPJ no modo de edição:
						return p.labeledInput(gtx, th, "CNPJ:*", material.Body1(th, p.cnpjInput.Text()).Layout, p.cnpjInputFeedback)
					}
					return p.labeledInput(gtx, th, "CNPJ:*", cnpjEditorWidget.Layout, p.cnpjInputFeedback)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { // Network ID Input
					netIDEditorWidget := material.Editor(th, &p.networkIDInput, p.networkIDInput.Hint)
					return p.labeledInput(gtx, th, "ID da Rede:*", netIDEditorWidget.Layout, p.networkIDInputFeedback)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { // Status ComboBox (apenas no modo de edição)
					if !p.isEditing {
						return layout.Dimensions{}
					}
					// Usar DropDown do material.Theme para um ComboBox.
					selectedLabel := p.statusEnum.Value // O Value é o Label aqui porque SetEnumValue foi usado
					return layout.Flex{Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
						layout.Rigid(material.Body1(th, "Status:").Layout),
						layout.Flexed(1, layout.Inset{Left: theme.DefaultVSpacer}.Layout(gtx, // Espaço para alinhar
							material.DropDown(th, &p.statusEnum, material.Body1(th, selectedLabel).Layout).Layout,
						)),
					)
				}),
				layout.Rigid(layout.Spacer{Height: theme.LargeVSpacer}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { // Botões do Formulário
					saveButtonText := "Adicionar Novo"
					if p.isEditing {
						saveButtonText = "Salvar Alterações"
					}
					saveButton := material.Button(th, &p.saveBtn, saveButtonText)

					clearButtonText := "Limpar Formulário"
					if p.isEditing {
						clearButtonText = "Cancelar Edição"
					}
					clearButton := material.Button(th, &p.clearOrCancelBtn, clearButtonText)

					// Habilitar/Desabilitar visualmente botão Salvar (cor/opacidade)
					// A lógica de clique já verificará se a ação é permitida.
					if !canEditForm || !p.isFormValidForSave() { // isFormValidForSave verifica se os campos obrigatórios estão OK
						saveButton.Style.TextColor = theme.Colors.TextMuted
						saveButton.Style.Background = theme.Colors.Grey300
					} else {
						saveButton.Background = theme.Colors.Primary // Cor primária se habilitado
					}

					return layout.Flex{Spacing: layout.SpaceBetween}.Layout(gtx,
						layout.Flexed(1, clearButton.Layout),
						layout.Flexed(1, saveButton.Layout),
					)
				}),
			)
		}).Layout(gtx)
}

// labeledInput é um helper para criar um Label + Widget de Input + FeedbackLabel.
func (p *CNPJPage) labeledInput(gtx layout.Context, th *material.Theme, labelText string, inputWidget layout.Widget, feedbackText string) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical, Spacing: layout.Tight}.Layout(gtx,
		layout.Rigid(material.Body1(th, labelText).Layout),
		layout.Rigid(inputWidget), // O widget de input já deve ter seu próprio padding/borda.
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { // Feedback de erro
			if feedbackText != "" {
				lbl := material.Body2(th, feedbackText)
				lbl.Color = theme.Colors.Danger // Cor vermelha para erro.
				return layout.Inset{Top: unit.Dp(2)}.Layout(gtx, lbl.Layout)
			}
			return layout.Dimensions{}
		}),
	)
}

// layoutCNPJTable desenha a lista/tabela de CNPJs.
func (p *CNPJPage) layoutCNPJTable(gtx layout.Context, th *material.Theme, currentSession *auth.SessionData) layout.Dimensions {
	headers := []string{"CNPJ", "Rede (ID)", "Data Cadastro", "Status"}
	colWeights := []float32{0.35, 0.15, 0.30, 0.20} // Ajustar pesos conforme necessário

	headerLayout := func(colIndex int, label string) layout.Widget {
		return func(gtx C) D {
			headerLabel := material.Body1(th, label)
			headerLabel.Font.Weight = font.Bold
			// Adicionar ícone de ordenação se esta coluna estiver sendo usada para ordenar
			if p.sortColumn == colIndex {
				// iconData := icons.NavigationArrowDropDown
				// if p.sortAscending { iconData = icons.NavigationArrowDropUp }
				// sortIcon, _ := widget.NewIcon(iconData)
				// return layout.Flex{Alignment:layout.Middle}.Layout(gtx, layout.Rigid(headerLabel.Layout), layout.Rigid(material.Icon(th, sortIcon).Layout))
			}
			if p.tableHeaderClicks[colIndex].Clicked(gtx) {
				if p.sortColumn == colIndex {
					p.sortAscending = !p.sortAscending
				} else {
					p.sortColumn = colIndex
					p.sortAscending = true
				}
				p.applyFiltersAndSort()
				p.selectedCNPJID = nil // Limpa seleção ao reordenar
				p.updateButtonStates(currentSession)
			}
			return headerLabel.Layout(gtx)
		}
	}

	rowLayout := func(gtx layout.Context, index int, cnpj *models.CNPJPublic) layout.Dimensions {
		isSelected := p.selectedCNPJID != nil && *p.selectedCNPJID == cnpj.ID
		bgColor := theme.Colors.Surface
		if index%2 != 0 {
			bgColor = theme.Colors.BackgroundAlt
		}
		if !cnpj.Active {
			bgColor = theme.Colors.Grey100
		} // Fundo diferente para inativos
		if isSelected {
			bgColor = theme.Colors.PrimaryLight
		}

		return material.Clickable(gtx, &p.cnpjClickables[index], func(gtx layout.Context) layout.Dimensions {
			textColor := theme.Colors.Text
			if !cnpj.Active {
				textColor = theme.Colors.TextMuted
			}
			if isSelected {
				textColor = theme.Colors.PrimaryText
			}

			cnpjLbl := material.Body2(th, cnpj.FormatCNPJ())
			cnpjLbl.Color = textColor
			netIDLbl := material.Body2(th, fmt.Sprint(cnpj.NetworkID))
			netIDLbl.Color = textColor
			regDateLbl := material.Body2(th, cnpj.RegistrationDate.Format("02/01/06 15:04"))
			regDateLbl.Color = textColor
			statusLbl := material.Body2(th, boolToString(cnpj.Active, "Ativo", "Inativo"))
			statusLbl.Color = textColor

			return layout.Background{Color: bgColor}.Layout(gtx, func(gtx C) D {
				return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx,
					layout.Flex{}.Layout(gtx,
						layout.Flexed(colWeights[0], cnpjLbl.Layout),
						layout.Flexed(colWeights[1], netIDLbl.Layout),
						layout.Flexed(colWeights[2], regDateLbl.Layout),
						layout.Flexed(colWeights[3], statusLbl.Layout),
					))
			})
		})
	}

	for i := range p.filteredCNPJs {
		if i >= len(p.cnpjClickables) {
			break
		}
		if p.cnpjClickables[i].Clicked(gtx) {
			p.loadCNPJIntoForm(p.filteredCNPJs[i])
			p.updateButtonStates(currentSession) // Atualiza estado dos botões com base na seleção
		}
	}

	deleteButton := material.Button(th, &p.deleteBtn, "Excluir Selecionado")
	canDelete, _ := p.permManager.HasPermission(currentSession, auth.PermCNPJDelete, nil)
	if !canDelete || p.selectedCNPJID == nil || !p.isEditing { // Botão só ativo se algo selecionado e com permissão
		deleteButton.Style.TextColor = theme.Colors.TextMuted
		deleteButton.Style.Background = theme.Colors.Grey300
	}

	return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceEnd}.Layout(gtx,
		layout.Rigid(func(gtx C) D { // Cabeçalho
			return layout.Background{Color: theme.Colors.Grey200}.Layout(gtx,
				layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx,
					layout.Flex{}.Layout(gtx,
						layout.Flexed(colWeights[0], headerLayout(cnpjColIndexCNPJ, headers[0]).Layout),
						layout.Flexed(colWeights[1], headerLayout(cnpjColIndexNetworkID, headers[1]).Layout),
						layout.Flexed(colWeights[2], headerLayout(cnpjColIndexRegDate, headers[2]).Layout),
						layout.Flexed(colWeights[3], headerLayout(cnpjColIndexStatus, headers[3]).Layout),
					)))
		}),
		layout.Flexed(1, func(gtx C) D { // Lista
			return p.cnpjList.Layout(gtx, len(p.filteredCNPJs), func(gtx C, index int) D {
				if index < 0 || index >= len(p.filteredCNPJs) {
					return D{}
				}
				return rowLayout(gtx, index, p.filteredCNPJs[index])
			})
		}),
		layout.Rigid(layout.Spacer{Height: theme.DefaultVSpacer}.Layout),
		layout.Rigid(func(gtx C) D { // Botões abaixo da lista
			return layout.Flex{Spacing: layout.SpaceBetween, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(deleteButton.Layout),
				layout.Flexed(1, func(gtx C) D { return D{} }), // Espaçador
				layout.Rigid(material.Button(th, &p.refreshListBtn, "Atualizar Lista").Layout),
			)
		}),
	)
}

// --- Lógica de Ações e Validações do Formulário ---

// formatAndValidateCNPJInputUI formata e valida o CNPJ no input da UI.
func (p *CNPJPage) formatAndValidateCNPJInputUI() bool {
	// A formatação em tempo real pode ser complexa de implementar perfeitamente
	// com o widget.Editor padrão do Gio, especialmente para manter a posição do cursor.
	// Uma validação mais simples no ChangeEvent e uma formatação/validação completa no submit é mais robusta.
	// Por agora, vamos focar na validação.
	cnpjRaw := p.cnpjInput.Text()
	p.cnpjInputFeedback = "" // Limpa feedback anterior

	cleanedCNPJ, errClean := models.CNPJCreate{CNPJ: cnpjRaw}.CleanAndValidateCNPJ()
	if errClean != nil {
		if valErr, ok := errClean.(*appErrors.ValidationError); ok {
			p.cnpjInputFeedback = valErr.Fields["cnpj"] // Pega mensagem específica do campo CNPJ
		} else {
			p.cnpjInputFeedback = errClean.Error()
		}
		return false
	}
	if !utils.IsValidCNPJ(cleanedCNPJ) {
		p.cnpjInputFeedback = "CNPJ inválido (dígitos verificadores)."
		return false
	}
	// Opcional: Atualizar o texto do editor com o CNPJ formatado (se a formatação for desejada no input)
	// if p.cnpjInput.Text() != models.CNPJPublic{CNPJ: cleanedCNPJ}.FormatCNPJ() {
	// p.cnpjInput.SetText(models.CNPJPublic{CNPJ: cleanedCNPJ}.FormatCNPJ())
	// }
	return true
}

// validateNetworkIDInputUI valida o Network ID no input da UI.
func (p *CNPJPage) validateNetworkIDInputUI() bool {
	netIDStr := strings.TrimSpace(p.networkIDInput.Text())
	p.networkIDInputFeedback = "" // Limpa

	if netIDStr == "" {
		p.networkIDInputFeedback = "ID da Rede é obrigatório."
		return false
	}
	_, errConv := strconv.ParseUint(netIDStr, 10, 64)
	if errConv != nil {
		p.networkIDInputFeedback = "ID da Rede deve ser um número positivo."
		return false
	}
	return true
}

// isFormValidForSave verifica se o formulário está em um estado válido para salvar.
func (p *CNPJPage) isFormValidForSave() bool {
	// Verifica se os inputs (sem feedback de erro) estão preenchidos.
	// A validação mais completa (dígitos CNPJ, existência NetworkID) é feita em handleSaveCNPJ.
	return strings.TrimSpace(p.cnpjInput.Text()) != "" &&
		strings.TrimSpace(p.networkIDInput.Text()) != "" &&
		p.cnpjInputFeedback == "" && // Sem erro de formato no CNPJ
		p.networkIDInputFeedback == "" // Sem erro de formato no Network ID
}

// loadCNPJIntoForm carrega os dados de um CNPJ selecionado para o formulário de edição.
func (p *CNPJPage) loadCNPJIntoForm(cnpj *models.CNPJPublic) {
	if cnpj == nil {
		p.clearFormAndSelection(false) // Apenas limpa o formulário
		return
	}
	p.selectedCNPJID = &cnpj.ID
	p.isEditing = true

	p.cnpjInput.SetText(cnpj.FormatCNPJ()) // Mostra formatado no input
	p.networkIDInput.SetText(fmt.Sprint(cnpj.NetworkID))
	if cnpj.Active {
		p.statusEnum.Value = "Ativo"
	} else {
		p.statusEnum.Value = "Inativo"
	}
	p.cnpjInputFeedback = "" // Limpa feedbacks ao carregar
	p.networkIDInputFeedback = ""
	p.statusMessage = "" // Limpa mensagem global
	// p.updateButtonStates() // Será chamado pelo clique na linha ou pelo Layout
	appLogger.Debugf("CNPJ ID %d ('%s') carregado no formulário para edição.", cnpj.ID, cnpj.FormatCNPJ())
}

// clearFormAndSelection limpa o formulário e, opcionalmente, a seleção na lista.
func (p *CNPJPage) clearFormAndSelection(clearListSelectionAlso bool) {
	p.cnpjInput.SetText("")
	p.networkIDInput.SetText("")
	p.statusEnum.Value = "Ativo" // Reset para o valor padrão
	p.cnpjInputFeedback = ""
	p.networkIDInputFeedback = ""
	p.isEditing = false

	if clearListSelectionAlso {
		p.selectedCNPJID = nil
	}
	p.statusMessage = "" // Limpa mensagem global
	// p.updateButtonStates() // Será chamado pelo Layout ou ação que o invocou
	appLogger.Debug("Formulário de CNPJ limpo. Seleção da lista limpa: %t", clearListSelectionAlso)
}

// updateButtonStates atualiza o estado visual (simulado) e lógico dos botões.
func (p *CNPJPage) updateButtonStates(currentSession *auth.SessionData) {
	// A lógica de habilitação real ocorre ao processar o clique, verificando permissões.
	// Esta função pode ser usada para feedback visual (ex: mudar cor de botões "desabilitados").
	// E para invalidar a UI para que os botões sejam redesenhados.
	p.router.GetAppWindow().Invalidate()
}

// handleSaveCNPJ lida com a submissão do formulário para criar ou atualizar um CNPJ.
func (p *CNPJPage) handleSaveCNPJ(currentSession *auth.SessionData) {
	if p.isLoading {
		return
	}

	// Validação completa do formulário
	validCNPJ := p.formatAndValidateCNPJInputUI()
	validNetID := p.validateNetworkIDInputUI()
	if !validCNPJ || !validNetID {
		p.statusMessage = "Corrija os erros no formulário."
		p.messageColor = theme.Colors.Warning
		p.router.GetAppWindow().Invalidate()
		return
	}

	cnpjRaw := p.cnpjInput.Text() // Já pode estar formatado ou não, CleanCNPJ lidará
	networkIDStr := p.networkIDInput.Text()
	isActive := p.statusEnum.Value == "Ativo" // Usado apenas no modo de edição

	// Limpa CNPJ para o formato de 14 dígitos para validação e persistência
	cleanedCNPJ := models.CleanCNPJ(cnpjRaw)
	if !utils.IsValidCNPJ(cleanedCNPJ) { // Valida dígitos verificadores
		p.cnpjInputFeedback = "CNPJ inválido (dígitos verificadores não conferem)."
		p.router.GetAppWindow().Invalidate()
		return
	}

	networkID, _ := strconv.ParseUint(networkIDStr, 10, 64) // Já validado por validateNetworkIDInputUI

	p.isLoading = true
	p.statusMessage = "Salvando CNPJ..."
	p.messageColor = theme.Colors.TextMuted
	p.spinner.Start(p.router.GetAppWindow().Context())
	p.router.GetAppWindow().Invalidate()

	go func(isEditMode bool, currentCNPJID *uint64, cnpjLabel string, netIDLabel uint64, activeLabel bool, sess *auth.SessionData) {
		var opErr error
		var successMsg string
		var resultingCNPJ *models.CNPJPublic

		if isEditMode {
			if currentCNPJID == nil { // Segurança
				opErr = errors.New("ID do CNPJ para edição não encontrado")
			} else {
				// No modo de edição, o campo CNPJ não é alterado. Apenas NetworkID e Active.
				updateData := models.CNPJUpdate{NetworkID: &netIDLabel, Active: &activeLabel}
				resultingCNPJ, opErr = s.cnpjService.UpdateCNPJ(*currentCNPJID, updateData, sess)
				if opErr == nil {
					successMsg = fmt.Sprintf("CNPJ %s (ID %d) atualizado.", resultingCNPJ.FormatCNPJ(), *currentCNPJID)
				}
			}
		} else { // Modo de Criação
			createData := models.CNPJCreate{CNPJ: cnpjLabel, NetworkID: netIDLabel} // Passa CNPJ limpo
			resultingCNPJ, opErr = s.cnpjService.RegisterCNPJ(createData, sess)
			if opErr == nil {
				successMsg = fmt.Sprintf("CNPJ %s cadastrado com sucesso.", resultingCNPJ.FormatCNPJ())
			}
		}

		p.router.GetAppWindow().Execute(func() {
			p.isLoading = false
			p.spinner.Stop(p.router.GetAppWindow().Context())
			if opErr != nil {
				p.statusMessage = fmt.Sprintf("Erro ao salvar CNPJ: %v", opErr)
				p.messageColor = theme.Colors.Danger
				// Tentar atribuir erro ao campo específico se possível
				if valErr, ok := opErr.(*appErrors.ValidationError); ok {
					if msg, found := valErr.Fields["cnpj"]; found {
						p.cnpjInputFeedback = msg
					}
					if msg, found := valErr.Fields["network_id"]; found {
						p.networkIDInputFeedback = msg
					}
				} else if errors.Is(opErr, appErrors.ErrConflict) {
					p.cnpjInputFeedback = opErr.Error() // Exibe erro de conflito no campo CNPJ
				} else if errors.Is(opErr, appErrors.ErrNotFound) && strings.Contains(opErr.Error(), "rede") {
					p.networkIDInputFeedback = opErr.Error()
				}
			} else {
				p.statusMessage = successMsg
				p.messageColor = theme.Colors.Success
				p.clearFormAndSelection(true) // Limpa formulário e seleção
				p.loadCNPJs(sess)             // Recarrega a lista de CNPJs
			}
			p.updateButtonStates(sess)
			p.router.GetAppWindow().Invalidate()
		})
	}(p.isEditing, p.selectedCNPJID, cleanedCNPJ, networkID, isActive, currentSession)
}

// handleDeleteCNPJ lida com a exclusão de um CNPJ selecionado.
func (p *CNPJPage) handleDeleteCNPJ(currentSession *auth.SessionData) {
	if p.selectedCNPJID == nil || !p.isEditing || p.isLoading { // Botão deve estar desabilitado se não aplicável
		if p.selectedCNPJID == nil {
			p.statusMessage = "Nenhum CNPJ selecionado para excluir."
			p.messageColor = theme.Colors.Warning
			p.router.GetAppWindow().Invalidate()
		}
		return
	}
	// Permissão já deve ser verificada para habilitar o botão, mas checar novamente é seguro.
	if errPerm := p.permManager.CheckPermission(currentSession, auth.PermCNPJDelete, nil); errPerm != nil {
		p.statusMessage = "Você não tem permissão para excluir CNPJs."
		p.messageColor = theme.Colors.Danger
		p.router.GetAppWindow().Invalidate()
		return
	}

	// TODO: Implementar um diálogo de confirmação antes de excluir.
	// Ex: p.router.GetAppWindow().ShowConfirmDialog("Excluir CNPJ", "Tem certeza?", func(confirmado bool){ if confirmado { ... }})

	p.isLoading = true
	p.statusMessage = "Excluindo CNPJ..."
	p.messageColor = theme.Colors.TextMuted
	p.spinner.Start(p.router.GetAppWindow().Context())
	p.router.GetAppWindow().Invalidate()

	idToDelete := *p.selectedCNPJID // Copia o ID antes de entrar na goroutine
	cnpjToLog := p.cnpjInput.Text() // Pega o CNPJ do formulário para log (pode ser o formatado)

	go func(id uint64, cnpjLabel string, sess *auth.SessionData) {
		var opErr error
		err := s.cnpjService.DeleteCNPJ(id, sess)
		if err != nil {
			opErr = err
		}

		p.router.GetAppWindow().Execute(func() {
			p.isLoading = false
			p.spinner.Stop(p.router.GetAppWindow().Context())
			if opErr != nil {
				p.statusMessage = fmt.Sprintf("Erro ao excluir CNPJ '%s': %v", cnpjLabel, opErr)
				p.messageColor = theme.Colors.Danger
				if errors.Is(opErr, appErrors.ErrNotFound) { // Se já foi excluído por outra ação
					p.statusMessage = fmt.Sprintf("CNPJ '%s' não encontrado (pode já ter sido excluído).", cnpjLabel)
				}
			} else {
				p.statusMessage = fmt.Sprintf("CNPJ '%s' (ID %d) excluído com sucesso.", cnpjLabel, id)
				p.messageColor = theme.Colors.Success
				p.clearFormAndSelection(true) // Limpa formulário e seleção
				p.loadCNPJs(sess)             // Recarrega a lista
			}
			p.updateButtonStates(sess)
			p.router.GetAppWindow().Invalidate()
		})
	}(idToDelete, cnpjToLog, currentSession)
}
