package pages

import (
	"fmt"
	"image/color"
	"sort"
	"strings"
	"time"

	// "time" // Not directly used for timing, but for time.Time fields in models

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/auth"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/config"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/errors" // For type checking errors
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/models"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/services"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui/components"
	// "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui"
	"github.com/google/uuid"
)

const (
	colIndexUsername   = 0
	colIndexEmail      = 1
	colIndexRoles      = 2
	colIndexStatus     = 3
	colIndexLastLogin  = 4
	numTableHeaders    = 5
	defaultSortColumn  = colIndexUsername
	defaultSortAsc     = true
	roleFilterAllValue = "Todos" // Value for "Todos os Perfis" no filtro
)

// AdminPermissionsPage gerencia usuários e suas permissões/roles.
type AdminPermissionsPage struct {
	router         *ui.Router
	cfg            *core.Config
	userService    services.UserService
	roleService    services.RoleService
	auditService   services.AuditLogService
	permManager    *auth.PermissionManager
	sessionManager *auth.SessionManager

	// Estado da UI
	isLoading      bool
	users          []*models.UserPublic // Lista completa de usuários carregados
	filteredUsers  []*models.UserPublic // Usuários após filtro e ordenação
	allRoles       []*models.RolePublic // Todos os roles disponíveis para filtros e diálogos
	selectedUserID *uuid.UUID           // ID do usuário atualmente selecionado na lista
	statusMessage  string               // Para mensagens de erro ou sucesso na página
	messageColor   color.NRGBA

	// Widgets de controle
	searchEditor   widget.Editor
	roleFilter     widget.Enum // Para o ComboBox de filtro de role
	refreshBtn     widget.Clickable
	manageRolesBtn widget.Clickable

	// Widgets de ação para usuário selecionado
	assignRolesBtn widget.Clickable
	deactivateBtn  widget.Clickable
	unlockBtn      widget.Clickable
	resetPassBtn   widget.Clickable

	// Para a lista/tabela de usuários
	userList          layout.List
	userClickables    []widget.Clickable                // Um clickable por usuário na lista filtrada
	tableHeaderClicks [numTableHeaders]widget.Clickable // Para ordenação
	sortColumn        int                               // Índice da coluna para ordenação
	sortAscending     bool                              // Direção da ordenação

	// Para diálogo de atribuição de roles
	showUserRoleDialog       bool
	userForRoleDialog        *models.UserPublic      // Usuário cujos roles estão sendo editados
	userRoleDialogCheckboxes map[string]*widget.Bool // map[roleName]*widget.Bool
	userRoleDialogSaveBtn    widget.Clickable
	userRoleDialogCancelBtn  widget.Clickable
	roleDialogStatusMessage  string // Mensagem específica para o diálogo de roles
	roleDialogMessageColor   color.NRGBA

	spinner *components.LoadingSpinner

	firstLoadDone bool // Para controlar o carregamento inicial de dados
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
		spinner:        components.NewLoadingSpinner(theme.Colors.Primary), // Cor customizada para o spinner
		sortColumn:     defaultSortColumn,
		sortAscending:  defaultSortAsc,
		roleFilter:     widget.Enum{Value: roleFilterAllValue}, // Valor inicial do filtro de role
	}
	p.searchEditor.SingleLine = true
	p.searchEditor.Hint = "Pesquisar por nome ou e-mail..."
	return p
}

// OnNavigatedTo é chamado quando a página se torna ativa.
func (p *AdminPermissionsPage) OnNavigatedTo(params interface{}) {
	appLogger.Info("Navegou para AdminPermissionsPage")
	p.statusMessage = ""   // Limpa mensagem global da página
	p.selectedUserID = nil // Limpa seleção ao entrar na página

	// Verifica permissão para acessar esta página.
	// Se não tiver, o MainAppLayout não deveria permitir a navegação,
	// mas uma checagem aqui é uma segurança adicional.
	currentAdminSession, errSess := p.sessionManager.GetCurrentSession()
	if errSess != nil || currentAdminSession == nil {
		p.router.GetAppWindow().HandleLogout() // Força logout se a sessão for inválida
		return
	}
	// PermUserRead é uma permissão base para ver a lista de usuários.
	// PermRoleManage pode ser necessária para o botão "Gerenciar Perfis".
	if err := p.permManager.CheckPermission(currentAdminSession, auth.PermUserRead, nil); err != nil {
		p.statusMessage = fmt.Sprintf("Acesso negado à página de gerenciamento de usuários: %v", err)
		p.messageColor = theme.Colors.Danger
		p.users = []*models.UserPublic{} // Limpa dados se não tem permissão
		p.applyFiltersAndSort()
		p.router.GetAppWindow().Invalidate()
		return
	}

	if !p.firstLoadDone {
		p.loadInitialData(currentAdminSession)
		p.firstLoadDone = true
	} else {
		// Se os dados já foram carregados, pode-se optar por recarregar para ter dados frescos,
		// ou apenas aplicar filtros se a lógica de filtro for puramente na UI.
		// Recarregar é mais seguro se os dados podem mudar em background.
		p.loadInitialData(currentAdminSession) // Recarrega os dados ao revisitar a página
	}
}

