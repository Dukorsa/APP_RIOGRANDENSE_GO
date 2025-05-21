package pages

import (
	"fmt"
	"image/color"
	"regexp"
	"sort"
	"strings"

	// "time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/auth"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/models"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/services"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui/components"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui/theme"
	// "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui/icons"
)

// RoleManagementPage gerencia a UI para Roles e Permissões.
type RoleManagementPage struct {
	router         *ui.Router
	cfg            *core.Config
	roleService    services.RoleService
	auditService   services.AuditLogService // Para logs feitos diretamente pela página, se houver
	permManager    *auth.PermissionManager  // Para listar todas as permissões disponíveis
	sessionManager *auth.SessionManager

	// Estado da UI
	isLoading      bool
	allSystemRoles []*models.RolePublic       // Lista completa de roles carregados
	allSystemPerms map[auth.Permission]string // Todas as permissões definidas no sistema
	selectedRole   *models.RolePublic         // Role atualmente selecionado na lista
	statusMessage  string
	messageColor   color.NRGBA

	// Widgets do Painel Esquerdo (Lista de Roles)
	roleList       layout.List
	roleClickables []widget.Clickable // Um por role na lista
	newRoleBtn     widget.Clickable
	deleteRoleBtn  widget.Clickable

	// Widgets do Painel Direito (Detalhes do Role e Permissões)
	roleNameInput        widget.Editor
	roleDescriptionInput widget.Editor
	permissionCheckboxes map[auth.Permission]*widget.Bool // Checkbox para cada permissão
	permList             layout.List                      // Para scroll das permissões
	saveRoleBtn          widget.Clickable
	cancelChangesBtn     widget.Clickable

	// Controle de edição
	isEditingNewRole bool // True se estiver criando um novo role
	formChanged      bool // True se algo no formulário do painel direito foi alterado

	spinner *components.LoadingSpinner

	firstLoadDone bool
}

// NewRoleManagementPage cria uma nova instância.
func NewRoleManagementPage(
	router *ui.Router,
	cfg *core.Config,
	roleSvc services.RoleService,
	auditSvc services.AuditLogService,
	permMan *auth.PermissionManager,
	sessMan *auth.SessionManager,
) *RoleManagementPage {
	p := &RoleManagementPage{
		router:               router,
		cfg:                  cfg,
		roleService:          roleSvc,
		auditService:         auditSvc,
		permManager:          permMan,
		sessionManager:       sessMan,
		roleList:             layout.List{Axis: layout.Vertical},
		permList:             layout.List{Axis: layout.Vertical},
		spinner:              components.NewLoadingSpinner(),
		allSystemPerms:       permMan.GetAllDefinedPermissions(), // Carrega definições de permissão
		permissionCheckboxes: make(map[auth.Permission]*widget.Bool),
	}
	p.roleNameInput.SingleLine = true
	p.roleNameInput.Hint = "Nome do Perfil (ex: editor_chefe)"
	// p.roleDescriptionInput.SingleLine = false; p.roleDescriptionInput.Hint = "Descrição..." // Editor de múltiplas linhas
	// O widget.Editor padrão é multilinhas se não for SingleLine=true.

	// Inicializa os checkboxes de permissão (uma vez)
	for permKey := range p.allSystemPerms {
		p.permissionCheckboxes[permKey] = new(widget.Bool)
	}

	return p
}

func (p *RoleManagementPage) OnNavigatedTo(params interface{}) {
	appLogger.Info("Navegou para RoleManagementPage")
	p.clearDetailsPanel(false) // Limpa painel direito, mas não a seleção
	if !p.firstLoadDone {
		p.loadRolesAndPermissions() // Carrega roles e preenche a lista de permissões
		p.firstLoadDone = true
	} else {
		// Se já carregou, apenas invalida. A lista de roles pode ter mudado.
		// Poderia recarregar os roles se houver chance de mudança externa.
		p.router.GetAppWindow().Invalidate()
	}
}

func (p *RoleManagementPage) OnNavigatedFrom() {
	appLogger.Info("Navegando para fora da RoleManagementPage")
	p.isLoading = false
	p.spinner.Stop(p.router.GetAppWindow().Context())
}

