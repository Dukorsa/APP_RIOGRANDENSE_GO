package pages

import (
	"fmt"
	"image/color"
	"regexp"
	"sort"
	"strings"

	// "time" // Não usado diretamente para timers aqui

	"gioui.org/font"
	"gioui.org/layout"

	// "gioui.org/op/clip"   // Para desenhar separador, se necessário
	// "gioui.org/op/paint"  // Para desenhar separador
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
)

// RoleManagementPage gerencia a UI para Roles (Perfis) e suas Permissões.
type RoleManagementPage struct {
	router         *ui.Router
	cfg            *core.Config
	roleService    services.RoleService
	auditService   services.AuditLogService
	permManager    *auth.PermissionManager // Para listar todas as permissões disponíveis e verificar PermRoleManage.
	sessionManager *auth.SessionManager

	// Estado da UI
	isLoading      bool
	allSystemRoles []*models.RolePublic       // Lista completa de roles carregados do serviço.
	allSystemPerms map[auth.Permission]string // Todas as permissões definidas no sistema (cacheado).
	sortedPermKeys []auth.Permission          // Chaves de `allSystemPerms` ordenadas para exibição.
	selectedRole   *models.RolePublic         // Role atualmente selecionado na lista para edição.
	statusMessage  string                     // Mensagem de feedback global para a página.
	messageColor   color.NRGBA

	// Widgets do Painel Esquerdo (Lista de Roles)
	roleList       layout.List
	roleClickables []widget.Clickable // Um clickable por role na lista.
	newRoleBtn     widget.Clickable
	deleteRoleBtn  widget.Clickable

	// Widgets do Painel Direito (Detalhes do Role e Permissões)
	roleNameInput        widget.Editor
	roleDescriptionInput widget.Editor                    // Editor para descrição (pode ser multilinhas).
	permissionCheckboxes map[auth.Permission]*widget.Bool // Checkbox para cada permissão.
	permList             layout.List                      // Para scroll da lista de permissões.
	saveRoleBtn          widget.Clickable
	cancelChangesBtn     widget.Clickable

	// Controle de edição
	isEditingNewRole bool // True se o painel direito estiver no modo de criação de um novo role.
	formChanged      bool // True se algo no formulário do painel direito (nome, descrição, permissões) foi alterado.

	spinner *components.LoadingSpinner

	firstLoadDone bool // Para controlar o carregamento inicial de dados.
}

// NewRoleManagementPage cria uma nova instância da página de gerenciamento de roles.
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
		spinner:              components.NewLoadingSpinner(theme.Colors.Primary),
		allSystemPerms:       permMan.GetAllDefinedPermissions(), // Carrega definições de permissão.
		permissionCheckboxes: make(map[auth.Permission]*widget.Bool),
	}

	p.roleNameInput.SingleLine = true
	p.roleNameInput.Hint = "Nome do Perfil (ex: editor_chefe)"
	p.roleDescriptionInput.Hint = "Descrição do perfil (opcional)"
	// `p.roleDescriptionInput.SingleLine = false` é o padrão para Editor.

	// Inicializa os checkboxes de permissão e ordena as chaves de permissão para exibição.
	p.sortedPermKeys = make([]auth.Permission, 0, len(p.allSystemPerms))
	for permKey := range p.allSystemPerms {
		p.sortedPermKeys = append(p.sortedPermKeys, permKey)
		p.permissionCheckboxes[permKey] = new(widget.Bool) // Cria um widget.Bool para cada.
	}
	// Ordena as chaves de permissão alfabeticamente para exibição consistente.
	sort.Slice(p.sortedPermKeys, func(i, j int) bool {
		return p.sortedPermKeys[i] < p.sortedPermKeys[j]
	})

	return p
}

// OnNavigatedTo é chamado quando a página se torna ativa.
func (p *RoleManagementPage) OnNavigatedTo(params interface{}) {
	appLogger.Info("Navegou para RoleManagementPage")
	p.clearDetailsPanel(false) // Limpa painel direito, mas mantém seleção da lista (se houver).
	p.statusMessage = ""

	currentSession, errSess := p.sessionManager.GetCurrentSession()
	if errSess != nil || currentSession == nil {
		p.router.GetAppWindow().HandleLogout()
		return
	}
	// Verifica permissão para gerenciar roles.
	if err := p.permManager.CheckPermission(currentSession, auth.PermRoleManage, nil); err != nil {
		p.statusMessage = fmt.Sprintf("Acesso negado à página de gerenciamento de perfis: %v", err)
		p.messageColor = theme.Colors.Danger
		p.allSystemRoles = []*models.RolePublic{} // Limpa dados se não tem permissão.
		p.router.GetAppWindow().Invalidate()
		return
	}

	if !p.firstLoadDone {
		p.loadRoles(currentSession)
		p.firstLoadDone = true
	} else {
		p.loadRoles(currentSession) // Recarrega para ter dados frescos.
	}
}

