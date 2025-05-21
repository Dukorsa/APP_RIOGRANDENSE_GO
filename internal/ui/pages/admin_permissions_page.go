package pages

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/google/uuid"
	"github.com/seu_usuario/riograndense_gio/internal/auth"
	"github.com/seu_usuario/riograndense_gio/internal/core"
	appErrors "github.com/seu_usuario/riograndense_gio/internal/core/errors"
	appLogger "github.com/seu_usuario/riograndense_gio/internal/core/logger"
	"github.com/seu_usuario/riograndense_gio/internal/data/models"
	"github.com/seu_usuario/riograndense_gio/internal/services"
	"github.com/seu_usuario/riograndense_gio/internal/ui"            // Para Router e PageID
	"github.com/seu_usuario/riograndense_gio/internal/ui/components" // Para LoadingSpinner
	"github.com/seu_usuario/riograndense_gio/internal/ui/theme"      // Para Cores
	// "github.com/seu_usuario/riograndense_gio/internal/ui/icons" // Para ícones
)

// AdminPermissionsPage gerencia usuários e suas permissões/roles.
type AdminPermissionsPage struct {
	router         *ui.Router
	cfg            *core.Config
	userService    services.UserService
	roleService    services.RoleService
	auditService   services.AuditLogService
	permManager    *auth.PermissionManager
	sessionManager *auth.SessionManager // Para obter a sessão admin atual

	// Estado da UI
	isLoading      bool
	users          []*models.UserPublic // Lista completa de usuários carregados
	filteredUsers  []*models.UserPublic // Usuários após filtro
	allRoles       []*models.RolePublic // Todos os roles disponíveis para filtros e diálogos
	selectedUserID *uuid.UUID
	statusMessage  string // Para mensagens de erro ou sucesso
	messageColor   color.NRGBA

	// Widgets de controle
	searchEditor   widget.Editor
	roleFilter     widget.Enum // Para o ComboBox de filtro de role
	refreshBtn     widget.Clickable
	manageRolesBtn widget.Clickable // Botão para abrir o gerenciador de roles

	// Widgets de ação para usuário selecionado
	assignRolesBtn widget.Clickable
	deactivateBtn  widget.Clickable
	// TODO: unlockBtn, resetPasswordBtn

	// Para a lista/tabela de usuários
	userList          layout.List
	userClickables    []widget.Clickable  // Um clickable por usuário na lista filtrada
	tableHeaderClicks [5]widget.Clickable // Para ordenação (Username, Email, Roles, Status, Last Login)
	sortColumn        int                 // 0: Username, 1: Email, ...
	sortAscending     bool

	// Para diálogos (simulados como overlays ou estados da página)
	showUserRoleDialog       bool
	userForRoleDialog        *models.UserPublic
	userRoleDialogCheckboxes map[string]*widget.Bool // map[roleName]*widget.Bool
	userRoleDialogSaveBtn    widget.Clickable
	userRoleDialogCancelBtn  widget.Clickable

	// Spinner
	spinner *components.LoadingSpinner

	firstLoadDone bool
}

// NewAdminPermissionsPage cria uma nova instância da página de gerenciamento de admin.
func NewAdminPermissionsPage(
	router *ui.Router,
	cfg *core.Config,
	userSvc services.UserService,
	roleSvc services.RoleService,
	auditSvc services.AuditLogService,
	permMan *auth.PermissionManager,
	sessMan *auth.SessionManager,
) *AdminPermissionsPage {
	p := &AdminPermissionsPage{
		router:         router,
		cfg:            cfg,
		userService:    userSvc,
		roleService:    roleSvc,
		auditService:   auditSvc,
		permManager:    permMan,
		sessionManager: sessMan,
		userList:       layout.List{Axis: layout.Vertical},
		spinner:        components.NewLoadingSpinner(),
		sortColumn:     0, // Default sort por Username
		sortAscending:  true,
	}
	p.searchEditor.SingleLine = true
	p.searchEditor.Hint = "Pesquisar por nome ou email..."
	// p.roleFilter.Value // Será preenchido com nomes de roles
	return p
}