func (p *RoleManagementPage) loadRolesAndPermissions() {
	p.isLoading = true
	p.statusMessage = "Carregando perfis e permissões..."
	p.messageColor = theme.Colors.TextMuted
	p.spinner.Start(p.router.GetAppWindow().Context())
	p.router.GetAppWindow().Invalidate()

	go func() {
		var loadErr error
		currentSession, errSess := p.sessionManager.GetCurrentSession()
		if errSess != nil || currentSession == nil {
			loadErr = fmt.Errorf("sessão de administrador inválida: %v", errSess)
		} else {
			roles, err := p.roleService.GetAllRoles(currentSession)
			if err != nil {
				loadErr = fmt.Errorf("falha ao carregar perfis: %w", err)
			} else {
				// Ordenar roles para exibição consistente
				sort.SliceStable(roles, func(i, j int) bool {
					return strings.ToLower(roles[i].Name) < strings.ToLower(roles[j].Name)
				})
				p.allSystemRoles = roles
			}
			// Permissões já carregadas em NewRoleManagementPage via p.permManager.GetAllDefinedPermissions()
		}

		p.router.GetAppWindow().Execute(func() {
			p.isLoading = false
			p.spinner.Stop(p.router.GetAppWindow().Context())
			if loadErr != nil {
				p.statusMessage = loadErr.Error()
				p.messageColor = theme.Colors.Danger
				appLogger.Errorf("Erro ao carregar dados para RoleManagementPage: %v", loadErr)
			} else {
				p.statusMessage = fmt.Sprintf("%d perfis carregados.", len(p.allSystemRoles))
				p.messageColor = theme.Colors.Success
				// Ajusta o tamanho do slice de clickables para os roles
				if len(p.allSystemRoles) != len(p.roleClickables) {
					p.roleClickables = make([]widget.Clickable, len(p.allSystemRoles))
				}
			}
			p.updateButtonStates()
			p.router.GetAppWindow().Invalidate()
		})
	}()
}

func (p *RoleManagementPage) Layout(gtx layout.Context) layout.Dimensions {
	th := p.router.GetAppWindow().Theme()

	// Processar eventos/cliques dos botões principais
	if p.newRoleBtn.Clicked(gtx) {
		p.handleNewRole()
	}
	if p.deleteRoleBtn.Clicked(gtx) {
		p.handleDeleteRole()
	}
	if p.saveRoleBtn.Clicked(gtx) {
		p.handleSaveRole()
	}
	if p.cancelChangesBtn.Clicked(gtx) {
		p.handleCancelChanges()
	}

	// Processar eventos de mudança nos inputs do painel direito
	for _, e := range p.roleNameInput.Events(gtx) {
		if _, ok := e.(widget.ChangeEvent); ok {
			p.formChanged = true
			p.updateButtonStates()
		}
	}
	for _, e := range p.roleDescriptionInput.Events(gtx) {
		if _, ok := e.(widget.ChangeEvent); ok {
			p.formChanged = true
			p.updateButtonStates()
		}
	}
	for _, chk := range p.permissionCheckboxes {
		if chk.Update(gtx) { // Update processa o evento de mudança de estado do checkbox
			p.formChanged = true
			p.updateButtonStates()
		}
	}

	// Layout com Splitter (simulado com Flex)
	// Um Splitter real em Gio requer mais trabalho com estado e eventos de arrastar.
	// Vamos usar Flex por simplicidade, mas a proporção pode ser fixa.
	leftPanelWeight := float32(0.3)
	rightPanelWeight := float32(0.7)

	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Flexed(leftPanelWeight, func(gtx C) D {
			return p.layoutLeftPanel(gtx, th)
		}),
		layout.Rigid(func(gtx C) D { // Separador Vertical
			// Linha vertical fina
			//return layout.Inset{Left: unit.Dp(1), Right: unit.Dp(1)}.Layout(gtx,
			//	func(gtx C) D {
			//		paint.FillShape(gtx.Ops, theme.Colors.Border, clip.Rect{Max: image.Pt(gtx.Dp(1), gtx.Constraints.Max.Y)}.Op())
			//		return layout.Dimensions{Size: image.Pt(gtx.Dp(1), gtx.Constraints.Max.Y)}
			//	})
			return layout.Dimensions{} // Sem separador visível por enquanto para simplificar
		}),
		layout.Flexed(rightPanelWeight, func(gtx C) D {
			return p.layoutRightPanel(gtx, th)
		}),
		// TODO: Spinner overlay sobre toda a página
	)
}