// OnNavigatedFrom é chamado quando o router navega para fora desta página.
func (p *RoleManagementPage) OnNavigatedFrom() {
	appLogger.Info("Navegando para fora da RoleManagementPage")
	p.isLoading = false
	p.spinner.Stop(p.router.GetAppWindow().Context())
}

// loadRoles carrega a lista de roles do serviço.
func (p *RoleManagementPage) loadRoles(currentSession *auth.SessionData) {
	if p.isLoading {
		return
	}
	p.isLoading = true
	p.statusMessage = "Carregando perfis..."
	p.messageColor = theme.Colors.TextMuted
	p.spinner.Start(p.router.GetAppWindow().Context())
	p.router.GetAppWindow().Invalidate()

	go func(sess *auth.SessionData) {
		var loadedRoles []*models.RolePublic
		var loadErr error

		// Permissão já verificada em OnNavigatedTo.
		roles, err := s.roleService.GetAllRoles(sess)
		if err != nil {
			loadErr = fmt.Errorf("falha ao carregar perfis: %w", err)
		} else {
			// Ordenar roles para exibição consistente na lista.
			sort.SliceStable(roles, func(i, j int) bool {
				// System roles primeiro, depois por nome.
				if roles[i].IsSystemRole != roles[j].IsSystemRole {
					return roles[i].IsSystemRole // true (system) vem antes de false (custom)
				}
				return strings.ToLower(roles[i].Name) < strings.ToLower(roles[j].Name)
			})
			loadedRoles = roles
		}

		p.router.GetAppWindow().Execute(func() {
			p.isLoading = false
			p.spinner.Stop(p.router.GetAppWindow().Context())
			if loadErr != nil {
				p.statusMessage = loadErr.Error()
				p.messageColor = theme.Colors.Danger
				p.allSystemRoles = []*models.RolePublic{}
				appLogger.Errorf("Erro ao carregar dados para RoleManagementPage: %v", loadErr)
			} else {
				p.allSystemRoles = loadedRoles
				if len(p.allSystemRoles) > 0 {
					p.statusMessage = fmt.Sprintf("%d perfis carregados.", len(p.allSystemRoles))
					p.messageColor = theme.Colors.Success
				} else {
					p.statusMessage = "Nenhum perfil encontrado."
					p.messageColor = theme.Colors.Info
				}
				// Ajusta o tamanho do slice de clickables.
				if len(p.allSystemRoles) != len(p.roleClickables) {
					p.roleClickables = make([]widget.Clickable, len(p.allSystemRoles))
				}
				// Se um role estava selecionado, tenta manter a seleção se ele ainda existir.
				if p.selectedRole != nil {
					foundSelectedAgain := false
					for _, r := range p.allSystemRoles {
						if r.ID == p.selectedRole.ID {
							p.selectRole(r) // Recarrega o painel direito com dados atualizados
							foundSelectedAgain = true
							break
						}
					}
					if !foundSelectedAgain {
						p.clearDetailsPanel(true) // Limpa se o selecionado não existe mais
					}
				}
			}
			p.updateButtonStates()
			p.router.GetAppWindow().Invalidate()
		})
	}(currentSession)
}