// OnNavigatedFrom é chamado quando o router está prestes a navegar para fora desta página.
func (p *AdminPermissionsPage) OnNavigatedFrom() {
	appLogger.Info("Navegando para fora da AdminPermissionsPage")
	p.showUserRoleDialog = false // Garante que diálogos sejam fechados
	p.isLoading = false
	p.spinner.Stop(p.router.GetAppWindow().Context()) // Garante que o spinner pare
}

// loadInitialData carrega os dados iniciais (usuários e roles) para a página.
func (p *AdminPermissionsPage) loadInitialData(currentAdminSession *auth.SessionData) {
	if p.isLoading {
		return
	}
	p.isLoading = true
	p.statusMessage = "Carregando dados..."
	p.messageColor = theme.Colors.TextMuted
	p.spinner.Start(p.router.GetAppWindow().Context())
	p.router.GetAppWindow().Invalidate()

	go func(adminSess *auth.SessionData) {
		var loadErr error
		var loadedUsers []*models.UserPublic
		var loadedRoles []*models.RolePublic

		// Carregar roles primeiro para o filtro. Requer PermRoleManage para listar todos.
		if err := p.permManager.CheckPermission(adminSess, auth.PermRoleManage, nil); err == nil {
			roles, errRoles := p.roleService.GetAllRoles(adminSess)
			if errRoles != nil {
				loadErr = fmt.Errorf("falha ao carregar perfis: %w", errRoles)
			} else {
				loadedRoles = roles
			}
		} else {
			appLogger.Warnf("Usuário %s não tem permissão para listar roles (PermRoleManage), filtro de roles pode estar limitado.", adminSess.Username)
			// Pode-se optar por não carregar o filtro de roles ou carregar apenas os roles do próprio usuário.
			// Por agora, o filtro pode ficar vazio ou com "Todos".
		}

		// Carregar usuários se a etapa anterior não falhou (ou se roles são opcionais para o filtro).
		// Requer PermUserRead.
		if loadErr == nil {
			if err := p.permManager.CheckPermission(adminSess, auth.PermUserRead, nil); err == nil {
				users, errUsers := p.userService.ListUsers(true, adminSess) // true = include inactive
				if errUsers != nil {
					loadErr = fmt.Errorf("falha ao carregar usuários: %w", errUsers)
				} else {
					loadedUsers = users
				}
			} else {
				loadErr = fmt.Errorf("usuário %s não tem permissão para listar usuários (PermUserRead).", adminSess.Username)
			}
		}

		p.router.GetAppWindow().Execute(func() { // Atualiza UI na thread principal
			p.isLoading = false
			p.spinner.Stop(p.router.GetAppWindow().Context())
			if loadErr != nil {
				p.statusMessage = loadErr.Error()
				p.messageColor = theme.Colors.Danger
				p.users = []*models.UserPublic{} // Limpa em caso de erro
				p.allRoles = []*models.RolePublic{}
				appLogger.Errorf("Erro ao carregar dados para AdminPermissionsPage: %v", loadErr)
			} else {
				p.users = loadedUsers
				p.allRoles = loadedRoles
				if len(p.users) > 0 || len(p.allRoles) > 0 {
					p.statusMessage = fmt.Sprintf("%d usuários e %d perfis carregados.", len(p.users), len(p.allRoles))
					p.messageColor = theme.Colors.Success
				} else {
					p.statusMessage = "Nenhum usuário ou perfil encontrado."
					p.messageColor = theme.Colors.Info
				}
				p.applyFiltersAndSort()

				// Preencher roleFilter Enum
				p.roleFilter.Value = roleFilterAllValue                     // Default
				p.roleFilter.SetEnum(roleFilterAllValue, "Todos os Perfis") // Adiciona a opção "Todos"
				// Ordena roles para o dropdown
				sort.SliceStable(p.allRoles, func(i, j int) bool {
					return strings.ToLower(p.allRoles[i].Name) < strings.ToLower(p.allRoles[j].Name)
				})
				for _, r := range p.allRoles {
					p.roleFilter.SetEnum(r.Name, strings.Title(r.Name)) // Key, Label (capitalizado para exibição)
				}
			}
			p.updateActionButtonsState() // Atualiza estado dos botões de ação
			p.router.GetAppWindow().Invalidate()
		})
	}(currentAdminSession)
}