func (p *RoleManagementPage) layoutLeftPanel(gtx layout.Context, th *material.Theme) layout.Dimensions {
	// Processar cliques na lista de roles
	for i := range p.allSystemRoles {
		if i >= len(p.roleClickables) {
			break
		}
		if p.roleClickables[i].Clicked(gtx) {
			p.selectRole(p.allSystemRoles[i])
		}
	}

	return layout.Inset{Right: unit.Dp(8)}.Layout(gtx, // Pequeno espaço antes do "divisor"
		func(gtx C) D {
			return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceEnd}.Layout(gtx,
				layout.Rigid(material.Subtitle1(th, "Perfis Existentes").Layout),
				layout.Rigid(layout.Spacer{Height: theme.DefaultVSpacer}.Layout),
				layout.Flexed(1, func(gtx C) D { // Lista de Roles
					return p.roleList.Layout(gtx, len(p.allSystemRoles), func(gtx C, index int) D {
						if index < 0 || index >= len(p.allSystemRoles) {
							return D{}
						}
						role := p.allSystemRoles[index]

						item := material.Clickable(gtx, &p.roleClickables[index], func(gtx C) D {
							label := material.Body1(th, strings.Title(role.Name)) // Capitaliza para exibição
							if p.selectedRole != nil && p.selectedRole.ID == role.ID {
								label.Font.Weight = font.Bold
								label.Color = theme.Colors.Primary
							}
							// TODO: Ícone (ex: cadeado para system role)
							return layout.UniformInset(unit.Dp(8)).Layout(gtx, label.Layout)
						})
						// Adicionar fundo se selecionado
						if p.selectedRole != nil && p.selectedRole.ID == role.ID {
							return layout.Background{Color: theme.Colors.PrimaryLight}.Layout(gtx, item.Layout)
						}
						return item
					})
				}),
				layout.Rigid(layout.Spacer{Height: theme.DefaultVSpacer}.Layout),
				layout.Rigid(func(gtx C) D { // Botões Novo/Excluir
					newBtn := material.Button(th, &p.newRoleBtn, "Novo Perfil")
					delBtn := material.Button(th, &p.deleteRoleBtn, "Excluir Perfil")
					// delBtn.SetEnabled(p.selectedRole != nil && !p.selectedRole.IsSystemRole)
					if p.selectedRole == nil || p.selectedRole.IsSystemRole {
						delBtn.Style.뭄Color = theme.Colors.TextMuted // Visualmente desabilitado
					}

					return layout.Flex{Spacing: layout.SpaceBetween}.Layout(gtx,
						layout.Flexed(1, newBtn.Layout),
						layout.Flexed(1, delBtn.Layout),
					)
				}),
			)
		})
}