// Layout é o método principal de desenho da página.
func (p *RoleManagementPage) Layout(gtx layout.Context) layout.Dimensions {
	th := p.router.GetAppWindow().Theme()

	// Processar eventos/cliques dos botões principais.
	if p.newRoleBtn.Clicked(gtx) && !p.isLoading {
		p.handleNewRole()
	}
	if p.deleteRoleBtn.Clicked(gtx) && !p.isLoading {
		p.handleDeleteRole()
	}
	if p.saveRoleBtn.Clicked(gtx) && !p.isLoading {
		p.handleSaveRole()
	}
	if p.cancelChangesBtn.Clicked(gtx) && !p.isLoading {
		p.handleCancelChanges()
	}

	// Processar eventos de mudança nos inputs do painel direito.
	if p.roleNameInput.Update(gtx) || p.roleDescriptionInput.Update(gtx) {
		p.formChanged = true
		p.updateButtonStates()
	}
	// Checkboxes de permissão já atualizam `p.formChanged` em seus eventos de Update.
	for _, chk := range p.permissionCheckboxes {
		if chk.Update(gtx) { // Update processa o evento de mudança e retorna true se mudou.
			p.formChanged = true
			p.updateButtonStates()
		}
	}

	// Layout dividido em dois painéis: Esquerdo (Lista de Roles) e Direito (Detalhes do Role).
	// Usar Flex para simular um splitter.
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Flexed(0.3, func(gtx C) D { // Painel Esquerdo (30% da largura)
			return p.layoutLeftPanel(gtx, th)
		}),
		layout.Rigid(func(gtx C) D { // Separador Vertical Fino
			return layout.Inset{Left: unit.Dp(1), Right: unit.Dp(1)}.Layout(gtx,
				func(gtx C) D {
					// paint.FillShape(gtx.Ops, theme.Colors.Border, clip.Rect{Max: image.Pt(gtx.Dp(1), gtx.Constraints.Max.Y)}.Op())
					// return layout.Dimensions{Size: image.Pt(gtx.Dp(1), gtx.Constraints.Max.Y)}
					return layout.Dimensions{} // Sem separador visível por agora para simplificar
				})
		}),
		layout.Flexed(0.7, func(gtx C) D { // Painel Direito (70% da largura)
			return p.layoutRightPanel(gtx, th)
		}),
		// Spinner overlay global para a página (se `p.isLoading`).
		//layout.Expanded(func(gtx C) D {
		//	if p.isLoading {
		//		return layout.Center.Layout(gtx, p.spinner.Layout)
		//	}
		//	return D{}
		//}),
	)
}

// layoutLeftPanel desenha o painel esquerdo com a lista de roles e botões de ação.
func (p *RoleManagementPage) layoutLeftPanel(gtx layout.Context, th *material.Theme) layout.Dimensions {
	// Processar cliques na lista de roles.
	for i := range p.allSystemRoles {
		if i >= len(p.roleClickables) {
			break
		} // Segurança
		if p.roleClickables[i].Clicked(gtx) && !p.isLoading {
			p.selectRole(p.allSystemRoles[i])
		}
	}

	return layout.UniformInset(theme.PagePadding).Layout(gtx, // Padding para o painel
		func(gtx C) D {
			return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceBetween}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(material.Subtitle1(th, "Perfis de Usuário").Layout),
						layout.Rigid(layout.Spacer{Height: theme.DefaultVSpacer}.Layout),
					)
				}),
				layout.Flexed(1, func(gtx C) D { // Lista de Roles com scroll
					return p.roleList.Layout(gtx, len(p.allSystemRoles), func(gtx C, index int) D {
						if index < 0 || index >= len(p.allSystemRoles) {
							return D{}
						}
						role := p.allSystemRoles[index]

						// Item da lista clicável
						item := material.Clickable(gtx, &p.roleClickables[index], func(gtx C) D {
							// Conteúdo do item (Nome do Role, talvez ícone de sistema)
							label := material.Body1(th, strings.Title(role.Name)) // Capitaliza para exibição
							if p.selectedRole != nil && p.selectedRole.ID == role.ID {
								label.Font.Weight = font.Bold
								label.Color = theme.Colors.PrimaryText // Texto branco se fundo primário
							} else if role.IsSystemRole {
								label.Color = theme.Colors.TextMuted // Cinza para roles do sistema não selecionados
							}

							// TODO: Adicionar ícone para system roles (ex: um cadeado)
							// iconWidget := layout.Dimensions{}
							// if role.IsSystemRole { iconWidget = ... }

							return layout.UniformInset(unit.Dp(8)).Layout(gtx, label.Layout)
						})

						// Destaque de fundo para o item selecionado.
						if p.selectedRole != nil && p.selectedRole.ID == role.ID {
							return layout.Background{Color: theme.Colors.PrimaryLight}.Layout(gtx, item.Layout)
						}
						return item
					})
				}),
				layout.Rigid(layout.Spacer{Height: theme.DefaultVSpacer}.Layout),
				layout.Rigid(func(gtx C) D { // Botões Novo/Excluir Perfil
					newBtnWidget := material.Button(th, &p.newRoleBtn, "Novo Perfil")
					delBtnWidget := material.Button(th, &p.deleteRoleBtn, "Excluir Perfil")

					// Habilitar/desabilitar visualmente o botão Excluir.
					// A lógica de clique já verifica `p.isLoading`.
					canDelete := (p.selectedRole != nil && !p.selectedRole.IsSystemRole)
					if !canDelete {
						delBtnWidget.Style.TextColor = theme.Colors.TextMuted
						delBtnWidget.Style.Background = theme.Colors.Grey300
					}

					return layout.Flex{Spacing: layout.SpaceBetween}.Layout(gtx,
						layout.Flexed(1, newBtnWidget.Layout),
						layout.Rigid(layout.Spacer{Width: theme.DefaultVSpacer}.Layout),
						layout.Flexed(1, delBtnWidget.Layout),
					)
				}),
			)
		})
}

