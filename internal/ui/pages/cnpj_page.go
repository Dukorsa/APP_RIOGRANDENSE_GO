package pages

import (
	"fmt"
	"image/color"
	"regexp"
	"strconv"
	"strings"

	// "time" // Se precisar para formatação de data, mas CNPJ não tem tanta data visível

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/seu_usuario/riograndense_gio/internal/auth"
	"github.com/seu_usuario/riograndense_gio/internal/core"
	appErrors "github.com/seu_usuario/riograndense_gio/internal/core/errors"
	appLogger "github.com/seu_usuario/riograndense_gio/internal/core/logger"
	"github.com/seu_usuario/riograndense_gio/internal/data/models"
	"github.com/seu_usuario/riograndense_gio/internal/services"
	"github.com/seu_usuario/riograndense_gio/internal/ui"            // Para Router e PageID
	"github.com/seu_usuario/riograndense_gio/internal/ui/components" // Para LoadingSpinner
	"github.com/seu_usuario/riograndense_gio/internal/ui/theme"      // Para Cores
	"github.com/seu_usuario/riograndense_gio/internal/utils"         // Para IsValidCNPJ
)

// CNPJPage gerencia a interface para CNPJs.
type CNPJPage struct {
	router         *ui.Router
	cfg            *core.Config
	cnpjService    services.CNPJService
	networkService services.NetworkService // Para validar NetworkID ou popular um ComboBox
	permManager    *auth.PermissionManager
	sessionManager *auth.SessionManager

	// Estado da UI
	isLoading      bool
	cnpjs          []*models.CNPJPublic // Lista completa de CNPJs carregados
	filteredCNPJs  []*models.CNPJPublic // CNPJs após filtro (se houver filtro na página)
	selectedCNPJID *uint64              // ID do CNPJ selecionado na lista para edição
	statusMessage  string
	messageColor   color.NRGBA

	// Widgets de formulário
	cnpjInput      widget.Editor
	networkIDInput widget.Editor
	statusEnum     widget.Enum // Para o ComboBox de status Ativo/Inativo

	// Feedback para inputs
	cnpjInputFeedback      string
	networkIDInputFeedback string

	// Botões de ação do formulário
	saveBtn          widget.Clickable
	clearOrCancelBtn widget.Clickable

	// Botões de ação da lista
	deleteBtn      widget.Clickable
	refreshListBtn widget.Clickable

	// Para a lista/tabela de CNPJs
	cnpjList       layout.List
	cnpjClickables []widget.Clickable
	// TODO: Cabeçalhos clicáveis para ordenação se necessário
	// tableHeaderClicks [4]widget.Clickable
	// sortColumn        int
	// sortAscending     bool

	spinner *components.LoadingSpinner

	firstLoadDone bool
	isEditing     bool // True se um CNPJ existente estiver carregado no formulário
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
		spinner:        components.NewLoadingSpinner(),
		// sortColumn:     2, // Default sort por Data Cadastro (se existir)
		// sortAscending:  false,
	}
	p.cnpjInput.SingleLine = true
	p.cnpjInput.Hint = "XX.XXX.XXX/XXXX-XX"
	p.networkIDInput.SingleLine = true
	p.networkIDInput.Hint = "ID numérico da Rede"
	p.networkIDInput.Filter = "0123456789" // Apenas números

	p.statusEnum.Set("Ativo", "Ativo") // Key, Label
	p.statusEnum.Set("Inativo", "Inativo")
	p.statusEnum.Value = "Ativo" // Default

	return p
}