func (p *RoleManagementPage) layoutRightPanel(gtx layout.Context, th *material.Theme) layout.Dimensions {
	canEditDetails := p.selectedRole != nil || p.isEditingNewRole
	canEditName := canEditDetails && (p.isEditingNewRole || (p.selectedRole != nil && !p.selectedRole.IsSystemRole))

	nameEditor := material.Editor(th, &p.roleNameInput, p.roleNameInput.Hint)
	// nameEditor.SetEnabled(canEditName) // Controlar interatividade externamente

	descEditor := material.Editor(th, &p.roleDescriptionInput, "Descrição do perfil...")
	// descEditor.SetEnabled(canEditDetails)

	// Agrupar permissões por prefixo para melhor UI
	groupedPerms := make(map[string][]auth.Permission)
	var prefixes []string
	for permKey := range p.allSystemPerms {
		prefix := strings.Split(string(permKey), ":")[0]
		if _, exists := groupedPerms[prefix]; !exists {
			prefixes = append(prefixes, prefix)
		}
		groupedPerms[prefix] = append(groupedPerms[prefix], permKey)
	}
	sort.Strings(prefixes) // Ordena os grupos de permissão

	return layout.Inset{Left: unit.Dp(8)}.Layout(gtx,
		func(gtx C) D {
			return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceEnd}.Layout(gtx,
				layout.Rigid(material.Subtitle1(th, "Detalhes do Perfil").Layout),
				layout.Rigid(layout.Spacer{Height: theme.DefaultVSpacer}.Layout),
				layout.Rigid(p.labeledEditor(gtx, th, "Nome do Perfil:*", nameEditor.Layout, "")), // Feedback aqui?
				layout.Rigid(layout.Spacer{Height: theme.DefaultVSpacer}.Layout),
				layout.Rigid(p.labeledEditor(gtx, th, "Descrição:", descEditor.Layout, "")),
				layout.Rigid(layout.Spacer{Height: theme.LargeVSpacer}.Layout),

				layout.Rigid(material.Subtitle1(th, "Permissões Associadas").Layout),
				layout.Rigid(layout.Spacer{Height: theme.DefaultVSpacer}.Layout),
				layout.Flexed(1, func(gtx C) D { // Lista de Permissões com Scroll
					return p.permList.Layout(gtx, len(prefixes), func(gtx C, groupIndex int) D {
						prefix := prefixes[groupIndex]
						permsInGroup := groupedPerms[prefix]
						sort.Slice(permsInGroup, func(i, j int) bool { return permsInGroup[i] < permsInGroup[j] })

						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx C) D {
								lbl := material.Body2(th, strings.Title(prefix))
								lbl.Font.Weight = font.Bold
								return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(4)}.Layout(gtx, lbl.Layout)
							}),
							layout.Rigid(func(gtx C) D {
								permGroupLayout := layout.List{Axis: layout.Vertical}
								return permGroupLayout.Layout(gtx, len(permsInGroup), func(gtx C, permIndex int) D {
									permKey := permsInGroup[permIndex]
									permDesc := p.allSystemPerms[permKey]
									chk, ok := p.permissionCheckboxes[permKey]
									if !ok {
										return D{}
									}

									cb := material.CheckBox(th, chk, string(permKey))
									// cb.SetEnabled(canEditDetails)
									// TODO: Adicionar tooltip com permDesc
									return layout.Inset{Left: unit.Dp(10), Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, cb.Layout)
								})
							}),
						)
					})
				}),
				layout.Rigid(layout.Spacer{Height: theme.DefaultVSpacer}.Layout),
				layout.Rigid(func(gtx C) D { // Mensagem de Status e Botões Salvar/Cancelar
					if p.statusMessage != "" && !p.isLoading {
						lblStatus := material.Body2(th, p.statusMessage)
						lblStatus.Color = p.messageColor
						layout.Inset{Bottom: theme.DefaultVSpacer}.Layout(gtx, lblStatus.Layout)
					}

					saveBtnWidget := material.Button(th, &p.saveRoleBtn, "Salvar Alterações")
					if p.isEditingNewRole {
						saveBtnWidget.Text = "Criar Novo Perfil"
					}
					// saveBtnWidget.SetEnabled(canEditDetails && p.formChanged && !p.isLoading)
					if !(canEditDetails && p.formChanged && !p.isLoading) {
						saveBtnWidget.Style.뭄Color = theme.Colors.TextMuted
					}

					cancelBtnWidget := material.Button(th, &p.cancelChangesBtn, "Cancelar")
					// cancelBtnWidget.SetEnabled(canEditDetails && !p.isLoading)
					if !(canEditDetails && !p.isLoading) {
						cancelBtnWidget.Style.뭄Color = theme.Colors.TextMuted
					}

					return layout.Flex{Spacing: layout.SpaceBetween, Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, func(gtx C) D { return layout.Dimensions{} }), // Espaçador
						layout.Rigid(cancelBtnWidget.Layout),
						layout.Rigid(layout.Spacer{Width: theme.DefaultVSpacer}.Layout),
						layout.Rigid(saveBtnWidget.Layout),
					)
				}),
			)
		})
}

// --- Lógica de Ações ---
func (p *RoleManagementPage) selectRole(role *models.RolePublic) {
	if p.isLoading {
		return
	}
	p.statusMessage = ""

	if p.formChanged && p.selectedRole != nil {
		// TODO: Mostrar diálogo "Descartar alterações não salvas?"
		// Por agora, apenas loga e continua.
		appLogger.Warnf("Mudando de role com alterações não salvas no role '%s'", p.selectedRole.Name)
	}

	p.selectedRole = role
	p.isEditingNewRole = false
	p.formChanged = false // Resetar ao selecionar novo role

	if role != nil {
		p.roleNameInput.SetText(role.Name)
		desc := ""
		if role.Description != nil {
			desc = *role.Description
		}
		p.roleDescriptionInput.SetText(desc)

		// Marcar checkboxes de permissão
		currentPermsSet := make(map[auth.Permission]bool)
		for _, pNameStr := range role.Permissions {
			currentPermsSet[auth.Permission(pNameStr)] = true
		}
		for permKey, chk := range p.permissionCheckboxes {
			chk.Value = currentPermsSet[permKey]
		}
		appLogger.Debugf("Role '%s' selecionado. %d permissões carregadas nos checkboxes.", role.Name, len(currentPermsSet))
	} else {
		p.clearDetailsPanel(false) // Limpa se role for nil (ex: desseleção)
	}
	p.updateButtonStates()
	p.router.GetAppWindow().Invalidate()
}