// layoutRightPanel desenha o painel direito com os detalhes do role selecionado e suas permissões.
func (p *RoleManagementPage) layoutRightPanel(gtx layout.Context, th *material.Theme) layout.Dimensions {
	// Determina se os campos no painel direito devem ser editáveis.
	canEditDetails := (p.selectedRole != nil && !p.selectedRole.IsSystemRole) || p.isEditingNewRole
	canEditName := canEditDetails && (p.isEditingNewRole || (p.selectedRole != nil && !p.selectedRole.IsSystemRole))

	nameEditorLayout := material.Editor(th, &p.roleNameInput, p.roleNameInput.Hint).Layout
	// Para desabilitar visualmente o editor de nome se `!canEditName`,
	// poderia-se mudar a cor de fundo/texto ou usar um Label em vez de Editor.
	// Por agora, a interatividade é controlada por não processar eventos se não deve editar.

	descEditorLayout := material.Editor(th, &p.roleDescriptionInput, p.roleDescriptionInput.Hint).Layout
	// Similarmente para descrição.

	// Agrupa permissões por prefixo (ex: "user:", "network:") para melhor organização na UI.
	groupedPerms := make(map[string][]auth.Permission)
	var sortedGroupPrefixes []string
	for _, permKey := range p.sortedPermKeys { // Usa as chaves já ordenadas.
		prefix := strings.SplitN(string(permKey), ":", 2)[0] // Pega a parte antes do primeiro ":"
		if _, exists := groupedPerms[prefix]; !exists {
			sortedGroupPrefixes = append(sortedGroupPrefixes, prefix) // Adiciona prefixo à lista de grupos se novo.
		}
		groupedPerms[prefix] = append(groupedPerms[prefix], permKey)
	}
	// `sortedGroupPrefixes` não precisa ser reordenado se `p.sortedPermKeys` já estiver ordenado.

	return layout.UniformInset(theme.PagePadding).Layout(gtx, // Padding para o painel
		func(gtx C) D {
			return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceBetween}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					title := "Detalhes do Perfil"
					if p.isEditingNewRole {
						title = "Criar Novo Perfil"
					}
					if p.selectedRole != nil && !p.isEditingNewRole {
						title = fmt.Sprintf("Editando Perfil: %s", strings.Title(p.selectedRole.Name))
					}
					return material.Subtitle1(th, title).Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Height: theme.DefaultVSpacer}.Layout),

				layout.Rigid(p.labeledInput(gtx, th, "Nome do Perfil:*", nameEditorLayout, "", canEditName)),
				layout.Rigid(layout.Spacer{Height: theme.DefaultVSpacer}.Layout),
				layout.Rigid(p.labeledInput(gtx, th, "Descrição:", descEditorLayout, "", canEditDetails)),
				layout.Rigid(layout.Spacer{Height: theme.LargeVSpacer}.Layout),

				layout.Rigid(material.Subtitle1(th, "Permissões Associadas").Layout),
				layout.Rigid(layout.Spacer{Height: theme.DefaultVSpacer}.Layout),
				// Lista de Permissões com Scroll
				layout.Flexed(1, func(gtx C) D {
					// Apenas mostra a lista se um role estiver selecionado/sendo criado.
					if !canEditDetails && !p.isEditingNewRole && p.selectedRole == nil {
						return material.Body2(th, "Selecione um perfil para ver ou editar permissões, ou clique em 'Novo Perfil'.").Layout(gtx)
					}

					return p.permList.Layout(gtx, len(sortedGroupPrefixes), func(gtx C, groupIndex int) D {
						prefix := sortedGroupPrefixes[groupIndex]
						permsInGroup := groupedPerms[prefix]
						// `permsInGroup` já estará ordenado se `p.sortedPermKeys` estava.

						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx C) D { // Título do Grupo de Permissão
								groupLabel := material.Body2(th, strings.Title(prefix)+" Permissões")
								groupLabel.Font.Weight = font.Bold
								return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(4)}.Layout(gtx, groupLabel.Layout)
							}),
							layout.Rigid(func(gtx C) D { // Checkboxes para este grupo
								permGroupLayout := layout.List{Axis: layout.Vertical}
								return permGroupLayout.Layout(gtx, len(permsInGroup), func(gtx C, permIndex int) D {
									permKey := permsInGroup[permIndex]
									permDesc := p.allSystemPerms[permKey] // Descrição da permissão
									chk, ok := p.permissionCheckboxes[permKey]
									if !ok {
										return D{}
									} // Não deveria acontecer.

									cb := material.CheckBox(th, chk, string(permKey))
									cb.IconColor = theme.Colors.Primary
									// Desabilitar checkbox se o painel não for editável.
									// A interatividade é controlada por não processar `chk.Update` se `!canEditDetails`.

									// Adicionar tooltip com a descrição da permissão.
									// Tooltip em Gio requer um gerenciador de tooltip ou um widget customizado.
									// Por agora, apenas o nome.
									return layout.Inset{Left: unit.Dp(10), Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, cb.Layout)
								})
							}),
						)
					})
				}),
				layout.Rigid(layout.Spacer{Height: theme.DefaultVSpacer}.Layout),
				// Mensagem de Status e Botões Salvar/Cancelar no painel direito
				layout.Rigid(func(gtx C) D {
					return layout.Flex{Axis: layout.Vertical, Alignment: layout.End}.Layout(gtx,
						layout.Rigid(func(gtx C) D { // Mensagem de Status
							if p.statusMessage != "" && !p.isLoading { // Só mostra se não estiver carregando
								lblStatus := material.Body2(th, p.statusMessage)
								lblStatus.Color = p.messageColor
								return layout.Inset{Bottom: theme.DefaultVSpacer}.Layout(gtx, lblStatus.Layout)
							}
							return D{}
						}),
						layout.Rigid(func(gtx C) D { // Botões Salvar/Cancelar
							saveBtnWidget := material.Button(th, &p.saveRoleBtn, "Salvar Alterações")
							if p.isEditingNewRole {
								saveBtnWidget.Text = "Criar Novo Perfil"
							}

							// Habilitar/desabilitar visualmente.
							shouldEnableSave := canEditDetails && p.formChanged && !p.isLoading
							if !shouldEnableSave {
								saveBtnWidget.Style.TextColor = theme.Colors.TextMuted
								saveBtnWidget.Style.Background = theme.Colors.Grey300
							} else {
								saveBtnWidget.Background = theme.Colors.Primary
							}

							cancelBtnWidget := material.Button(th, &p.cancelChangesBtn, "Cancelar")
							shouldEnableCancel := canEditDetails && !p.isLoading
							if !shouldEnableCancel {
								cancelBtnWidget.Style.TextColor = theme.Colors.TextMuted
								cancelBtnWidget.Style.Background = theme.Colors.Grey300
							}

							return layout.Flex{Spacing: layout.SpaceBetween, Alignment: layout.Middle}.Layout(gtx,
								layout.Flexed(1, func(gtx C) D { return layout.Dimensions{} }), // Espaçador para alinhar à direita
								layout.Rigid(cancelBtnWidget.Layout),
								layout.Rigid(layout.Spacer{Width: theme.DefaultVSpacer}.Layout),
								layout.Rigid(saveBtnWidget.Layout),
							)
						}),
					)
				}),
			)
		})
}