func (p *AdminPermissionsPage) OnNavigatedTo(params interface{}) {
	appLogger.Info("Navegou para AdminPermissionsPage")
	p.statusMessage = "" // Limpa mensagem
	if !p.firstLoadDone {
		p.loadInitialData()
		p.firstLoadDone = true
	} else {
		// Se já carregou antes, talvez apenas aplicar filtros ou invalidar
		p.applyFiltersAndSort()
		p.router.GetAppWindow().Invalidate()
	}
}

func (p *AdminPermissionsPage) OnNavigatedFrom() {
	appLogger.Info("Navegando para fora da AdminPermissionsPage")
	p.showUserRoleDialog = false // Garante que diálogos sejam fechados
}

func (p *AdminPermissionsPage) loadInitialData() {
	p.isLoading = true
	p.spinner.Start(p.router.GetAppWindow().Context()) // Precisa de um gtx válido
	p.router.GetAppWindow().Invalidate()

	go func() {
		var loadErr error
		currentAdminSession, errSess := p.sessionManager.GetCurrentSession()
		if errSess != nil || currentAdminSession == nil {
			loadErr = fmt.Errorf("sessão de administrador inválida: %v", errSess)
		} else {
			// Carregar roles primeiro para o filtro
			roles, err := p.roleService.GetAllRoles(currentAdminSession)
			if err != nil {
				loadErr = fmt.Errorf("falha ao carregar perfis: %w", err)
			} else {
				p.allRoles = roles
			}

			// Carregar usuários se roles foram carregados
			if loadErr == nil {
				users, err := p.userService.ListUsers(true, currentAdminSession) // true = include inactive
				if err != nil {
					loadErr = fmt.Errorf("falha ao carregar usuários: %w", err)
				} else {
					p.users = users
				}
			}
		}

		p.router.GetAppWindow().Execute(func() { // Atualiza UI na thread principal
			p.isLoading = false
			p.spinner.Stop(p.router.GetAppWindow().Context())
			if loadErr != nil {
				p.statusMessage = loadErr.Error()
				p.messageColor = theme.Colors.Danger
				appLogger.Errorf("Erro ao carregar dados para AdminPermissionsPage: %v", loadErr)
			} else {
				p.statusMessage = fmt.Sprintf("%d usuários e %d perfis carregados.", len(p.users), len(p.allRoles))
				p.messageColor = theme.Colors.Success
				p.applyFiltersAndSort() // Aplica filtros e ordenação após carregar
				// Preencher roleFilter Enum
				p.roleFilter.Value = "Todos" // Default
				for _, r := range p.allRoles {
					p.roleFilter.SetEnum(r.Name, r.Name) // Key, Label
				}

			}
			p.router.GetAppWindow().Invalidate()
		})
	}()
}

// applyFiltersAndSort filtra e ordena a lista de usuários.
func (p *AdminPermissionsPage) applyFiltersAndSort() {
	// TODO: Implementar lógica de filtro e ordenação
	// Por agora, apenas copia todos os usuários.
	p.filteredUsers = make([]*models.UserPublic, len(p.users))
	copy(p.filteredUsers, p.users)

	// Ajusta o tamanho do slice de clickables
	if len(p.filteredUsers) != len(p.userClickables) {
		p.userClickables = make([]widget.Clickable, len(p.filteredUsers))
	}
	appLogger.Debugf("Filtros aplicados, %d usuários visíveis", len(p.filteredUsers))
}