// applyFiltersAndSort filtra e ordena a lista de usuários.
func (p *AdminPermissionsPage) applyFiltersAndSort() {
	searchTerm := strings.ToLower(strings.TrimSpace(p.searchEditor.Text()))
	selectedRoleFilter := p.roleFilter.Value

	tempFiltered := make([]*models.UserPublic, 0, len(p.users))

	for _, user := range p.users {
		// Filtro de pesquisa (nome ou email)
		matchesSearch := true
		if searchTerm != "" {
			matchesSearch = strings.Contains(strings.ToLower(user.Username), searchTerm) ||
				strings.Contains(strings.ToLower(user.Email), searchTerm)
		}

		// Filtro de Role
		matchesRole := true
		if selectedRoleFilter != roleFilterAllValue {
			userHasRole := false
			for _, roleName := range user.Roles {
				if strings.EqualFold(roleName, selectedRoleFilter) {
					userHasRole = true
					break
				}
			}
			matchesRole = userHasRole
		}

		if matchesSearch && matchesRole {
			tempFiltered = append(tempFiltered, user)
		}
	}

	// Ordenação
	sort.SliceStable(tempFiltered, func(i, j int) bool {
		u1 := tempFiltered[i]
		u2 := tempFiltered[j]
		var less bool
		switch p.sortColumn {
		case colIndexUsername:
			less = strings.ToLower(u1.Username) < strings.ToLower(u2.Username)
		case colIndexEmail:
			less = strings.ToLower(u1.Email) < strings.ToLower(u2.Email)
		case colIndexRoles:
			// Ordenar por string concatenada de roles pode ser simples, mas não ideal.
			// Uma ordenação mais sofisticada poderia ser por número de roles ou por um role primário.
			less = strings.Join(u1.Roles, ",") < strings.Join(u2.Roles, ",")
		case colIndexStatus:
			less = u1.Active && !u2.Active // Ativos primeiro
		case colIndexLastLogin:
			// Tratar nulos (nunca logou) como mais antigos.
			if u1.LastLogin == nil && u2.LastLogin != nil {
				less = true
			}
			if u1.LastLogin != nil && u2.LastLogin == nil {
				less = false
			}
			if u1.LastLogin != nil && u2.LastLogin != nil {
				less = u1.LastLogin.Before(*u2.LastLogin)
			}
		default:
			less = u1.ID.String() < u2.ID.String() // Fallback
		}
		if !p.sortAscending {
			return !less
		}
		return less
	})

	p.filteredUsers = tempFiltered
	// Ajusta o tamanho do slice de clickables para corresponder aos usuários filtrados.
	if len(p.filteredUsers) != len(p.userClickables) {
		p.userClickables = make([]widget.Clickable, len(p.filteredUsers))
	}
	// appLogger.Debugf("Filtros e ordenação aplicados. %d usuários visíveis.", len(p.filteredUsers))
}

// Layout é o método principal de desenho da página.
func (p *AdminPermissionsPage) Layout(gtx layout.Context) layout.Dimensions {
	th := p.router.GetAppWindow().Theme()

	// Processar eventos dos inputs e filtros
	if p.searchEditor.Update(gtx) { // Update retorna true se o texto mudou
		p.applyFiltersAndSort()
		p.selectedUserID = nil // Limpa seleção ao mudar filtro
		p.updateActionButtonsState()
	}
	if p.roleFilter.Update(gtx) { // Se o valor do Enum (ComboBox) mudar
		p.applyFiltersAndSort()
		p.selectedUserID = nil // Limpa seleção ao mudar filtro
		p.updateActionButtonsState()
	}

	// Processar cliques nos botões de ação
	if p.refreshBtn.Clicked(gtx) {
		currentAdminSession, _ := p.sessionManager.GetCurrentSession()
		if currentAdminSession != nil {
			p.loadInitialData(currentAdminSession)
		}
	}
	if p.manageRolesBtn.Clicked(gtx) {
		// O botão "Gerenciar Perfis" deve ser habilitado/desabilitado com base na permissão PermRoleManage.
		// A verificação de permissão para navegar é feita aqui ou na página de destino.
		currentAdminSession, _ := p.sessionManager.GetCurrentSession()
		if err := p.permManager.CheckPermission(currentAdminSession, auth.PermRoleManage, nil); err == nil {
			p.router.NavigateTo(ui.PageRoleManagement, nil)
		} else {
			p.statusMessage = "Você não tem permissão para gerenciar perfis."
			p.messageColor = theme.Colors.Danger
		}
	}

	canManageUserRoles, canDeactivateUser, canUnlockUser, canResetPass := p.updateActionButtonsState()

	if p.assignRolesBtn.Clicked(gtx) {
		if canManageUserRoles {
			p.openUserRoleDialog()
		}
	}
	if p.deactivateBtn.Clicked(gtx) {
		if canDeactivateUser {
			p.handleDeactivateUser()
		}
	}
	if p.unlockBtn.Clicked(gtx) {
		if canUnlockUser {
			p.handleUnlockUser()
		}
	}
	if p.resetPassBtn.Clicked(gtx) {
		if canResetPass {
			p.handleAdminResetPassword()
		}
	}

	// Lógica para o diálogo de edição de roles do usuário
	if p.showUserRoleDialog {
		// O diálogo sobrepõe o layout principal.
		// layout.Stack é usado na AppWindow para sobrepor o spinner global.
		// Para diálogos modais, uma abordagem comum é desenhá-los por último
		// com um fundo semi-transparente que cobre a tela inteira.
		mainLayout := p.layoutMainContent(gtx, th) // Desenha o conteúdo principal por baixo
		dialogLayout := p.layoutUserRoleDialog(gtx, th)
		// Empilha o diálogo sobre o conteúdo principal.
		return layout.Stack{}.Layout(gtx,
			layout.Expanded(func(gtx C) D { return mainLayout }),
			layout.Expanded(func(gtx C) D { return dialogLayout }), // O diálogo deve cobrir a tela se modal
		)
	}

	return p.layoutMainContent(gtx, th)
}