// --- Lógica de Ações e Manipulação de Estado ---

// selectRole é chamado quando um role é selecionado na lista do painel esquerdo.
func (p *RoleManagementPage) selectRole(role *models.RolePublic) {
	if p.isLoading {
		return
	} // Impede seleção durante carregamento.
	p.statusMessage = "" // Limpa mensagem global da página.

	if p.formChanged && p.selectedRole != nil && p.selectedRole.ID != role.ID {
		// Se havia um role selecionado e o formulário tem mudanças não salvas,
		// poderia-se exibir um diálogo "Descartar alterações não salvas?".
		// Por agora, apenas loga e prossegue, descartando as mudanças.
		appLogger.Warnf("Mudando de role (ID: %d para ID: %d) com alterações não salvas no formulário.", p.selectedRole.ID, role.ID)
	}

	p.selectedRole = role
	p.isEditingNewRole = false // Cancela o modo de "novo role" se estiver ativo.
	p.formChanged = false      // Reseta flag de mudanças ao selecionar um novo role.

	if role != nil {
		p.roleNameInput.SetText(role.Name) // Nome já é minúsculo do DB.
		desc := ""
		if role.Description != nil {
			desc = *role.Description
		}
		p.roleDescriptionInput.SetText(desc)

		// Marca os checkboxes de permissão correspondentes ao role selecionado.
		currentPermsSet := make(map[auth.Permission]bool)
		for _, pNameStr := range role.Permissions {
			currentPermsSet[auth.Permission(pNameStr)] = true
		}
		for permKey, chk := range p.permissionCheckboxes {
			chk.Value = currentPermsSet[permKey]
		}
		appLogger.Debugf("Role '%s' selecionado. %d permissões carregadas nos checkboxes.", role.Name, len(currentPermsSet))
	} else {
		p.clearDetailsPanel(false) // Limpa painel direito se `role` for nil (ex: desseleção).
	}
	p.updateButtonStates()
	p.router.GetAppWindow().Invalidate()
}

