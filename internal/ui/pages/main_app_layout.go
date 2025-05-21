package pages

import (
	// "fmt"
	"fmt"
	"image"
	"image/color"
	"strings"
	// "strings"
	// "time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/seu_usuario/riograndense_gio/internal/auth"
	"github.com/seu_usuario/riograndense_gio/internal/core"
	appLogger "github.com/seu_usuario/riograndense_gio/internal/core/logger"

	// "github.com/seu_usuario/riograndense_gio/internal/data/models"
	"github.com/seu_usuario/riograndense_gio/internal/services"
	"github.com/seu_usuario/riograndense_gio/internal/ui"       // Para Router e PageID
	"github.com/seu_usuario/riograndense_gio/internal/ui/theme" // Para Cores
	// "github.com/seu_usuario/riograndense_gio/internal/ui/icons" // Para Ícones
)

// ModuleConfig define a configuração para cada módulo na sidebar.
type ModuleConfig struct {
	ID                 ui.PageID       // ID da página/módulo para navegação no router principal
	Title              string          // Título exibido na sidebar
	Icon               *widget.Icon    // TODO: Usar seu sistema de ícones SVG/PNG
	RequiredPermission auth.Permission // Permissão para ver/acessar este módulo
	// ActionOnClick func() // Se for um diálogo direto em vez de navegar para uma página
}

// MainAppLayout é a página principal da aplicação após o login.
type MainAppLayout struct {
	router *ui.Router
	cfg    *core.Config
	// Serviços necessários para as sub-páginas ou para a própria MainAppLayout
	userService    services.UserService
	roleService    services.RoleService
	networkService services.NetworkService
	cnpjService    services.CNPJService
	importService  services.ImportService
	permManager    *auth.PermissionManager
	sessionManager *auth.SessionManager

	// Estado da UI
	currentModuleID ui.PageID          // Qual módulo está ativo na área de conteúdo
	sidebarModules  []ModuleConfig     // Configuração dos módulos da sidebar
	sidebarClicks   []widget.Clickable // Para os botões da sidebar
	logoutBtn       widget.Clickable

	// Sub-páginas/layouts para cada módulo
	// Estes serão instanciados conforme necessário ou na inicialização.
	// Eles implementam a interface ui.Page
	modulePages map[ui.PageID]ui.Page

	// Para diálogo de confirmação de logout (simulado)
	showLogoutConfirm bool
	logoutYesBtn      widget.Clickable
	logoutNoBtn       widget.Clickable

	currentSession *auth.SessionData // Armazena a sessão do usuário logado
}

// NewMainAppLayout cria uma nova instância do layout principal da aplicação.
func NewMainAppLayout(
	router *ui.Router,
	cfg *core.Config,
	userSvc services.UserService,
	roleSvc services.RoleService,
	netSvc services.NetworkService,
	cnpjSvc services.CNPJService,
	importSvc services.ImportService,
	permMan *auth.PermissionManager,
	sessMan *auth.SessionManager,
) *MainAppLayout {
	ml := &MainAppLayout{
		router:         router,
		cfg:            cfg,
		userService:    userSvc,
		roleService:    roleSvc,
		networkService: netSvc,
		cnpjService:    cnpjSvc,
		importService:  importSvc,
		permManager:    permMan,
		sessionManager: sessMan,
		modulePages:    make(map[ui.PageID]ui.Page),
	}
	// A primeira página a ser exibida pode ser definida aqui ou em OnNavigatedTo
	// ml.currentModuleID = ui.PageNetworks // Exemplo
	return ml
}