// layoutMainContent desenha o conteúdo principal da página (sem diálogos sobrepostos).
func (p *AdminPermissionsPage) layoutMainContent(gtx layout.Context, th *material.Theme) layout.Dimensions {
	// Layout principal da página
	return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceBetween}.Layout(gtx,
		// Linha 1: Controles superiores (Pesquisa, Filtro, Atualizar, Gerenciar Perfis)
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: theme.DefaultVSpacer}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					// Botão "Gerenciar Perfis"
					btnManageRolesWidget := material.Button(th, &p.manageRolesBtn, "Gerenciar Perfis")
					// Desabilitar visualmente se não tiver permissão PermRoleManage
					currentAdminSession, _ := p.sessionManager.GetCurrentSession()
					if hasPerm, _ := p.permManager.HasPermission(currentAdminSession, auth.PermRoleManage, nil); !hasPerm {
						btnManageRolesWidget.Style.TextColor = theme.Colors.TextMuted
						btnManageRolesWidget.Style.Background = theme.Colors.Grey300 // Cor de fundo para desabilitado
					}

					return layout.Flex{Spacing: layout.SpaceBetween, Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(0.4, material.Editor(th, &p.searchEditor, p.searchEditor.Hint).Layout), // Editor de pesquisa
						layout.Rigid(layout.Spacer{Width: theme.DefaultVSpacer}.Layout),
						layout.Flexed(0.3, func(gtx C) D { // Dropdown de Filtro de Role
							// Um ComboBox real seria mais complexo, usando material.DropDown ou um customizado.
							// Por agora, um widget.Enum com layout básico.
							// O texto exibido no DropDown deve ser o Label, não o Value.
							selectedLabel := p.roleFilter.Value
							for _, role := range p.allRoles {
								if role.Name == p.roleFilter.Value {
									selectedLabel = strings.Title(role.Name)
									break
								}
							}
							if p.roleFilter.Value == roleFilterAllValue {
								selectedLabel = "Todos os Perfis"
							}

							return material.DropDown(th, &p.roleFilter, material.Body1(th, selectedLabel).Layout).Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Width: theme.DefaultVSpacer}.Layout),
						layout.Rigid(material.Button(th, &p.refreshBtn, "Atualizar Lista").Layout),
						layout.Rigid(layout.Spacer{Width: theme.DefaultVSpacer}.Layout),
						layout.Rigid(btnManageRolesWidget.Layout),
					)
				})
		}),

		// Linha 2: Tabela/Lista de Usuários
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return p.layoutUserTable(gtx, th)
		}),

		// Linha 3: Botões de Ação Inferiores para o usuário selecionado
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: theme.DefaultVSpacer}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					canManageUserRoles, canDeactivateUser, canUnlockUser, canResetPass := p.updateActionButtonsState()

					assignBtnWidget := material.Button(th, &p.assignRolesBtn, "Atribuir Perfis")
					if !canManageUserRoles {
						assignBtnWidget.Style.TextColor = theme.Colors.TextMuted
						assignBtnWidget.Style.Background = theme.Colors.Grey300
					}

					deactivateBtnWidget := material.Button(th, &p.deactivateBtn, "Desativar")
					if !canDeactivateUser {
						deactivateBtnWidget.Style.TextColor = theme.Colors.TextMuted
						deactivateBtnWidget.Style.Background = theme.Colors.Grey300
					}

					unlockBtnWidget := material.Button(th, &p.unlockBtn, "Desbloquear")
					if !canUnlockUser {
						unlockBtnWidget.Style.TextColor = theme.Colors.TextMuted
						unlockBtnWidget.Style.Background = theme.Colors.Grey300
					}

					resetPassBtnWidget := material.Button(th, &p.resetPassBtn, "Resetar Senha")
					if !canResetPass {
						resetPassBtnWidget.Style.TextColor = theme.Colors.TextMuted
						resetPassBtnWidget.Style.Background = theme.Colors.Grey300
					}

					return layout.Flex{Spacing: layout.SpaceBetween}.Layout(gtx,
						layout.Rigid(assignBtnWidget.Layout),
						layout.Rigid(layout.Spacer{Width: theme.DefaultVSpacer}.Layout),
						layout.Rigid(deactivateBtnWidget.Layout),
						layout.Rigid(layout.Spacer{Width: theme.DefaultVSpacer}.Layout),
						layout.Rigid(unlockBtnWidget.Layout),
						layout.Rigid(layout.Spacer{Width: theme.DefaultVSpacer}.Layout),
						layout.Rigid(resetPassBtnWidget.Layout),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }), // Espaçador
					)
				})
		}),

		// Linha 4: Mensagem de Status global da página
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.statusMessage != "" {
				lbl := material.Body2(th, p.statusMessage)
				lbl.Color = p.messageColor
				return layout.Inset{Top: theme.DefaultVSpacer}.Layout(gtx, lbl.Layout)
			}
			return layout.Dimensions{}
		}),

		// Spinner de Carregamento (sobrepõe tudo se ativo via Stack na AppWindow)
		// Se o spinner for local à página e precisar sobrepor, um layout.Stack seria usado aqui.
		// Se p.isLoading { return p.spinner.Layout(gtx) }
	)
}