func (p *RoleManagementPage) clearDetailsPanel(resetSelection bool) {
	if resetSelection {
		p.selectedRole = nil
		// TODO: Limpar seleção na QListWidget (se Gio List tiver essa noção)
	}
	p.isEditingNewRole = false
	p.formChanged = false
	p.roleNameInput.SetText("")
	p.roleDescriptionInput.SetText("")
	for _, chk := range p.permissionCheckboxes {
		chk.Value = false
	}
	p.statusMessage = ""
	p.updateButtonStates()
	// p.router.GetAppWindow().Invalidate() // Será invalidado pelo chamador ou próximo Layout
}

func (p *RoleManagementPage) updateButtonStates() {
	// Botão Excluir
	canDelete := p.selectedRole != nil && !p.selectedRole.IsSystemRole && !p.isLoading
	// deleteBtn.SetEnabled(canDelete)

	// Botão Salvar
	canEditDetails := (p.selectedRole != nil || p.isEditingNewRole) && !p.isLoading
	canSave := canEditDetails && p.formChanged
	// saveRoleBtn.SetEnabled(canSave)

	// Botão Cancelar
	// cancelChangesBtn.SetEnabled(canEditDetails)

	p.router.GetAppWindow().Invalidate() // Para redesenhar os botões com novo estado (visual)
}

func (p *RoleManagementPage) handleNewRole() {
	if p.isLoading {
		return
	}
	if p.formChanged && p.selectedRole != nil {
		// TODO: Diálogo "Descartar alterações?"
		appLogger.Warnf("Iniciando novo role com alterações não salvas no role '%s'", p.selectedRole.Name)
	}
	p.clearDetailsPanel(true) // Limpa seleção da lista e painel direito
	p.isEditingNewRole = true
	p.roleNameInput.Focus()
	p.updateButtonStates()
	p.statusMessage = "Preencha os dados para o novo perfil."
	p.messageColor = theme.Colors.Info
	p.router.GetAppWindow().Invalidate()
}

func (p *RoleManagementPage) handleSaveRole() {
	if p.isLoading {
		return
	}
	p.statusMessage = ""

	roleName := strings.TrimSpace(p.roleNameInput.Text())
	roleDescriptionText := strings.TrimSpace(p.roleDescriptionInput.Text())
	var roleDescription *string
	if roleDescriptionText != "" {
		roleDescription = &roleDescriptionText
	}

	selectedPermNames := []string{}
	for permKey, chk := range p.permissionCheckboxes {
		if chk.Value {
			selectedPermNames = append(selectedPermNames, string(permKey))
		}
	}

	// Validação básica
	if roleName == "" {
		p.statusMessage = "Nome do perfil é obrigatório."
		p.messageColor = theme.Colors.Danger
		p.router.GetAppWindow().Invalidate()
		return
	}
	if matched, _ := regexp.MatchString(`^[a-zA-Z0-9_]{3,50}$`, roleName); !matched {
		p.statusMessage = "Nome: 3-50 caracteres (letras, números, _)."
		p.messageColor = theme.Colors.Danger
		p.router.GetAppWindow().Invalidate()
		return
	}

	p.isLoading = true
	p.statusMessage = "Salvando perfil..."
	p.messageColor = theme.Colors.TextMuted
	p.spinner.Start(p.router.GetAppWindow().Context())
	p.router.GetAppWindow().Invalidate()

	go func(isNew bool, currentRoleID uint64, name, descText string, descPtr *string, perms []string) {
		var opErr error
		var successMsg string
		currentAdminSession, _ := p.sessionManager.GetCurrentSession()

		if isNew {
			createData := models.RoleCreate{Name: name, Description: descPtr, PermissionNames: perms}
			if errVal := createData.CleanAndValidate(); errVal != nil { // Valida e normaliza
				opErr = errVal
			} else {
				newRole, err := p.roleService.CreateRole(createData, currentAdminSession)
				if err != nil {
					opErr = err
				} else {
					successMsg = fmt.Sprintf("Perfil '%s' criado!", newRole.Name)
				}
			}
		} else {
			updateData := models.RoleUpdate{}
			changed := false
			if p.selectedRole.Name != name {
				updateData.Name = &name
				changed = true
			}
			currentDesc := ""
			if p.selectedRole.Description != nil {
				currentDesc = *p.selectedRole.Description
			}
			if currentDesc != descText {
				updateData.Description = descPtr
				changed = true
			}

			// Compara permissões
			currentPermsSet := make(map[string]bool)
			for _, pStr := range p.selectedRole.Permissions {
				currentPermsSet[pStr] = true
			}
			newPermsSet := make(map[string]bool)
			for _, pStr := range perms {
				newPermsSet[pStr] = true
			}
			if len(currentPermsSet) != len(newPermsSet) || !mapsEqual(currentPermsSet, newPermsSet) {
				updateData.PermissionNames = &perms
				changed = true
			}

			if !changed { // Nada mudou
				successMsg = fmt.Sprintf("Nenhuma alteração para o perfil '%s'.", name)
			} else {
				if errVal := updateData.CleanAndValidate(); errVal != nil {
					opErr = errVal
				} else {
					updatedRole, err := p.roleService.UpdateRole(currentRoleID, updateData, currentAdminSession)
					if err != nil {
						opErr = err
					} else {
						successMsg = fmt.Sprintf("Perfil '%s' atualizado!", updatedRole.Name)
					}
				}
			}
		}

		p.router.GetAppWindow().Execute(func() {
			p.isLoading = false
			p.spinner.Stop(p.router.GetAppWindow().Context())
			if opErr != nil {
				p.statusMessage = fmt.Sprintf("Erro ao salvar: %v", opErr)
				p.messageColor = theme.Colors.Danger
				appLogger.Errorf("Erro ao salvar role '%s': %v", name, opErr)
			} else {
				p.statusMessage = successMsg
				p.messageColor = theme.Colors.Success
				p.formChanged = false
				p.isEditingNewRole = false  // Sai do modo de novo role
				p.loadRolesAndPermissions() // Recarrega lista e seleciona o role (se possível)
				// TODO: Selecionar o role recém-criado/editado na lista
			}
			p.updateButtonStates()
			p.router.GetAppWindow().Invalidate()
		})
	}(p.isEditingNewRole, p.selectedRole.ID, roleName, roleDescriptionText, roleDescription, selectedPermNames)
}