func (p *CNPJPage) OnNavigatedTo(params interface{}) {
	appLogger.Info("Navegou para CNPJPage")
	p.clearFormAndSelection(false) // Limpa formulário, mas não necessariamente a lista
	if !p.firstLoadDone {
		p.loadCNPJs()
		p.firstLoadDone = true
	} else {
		// A lista já pode estar carregada, talvez apenas aplicar filtros
		p.applyFiltersAndSort() // Implementar se houver filtros
		p.router.GetAppWindow().Invalidate()
	}
	// Verificar permissão de visualização
	currentSession, _ := p.sessionManager.GetCurrentSession()
	if err := p.permManager.CheckPermission(currentSession, auth.PermCNPJView, nil); err != nil {
		p.statusMessage = fmt.Sprintf("Acesso negado: %v", err)
		p.messageColor = theme.Colors.Danger
		p.cnpjs = []*models.CNPJPublic{} // Limpa dados se não tem permissão
		p.filteredCNPJs = []*models.CNPJPublic{}
	}
}

func (p *CNPJPage) OnNavigatedFrom() {
	appLogger.Info("Navegando para fora da CNPJPage")
	p.selectedCNPJID = nil
	p.isEditing = false
}

func (p *CNPJPage) loadCNPJs() {
	p.isLoading = true
	p.statusMessage = "Carregando CNPJs..."
	p.messageColor = theme.Colors.TextMuted
	p.spinner.Start(p.router.GetAppWindow().Context())
	p.router.GetAppWindow().Invalidate()

	go func() {
		var loadErr error
		currentSession, errSess := p.sessionManager.GetCurrentSession()
		if errSess != nil || currentSession == nil {
			loadErr = fmt.Errorf("sessão de usuário inválida: %v", errSess)
		} else {
			cnpjs, err := p.cnpjService.GetAllCNPJs(true, currentSession) // true = include inactive
			if err != nil {
				loadErr = fmt.Errorf("falha ao carregar CNPJs: %w", err)
			} else {
				p.cnpjs = cnpjs
			}
		}

		p.router.GetAppWindow().Execute(func() {
			p.isLoading = false
			p.spinner.Stop(p.router.GetAppWindow().Context())
			if loadErr != nil {
				p.statusMessage = loadErr.Error()
				p.messageColor = theme.Colors.Danger
				appLogger.Errorf("Erro ao carregar CNPJs para CNPJPage: %v", loadErr)
			} else {
				p.statusMessage = fmt.Sprintf("%d CNPJs carregados.", len(p.cnpjs))
				p.messageColor = theme.Colors.Success
				p.applyFiltersAndSort() // Aplica filtros e ordenação após carregar
			}
			p.updateButtonStates()
			p.router.GetAppWindow().Invalidate()
		})
	}()
}

func (p *CNPJPage) applyFiltersAndSort() {
	// TODO: Implementar filtro e ordenação se a página tiver esses controles.
	// Por agora, apenas copia todos.
	p.filteredCNPJs = make([]*models.CNPJPublic, len(p.cnpjs))
	copy(p.filteredCNPJs, p.cnpjs)

	if len(p.filteredCNPJs) != len(p.cnpjClickables) {
		p.cnpjClickables = make([]widget.Clickable, len(p.filteredCNPJs))
	}
}