// loadSidebarModules configura os módulos da sidebar com base nas permissões do usuário.
func (ml *MainAppLayout) loadSidebarModules() {
	// Obter sessão atual para checagem de permissões
	session, err := ml.sessionManager.GetCurrentSession()
	if err != nil || session == nil {
		appLogger.Errorf("MainAppLayout: Sessão inválida ao carregar módulos da sidebar: %v", err)
		ml.router.NavigateTo(ui.PageLogin, "Sessão inválida ou expirada.") // Força logout
		return
	}
	ml.currentSession = session // Armazena a sessão atual

	allModules := []ModuleConfig{
		{ID: ui.PageNetworks, Title: "Redes", Icon: nil /* TODO */, RequiredPermission: auth.PermNetworkView},
		{ID: ui.PageCNPJ, Title: "CNPJs", Icon: nil /* TODO */, RequiredPermission: auth.PermCNPJView},
		{ID: ui.PageAdminPermissions, Title: "Usuários e Perfis", Icon: nil /* TODO */, RequiredPermission: auth.PermUserRead}, // Ou uma permissão mais genérica de admin
		{ID: ui.PageRoleManagement, Title: "Gerenciar Perfis", Icon: nil /* TODO */, RequiredPermission: auth.PermRoleManage},
		{ID: ui.PageImport, Title: "Importar Dados", Icon: nil /* TODO */, RequiredPermission: auth.PermImportExecute},
		// Adicionar outros módulos como Logs, etc.
	}

	ml.sidebarModules = []ModuleConfig{}
	for _, modCfg := range allModules {
		hasPerm, _ := ml.permManager.HasPermission(session, modCfg.RequiredPermission, nil)
		if hasPerm {
			ml.sidebarModules = append(ml.sidebarModules, modCfg)
		}
	}
	ml.sidebarClicks = make([]widget.Clickable, len(ml.sidebarModules))

	// Define o módulo inicial se nenhum estiver definido ou se o atual não for mais acessível
	if ml.currentModuleID == 0 || !ml.isModuleAccessible(ml.currentModuleID, session) {
		if len(ml.sidebarModules) > 0 {
			ml.currentModuleID = ml.sidebarModules[0].ID
		} else {
			// Nenhum módulo acessível, o que é estranho se o login foi bem-sucedido
			appLogger.Error("Nenhum módulo acessível para o usuário na MainAppLayout.")
			// Poderia mostrar uma página de "sem acesso" ou forçar logout
			ml.currentModuleID = ui.PageLogin // Fallback para login
		}
	}
}

// isModuleAccessible verifica se um módulo específico está na lista de módulos acessíveis.
func (ml *MainAppLayout) isModuleAccessible(moduleID ui.PageID, session *auth.SessionData) bool {
	for _, modCfg := range ml.sidebarModules { // sidebarModules já é filtrado por permissão
		if modCfg.ID == moduleID {
			return true
		}
	}
	return false
}

func (ml *MainAppLayout) OnNavigatedTo(params interface{}) {
	appLogger.Info("Navegou para MainAppLayout")
	ml.loadSidebarModules() // Carrega/recarrega módulos da sidebar com base nas permissões

	// Notifica a sub-página atual que ela se tornou ativa
	if page, ok := ml.modulePages[ml.currentModuleID]; ok && page != nil {
		page.OnNavigatedTo(nil) // Ou passar params se MainAppLayout receber params
	}
	ml.showLogoutConfirm = false
}

func (ml *MainAppLayout) OnNavigatedFrom() {
	appLogger.Info("Navegando para fora da MainAppLayout")
	if page, ok := ml.modulePages[ml.currentModuleID]; ok && page != nil {
		page.OnNavigatedFrom()
	}
	ml.currentSession = nil // Limpa sessão ao sair
}