func (p *AdminPermissionsPage) Layout(gtx layout.Context) layout.Dimensions {
	th := p.router.GetAppWindow().th // Acessa o tema

	// Processar cliques nos botões
	if p.refreshBtn.Clicked(gtx) {
		p.loadInitialData()
	}
	if p.manageRolesBtn.Clicked(gtx) {
		// TODO: Verificar permissão 'role:manage'
		p.router.NavigateTo(ui.PageRoleManagement, nil)
	}
	if p.assignRolesBtn.Clicked(gtx) && p.selectedUserID != nil {
		p.openUserRoleDialog()
	}
	if p.deactivateBtn.Clicked(gtx) && p.selectedUserID != nil {
		p.handleDeactivateUser()
	}

	// Lógica para o diálogo de edição de roles do usuário
	if p.showUserRoleDialog {
		return p.layoutUserRoleDialog(gtx, th)
	}

	// Layout principal da página
	return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceEnd}.Layout(gtx,
		// Linha 1: Controles superiores
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: theme.DefaultVSpacer}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Spacing: layout.SpaceBetween, Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, material.Editor(th, &p.searchEditor, "Pesquisar...").Layout),
						layout.Rigid(layout.Spacer{Width: theme.DefaultVSpacer}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							// TODO: Transformar roleFilter em um ComboBox real
							// Por agora, um label placeholder
							if p.roleFilter.Update(gtx) {
								p.applyFiltersAndSort()
							} // Se o valor do Enum mudar
							return material.DropDown(th, &p.roleFilter, material.Body1(th, p.roleFilter.Value).Layout).Layout(gtx)
							// return material.Body1(th, "Filtro Role: Todos").Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Width: theme.DefaultVSpacer}.Layout),
						layout.Rigid(material.Button(th, &p.refreshBtn, "Atualizar").Layout),
						layout.Rigid(layout.Spacer{Width: theme.DefaultVSpacer}.Layout),
						layout.Rigid(material.Button(th, &p.manageRolesBtn, "Gerenciar Perfis").Layout),
					)
				})
		}),

		// Linha 2: Tabela/Lista de Usuários
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return p.layoutUserTable(gtx, th)
		}),

		// Linha 3: Botões de Ação Inferiores
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: theme.DefaultVSpacer}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					assignBtn := material.Button(th, &p.assignRolesBtn, "Atribuir Perfis")
					deactivateUserBtn := material.Button(th, &p.deactivateBtn, "Desativar Usuário")

					// Habilitar/desabilitar botões com base na seleção e permissões
					canManageUserRoles, canDeactivateUser := p.getActionButtonsState()
					if !canManageUserRoles {
						assignBtn.Style.뭄Color = theme.Colors.TextMuted
					} // Visualmente desabilitado
					if !canDeactivateUser {
						deactivateUserBtn.Style.뭄Color = theme.Colors.TextMuted
					}

					return layout.Flex{Spacing: layout.SpaceBetween}.Layout(gtx,
						layout.Rigid(assignBtn.Layout),
						layout.Rigid(deactivateUserBtn.Layout),
						// TODO: Adicionar outros botões (resetar senha, desbloquear)
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }), // Espaçador
					)
				})
		}),

		// Linha 4: Mensagem de Status
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.statusMessage != "" {
				lbl := material.Body2(th, p.statusMessage)
				lbl.Color = p.messageColor
				return layout.Inset{Top: theme.DefaultVSpacer}.Layout(gtx, lbl.Layout)
			}
			return layout.Dimensions{}
		}),

		// Spinner de Carregamento (sobrepõe tudo se ativo)
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.isLoading {
				// O spinner precisa de um contexto para se desenhar, e será posicionado
				// pela AppWindow ou por um Stack aqui.
				// Por agora, apenas uma indicação.
				// return p.spinner.Layout(gtx)
				// A forma mais simples de sobrepor é desenhá-lo por último e cobrir a área.
				// Ou usar layout.Stack.
			}
			return layout.Dimensions{}
		}),
	)
}

// getActionButtonsState determina se os botões de ação devem estar habilitados.
func (p *AdminPermissionsPage) getActionButtonsState() (canManageRoles bool, canDeactivate bool) {
	if p.selectedUserID == nil || p.sessionManager == nil {
		return false, false
	}
	currentAdminSession, _ := p.sessionManager.GetCurrentSession()
	if currentAdminSession == nil {
		return false, false
	}

	isSelf := currentAdminSession.UserID == *p.selectedUserID

	// Permissão para gerenciar roles + não ser self
	hasPermManageRoles, _ := p.permManager.HasPermission(currentAdminSession, auth.PermUserManageRoles, nil)
	canManageRoles = hasPermManageRoles && !isSelf

	// Permissão para desativar + não ser self
	var selectedUserIsActive = true // Precisamos buscar o estado do usuário selecionado
	for _, u := range p.filteredUsers {
		if u.ID == *p.selectedUserID {
			selectedUserIsActive = u.Active
			break
		}
	}
	hasPermDelete, _ := p.permManager.HasPermission(currentAdminSession, auth.PermUserDelete, nil)
	canDeactivate = hasPermDelete && !isSelf && selectedUserIsActive

	return canManageRoles, canDeactivate
}