func (p *CNPJPage) Layout(gtx layout.Context) layout.Dimensions {
	th := p.router.GetAppWindow().th

	// Processar eventos dos widgets de formulário
	for _, e := range p.cnpjInput.Events(gtx) {
		if _, ok := e.(widget.ChangeEvent); ok {
			p.formatCNPJInput()      // Formata enquanto digita
			p.cnpjInputFeedback = "" // Limpa feedback ao digitar
			p.updateButtonStates()
		}
	}
	for _, e := range p.networkIDInput.Events(gtx) {
		if _, ok := e.(widget.ChangeEvent); ok {
			p.networkIDInputFeedback = ""
			p.updateButtonStates()
		}
	}
	if p.statusEnum.Update(gtx) { // Se o valor do ComboBox mudar
		p.updateButtonStates()
	}

	// Processar cliques nos botões
	if p.saveBtn.Clicked(gtx) {
		p.handleSaveCNPJ()
	}
	if p.clearOrCancelBtn.Clicked(gtx) {
		p.clearFormAndSelection(true)
	}
	if p.deleteBtn.Clicked(gtx) {
		p.handleDeleteCNPJ()
	}
	if p.refreshListBtn.Clicked(gtx) {
		p.loadCNPJs()
	}

	// Layout da página
	return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceEnd}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.layoutForm(gtx, th)
		}),
		layout.Rigid(layout.Spacer{Height: theme.LargeVSpacer}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return p.layoutCNPJTable(gtx, th)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.statusMessage != "" {
				lbl := material.Body2(th, p.statusMessage)
				lbl.Color = p.messageColor
				return layout.Inset{Top: theme.DefaultVSpacer}.Layout(gtx, lbl.Layout)
			}
			return layout.Dimensions{}
		}),
		// Spinner (desenhado por último para ficar no topo se visível)
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.isLoading {
				// Posicionar spinner (exemplo: centralizado na área da página)
				// Este é um desafio em Gio. A AppWindow pode ter um spinner global
				// ou usar layout.Stack aqui.
				// return p.spinner.Layout(gtx)
			}
			return layout.Dimensions{}
		}),
	)
}

// layoutForm desenha o formulário de entrada/edição.
func (p *CNPJPage) layoutForm(gtx layout.Context, th *material.Theme) layout.Dimensions {
	title := "Novo CNPJ"
	if p.isEditing {
		title = "Editar CNPJ Selecionado"
	}

	return material.Card(th, theme.Colors.BackgroundAlt, 主题.ElevationSmall, layout.UniformInset(unit.Dp(16)),
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceSides}.Layout(gtx,
				layout.Rigid(material.Subtitle1(th, title).Layout),
				layout.Rigid(layout.Spacer{Height: theme.DefaultVSpacer}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { // CNPJ Input
					return p.labeledEditor(gtx, th, "CNPJ:*", &p.cnpjInput, p.cnpjInputFeedback, !p.isEditing)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { // Network ID Input
					return p.labeledEditor(gtx, th, "ID da Rede:*", &p.networkIDInput, p.networkIDInputFeedback, true)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { // Status ComboBox
					if !p.isEditing {
						return layout.Dimensions{}
					} // Só mostra status na edição
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
						layout.Rigid(material.Body1(th, "Status:").Layout),
						layout.Rigid(material.DropDown(th, &p.statusEnum, material.Body1(th, p.statusEnum.Value).Layout).Layout(gtx)),
					)
				}),
				layout.Rigid(layout.Spacer{Height: theme.LargeVSpacer}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { // Botões do Formulário
					saveButton := material.Button(th, &p.saveBtn, "Salvar")
					if !p.isEditing {
						saveButton.Text = "Adicionar Novo"
					}

					clearButton := material.Button(th, &p.clearOrCancelBtn, "Limpar")
					if p.isEditing {
						clearButton.Text = "Cancelar Edição"
					}

					// Habilitar/Desabilitar botão Salvar
					// saveButton.setEnable(p.canSaveForm()) // Implementar canSaveForm

					return layout.Flex{Spacing: layout.SpaceBetween}.Layout(gtx,
						layout.Flexed(1, clearButton.Layout),
						layout.Flexed(1, saveButton.Layout),
					)
				}),
			)
		}).Layout(gtx)
}

// labeledEditor é um helper para criar um Label + Editor + FeedbackLabel.
func (p *CNPJPage) labeledEditor(gtx layout.Context, th *material.Theme, label string, editor *widget.Editor, feedbackText string, enabled bool) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical, Spacing: layout.Tight}.Layout(gtx,
		layout.Rigid(material.Body1(th, label).Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			ed := material.Editor(th, editor, editor.Hint)
			// ed.setEnable(enabled) // GIO editor não tem SetEnabled, o widget pai controla.
			// A interatividade do editor é controlada pela FocusPolicy e se ele recebe eventos.
			// Se o widget pai estiver desabilitado, o editor também estará.
			// Para desabilitar visualmente, pode-se mudar a cor de fundo/texto.
			return ed.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if feedbackText != "" {
				lbl := material.Body2(th, feedbackText)
				lbl.Color = theme.Colors.Danger
				return layout.Inset{Top: unit.Dp(2)}.Layout(gtx, lbl.Layout)
			}
			return layout.Dimensions{}
		}),
	)
}