// getModulePage retorna (e instancia se necessário) a página para um ID de módulo.
func (ml *MainAppLayout) getModulePage(moduleID ui.PageID) ui.Page {
	if page, ok := ml.modulePages[moduleID]; ok && page != nil {
		return page
	}

	var newPage ui.Page
	// Instanciar a página correta com base no moduleID
	// Isso pode ser movido para o NewMainAppLayout para instanciar todas as páginas upfront
	// ou instanciar sob demanda como aqui.
	switch moduleID {
	case ui.PageNetworks:
		// newPage = NewNetworksPage(ml.router, ml.cfg, ml.networkService, ...)
		appLogger.Warnf("Página para módulo Networks não implementada ainda.")
	case ui.PageCNPJ:
		newPage = NewCNPJPage(ml.router, ml.cfg, ml.cnpjService, ml.networkService, ml.permManager, ml.sessionManager)
	case ui.PageAdminPermissions:
		newPage = NewAdminPermissionsPage(ml.router, ml.cfg, ml.userService, ml.roleService, ml.auditService, ml.permManager, ml.sessionManager)
	case ui.PageRoleManagement:
		newPage = NewRoleManagementPage(ml.router, ml.cfg, ml.roleService, ml.auditService, ml.permManager, ml.sessionManager)
	case ui.PageImport:
		newPage = NewImportPage(ml.router, ml.cfg, ml.importService, ml.permManager, ml.sessionManager)
	default:
		appLogger.Errorf("Tentativa de obter página para módulo desconhecido: %v", moduleID)
		return &PlaceholderPage{Title: fmt.Sprintf("Módulo %v não encontrado", moduleID)}
	}

	if newPage != nil {
		ml.modulePages[moduleID] = newPage
		newPage.OnNavigatedTo(nil) // Notifica a nova página que ela está ativa
	}
	return newPage
}

func (ml *MainAppLayout) Layout(gtx layout.Context) layout.Dimensions {
	th := ml.router.GetAppWindow().Theme()

	// Processar cliques da sidebar
	for i := range ml.sidebarModules {
		if ml.sidebarClicks[i].Clicked(gtx) {
			selectedModuleID := ml.sidebarModules[i].ID
			if ml.currentModuleID != selectedModuleID {
				// Notifica a página antiga
				if oldPage, ok := ml.modulePages[ml.currentModuleID]; ok && oldPage != nil {
					oldPage.OnNavigatedFrom()
				}
				ml.currentModuleID = selectedModuleID
				// `getModulePage` chamará OnNavigatedTo na nova página
				ml.getModulePage(selectedModuleID)
				appLogger.Infof("Módulo alterado para: %s", ml.sidebarModules[i].Title)
			}
		}
	}
	if ml.logoutBtn.Clicked(gtx) {
		ml.showLogoutConfirm = true
	}

	// Diálogo de confirmação de Logout
	if ml.showLogoutConfirm {
		if ml.logoutYesBtn.Clicked(gtx) {
			ml.showLogoutConfirm = false
			ml.router.GetAppWindow().HandleLogout() // Chama o logout na AppWindow
		}
		if ml.logoutNoBtn.Clicked(gtx) {
			ml.showLogoutConfirm = false
		}
		// Desenha o diálogo de logout sobreposto
		return ml.layoutLogoutConfirmDialog(gtx, th)
	}

	// Layout principal: Sidebar | Área de Conteúdo
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		// Sidebar
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ml.layoutSidebar(gtx, th)
		}),
		// Área de Conteúdo
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(1)}.Layout(gtx, // Pequena linha divisória visual
				func(gtx C) D {
					paint.FillShape(gtx.Ops, theme.Colors.Border, clip.Rect{Max: image.Pt(gtx.Dp(1), gtx.Constraints.Max.Y)}.Op())
					return layout.Inset{Left: unit.Dp(1)}.Layout(gtx, // Conteúdo real após a linha
						func(gtx layout.Context) layout.Dimensions {
							currentPage := ml.getModulePage(ml.currentModuleID)
							if currentPage == nil {
								return material.Body1(th, fmt.Sprintf("Erro: Módulo %v não pôde ser carregado.", ml.currentModuleID)).Layout(gtx)
							}
							// Adiciona um padding padrão à área de conteúdo
							return layout.UniformInset(unit.Dp(16)).Layout(gtx, currentPage.Layout)
						})
				})
		}),
	)
}