// layoutUserTable desenha a lista de usuários.
func (p *AdminPermissionsPage) layoutUserTable(gtx layout.Context, th *material.Theme) layout.Dimensions {
	// Cabeçalho da Tabela
	header := func(gtx layout.Context, colIndex int, label string) layout.Dimensions {
		return material.Clickable(gtx, &p.tableHeaderClicks[colIndex], func(gtx layout.Context) layout.Dimensions {
			// TODO: Lógica de ordenação ao clicar no cabeçalho
			// if p.sortColumn == colIndex { p.sortAscending = !p.sortAscending } else { p.sortColumn = colIndex; p.sortAscending = true }
			// p.applyFiltersAndSort()
			return material.Body1(th, label).Layout(gtx) // Placeholder para ícone de ordenação
		})
	}

	// Linha de Usuário
	userRow := func(gtx layout.Context, index int, user *models.UserPublic) layout.Dimensions {
		isSelected := p.selectedUserID != nil && *p.selectedUserID == user.ID

		rowBgColor := theme.Colors.Background
		if index%2 != 0 {
			rowBgColor = theme.Colors.BackgroundAlt
		}
		if isSelected {
			rowBgColor = theme.Colors.PrimaryLight // Destaque para selecionado
		}
		if !user.Active {
			// Poderia mudar a cor do texto ou fundo para inativos
		}

		return material.Clickable(gtx, &p.userClickables[index], func(gtx layout.Context) layout.Dimensions {
			textColor := theme.Colors.Text
			if !user.Active {
				textColor = theme.Colors.TextMuted
			}
			if isSelected {
				textColor = theme.Colors.PrimaryText
			}

			return layout.Background{Color: rowBgColor}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{}.Layout(gtx,
								layout.Flexed(0.2, material.Body2(th, user.Username).Layout),                                 // Col 1
								layout.Flexed(0.3, material.Body2(th, user.Email).Layout),                                    // Col 2
								layout.Flexed(0.2, material.Body2(th, strings.Join(user.Roles, ", ")).Layout),                // Col 3
								layout.Flexed(0.1, material.Body2(th, boolToString(user.Active, "Ativo", "Inativo")).Layout), // Col 4
								layout.Flexed(0.2, material.Body2(th, formatOptionalTime(user.LastLogin)).Layout),            // Col 5
							)
						})
				})
		})
	}

	// Processar cliques na linha
	for i := range p.filteredUsers { // Itera sobre o slice de clickables que corresponde aos usuários filtrados
		if p.userClickables[i].Clicked(gtx) {
			if p.selectedUserID != nil && *p.selectedUserID == p.filteredUsers[i].ID {
				p.selectedUserID = nil // Desseleciona se clicar no mesmo
			} else {
				selectedID := p.filteredUsers[i].ID // Copia o UUID
				p.selectedUserID = &selectedID
			}
			// appLogger.Debugf("Usuário selecionado ID: %v", p.selectedUserID)
			p.router.GetAppWindow().Invalidate() // Força redesenho para atualizar botões de ação
		}
	}

	// Layout da Tabela
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// Cabeçalho
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Background{Color: theme.Colors.Grey100}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{}.Layout(gtx,
								layout.Flexed(0.2, header(gtx, 0, "Usuário")),
								layout.Flexed(0.3, header(gtx, 1, "Email")),
								layout.Flexed(0.2, header(gtx, 2, "Perfis")),
								layout.Flexed(0.1, header(gtx, 3, "Status")),
								layout.Flexed(0.2, header(gtx, 4, "Último Login")),
							)
						})
				})
		}),
		// Lista de Usuários
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return p.userList.Layout(gtx, len(p.filteredUsers), func(gtx layout.Context, index int) layout.Dimensions {
				if index < 0 || index >= len(p.filteredUsers) {
					return layout.Dimensions{}
				}
				return userRow(gtx, index, p.filteredUsers[index])
			})
		}),
	)
}