// updateActionButtonsState determina se os botões de ação devem estar habilitados e atualiza o estado.
// Retorna os estados para uso imediato no Layout.
func (p *AdminPermissionsPage) updateActionButtonsState() (canManageRoles, canDeactivate, canUnlock, canResetPass bool) {
	if p.selectedUserID == nil || p.sessionManager == nil {
		return false, false, false, false
	}
	currentAdminSession, _ := p.sessionManager.GetCurrentSession()
	if currentAdminSession == nil {
		return false, false, false, false
	}

	isSelf := currentAdminSession.UserID == *p.selectedUserID
	var selectedUser *models.UserPublic
	for _, u := range p.filteredUsers { // Procura na lista filtrada (ou na `p.users` completa)
		if u.ID == *p.selectedUserID {
			selectedUser = u
			break
		}
	}
	if selectedUser == nil { // Usuário selecionado não encontrado na lista (pode acontecer se a lista mudar)
		return false, false, false, false
	}

	// Permissão para gerenciar roles + não ser si mesmo
	hasPermManageRoles, _ := p.permManager.HasPermission(currentAdminSession, auth.PermUserManageRoles, nil)
	canManageRoles = hasPermManageRoles && !isSelf

	// Permissão para desativar + não ser si mesmo + usuário estar ativo
	hasPermDelete, _ := p.permManager.HasPermission(currentAdminSession, auth.PermUserDelete, nil)
	canDeactivate = hasPermDelete && !isSelf && selectedUser.Active

	// Permissão para desbloquear + não ser si mesmo + usuário estar bloqueado (FailedAttempts > 0)
	// A verificação se o usuário está realmente bloqueado é melhor feita no serviço.
	// Aqui, apenas verificamos a permissão.
	hasPermUnlock, _ := p.permManager.HasPermission(currentAdminSession, auth.PermUserUnlock, nil)
	canUnlock = hasPermUnlock && !isSelf // E o serviço verificará se está bloqueado.

	// Permissão para resetar senha + não ser si mesmo
	hasPermResetPass, _ := p.permManager.HasPermission(currentAdminSession, auth.PermUserResetPassword, nil)
	canResetPass = hasPermResetPass && !isSelf

	return canManageRoles, canDeactivate, canUnlock, canResetPass
}

// layoutUserTable desenha a lista de usuários.
func (p *AdminPermissionsPage) layoutUserTable(gtx layout.Context, th *material.Theme) layout.Dimensions {
	// Cabeçalho da Tabela
	headerLayout := func(colIndex int, label string) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			// TODO: Adicionar ícone de ordenação (seta para cima/baixo) ao lado do label
			// se p.sortColumn == colIndex.
			headerLabel := material.Body1(th, label)
			headerLabel.Font.Weight = font.Bold
			if p.tableHeaderClicks[colIndex].Clicked(gtx) {
				if p.sortColumn == colIndex {
					p.sortAscending = !p.sortAscending
				} else {
					p.sortColumn = colIndex
					p.sortAscending = true
				}
				p.applyFiltersAndSort()
				p.selectedUserID = nil // Limpa seleção ao reordenar
				p.updateActionButtonsState()
			}
			return headerLabel.Layout(gtx)
		}
	}

	// Linha de Usuário
	userRowLayout := func(gtx layout.Context, index int, user *models.UserPublic) layout.Dimensions {
		isSelected := p.selectedUserID != nil && *p.selectedUserID == user.ID

		rowBgColor := theme.Colors.Surface // Cor padrão para linhas
		if index%2 != 0 {
			rowBgColor = theme.Colors.BackgroundAlt // Cor alternada para zebrado
		}
		if !user.Active { // Cinza mais escuro para usuários inativos
			rowBgColor = theme.Colors.Grey100 // Ou outra cor que indique inatividade
		}
		if isSelected {
			rowBgColor = theme.Colors.PrimaryLight // Destaque para linha selecionada
		}

		return material.Clickable(gtx, &p.userClickables[index], func(gtx layout.Context) layout.Dimensions {
			textColor := theme.Colors.Text
			if !user.Active {
				textColor = theme.Colors.TextMuted
			}
			if isSelected {
				textColor = theme.Colors.PrimaryText
			}

			// Criar widgets de label para cada coluna com a cor de texto apropriada.
			usernameLbl := material.Body2(th, user.Username)
			usernameLbl.Color = textColor
			emailLbl := material.Body2(th, user.Email)
			emailLbl.Color = textColor
			rolesLbl := material.Body2(th, strings.Join(user.Roles, ", "))
			rolesLbl.Color = textColor
			statusLbl := material.Body2(th, boolToString(user.Active, "Ativo", "Inativo"))
			statusLbl.Color = textColor
			lastLoginLbl := material.Body2(th, formatOptionalTime(user.LastLogin))
			lastLoginLbl.Color = textColor

			return layout.Background{Color: rowBgColor}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{}.Layout(gtx,
								layout.Flexed(0.20, usernameLbl.Layout),
								layout.Flexed(0.30, emailLbl.Layout),
								layout.Flexed(0.20, rolesLbl.Layout),
								layout.Flexed(0.10, statusLbl.Layout),
								layout.Flexed(0.20, lastLoginLbl.Layout),
							)
						})
				})
		})
	}

	// Processar cliques na linha para seleção.
	for i := range p.filteredUsers {
		if p.userClickables[i].Clicked(gtx) {
			if p.selectedUserID != nil && *p.selectedUserID == p.filteredUsers[i].ID {
				p.selectedUserID = nil // Desseleciona se clicar no mesmo usuário.
			} else {
				selectedID := p.filteredUsers[i].ID // Copia o UUID para evitar problemas de ponteiro com o loop.
				p.selectedUserID = &selectedID
			}
			p.updateActionButtonsState() // Atualiza estado dos botões com base na nova seleção.
			p.router.GetAppWindow().Invalidate()
		}
	}

	// Layout da Tabela
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// Cabeçalho da tabela
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Background{Color: theme.Colors.Grey200}.Layout(gtx, // Fundo para o cabeçalho
				func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{}.Layout(gtx,
								layout.Flexed(0.20, headerLayout(colIndexUsername, "Usuário").Layout),
								layout.Flexed(0.30, headerLayout(colIndexEmail, "Email").Layout),
								layout.Flexed(0.20, headerLayout(colIndexRoles, "Perfis").Layout),
								layout.Flexed(0.10, headerLayout(colIndexStatus, "Status").Layout),
								layout.Flexed(0.20, headerLayout(colIndexLastLogin, "Último Login").Layout),
							)
						})
				})
		}),
		// Lista de Usuários com scroll
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return p.userList.Layout(gtx, len(p.filteredUsers), func(gtx layout.Context, index int) layout.Dimensions {
				if index < 0 || index >= len(p.filteredUsers) { // Segurança
					return layout.Dimensions{}
				}
				return userRowLayout(gtx, index, p.filteredUsers[index])
			})
		}),
	)
}