// layoutCNPJTable desenha a lista/tabela de CNPJs.
func (p *CNPJPage) layoutCNPJTable(gtx layout.Context, th *material.Theme) layout.Dimensions {
	// Cabeçalhos
	headers := []string{"CNPJ", "Rede (ID)", "Data Cadastro", "Status"}
	colWeights := []float32{0.3, 0.2, 0.3, 0.2}

	// Renderização de linha
	rowLayout := func(gtx layout.Context, index int, cnpj *models.CNPJPublic) layout.Dimensions {
		isSelected := p.selectedCNPJID != nil && *p.selectedCNPJID == cnpj.ID
		bgColor := theme.Colors.Background
		if index%2 != 0 {
			bgColor = theme.Colors.BackgroundAlt
		}
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

			return layout.Background{Color: bgColor}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{}.Layout(gtx,
							layout.Flexed(colWeights[0], material.Body2(th, cnpj.FormatCNPJ()).Layout),
							layout.Flexed(colWeights[1], material.Body2(th, fmt.Sprint(cnpj.NetworkID)).Layout),
							layout.Flexed(colWeights[2], material.Body2(th, cnpj.RegistrationDate.Format("02/01/2006 15:04")).Layout),
							layout.Flexed(colWeights[3], material.Body2(th, boolToString(cnpj.Active, "Ativo", "Inativo")).Layout),
						)
					})
			})
		})
	}

	// Processar cliques na linha
	for i := range p.filteredCNPJs {
		if i >= len(p.cnpjClickables) {
			break
		} // Segurança
		if p.cnpjClickables[i].Clicked(gtx) {
			p.loadCNPJIntoForm(p.filteredCNPJs[i])
		}
	}

	// Botão de Excluir (ao lado da lista, ou abaixo dela)
	deleteButton := material.Button(th, &p.deleteBtn, "Excluir Selecionado")
	deleteButton.Style.Font.Weight = font.Bold
	// deleteButton.setEnable(p.selectedCNPJID != nil && p.canDelete()) // Implementar canDelete (permissão)

	return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceEnd}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { // Cabeçalho
			return layout.Background{Color: theme.Colors.Grey100}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{}.Layout(gtx,
								layout.Flexed(colWeights[0], material.Body1(th, headers[0]).Layout),
								layout.Flexed(colWeights[1], material.Body1(th, headers[1]).Layout),
								layout.Flexed(colWeights[2], material.Body1(th, headers[2]).Layout),
								layout.Flexed(colWeights[3], material.Body1(th, headers[3]).Layout),
							)
						})
				})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { // Lista
			return p.cnpjList.Layout(gtx, len(p.filteredCNPJs), func(gtx layout.Context, index int) layout.Dimensions {
				if index < 0 || index >= len(p.filteredCNPJs) {
					return layout.Dimensions{}
				}
				return rowLayout(gtx, index, p.filteredCNPJs[index])
			})
		}),
		layout.Rigid(layout.Spacer{Height: theme.DefaultVSpacer}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { // Botões abaixo da lista
			return layout.Flex{Spacing: layout.SpaceBetween}.Layout(gtx,
				layout.Rigid(deleteButton.Layout),
				layout.Flexed(1, func(gtx C) D { return D{} }), // Espaçador
				layout.Rigid(material.Button(th, &p.refreshListBtn, "Atualizar Lista").Layout),
			)
		}),
	)
}