// clearDetailsPanel limpa o painel direito (detalhes do role e permissões).
// `resetListSelection` define se a seleção na lista de roles também deve ser limpa.
func (p *RoleManagementPage) clearDetailsPanel(resetListSelection bool) {
	if resetListSelection {
		p.selectedRole = nil
		// Em Gio, a "desseleção" na lista é visual, não há um estado de widget a ser resetado.
		// Apenas não desenhar o destaque de seleção é suficiente.
	}
	p.isEditingNewRole = false
	p.formChanged = false
	p.roleNameInput.SetText("")
	p.roleDescriptionInput.SetText("")
	for _, chk := range p.permissionCheckboxes { // Desmarca todos os checkboxes.
		chk.Value = false
	}
	p.statusMessage = "" // Limpa mensagem global da página.
	// p.updateButtonStates() // Será chamado pelo chamador ou próximo Layout.
}

// updateButtonStates atualiza o estado lógico (e visual simulado) dos botões.
// A interatividade real é controlada no manipulador de cliques.
func (p *RoleManagementPage) updateButtonStates() {
	// Apenas força um Invalidate para que o Layout redesenhe os botões
	// com base nas condições atuais (isLoading, selectedRole, formChanged, etc.).
	p.router.GetAppWindow().Invalidate()
}

// handleNewRole prepara o painel direito para criar um novo role.
func (p *RoleManagementPage) handleNewRole() {
	if p.isLoading {
		return
	}
	if p.formChanged && p.selectedRole != nil {
		appLogger.Warnf("Iniciando novo role com alterações não salvas no role '%s'. (Alterações descartadas)", p.selectedRole.Name)
	}
	p.clearDetailsPanel(true) // Limpa seleção da lista e painel direito.
	p.isEditingNewRole = true
	p.formChanged = false   // Novo formulário começa sem mudanças.
	p.roleNameInput.Focus() // Tenta focar no campo de nome.
	p.statusMessage = "Preencha os dados para o novo perfil e clique em 'Criar'."
	p.messageColor = theme.Colors.Info
	p.updateButtonStates()
	p.router.GetAppWindow().Invalidate()
}