// --- Lógica para Diálogo de Atribuição de Roles ---
func (p *AdminPermissionsPage) openUserRoleDialog() {
	if p.selectedUserID == nil {
		p.statusMessage = "Nenhum usuário selecionado para atribuir perfis."
		p.messageColor = theme.Colors.Warning
		p.router.GetAppWindow().Invalidate()
		return
	}

	foundUser := false
	for _, u := range p.users { // Busca na lista completa de usuários para obter dados frescos
		if u.ID == *p.selectedUserID {
			p.userForRoleDialog = u
			foundUser = true
			break
		}
	}
	if !foundUser {
		p.statusMessage = "Usuário selecionado não encontrado na lista (pode ter sido removido). Recarregue a lista."
		p.messageColor = theme.Colors.Danger
		p.selectedUserID = nil // Limpa seleção inválida
		p.updateActionButtonsState()
		p.router.GetAppWindow().Invalidate()
		return
	}

	// Inicializar checkboxes do diálogo com os roles atuais do usuário.
	p.userRoleDialogCheckboxes = make(map[string]*widget.Bool)
	currentUserRolesSet := make(map[string]bool)
	for _, rName := range p.userForRoleDialog.Roles {
		currentUserRolesSet[strings.ToLower(rName)] = true
	}

	// Ordena os roles disponíveis para exibição no diálogo.
	sort.SliceStable(p.allRoles, func(i, j int) bool {
		return strings.ToLower(p.allRoles[i].Name) < strings.ToLower(p.allRoles[j].Name)
	})

	for _, availableRole := range p.allRoles {
		chk := new(widget.Bool) // Cria um novo widget.Bool para cada role.
		if currentUserRolesSet[strings.ToLower(availableRole.Name)] {
			chk.Value = true
		}
		p.userRoleDialogCheckboxes[availableRole.Name] = chk // Usa o nome original do role como chave
	}
	p.showUserRoleDialog = true
	p.statusMessage = ""           // Limpa mensagem global da página
	p.roleDialogStatusMessage = "" // Limpa mensagem do diálogo
	p.router.GetAppWindow().Invalidate()
}

