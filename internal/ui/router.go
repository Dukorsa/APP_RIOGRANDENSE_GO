package ui

import (
	"fmt"

	"gioui.org/layout"
	"gioui.org/widget/material"

	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/auth"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/config"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/services"
)

// PageID define um identificador único para cada página/view da aplicação.
// Usado pelo Router para gerenciar a navegação.
type PageID int

// Definição das PageIDs para todas as páginas de nível superior ou módulos principais.
const (
	PageNone PageID = iota // Valor zero para indicar nenhuma página específica ou estado inicial.

	// Páginas de Autenticação e Gerenciamento de Conta
	PageLogin
	PageRegistration
	PageForgotPassword

	// Layout Principal da Aplicação (contém a navegação para os módulos)
	PageMain

	// Módulos principais que são exibidos dentro da área de conteúdo de `PageMain`.
	// Estes são tratados como "sub-páginas" ou "views de módulo" gerenciadas por `MainAppLayout`.
	// O `Router` principal pode não navegar diretamente para eles se `MainAppLayout` tiver seu próprio
	// mecanismo de navegação interna (ex: abas, sub-roteador).
	// No entanto, listá-los aqui pode ser útil para referência ou se houver navegação direta em alguns casos.
	PageNetworks         // Módulo de Gerenciamento de Redes.
	PageCNPJ             // Módulo de Gerenciamento de CNPJs.
	PageAdminPermissions // Módulo de Gerenciamento de Usuários e Permissões de Admin.
	PageRoleManagement   // Módulo de Gerenciamento de Perfis (Roles).
	PageImport           // Módulo de Importação de Dados.
	// PageAuditLogs     // Exemplo: Módulo para visualização de Logs de Auditoria.
)

// Page define a interface que cada página/view da aplicação deve implementar.
type Page interface {
	// Layout desenha o conteúdo da página no contexto gráfico fornecido.
	Layout(gtx layout.Context) layout.Dimensions

	// OnNavigatedTo é chamado quando a página se torna a página ativa (visível).
	// `params` pode ser usado para passar dados para a página durante a navegação
	// (ex: dados do usuário ao logar, ID de um item para editar).
	OnNavigatedTo(params interface{})

	// OnNavigatedFrom é chamado quando o router está prestes a navegar para fora desta página.
	// Útil para limpar estado, parar animações, salvar dados não salvos, etc.
	OnNavigatedFrom()

	// ID (Opcional) retorna o PageID único desta página.
	// Pode ser útil para depuração ou para o router verificar o tipo da página.
	// ID() PageID
}