// handleSaveRole lida com salvar um role novo ou existente.
func (p *RoleManagementPage) handleSaveRole() {
	if p.isLoading || !p.formChanged { // Não faz nada se não houver mudanças ou estiver carregando.
		if !p.formChanged {
			p.statusMessage = "Nenhuma alteração para salvar."
			p.messageColor = theme.Colors.Info
		}
		return
	}
	p.statusMessage = "" // Limpa mensagem anterior.

	roleName := strings.TrimSpace(p.roleNameInput.Text())
	roleDescriptionText := strings.TrimSpace(p.roleDescriptionInput.Text())
	var roleDescriptionPtr *string
	if roleDescriptionText != "" {
		roleDescriptionPtr = &roleDescriptionText
	}

	selectedPermNames := []string{}
	for permKey, chk := range p.permissionCheckboxes {
		if chk.Value {
			selectedPermNames = append(selectedPermNames, string(permKey))
		}
	}

	// Validação do nome do role (formato e obrigatoriedade).
	if roleName == "" {
		p.statusMessage = "Nome do perfil é obrigatório."
		p.messageColor = theme.Colors.Danger
		p.router.GetAppWindow().Invalidate()
		return
	}
	// Regex para nome do role (ex: `^[a-zA-Z0-9_]{3,50}$`)
	if matched, _ := regexp.MatchString(`^[a-zA-Z0-9_]{3,50}$`, roleName); !matched {
		p.statusMessage = "Nome do perfil deve ter 3-50 caracteres (letras, números, _)."
		p.messageColor = theme.Colors.Danger
		p.router.GetAppWindow().Invalidate()
		return
	}
	normalizedRoleName := strings.ToLower(roleName) // Normaliza para minúsculas.

	p.isLoading = true
	p.statusMessage = "Salvando perfil..."
	p.messageColor = theme.Colors.TextMuted
	p.spinner.Start(p.router.GetAppWindow().Context())
	p.updateButtonStates() // Para desabilitar botões visualmente
	p.router.GetAppWindow().Invalidate()

	currentAdminSession, _ := p.sessionManager.GetCurrentSession() // Para o serviço

	go func(isNew bool, currentRoleID uint64, name, normalizedName, descText string, descPtr *string, perms []string, sess *auth.SessionData) {
		var opErr error
		var successMsg string
		var resultingRole *models.RolePublic

		if isNew {
			createData := models.RoleCreate{Name: normalizedName, Description: descPtr, PermissionNames: perms}
			// CleanAndValidate já foi feito implicitamente pela coleta/trimming,
			// mas o modelo pode ter validações mais robustas.
			// if errVal := createData.CleanAndValidate(); errVal != nil { opErr = errVal }
			if opErr == nil {
				resultingRole, opErr = s.roleService.CreateRole(createData, sess)
				if opErr == nil {
					successMsg = fmt.Sprintf("Perfil '%s' criado com sucesso!", name)
				}
			}
		} else { // Editando role existente
			updateData := models.RoleUpdate{}
			// O serviço `UpdateRole` compara com o estado atual no DB para determinar o que mudou.
			// Aqui, apenas montamos o payload com o que está no formulário.
			if p.selectedRole.Name != normalizedName {
				updateData.Name = &normalizedName
			}

			currentDescInForm := ""
			if descPtr != nil {
				currentDescInForm = *descPtr
			}
			selectedDescInDB := ""
			if p.selectedRole.Description != nil {
				selectedDescInDB = *p.selectedRole.Description
			}
			if currentDescInForm != selectedDescInDB {
				updateData.Description = descPtr
			}

			// Compara permissões para ver se mudaram.
			currentPermsSetDB := make(map[string]bool)
			for _, pStr := range p.selectedRole.Permissions {
				currentPermsSetDB[pStr] = true
			}
			newPermsSetForm := make(map[string]bool)
			for _, pStr := range perms {
				newPermsSetForm[pStr] = true
			}

			if len(currentPermsSetDB) != len(newPermsSetForm) || !mapsEqual(currentPermsSetDB, newPermsSetForm) {
				updateData.PermissionNames = &perms
			}

			// Se nada mudou efetivamente (apesar de `formChanged` poder estar true por digitação)
			if updateData.Name == nil && updateData.Description == nil && updateData.PermissionNames == nil {
				successMsg = fmt.Sprintf("Nenhuma alteração efetiva para o perfil '%s'.", name)
			} else {
				// if errVal := updateData.CleanAndValidate(); errVal != nil { opErr = errVal }
				if opErr == nil {
					resultingRole, opErr = s.roleService.UpdateRole(currentRoleID, updateData, sess)
					if opErr == nil {
						successMsg = fmt.Sprintf("Perfil '%s' atualizado com sucesso!", name)
					}
				}
			}
		}

		p.router.GetAppWindow().Execute(func() {
			p.isLoading = false
			p.spinner.Stop(p.router.GetAppWindow().Context())
			if opErr != nil {
				p.statusMessage = fmt.Sprintf("Erro ao salvar perfil '%s': %v", name, opErr)
				p.messageColor = theme.Colors.Danger
				appLogger.Errorf("Erro ao salvar role '%s': %v", name, opErr)
				// Manter `formChanged = true` para permitir nova tentativa de salvar.
			} else {
				p.statusMessage = successMsg
				p.messageColor = theme.Colors.Success
				p.formChanged = false      // Reseta flag de mudanças.
				p.isEditingNewRole = false // Sai do modo de novo role.
				p.loadRoles(sess)          // Recarrega lista.
				// Se um novo role foi criado ou um existente editado, tenta selecioná-lo.
				if resultingRole != nil {
					p.selectRole(resultingRole) // Seleciona o role (ou mantém o atual se for update sem mudança de ID/nome)
				} else if !isNew { // Se foi update e não retornou role (ex: nenhuma mudança), tenta manter o selecionado
					p.selectRole(p.selectedRole)
				}
			}
			p.updateButtonStates()
			p.router.GetAppWindow().Invalidate()
		})
	}(p.isEditingNewRole, p.selectedRole.ID, roleName, normalizedRoleName, roleDescriptionText, roleDescriptionPtr, selectedPermNames, currentAdminSession)
}