// layoutUserRoleDialog desenha o diálogo para atribuir roles a um usuário.
func (p *AdminPermissionsPage) layoutUserRoleDialog(gtx layout.Context, th *material.Theme) layout.Dimensions {
	if p.userForRoleDialog == nil { // Segurança
		p.showUserRoleDialog = false
		return layout.Dimensions{}
	}

	// Processar cliques nos botões do diálogo
	if p.userRoleDialogSaveBtn.Clicked(gtx) {
		p.handleSaveUserRoles() // Não fecha o diálogo imediatamente, handleSaveUserRoles o fará no sucesso/erro
	}
	if p.userRoleDialogCancelBtn.Clicked(gtx) {
		p.showUserRoleDialog = false
		p.roleDialogStatusMessage = ""
		p.router.GetAppWindow().Invalidate()
	}

	// Fundo semi-transparente para o efeito modal
	paint.FillShape(gtx.Ops, color.NRGBA{A: 180}, clip.Rect{Max: gtx.Constraints.Max}.Op())

	// Centraliza o diálogo na tela
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		// Limitar largura do diálogo
		// gtx.Constraints.Max.X = gtx.Dp(unit.Dp(450))
		// if gtx.Constraints.Max.X > gtx.Constraints.Min.X { // Garante que não seja menor que o mínimo
		// 	gtx.Constraints.Min.X = gtx.Constraints.Max.X
		// }

		return material.Dialog(th, fmt.Sprintf("Atribuir Perfis para: %s", p.userForRoleDialog.Username)).Layout(gtx,
			material.Inset(unit.Dp(16), layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceBetween}.Layout(gtx,
				// Lista de Checkboxes de Roles (com scroll se necessário)
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					list := &layout.List{Axis: layout.Vertical}
					// Usa p.allRoles que já está ordenado para iterar e garantir ordem no diálogo.
					return list.Layout(gtx, len(p.allRoles), func(gtx layout.Context, i int) layout.Dimensions {
						role := p.allRoles[i]
						chk, ok := p.userRoleDialogCheckboxes[role.Name]
						if !ok {
							return layout.Dimensions{}
						} // Não deveria acontecer

						// Lógica para impedir desmarcar o último admin (se aplicável)
						// if strings.ToLower(role.Name) == "admin" && p._isLastActiveAdmin(p.userForRoleDialog.ID) && !chk.Value {
						// 	// Impedir que desmarque ou mostrar aviso
						// }
						cb := material.CheckBox(th, chk, strings.Title(role.Name)) // Nome capitalizado para exibição
						cb.IconColor = theme.Colors.Primary                        // Cor do ícone do checkbox
						return layout.Inset{Bottom: theme.DefaultVSpacer}.Layout(gtx, cb.Layout)
					})
				}),
				// Mensagem de status do diálogo
				layout.Rigid(func(gtx C) D {
					if p.roleDialogStatusMessage != "" {
						lbl := material.Body2(th, p.roleDialogStatusMessage)
						lbl.Color = p.roleDialogMessageColor
						return layout.Inset{Top: theme.DefaultVSpacer, Bottom: theme.DefaultVSpacer}.Layout(gtx, lbl.Layout)
					}
					return layout.Dimensions{}
				}),
				// Botões Salvar/Cancelar do diálogo
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Spacing: layout.SpaceBetween}.Layout(gtx,
						layout.Flexed(1, material.Button(th, &p.userRoleDialogCancelBtn, "Cancelar").Layout),
						layout.Flexed(1, material.Button(th, &p.userRoleDialogSaveBtn, "Salvar Perfis").Layout),
					)
				}),
			)),
		)
	})
}

func (p *AdminPermissionsPage) handleSaveUserRoles() {
	if p.userForRoleDialog == nil || p.isLoading { // Previne múltiplas submissões
		return
	}

	p.isLoading = true // Indica que uma operação está em andamento (para o diálogo)
	p.roleDialogStatusMessage = "Salvando perfis..."
	p.roleDialogMessageColor = theme.Colors.TextMuted
	p.spinner.Start(p.router.GetAppWindow().Context()) // Pode usar um spinner local ao diálogo
	p.router.GetAppWindow().Invalidate()

	selectedRoleNames := []string{}
	for roleName, chk := range p.userRoleDialogCheckboxes {
		if chk.Value {
			selectedRoleNames = append(selectedRoleNames, roleName) // Usa o nome original (chave do mapa)
		}
	}

	// Regra de negócio: garantir que pelo menos um role seja atribuído?
	// Ou que o role "user" seja atribuído se nenhum outro for?
	// Se admin está editando, talvez permita remover todos.
	// Se for o último admin, não permitir remover o role "admin".
	if p._isLastActiveAdmin(p.userForRoleDialog.ID) {
		isAdminRoleStillSelected := false
		for _, rn := range selectedRoleNames {
			if strings.ToLower(rn) == "admin" {
				isAdminRoleStillSelected = true
				break
			}
		}
		if !isAdminRoleStillSelected {
			p.isLoading = false
			p.spinner.Stop(p.router.GetAppWindow().Context())
			p.roleDialogStatusMessage = "Não é possível remover o perfil 'admin' do último administrador ativo."
			p.roleDialogMessageColor = theme.Colors.Danger
			p.router.GetAppWindow().Invalidate()
			return
		}
	}

	go func(userID uuid.UUID, rolesToSet []string, usernameToLog string) {
		var opErr error
		currentAdminSession, _ := p.sessionManager.GetCurrentSession() // Obter sessão atual para o serviço

		// O serviço espera `models.UserUpdate`
		updatePayload := models.UserUpdate{RoleNames: &rolesToSet}
		_, err := p.userService.UpdateUser(userID, updatePayload, currentAdminSession)
		if err != nil {
			opErr = err
		}

		p.router.GetAppWindow().Execute(func() {
			p.isLoading = false
			p.spinner.Stop(p.router.GetAppWindow().Context())
			if opErr != nil {
				p.roleDialogStatusMessage = fmt.Sprintf("Erro: %v", opErr)
				p.roleDialogMessageColor = theme.Colors.Danger
				appLogger.Errorf("Erro ao salvar roles para %s: %v", usernameToLog, opErr)
				// Não fecha o diálogo em caso de erro para o usuário corrigir.
			} else {
				p.showUserRoleDialog = false // Fecha o diálogo em caso de sucesso.
				p.statusMessage = fmt.Sprintf("Perfis de '%s' atualizados com sucesso!", usernameToLog)
				p.messageColor = theme.Colors.Success
				appLogger.Infof("Perfis de '%s' atualizados para: %v", usernameToLog, rolesToSet)
				p.loadInitialData(currentAdminSession) // Recarrega toda a lista de usuários para refletir mudanças.
				p.selectedUserID = nil                 // Limpa seleção após salvar.
				p.updateActionButtonsState()
			}
			p.router.GetAppWindow().Invalidate()
		})
	}(p.userForRoleDialog.ID, selectedRoleNames, p.userForRoleDialog.Username)
}