// Router gerencia a navegação entre as diferentes páginas da aplicação.
type Router struct {
	th        *material.Theme // Tema global da aplicação.
	cfg       *core.Config    // Configurações globais.
	appWindow *AppWindow      // Referência à janela principal para callbacks (ex: Invalidate, ShowMessage).

	pages          map[PageID]Page // Mapa de PageIDs para instâncias de Page registradas.
	currentPageID  PageID          // ID da página atualmente ativa.
	previousPageID PageID          // ID da página anterior (para funcionalidade de "voltar" simples).
	// currentPageParams interface{} // Parâmetros passados para a página atual (já tratados em OnNavigatedTo).

	// Serviços centralizados para acesso pelas páginas através do router,
	// ou as páginas podem obtê-los da AppWindow se o router não os expor.
	// Expor aqui pode simplificar a passagem de dependências para as páginas.
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
// Recebe todas as dependências necessárias para ele e para as páginas que gerencia.
func NewRouter(
	th *material.Theme,
	cfg *core.Config,
	aw *AppWindow, // Referência à AppWindow é crucial.
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
	// Validação de dependências críticas.
	if th == nil || cfg == nil || aw == nil || userSvc == nil || roleSvc == nil ||
		netSvc == nil || cnpjSvc == nil || importSvc == nil || auditSvc == nil ||
		authN == nil || sessMan == nil || permMan == nil {
		appLogger.Fatalf("Dependências nulas fornecidas ao criar NewRouter. Verifique a inicialização.")
	}

	return &Router{
		th:             th,
		cfg:            cfg,
		appWindow:      aw,
		pages:          make(map[PageID]Page),
		currentPageID:  PageNone, // Inicia sem página definida; AppWindow definirá a página inicial.
		previousPageID: PageNone,

		// Atribui os serviços.
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
// É chamado pela AppWindow ao inicializar as páginas.
func (r *Router) Register(id PageID, page Page) {
	if page == nil {
		appLogger.Warnf("Router: Tentativa de registrar uma página nula para PageID: %v. Ignorando.", id)
		return
	}
	if r.pages == nil { // Segurança, embora NewRouter já inicialize.
		r.pages = make(map[PageID]Page)
	}
	if _, exists := r.pages[id]; exists {
		appLogger.Warnf("Router: Substituindo página já registrada para PageID: %v (Tipo anterior: %T, Novo tipo: %T).", id, r.pages[id], page)
	}
	r.pages[id] = page
	appLogger.Debugf("Router: Página registrada - ID=%v, Tipo=%T", id, page)
}

// NavigateTo muda a página ativa para a página com o `id` fornecido.
// `params` são os dados passados para o método `OnNavigatedTo` da nova página.
func (r *Router) NavigateTo(id PageID, params interface{}) {
	// Evita navegação redundante se já estiver na página e os parâmetros forem os mesmos.
	// A comparação de `params` pode ser complexa se forem structs ou slices.
	// Por simplicidade, apenas verifica o ID. Se a lógica de `OnNavigatedTo` for idempotente,
	// ou se a página lida com atualização baseada em params, a navegação pode ser permitida.
	// if r.currentPageID == id /* && r.currentPageParams == params */ {
	// 	// appLogger.Debugf("Router: Já na página %v. Navegação ignorada ou OnNavigatedTo será chamada novamente.", id)
	// 	// return // Descomentar para evitar navegação redundante.
	// }

	logParamsType := "nil"
	if params != nil {
		logParamsType = fmt.Sprintf("%T", params)
	}
	appLogger.Infof("Router: Navegando de PageID %v para PageID %v (Params: %s).", r.currentPageID, id, logParamsType)

	// Notifica a página antiga (se houver e for válida) que a navegação está saindo dela.
	if oldPage, exists := r.pages[r.currentPageID]; exists && oldPage != nil {
		oldPage.OnNavigatedFrom()
	}

	r.previousPageID = r.currentPageID // Guarda a página anterior para possível "voltar".
	r.currentPageID = id
	// r.currentPageParams = params // Não é mais necessário armazenar aqui, é passado para OnNavigatedTo.

	// Notifica a nova página que ela se tornou ativa e passa os parâmetros.
	if newPage, exists := r.pages[id]; exists && newPage != nil {
		newPage.OnNavigatedTo(params)
	} else {
		appLogger.Errorf("Router: Tentativa de navegar para PageID %v não registrada ou nula.", id)
		// Política de fallback: voltar para a página anterior ou para uma página de erro.
		// Evitar loops se a página anterior também for inválida.
		// Se r.previousPageID for válido e diferente de id:
		// if r.previousPageID != PageNone && r.previousPageID != id {
		// 	appLogger.Warnf("Router: Navegando de volta para a página anterior %v devido a erro.", r.previousPageID)
		// 	r.NavigateTo(r.previousPageID, nil) // Cuidado com recursão aqui.
		// } else {
		// 	// Navegar para uma página de erro padrão ou login.
		// 	r.NavigateTo(PageLogin, "Erro de navegação: página não encontrada.")
		// }
		// Por agora, apenas loga o erro. O Layout mostrará uma mensagem de erro.
	}

	r.appWindow.Invalidate() // Solicita um redesenho da AppWindow para exibir a nova página.
}

// NavigateBack navega para a página anterior no histórico simples.
// Retorna true se conseguiu voltar, false caso contrário (ex: não há página anterior).
// `params` podem ser usados para passar um resultado da página "modal" que foi fechada.
func (r *Router) NavigateBack(params interface{}) bool {
	if r.previousPageID != PageNone {
		appLogger.Infof("Router: Navegando de volta para a página anterior: %v.", r.previousPageID)
		pageToReturnTo := r.previousPageID
		// Limpa `previousPageID` antes de navegar para evitar que uma segunda chamada a `NavigateBack`
		// volte para a mesma `pageToReturnTo` se `previousPageID` não for resetado na página de destino.
		// No entanto, a lógica de `NavigateTo` já atualiza `previousPageID` para a página atual *antes* de navegar.
		// O `previousPageID` é mais para saber de onde veio, não para um histórico profundo.
		// Se uma página "A" navega para "B", previousPageID = A.
		// Se "B" chama NavigateBack, vai para "A". previousPageID agora é "B".
		// Se "A" chama NavigateBack, e não havia nada antes de "A", não volta.

		// Guarda o `previousPageID` atual para restaurar se a navegação para `pageToReturnTo` falhar.
		// currentPrevious := r.previousPageID // Não estritamente necessário com a lógica atual.

		r.NavigateTo(pageToReturnTo, params)
		// `previousPageID` já foi atualizado por `NavigateTo` para ser a página da qual estamos voltando.
		// Não precisamos resetá-lo para `PageNone` aqui, a menos que queiramos um "voltar" de um nível apenas.
		// Se quisermos um histórico mais simples (A -> B, B volta para A, e A não tem mais "B" como anterior):
		// r.previousPageID = PageNone // Descomentar para limpar a "página anterior" após voltar.
		return true
	}
	appLogger.Warn("Router: Nenhuma página anterior no histórico simples para navegar de volta.")
	return false
}

// Layout renderiza a página ativa atual.
// É chamado pelo método Layout da AppWindow.
func (r *Router) Layout(gtx layout.Context) layout.Dimensions {
	currentPageLayout, exists := r.pages[r.currentPageID]
	if !exists || currentPageLayout == nil {
		errorMsg := fmt.Sprintf("Erro crítico: Página para ID %v não encontrada ou não inicializada no router.", r.currentPageID)
		appLogger.Error(errorMsg)
		// Layout de fallback em caso de erro grave (página não registrada).
		return layout.Center.Layout(gtx, material.Body1(r.th, errorMsg).Layout)
	}
	// Desenha a página atual.
	return currentPageLayout.Layout(gtx)
}

// --- Getters para acesso a dependências globais (usados pelas páginas) ---

// CurrentPageID retorna o ID da página ativa.
func (r *Router) CurrentPageID() PageID { return r.currentPageID }

// PreviousPageID retorna o ID da página anterior no histórico simples.
func (r *Router) PreviousPageID() PageID { return r.previousPageID }

// GetAppWindow retorna a instância da AppWindow, permitindo que as páginas
// interajam com funcionalidades globais da janela (ex: Invalidate, ShowGlobalMessage).
func (r *Router) GetAppWindow() *AppWindow { return r.appWindow }

// GetTheme retorna o tema Material Design global da aplicação.
func (r *Router) GetTheme() *material.Theme { return r.th }

// GetConfig retorna as configurações globais da aplicação.
func (r *Router) GetConfig() *core.Config { return r.cfg }

// --- Getters para acesso aos serviços (as páginas podem chamar estes) ---
func (r *Router) UserService() services.UserService          { return r.userService }
func (r *Router) RoleService() services.RoleService          { return r.roleService }
func (r *Router) NetworkService() services.NetworkService    { return r.networkService }
func (r *Router) CNPJService() services.CNPJService          { return r.cnpjService }
func (r *Router) ImportService() services.ImportService      { return r.importService }
func (r *Router) AuditLogService() services.AuditLogService  { return r.auditService }
func (r *Router) Authenticator() auth.AuthenticatorInterface { return r.authenticator }
func (r *Router) SessionManager() *auth.SessionManager       { return r.sessionManager }
func (r *Router) PermissionManager() *auth.PermissionManager { return r.permManager }