// --- Lógica para Diálogo de Atribuição de Roles ---
func (p *AdminPermissionsPage) openUserRoleDialog() {
	if p.selectedUserID == nil {
		return
	}

	foundUser := false
	for _, u := range p.users { // Busca na lista completa de usuários
		if u.ID == *p.selectedUserID {
			p.userForRoleDialog = u
			foundUser = true
			break
		}
	}
	if !foundUser {
		p.statusMessage = "Usuário selecionado não encontrado para editar roles."
		p.messageColor = theme.Colors.Danger
		return
	}

	// Inicializar checkboxes do diálogo
	p.userRoleDialogCheckboxes = make(map[string]*widget.Bool)
	currentUserRolesSet := make(map[string]bool)
	for _, rName := range p.userForRoleDialog.Roles {
		currentUserRolesSet[strings.ToLower(rName)] = true
	}

	for _, availableRole := range p.allRoles {
		chk := new(widget.Bool)
		if currentUserRolesSet[strings.ToLower(availableRole.Name)] {
			chk.Value = true
		}
		p.userRoleDialogCheckboxes[availableRole.Name] = chk
	}
	p.showUserRoleDialog = true
	p.statusMessage = "" // Limpa mensagem principal
	p.router.GetAppWindow().Invalidate()
}

func (p *AdminPermissionsPage) layoutUserRoleDialog(gtx layout.Context, th *material.Theme) layout.Dimensions {
	if p.userRoleDialogSaveBtn.Clicked(gtx) {
		p.handleSaveUserRoles()
	}
	if p.userRoleDialogCancelBtn.Clicked(gtx) {
		p.showUserRoleDialog = false
	}

	// Simulação de um modal/overlay
	// Desenha um fundo semi-transparente
	paint.FillShape(gtx.Ops, color.NRGBA{A: 180}, clip.Rect{Max: gtx.Constraints.Max}.Op())

	dialogWidth := gtx.Dp(unit.Dp(450))
	if dialogWidth > gtx.Constraints.Max.X-gtx.Dp(20) {
		dialogWidth = gtx.Constraints.Max.X - gtx.Dp(20)
	}

	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return material.Dialog(th, "Atribuir Perfis para "+p.userForRoleDialog.Username).Layout(gtx,
			material.Inset(unit.Dp(16), layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceEnd}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					list := &layout.List{Axis: layout.Vertical}
					return list.Layout(gtx, len(p.allRoles), func(gtx layout.Context, i int) layout.Dimensions {
						role := p.allRoles[i] // Ordenar p.allRoles se necessário
						chk, ok := p.userRoleDialogCheckboxes[role.Name]
						if !ok {
							return layout.Dimensions{}
						} // Não deveria acontecer

						// TODO: Lógica para não permitir desmarcar o último admin
						// if strings.ToLower(role.Name) == "admin" && p._isPotentiallyLastAdmin(p.userForRoleDialog.ID) && !chk.Value ...

						return material.CheckBox(th, chk, role.Name).Layout(gtx)
					})
				}),
				layout.Rigid(layout.Spacer{Height: theme.LargeVSpacer}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Spacing: layout.SpaceBetween}.Layout(gtx,
						layout.Flexed(1, material.Button(th, &p.userRoleDialogCancelBtn, "Cancelar").Layout),
						layout.Flexed(1, material.Button(th, &p.userRoleDialogSaveBtn, "Salvar").Layout),
					)
				}),
			)),
		)
	})
}