// --- Lógica para Desativar Usuário ---
func (p *AdminPermissionsPage) handleDeactivateUser() {
	if p.selectedUserID == nil || p.isLoading {
		return
	}

	var userToDeactivate *models.UserPublic
	for _, u := range p.users { // Busca na lista completa
		if u.ID == *p.selectedUserID {
			userToDeactivate = u
			break
		}
	}
	if userToDeactivate == nil {
		p.statusMessage = "Usuário selecionado não encontrado para desativação."
		p.messageColor = theme.Colors.Danger
		p.updateActionButtonsState()
		p.router.GetAppWindow().Invalidate()
		return
	}
	// O botão já deve estar desabilitado se o usuário já estiver inativo.
	// `updateActionButtonsState` deve cuidar disso.

	// Simular um diálogo de confirmação (idealmente, usar um widget de diálogo real)
	// if !p.router.GetAppWindow().ConfirmDialog("Desativar Usuário", fmt.Sprintf("Tem certeza que deseja desativar %s?", userToDeactivate.Username)) {
	// 	return
	// }

	p.isLoading = true
	p.statusMessage = fmt.Sprintf("Desativando usuário '%s'...", userToDeactivate.Username)
	p.messageColor = theme.Colors.TextMuted
	p.spinner.Start(p.router.GetAppWindow().Context())
	p.router.GetAppWindow().Invalidate()

	go func(userID uuid.UUID, usernameToLog string) {
		var opErr error
		currentAdminSession, _ := p.sessionManager.GetCurrentSession()
		err := p.userService.DeactivateUser(userID, currentAdminSession)
		if err != nil {
			opErr = err
		}

		p.router.GetAppWindow().Execute(func() {
			p.isLoading = false
			p.spinner.Stop(p.router.GetAppWindow().Context())
			if opErr != nil {
				p.statusMessage = fmt.Sprintf("Erro ao desativar '%s': %v", usernameToLog, opErr)
				p.messageColor = theme.Colors.Danger
				appLogger.Errorf("Erro ao desativar usuário %s: %v", usernameToLog, opErr)
			} else {
				p.statusMessage = fmt.Sprintf("Usuário '%s' desativado com sucesso!", usernameToLog)
				p.messageColor = theme.Colors.Success
				appLogger.Infof("Usuário '%s' desativado.", usernameToLog)
				p.loadInitialData(currentAdminSession) // Recarrega para refletir status
			}
			p.selectedUserID = nil // Limpa seleção
			p.updateActionButtonsState()
			p.router.GetAppWindow().Invalidate()
		})
	}(*p.selectedUserID, userToDeactivate.Username)
}

// --- Lógica para Desbloquear Usuário ---
func (p *AdminPermissionsPage) handleUnlockUser() {
	if p.selectedUserID == nil || p.isLoading {
		return
	}
	// Lógica similar a handleDeactivateUser: buscar usuário, confirmar, chamar serviço, atualizar UI.
	// userService.UnlockUser(userID, currentUserSession)
	// Precisa verificar se o usuário está realmente bloqueado antes, ou deixar o serviço tratar.
	p.statusMessage = "Funcionalidade 'Desbloquear Usuário' ainda não implementada."
	p.messageColor = theme.Colors.Warning
	p.router.GetAppWindow().Invalidate()
}

// --- Lógica para Resetar Senha (Admin) ---
func (p *AdminPermissionsPage) handleAdminResetPassword() {
	if p.selectedUserID == nil || p.isLoading {
		return
	}
	// Lógica similar:
	// 1. Pedir nova senha (talvez em um diálogo).
	// 2. Validar nova senha.
	// 3. Chamar userService.AdminResetPassword(userID, newPassword, currentUserSession)
	// 4. Atualizar UI.
	p.statusMessage = "Funcionalidade 'Resetar Senha (Admin)' ainda não implementada."
	p.messageColor = theme.Colors.Warning
	p.router.GetAppWindow().Invalidate()
}

// --- Helpers ---
// `boolToString` e `formatOptionalTime` podem ser movidos para um pacote `ui/utils` se usados em múltiplas páginas.

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
	// Formato local para exibição, pode ser configurável.
	return t.Local().Format("02/01/06 15:04") // Formato mais curto
}