func (ml *MainAppLayout) layoutSidebar(gtx layout.Context, th *material.Theme) layout.Dimensions {
	sidebarWidth := gtx.Dp(unit.Dp(220)) // Largura da sidebar
	gtx.Constraints.Min.X = sidebarWidth
	gtx.Constraints.Max.X = sidebarWidth

	return layout.Background{Color: theme.Colors.Grey50}.Layout(gtx, // Cor de fundo da sidebar
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(10), Right: unit.Dp(5), Bottom: unit.Dp(10), Left: unit.Dp(5)}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceEnd}.Layout(gtx,
						// Informações do Usuário
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if ml.currentSession == nil {
								return layout.Dimensions{}
							}
							// TODO: Card ou GroupBox para informações do usuário
							rolesStr := "N/A"
							if len(ml.currentSession.Roles) > 0 {
								rolesStr = strings.Join(ml.currentSession.Roles, ", ")
							}
							return layout.UniformInset(unit.Dp(8)).Layout(gtx,
								layout.Flex{Axis: layout.Vertical, Spacing: layout.Tight}.Layout(gtx,
									layout.Rigid(material.Body1(th, "Usuário Logado:").Layout),
									layout.Rigid(material.Body2(th, ml.currentSession.Username).Layout),
									layout.Rigid(material.Body2(th, fmt.Sprintf("Perfis: %s", rolesStr)).Layout),
									layout.Rigid(layout.Spacer{Height: theme.DefaultVSpacer}.Layout),
									layout.Rigid(material.όταν(th, unit.Dp(1)).Layout), // Separador
									layout.Rigid(layout.Spacer{Height: theme.DefaultVSpacer}.Layout),
								),
							)
						}),
						// Lista de Módulos
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							list := layout.List{Axis: layout.Vertical}
							return list.Layout(gtx, len(ml.sidebarModules), func(gtx layout.Context, index int) layout.Dimensions {
								modCfg := ml.sidebarModules[index]
								btn := material.Button(th, &ml.sidebarClicks[index], modCfg.Title)
								// Estilo de botão de sidebar (plano, muda cor no hover/selecionado)
								btn.Background = color.NRGBA{} // Transparente
								btn.Color = theme.Colors.Text
								if ml.currentModuleID == modCfg.ID {
									btn.Background = theme.Colors.PrimaryLight
									btn.Color = theme.Colors.PrimaryText
									btn.Font.Weight = font.Bold
								}
								// TODO: Adicionar ícones aos botões
								return layout.Inset{Bottom: unit.Dp(4)}.Layout(gtx, btn.Layout)
							})
						}),
						// Botão Logout
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							logoutButton := material.Button(th, &ml.logoutBtn, " Sair / Logout")
							// TODO: Adicionar ícone de logout
							// TODO: Estilo para logout (talvez cor de perigo leve)
							return logoutButton.Layout(gtx)
						}),
					)
				})
		})
}

// layoutLogoutConfirmDialog desenha um diálogo de confirmação para logout.
func (ml *MainAppLayout) layoutLogoutConfirmDialog(gtx layout.Context, th *material.Theme) layout.Dimensions {
	// Desenha um fundo semi-transparente para o efeito modal
	paint.FillShape(gtx.Ops, color.NRGBA{A: 180}, clip.Rect{Max: gtx.Constraints.Max}.Op())

	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return material.Dialog(th, "Confirmar Saída").Layout(gtx,
			material.Inset(unit.Dp(16), layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(material.Body1(th, "Tem certeza que deseja encerrar a sessão?").Layout),
				layout.Rigid(layout.Spacer{Height: theme.LargeVSpacer}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Spacing: layout.SpaceBetween}.Layout(gtx,
						layout.Flexed(1, material.Button(th, &ml.logoutNoBtn, "Não").Layout),
						layout.Flexed(1, material.Button(th, &ml.logoutYesBtn, "Sim, Sair").Layout),
					)
				}),
			)),
		)
	})
}

// PlaceholderPage é uma página simples para módulos não implementados.
type PlaceholderPage struct {
	Title string
}

func (p *PlaceholderPage) Layout(gtx layout.Context) layout.Dimensions {
	th := material.NewTheme() // Pode pegar do router se precisar
	return layout.Center.Layout(gtx, material.H6(th, fmt.Sprintf("Conteúdo para: %s (Em Desenvolvimento)", p.Title)).Layout)
}
func (p *PlaceholderPage) OnNavigatedTo(params interface{}) {
	appLogger.Debugf("PlaceholderPage '%s' ativada", p.Title)
}
func (p *PlaceholderPage) OnNavigatedFrom() {
	appLogger.Debugf("PlaceholderPage '%s' desativada", p.Title)
}
