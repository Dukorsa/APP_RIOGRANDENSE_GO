package ui

import (
	"fmt"

	"gioui.org/layout"
	"gioui.org/widget/material"

	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/auth"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/services"
)

// PageID define um identificador único para cada página/view da aplicação.
type PageID int

// Definição das PageIDs
const (
	PageNone PageID = iota // Um valor zero para indicar nenhuma página específica
	PageLogin
	PageRegistration
	PageForgotPassword
	PageMain // O layout principal que contém sidebar e área de conteúdo para módulos

	// Módulos que podem ser exibidos dentro da área de conteúdo de PageMain
	// ou que podem ser "diálogos" sobrepostos.
	// Se forem layouts dentro de PageMain, MainAppLayout os gerenciará.
	// Se forem páginas de nível superior acessadas via AppWindow.router diretamente,
	// elas são listadas aqui.
	PageNetworks         // Exemplo, se for um layout gerenciado por MainAppLayout, pode não ser necessário aqui.
	PageCNPJ             // Exemplo, se for um layout gerenciado por MainAppLayout
	PageAdminPermissions // Usado por MainAppLayout
	PageRoleManagement   // Usado por MainAppLayout ou AdminPermissionsPage
	PageImport           // Usado por MainAppLayout
	// Adicione outros IDs de página/módulo conforme necessário
)

// Page define a interface que cada página da aplicação deve implementar.
type Page interface {
	Layout(gtx layout.Context) layout.Dimensions
	// OnNavigatedTo é chamado quando a página se torna a página ativa.
	// `params` pode ser usado para passar dados para a página durante a navegação.
	OnNavigatedTo(params interface{})
	// OnNavigatedFrom é chamado quando o router está prestes a navegar para fora desta página.
	OnNavigatedFrom()
	// ID retorna o PageID único desta página (opcional, mas pode ser útil).
	// ID() PageID
}

// Router gerencia a navegação entre as diferentes páginas da aplicação.
type Router struct {
	th        *material.Theme
	cfg       *core.Config
	appWindow *AppWindow // Referência à janela principal para callbacks e acesso a serviços

	pages             map[PageID]Page
	currentPageID     PageID
	previousPageID    PageID // Para funcionalidade de "voltar" simples
	currentPageParams interface{}

	// Serviços (para facilitar o acesso pelas páginas através do router,
	// ou as páginas podem obtê-los diretamente da AppWindow via GetAppWindow())
	userService    services.UserService
	roleService    services.RoleService
	networkService services.NetworkService
	cnpjService    services.CNPJService
	importService  services.ImportService
	auditService   services.AuditLogService
	authenticator  auth.AuthenticatorInterface
	sessionManager *auth.SessionManager
	permManager    *auth.PermissionManager
}

// NewRouter cria uma nova instância do Router.
func NewRouter(
	th *material.Theme,
	cfg *core.Config,
	aw *AppWindow, // Passa a AppWindow para que o router possa interagir com ela
	userSvc services.UserService,
	roleSvc services.RoleService,
	netSvc services.NetworkService,
	cnpjSvc services.CNPJService,
	importSvc services.ImportService,
	auditSvc services.AuditLogService,
	authN auth.AuthenticatorInterface,
	sessMan *auth.SessionManager,
	permMan *auth.PermissionManager,
) *Router {
	if th == nil || cfg == nil || aw == nil || userSvc == nil || roleSvc == nil || netSvc == nil || cnpjSvc == nil || importSvc == nil || auditSvc == nil || authN == nil || sessMan == nil || permMan == nil {
		appLogger.Fatalf("Dependências nulas fornecidas para NewRouter")
	}
	return &Router{
		th:            th,
		cfg:           cfg,
		appWindow:     aw,
		pages:         make(map[PageID]Page),
		currentPageID: PageNone, // Inicia sem página definida, AppWindow definirá a inicial
		// Inicializar serviços
		userService:    userSvc,
		roleService:    roleSvc,
		networkService: netSvc,
		cnpjService:    cnpjSvc,
		importService:  importSvc,
		auditService:   auditSvc,
		authenticator:  authN,
		sessionManager: sessMan,
		permManager:    permMan,
	}
}

// Register associa um PageID a uma instância de Page.
func (r *Router) Register(id PageID, page Page) {
	if page == nil {
		appLogger.Warnf("Tentativa de registrar uma página nula para ID: %v", id)
		return
	}
	if r.pages == nil {
		r.pages = make(map[PageID]Page)
	}
	if _, exists := r.pages[id]; exists {
		appLogger.Warnf("Substituindo página já registrada para ID: %v", id)
	}
	r.pages[id] = page
	appLogger.Debugf("Página registrada: ID=%v, Tipo=%T", id, page)
}