// --- Lógica de Ações ---
func (p *CNPJPage) formatCNPJInput() {
	text := p.cnpjInput.Text()
	cleaned := models.CleanCNPJ(text) // Reutiliza helper do models

	// Evitar reformatar se já estiver no formato máximo ou se a limpeza não mudou muito
	// (para evitar loop de cursor com formatação muito agressiva)
	if len(cleaned) > 14 {
		cleaned = cleaned[:14]
	}

	formatted := cleaned // Inicia com limpo
	if len(cleaned) > 2 {
		formatted = cleaned[:2] + "." + cleaned[2:]
	}
	if len(cleaned) > 5 {
		formatted = cleaned[:2] + "." + cleaned[2:5] + "." + cleaned[5:]
	}
	if len(cleaned) > 8 {
		formatted = cleaned[:2] + "." + cleaned[2:5] + "." + cleaned[5:8] + "/" + cleaned[8:]
	}
	if len(cleaned) > 12 {
		formatted = cleaned[:2] + "." + cleaned[2:5] + "." + cleaned[5:8] + "/" + cleaned[8:12] + "-" + cleaned[12:]
	}

	if p.cnpjInput.Text() != formatted {
		// Salvar posição do cursor (desafio em Gio editor puro)
		// p.cnpjInput.SetText(formatted) // Causa loop se não for cuidadoso
		// Por agora, a limpeza e validação mais forte ocorrerá no submit
	}
}