func (p *AdminPermissionsPage) handleSaveUserRoles() {
	if p.userForRoleDialog == nil {
		return
	}

	p.isLoading = true
	p.statusMessage = "Salvando perfis..."
	p.messageColor = theme.Colors.TextMuted
	p.spinner.Start(p.router.GetAppWindow().Context())
	p.showUserRoleDialog = false // Fecha o diálogo enquanto salva
	p.router.GetAppWindow().Invalidate()

	selectedRoleNames := []string{}
	for roleName, chk := range p.userRoleDialogCheckboxes {
		if chk.Value {
			selectedRoleNames = append(selectedRoleNames, roleName)
		}
	}
	if len(selectedRoleNames) == 0 {
		// Forçar pelo menos um role? Ou permitir usuário sem roles?
		// Se forçar, "user" pode ser um default.
		// Por agora, permite.
		appLogger.Warnf("Usuário %s (ID: %s) ficaria sem roles. Permitindo por enquanto.", p.userForRoleDialog.Username, p.userForRoleDialog.ID)
	}

	go func(userID uuid.UUID, rolesToSet []string, usernameToLog string) {
		var opErr error
		currentAdminSession, _ := p.sessionManager.GetCurrentSession() // Obter sessão atual

		updatePayload := models.UserUpdate{RoleNames: &rolesToSet}
		_, err := p.userService.UpdateUser(userID, updatePayload, currentAdminSession)
		if err != nil {
			opErr = fmt.Errorf("falha ao atualizar perfis do usuário: %w", err)
		}

		p.router.GetAppWindow().Execute(func() {
			p.isLoading = false
			p.spinner.Stop(p.router.GetAppWindow().Context())
			if opErr != nil {
				p.statusMessage = opErr.Error()
				p.messageColor = theme.Colors.Danger
				appLogger.Errorf("Erro ao salvar roles para %s: %v", usernameToLog, opErr)
			} else {
				p.statusMessage = fmt.Sprintf("Perfis de '%s' atualizados com sucesso!", usernameToLog)
				p.messageColor = theme.Colors.Success
				appLogger.Infof("Perfis de '%s' atualizados para: %v", usernameToLog, rolesToSet)
				p.loadInitialData() // Recarrega tudo para refletir mudanças
			}
			p.router.GetAppWindow().Invalidate()
		})
	}(p.userForRoleDialog.ID, selectedRoleNames, p.userForRoleDialog.Username)
}

// --- Lógica para Desativar Usuário ---
func (p *AdminPermissionsPage) handleDeactivateUser() {
	if p.selectedUserID == nil {
		return
	}

	var userToDeactivate *models.UserPublic
	for _, u := range p.users {
		if u.ID == *p.selectedUserID {
			userToDeactivate = u
			break
		}
	}
	if userToDeactivate == nil {
		p.statusMessage = "Usuário selecionado não encontrado para desativação."
		p.messageColor = theme.Colors.Danger
		return
	}
	if !userToDeactivate.Active {
		p.statusMessage = fmt.Sprintf("Usuário '%s' já está inativo.", userToDeactivate.Username)
		p.messageColor = theme.Colors.Info
		return
	}

	// TODO: Mostrar um diálogo de confirmação antes de desativar.
	// Por agora, desativa diretamente.

	p.isLoading = true
	p.statusMessage = "Desativando usuário..."
	p.messageColor = theme.Colors.TextMuted
	p.spinner.Start(p.router.GetAppWindow().Context())
	p.router.GetAppWindow().Invalidate()

	go func(userID uuid.UUID, usernameToLog string) {
		var opErr error
		currentAdminSession, _ := p.sessionManager.GetCurrentSession()
		err := p.userService.DeactivateUser(userID, currentAdminSession)
		if err != nil {
			opErr = fmt.Errorf("falha ao desativar usuário: %w", err)
		}

		p.router.GetAppWindow().Execute(func() {
			p.isLoading = false
			p.spinner.Stop(p.router.GetAppWindow().Context())
			if opErr != nil {
				p.statusMessage = opErr.Error()
				p.messageColor = theme.Colors.Danger
				appLogger.Errorf("Erro ao desativar usuário %s: %v", usernameToLog, opErr)
			} else {
				p.statusMessage = fmt.Sprintf("Usuário '%s' desativado com sucesso!", usernameToLog)
				p.messageColor = theme.Colors.Success
				appLogger.Infof("Usuário '%s' desativado.", usernameToLog)
				p.loadInitialData() // Recarrega para refletir status
			}
			p.selectedUserID = nil // Limpa seleção
			p.router.GetAppWindow().Invalidate()
		})
	}(*p.selectedUserID, userToDeactivate.Username)
}

// --- Helpers ---
func boolToString(b bool, trueStr, falseStr string) string {
	if b {
		return trueStr
	}
	return falseStr
}

func formatOptionalTime(t *time.Time) string {
	if t == nil || t.IsZero() {
		return "Nunca"
	}
	return t.Local().Format("02/01/2006 15:04") // Formato local
}

// mapsEqual é um helper que você usou no UserService, pode ser movido para utils
func mapsEqual(a, b map[string]bool) bool { /* ... implementação ... */ return true }