func (p *RoleManagementPage) handleCancelChanges() {
	if p.isLoading {
		return
	}
	if p.isEditingNewRole {
		p.clearDetailsPanel(true) // Limpa tudo, incluindo seleção da lista
	} else if p.selectedRole != nil {
		p.selectRole(p.selectedRole) // Recarrega dados do role selecionado, descartando mudanças
	} else {
		p.clearDetailsPanel(true)
	}
	p.formChanged = false
	p.updateButtonStates()
	p.router.GetAppWindow().Invalidate()
}

func (p *RoleManagementPage) handleDeleteRole() {
	if p.isLoading || p.selectedRole == nil || p.selectedRole.IsSystemRole {
		if p.selectedRole != nil && p.selectedRole.IsSystemRole {
			p.statusMessage = "Perfis do sistema não podem ser excluídos."
			p.messageColor = theme.Colors.Warning
		}
		return
	}

	// TODO: Diálogo de confirmação para exclusão

	p.isLoading = true
	p.statusMessage = fmt.Sprintf("Excluindo perfil '%s'...", p.selectedRole.Name)
	p.messageColor = theme.Colors.TextMuted
	p.spinner.Start(p.router.GetAppWindow().Context())
	p.router.GetAppWindow().Invalidate()

	go func(roleIDToDelete uint64, roleNameToLog string) {
		var opErr error
		currentAdminSession, _ := p.sessionManager.GetCurrentSession()
		err := p.roleService.DeleteRole(roleIDToDelete, currentAdminSession)
		if err != nil {
			opErr = err
		}

		p.router.GetAppWindow().Execute(func() {
			p.isLoading = false
			p.spinner.Stop(p.router.GetAppWindow().Context())
			if opErr != nil {
				p.statusMessage = fmt.Sprintf("Erro ao excluir '%s': %v", roleNameToLog, opErr)
				p.messageColor = theme.Colors.Danger
				appLogger.Errorf("Erro ao excluir role '%s': %v", roleNameToLog, opErr)
			} else {
				p.statusMessage = fmt.Sprintf("Perfil '%s' excluído com sucesso!", roleNameToLog)
				p.messageColor = theme.Colors.Success
				appLogger.Infof("Perfil '%s' excluído.", roleNameToLog)
				p.clearDetailsPanel(true)
				p.loadRolesAndPermissions() // Recarrega lista
			}
			p.updateButtonStates()
			p.router.GetAppWindow().Invalidate()
		})
	}(p.selectedRole.ID, p.selectedRole.Name)
}