// handleCancelChanges descarta as mudanças no painel direito.
func (p *RoleManagementPage) handleCancelChanges() {
	if p.isLoading {
		return
	}
	if p.isEditingNewRole {
		p.clearDetailsPanel(true) // Limpa tudo, incluindo seleção da lista.
	} else if p.selectedRole != nil {
		// Recarrega os dados do role atualmente selecionado, descartando quaisquer mudanças não salvas.
		p.selectRole(p.selectedRole)
	} else {
		p.clearDetailsPanel(true) // Se nada selecionado e não era novo, apenas limpa.
	}
	p.formChanged = false // Garante que `formChanged` seja resetado.
	p.statusMessage = "Alterações descartadas."
	p.messageColor = theme.Colors.Info
	p.updateButtonStates()
	p.router.GetAppWindow().Invalidate()
}

// handleDeleteRole lida com a exclusão de um role selecionado.
func (p *RoleManagementPage) handleDeleteRole() {
	if p.isLoading || p.selectedRole == nil || p.selectedRole.IsSystemRole {
		if p.selectedRole != nil && p.selectedRole.IsSystemRole {
			p.statusMessage = "Perfis do sistema não podem ser excluídos."
			p.messageColor = theme.Colors.Warning
			p.router.GetAppWindow().Invalidate()
		}
		return
	}

	// TODO: Implementar um diálogo de confirmação antes de excluir.
	// Ex: p.router.GetAppWindow().ShowConfirmDialog("Excluir Perfil", "Tem certeza?", func(confirmado bool){ if confirmado { ... }})

	p.isLoading = true
	p.statusMessage = fmt.Sprintf("Excluindo perfil '%s'...", p.selectedRole.Name)
	p.messageColor = theme.Colors.TextMuted
	p.spinner.Start(p.router.GetAppWindow().Context())
	p.updateButtonStates()
	p.router.GetAppWindow().Invalidate()

	roleIDToDelete := p.selectedRole.ID  // Copia o ID.
	roleNameToLog := p.selectedRole.Name // Copia o nome para log.
	currentAdminSession, _ := p.sessionManager.GetCurrentSession()

	go func(id uint64, name string, sess *auth.SessionData) {
		var opErr error
		err := s.roleService.DeleteRole(id, sess)
		if err != nil {
			opErr = err
		}

		p.router.GetAppWindow().Execute(func() {
			p.isLoading = false
			p.spinner.Stop(p.router.GetAppWindow().Context())
			if opErr != nil {
				p.statusMessage = fmt.Sprintf("Erro ao excluir perfil '%s': %v", name, opErr)
				p.messageColor = theme.Colors.Danger
				appLogger.Errorf("Erro ao excluir role '%s' (ID: %d): %v", name, id, opErr)
				// Se o erro for porque o role não existe mais (ex: deletado por outra ação),
				// ErrNotFound será retornado pelo serviço.
			} else {
				p.statusMessage = fmt.Sprintf("Perfil '%s' excluído com sucesso!", name)
				p.messageColor = theme.Colors.Success
				appLogger.Infof("Perfil '%s' (ID: %d) excluído.", name, id)
				p.clearDetailsPanel(true) // Limpa painel e seleção.
				p.loadRoles(sess)         // Recarrega lista de roles.
			}
			p.updateButtonStates()
			p.router.GetAppWindow().Invalidate()
		})
	}(roleIDToDelete, roleNameToLog, currentAdminSession)
}

// Helper para o layout de input com label.
func (p *RoleManagementPage) labeledInput(gtx layout.Context, th *material.Theme, labelText string, inputWidgetLayout layout.Widget, feedbackText string, enabled bool) layout.Dimensions {
	label := material.Body1(th, labelText)
	if !enabled {
		label.Color = theme.Colors.TextMuted
	}
	// A interatividade do inputWidgetLayout em si deve ser controlada pelo seu próprio estado
	// ou por não passar eventos para ele se `!enabled`.

	return layout.Flex{Axis: layout.Vertical, Spacing: layout.Tight}.Layout(gtx,
		layout.Rigid(label.Layout),
		layout.Rigid(inputWidgetLayout),
		layout.Rigid(func(gtx C) D {
			if feedbackText != "" {
				feedbackLabel := material.Body2(th, feedbackText)
				feedbackLabel.Color = theme.Colors.Danger
				return layout.Inset{Top: unit.Dp(2)}.Layout(gtx, feedbackLabel.Layout)
			}
			return layout.Dimensions{}
		}),
	)
}

// mapsEqual (helper para comparar se dois maps string->bool são iguais)
// Usado para verificar se as permissões realmente mudaram.
func mapsEqual(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}