// NavigateTo muda a página ativa.
func (r *Router) NavigateTo(id PageID, params interface{}) {
	if r.currentPageID == id && r.currentPageParams == params { // Evita navegação redundante se já estiver na página com os mesmos params
		// appLogger.Debugf("Já na página %v com os mesmos parâmetros. Navegação ignorada.", id)
		// return // Descomentar se quiser esse comportamento
	}

	appLogger.Infof("Navegando de %v para %v com params: %T %v", r.currentPageID, id, params, params)

	// Notifica a página antiga (se houver e for válida)
	if oldPage, exists := r.pages[r.currentPageID]; exists && oldPage != nil {
		oldPage.OnNavigatedFrom()
	}

	r.previousPageID = r.currentPageID // Guarda a página anterior
	r.currentPageID = id
	r.currentPageParams = params

	// Notifica a nova página
	if newPage, exists := r.pages[id]; exists && newPage != nil {
		newPage.OnNavigatedTo(params)
	} else {
		appLogger.Errorf("Tentativa de navegar para página não registrada ou nula: ID=%v", id)
		// Poderia navegar para uma página de erro ou voltar para a anterior
		// Se voltar, cuidado com loops.
		// r.currentPageID = r.previousPageID // Volta
		// r.previousPageID = PageNone
	}

	r.appWindow.Invalidate() // Força o redesenho da AppWindow
}

// NavigateBack navega para a página anterior no histórico simples.
// Retorna true se conseguiu voltar, false caso contrário (ex: não há página anterior).
func (r *Router) NavigateBack(params interface{}) bool {
	if r.previousPageID != PageNone {
		appLogger.Infof("Navegando de volta para página anterior: %v", r.previousPageID)
		// params para NavigateBack pode ser útil para passar um resultado da página "modal" fechada
		r.NavigateTo(r.previousPageID, params)
		r.previousPageID = PageNone // Limpa para evitar voltas múltiplas acidentais para o mesmo lugar
		return true
	}
	appLogger.Warn("Nenhuma página anterior para navegar de volta.")
	return false
}

// Layout renderiza a página ativa atual.
func (r *Router) Layout(gtx layout.Context) layout.Dimensions {
	currentPageLayout, exists := r.pages[r.currentPageID]
	if !exists || currentPageLayout == nil {
		errorMsg := fmt.Sprintf("Erro: Página ID %v não encontrada ou não inicializada no router.", r.currentPageID)
		appLogger.Error(errorMsg)
		// Layout de fallback/erro
		return layout.Center.Layout(gtx, material.Body1(r.th, errorMsg).Layout)
	}
	return currentPageLayout.Layout(gtx)
}

// CurrentPageID retorna o ID da página ativa.
func (r *Router) CurrentPageID() PageID {
	return r.currentPageID
}

// PreviousPageID retorna o ID da página anterior (se houver).
func (r *Router) PreviousPageID() PageID {
	return r.previousPageID
}

// GetAppWindow retorna a instância da AppWindow.
// As páginas podem usar isso para chamar métodos globais da AppWindow (ex: ShowGlobalMessage, Invalidate).
func (r *Router) GetAppWindow() *AppWindow {
	return r.appWindow
}

// GetTheme retorna o tema da aplicação.
func (r *Router) GetTheme() *material.Theme {
	return r.th
}

// GetConfig retorna as configurações da aplicação.
func (r *Router) GetConfig() *core.Config {
	return r.cfg
}

// --- Métodos para acesso aos serviços (as páginas podem chamar isso) ---

func (r *Router) UserService() services.UserService          { return r.userService }
func (r *Router) RoleService() services.RoleService          { return r.roleService }
func (r *Router) NetworkService() services.NetworkService    { return r.networkService }
func (r *Router) CNPJService() services.CNPJService          { return r.cnpjService }
func (r *Router) ImportService() services.ImportService      { return r.importService }
func (r *Router) AuditLogService() services.AuditLogService  { return r.auditService }
func (r *Router) Authenticator() auth.AuthenticatorInterface { return r.authenticator }
func (r *Router) SessionManager() *auth.SessionManager       { return r.sessionManager }
func (r *Router) PermissionManager() *auth.PermissionManager { return r.permManager }