func (p *CNPJPage) loadCNPJIntoForm(cnpj *models.CNPJPublic) {
	if cnpj == nil {
		p.clearFormAndSelection(false)
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
	p.cnpjInputFeedback = ""
	p.networkIDInputFeedback = ""
	p.updateButtonStates()
	appLogger.Debugf("CNPJ ID %d carregado no formulário para edição.", cnpj.ID)
}

func (p *CNPJPage) clearFormAndSelection(clearListSelection bool) {
	p.cnpjInput.SetText("")
	p.networkIDInput.SetText("")
	p.statusEnum.Value = "Ativo" // Default
	p.cnpjInputFeedback = ""
	p.networkIDInputFeedback = ""
	p.selectedCNPJID = nil
	p.isEditing = false

	// if clearListSelection {
	// TODO: Como desselecionar item na lista em Gio?
	// Normalmente, apenas não desenhar o destaque de seleção.
	// O estado p.selectedCNPJID = nil já cuida disso.
	// }
	p.updateButtonStates()
	appLogger.Debug("Formulário de CNPJ e seleção limpos.")
}

func (p *CNPJPage) updateButtonStates() {
	// Habilitar/desabilitar botão de salvar do formulário
	canSave := p.cnpjInput.Text() != "" && p.networkIDInput.Text() != ""
	// saveBtn.SetEnabled(canSave) // Em Gio, o botão não é desabilitado, mas a ação pode ser ignorada
	// ou o visual do botão pode mudar.

	// Habilitar/desabilitar botão de excluir
	canDelete := p.selectedCNPJID != nil && p.isEditing
	// deleteBtn.SetEnabled(canDelete)

	// Forçar redesenho para que os botões reflitam o estado
	p.router.GetAppWindow().Invalidate()
}

func (p *CNPJPage) handleSaveCNPJ() {
	// Validação
	cnpjRaw := p.cnpjInput.Text()
	networkIDStr := p.networkIDInput.Text()
	isActive := p.statusEnum.Value == "Ativo"

	cleanedCNPJ, validationErr := models.CNPJCreate{CNPJ: cnpjRaw}.CleanAndValidateCNPJ() // Usa o helper
	if validationErr != nil {
		p.cnpjInputFeedback = validationErr.Error() // Assumindo que o erro é uma string simples
		p.router.GetAppWindow().Invalidate()
		return
	}
	if !utils.IsValidCNPJ(cleanedCNPJ) {
		p.cnpjInputFeedback = "CNPJ inválido (dígitos verificadores)."
		p.router.GetAppWindow().Invalidate()
		return
	}

	networkID, errConv := strconv.ParseUint(networkIDStr, 10, 64)
	if errConv != nil || networkID == 0 {
		p.networkIDInputFeedback = "ID da Rede inválido."
		p.router.GetAppWindow().Invalidate()
		return
	}

	p.isLoading = true
	p.statusMessage = "Salvando CNPJ..."
	p.messageColor = theme.Colors.TextMuted
	p.spinner.Start(p.router.GetAppWindow().Context())
	p.router.GetAppWindow().Invalidate()

	go func(isEditMode bool, cID *uint64, cnpjLabel, netIDLabel uint64, activeLabel bool) {
		var opErr error
		var successMsg string
		currentSession, _ := p.sessionManager.GetCurrentSession()

		if isEditMode {
			updateData := models.CNPJUpdate{NetworkID: &netIDLabel, Active: &activeLabel}
			_, err := p.cnpjService.UpdateCNPJ(*cID, updateData, currentSession)
			if err != nil {
				opErr = err
			} else {
				successMsg = fmt.Sprintf("CNPJ ID %d atualizado.", *cID)
			}
		} else {
			createData := models.CNPJCreate{CNPJ: models.CleanCNPJ(p.cnpjInput.Text()), NetworkID: netIDLabel} // Passa limpo
			newCNPJ, err := p.cnpjService.RegisterCNPJ(createData, currentSession)
			if err != nil {
				opErr = err
			} else {
				successMsg = fmt.Sprintf("CNPJ %s cadastrado.", newCNPJ.FormatCNPJ())
			}
		}

		p.router.GetAppWindow().Execute(func() {
			p.isLoading = false
			p.spinner.Stop(p.router.GetAppWindow().Context())
			if opErr != nil {
				p.statusMessage = fmt.Sprintf("Erro ao salvar: %v", opErr)
				p.messageColor = theme.Colors.Danger
				if strings.Contains(opErr.Error(), "CNPJ") {
					p.cnpjInputFeedback = opErr.Error()
				}
				if strings.Contains(opErr.Error(), "Rede") {
					p.networkIDInputFeedback = opErr.Error()
				}

			} else {
				p.statusMessage = successMsg
				p.messageColor = theme.Colors.Success
				p.clearFormAndSelection(true)
				p.loadCNPJs() // Recarrega a lista
			}
			p.updateButtonStates()
			p.router.GetAppWindow().Invalidate()
		})
	}(p.isEditing, p.selectedCNPJID, networkID, isActive) // Passa cópias dos valores
}

func (p *CNPJPage) handleDeleteCNPJ() {
	if p.selectedCNPJID == nil || !p.isEditing {
		p.statusMessage = "Nenhum CNPJ selecionado para excluir."
		p.messageColor = theme.Colors.Warning
		return
	}

	// TODO: Mostrar diálogo de confirmação
	// Por agora, deleta diretamente.

	p.isLoading = true
	p.statusMessage = "Excluindo CNPJ..."
	p.messageColor = theme.Colors.TextMuted
	p.spinner.Start(p.router.GetAppWindow().Context())
	p.router.GetAppWindow().Invalidate()

	go func(idToDelete uint64) {
		var opErr error
		currentSession, _ := p.sessionManager.GetCurrentSession()
		err := p.cnpjService.DeleteCNPJ(idToDelete, currentSession)
		if err != nil {
			opErr = err
		}

		p.router.GetAppWindow().Execute(func() {
			p.isLoading = false
			p.spinner.Stop(p.router.GetAppWindow().Context())
			if opErr != nil {
				p.statusMessage = fmt.Sprintf("Erro ao excluir: %v", opErr)
				p.messageColor = theme.Colors.Danger
			} else {
				p.statusMessage = fmt.Sprintf("CNPJ ID %d excluído com sucesso.", idToDelete)
				p.messageColor = theme.Colors.Success
				p.clearFormAndSelection(true)
				p.loadCNPJs()
			}
			p.updateButtonStates()
			p.router.GetAppWindow().Invalidate()
		})
	}(*p.selectedCNPJID)
}
