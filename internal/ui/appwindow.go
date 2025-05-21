package ui

import (

	// "log" // Usar appLogger
	// "os"  // Se precisar para os.Exit

	"fmt"
	"image"
	"strings"
	"time"

	"gioui.org/app"
	"gioui.org/font/gofont"
	"gioui.org/io/key"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/auth"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/models" // Para UserPublic em HandleLoginSuccess
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/services"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui/components" // Para LoadingSpinner
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui/pages"      // Para instanciar páginas
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui/theme"      // Para cores e tamanhos padrão
)

// AppWindow gerencia a janela principal da aplicação e o roteamento de páginas.
type AppWindow struct {
	window *app.Window
	th     *material.Theme
	cfg    *core.Config
	router *Router

	// Serviços que a AppWindow ou suas páginas podem precisar
	authenticator  auth.AuthenticatorInterface
	sessionManager *auth.SessionManager
	userService    services.UserService
	// ... outros serviços ...

	// Estado global da UI, se necessário
	globalSpinner   *components.LoadingSpinner
	isGlobalLoading bool

	// Para mensagens globais/notificações (simples)
	globalMessage     string
	globalMessageType string // "info", "error", "success"
	showGlobalMessage bool
	// TODO: Adicionar botões e lógica para fechar a mensagem global
}

// NewAppWindow cria e inicializa a janela principal da aplicação.
func NewAppWindow(
	th *material.Theme,
	cfg *core.Config,
	authN auth.AuthenticatorInterface,
	sessMan *auth.SessionManager,
	userSvc services.UserService,
	roleSvc services.RoleService,
	netSvc services.NetworkService,
	cnpjSvc services.CNPJService,
	importSvc services.ImportService,
	auditSvc services.AuditLogService,
) *AppWindow {
	gofont.Register() // Garante que as fontes padrão estejam registradas
	if th == nil {
		th = material.NewTheme()
	}

	win := app.NewWindow(
		app.Title(fmt.Sprintf("%s v%s", cfg.AppName, cfg.AppVersion)),
		app.Size(theme.WindowDefaultWidth, theme.WindowDefaultHeight),
		app.MinSize(theme.WindowMinWidth, theme.WindowMinHeight),
		// app.Decorated(false), // Se quiser janela sem decoração do OS (requer mais trabalho)
	)

	aw := &AppWindow{
		window: win,
		th:     th,
		cfg:    cfg,
		// Inicializar serviços
		authenticator:  authN,
		sessionManager: sessMan,
		userService:    userSvc,
		// ...
		globalSpinner: components.NewLoadingSpinner(theme.Colors.Primary), // Spinner global
	}

	// Inicializar o Router, passando aw (para callbacks) e os serviços necessários
	aw.router = NewRouter(th, cfg, aw, userSvc, roleSvc, netSvc, cnpjSvc, importSvc, auditSvc, authN, sessMan, auth.GetPermissionManager())

	// Registrar todas as páginas no router
	aw.router.Register(PageLogin, pages.NewLoginPage(aw.router, cfg, authN))                      // LoginPage recebe Authenticator
	aw.router.Register(PageRegistration, pages.NewRegistrationPage(aw.router, cfg, userSvc, nil)) // nil para adminSession inicialmente
	aw.router.Register(PageForgotPassword, pages.NewForgotPasswordPage(aw.router, cfg, userSvc))

	mainAppLayout := pages.NewMainAppLayout(aw.router, cfg, userSvc, roleSvc, netSvc, cnpjSvc, importSvc, auth.GetPermissionManager(), sessMan)
	aw.router.Register(PageMain, mainAppLayout)

	// As sub-páginas da MainAppLayout são instanciadas dentro dela ou pelo router quando necessário
	// Ex: CNPJPage, AdminPermissionsPage, RoleManagementPage, ImportPage
	// Elas são registradas no router da MainAppLayout ou navegadas diretamente pela MainAppLayout
	// mas o Router principal precisa saber quais são os "entry points" dos módulos.
	// Para simplificar, podemos registrar algumas delas aqui se forem navegáveis diretamente de algum lugar
	// ou se a MainAppLayout não tiver seu próprio sub-router.
	// Se MainAppLayout gerencia a navegação interna, então não precisa registrar aqui.
	// Assumindo que MainAppLayout gerencia seus "filhos":
	// - PageNetworks, PageCNPJTable (se forem layouts dentro de MainAppLayout)
	// - PageAdminPermissions, PageRoleManagement, PageImport (se forem layouts dentro de MainAppLayout)
	// Se forem janelas de diálogo separadas (menos comum em Gio puro), seriam estados dentro de MainAppLayout.

	// Verifica se há uma sessão ativa ao iniciar
	if currentSess, _ := sessMan.GetCurrentSession(); currentSess != nil {
		aw.router.NavigateTo(PageMain, currentSess.Username) // Ou passar o objeto UserPublic
	} else {
		aw.router.NavigateTo(PageLogin, nil)
	}

	return aw
}

// Run inicia o loop de eventos da janela da aplicação.
// Esta função bloqueará até que a janela seja fechada.
func (aw *AppWindow) Run() error {
	var ops op.Ops
	for {
		// Espera pelo próximo evento do sistema ou pela necessidade de redesenhar.
		evt := aw.window.Event()
		switch e := evt.(type) {
		case app.DestroyEvent:
			appLogger.Info("AppWindow: Evento Destroy recebido, encerrando aplicação.")
			// TODO: Chamar shutdown de outros componentes se necessário (ex: SessionManager já tem defer)
			return e.Err
		case app.FrameEvent:
			// gtx é o contexto para o frame atual.
			gtx := app.NewContext(&ops, e)

			// Layout da aplicação inteira
			aw.Layout(gtx)

			// Envia as operações de desenho para a janela.
			e.Frame(gtx.Ops)

		// TODO: Lidar com outros eventos como system.Clipboard eventful, etc.
		case key.Event:
			// Pode lidar com atalhos de teclado globais aqui
			// if e.Name == key.NameEscape { aw.window.Perform(system.ActionClose) }
		}
	}
}

// Layout é o método de desenho principal para a AppWindow.
func (aw *AppWindow) Layout(gtx layout.Context) layout.Dimensions {
	// Define uma cor de fundo padrão para a janela inteira
	paint.FillShape(gtx.Ops, theme.Colors.Background, clip.Rect{Max: gtx.Constraints.Max}.Op())

	// Layout em Stack para permitir sobreposições (spinner global, mensagens globais)
	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		// Camada principal: Conteúdo do Router
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			if aw.router != nil {
				return aw.router.Layout(gtx)
			}
			// Fallback se o router não estiver pronto (não deveria acontecer após NewAppWindow)
			return material.Body1(aw.th, "Erro: Router não inicializado.").Layout(gtx)
		}),

		// Camada de Mensagem Global (se showGlobalMessage for true)
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			if aw.showGlobalMessage {
				return aw.layoutGlobalMessage(gtx)
			}
			return layout.Dimensions{}
		}),

		// Camada de Spinner Global (se isGlobalLoading for true)
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			if aw.isGlobalLoading {
				// Centraliza o spinner
				return layout.Center.Layout(gtx, aw.globalSpinner.Layout)
			}
			return layout.Dimensions{}
		}),
	)
}

// layoutGlobalMessage desenha a mensagem global.
func (aw *AppWindow) layoutGlobalMessage(gtx layout.Context) layout.Dimensions {
	// TODO: Implementar um widget de mensagem global mais robusto (com botão de fechar, timeout)
	// Por agora, um label simples no topo ou centro.
	return layout.N.Layout(gtx, // Posiciona no topo (North)
		func(gtx C) D {
			bgColor := theme.Colors.InfoBackground // Cor de fundo baseada no tipo
			textColor := theme.Colors.InfoText
			switch aw.globalMessageType {
			case "error":
				bgColor = theme.Colors.DangerBackground
				textColor = theme.Colors.DangerText
			case "success":
				bgColor = theme.Colors.SuccessBackground
				textColor = theme.Colors.SuccessText
			}
			// Adicionar um botão para fechar a mensagem
			// Ou fazer desaparecer após um tempo

			return layout.Background{Color: bgColor}.Layout(gtx,
				func(gtx C) D {
					return layout.UniformInset(unit.Dp(10)).Layout(gtx,
						material.Body1(aw.th, aw.globalMessage).Layout, // Não suporta HTML
					)
				})
		})
}

// --- Métodos de Callback / Interação com Páginas ---

// Theme retorna o tema atual da aplicação.
func (aw *AppWindow) Theme() *material.Theme {
	return aw.th
}

// Context retorna um layout.Context válido (usado para iniciar animações no spinner, etc.)
// ATENÇÃO: Este método só deve ser chamado durante um evento de frame.
// Se chamado fora, o gtx pode não ser válido.
// É melhor que componentes como spinner recebam gtx em seus métodos Start/Stop.
func (aw *AppWindow) Context() layout.Context {
	// Esta é uma forma de obter um gtx "artificialmente", mas é arriscado.
	// A melhor maneira é passar gtx para os métodos que precisam dele.
	// var ops op.Ops
	// return layout.NewContext(&ops, system.FrameEvent{}) // Não recomendado para uso geral
	appLogger.Warn("AppWindow.Context() chamado - este método é para uso limitado e pode não funcionar fora de um FrameEvent.")
	// Para Start/Stop do spinner, é melhor passar o gtx do evento que disparou a ação.
	// Se for uma ação de goroutine, a atualização da UI deve ser via aw.Execute()
	// e o spinner pode ser atualizado lá.
	var ops op.Ops // Precisa de uma ops, mesmo que não seja usada diretamente aqui
	// Criar um FrameEvent mínimo. O tamanho não importa muito se for só para gtx.Dp()
	// A hora do frame importa para animações.
	dummyEvent := system.FrameEvent{
		Now:    time.Now(),
		Size:   image.Point{X: 800, Y: 600}, // Tamanho placeholder
		Metric: unit.Metric{},               // Precisa de uma métrica válida
	}
	gtx := app.NewContext(&ops, dummyEvent)
	return gtx
}

// Invalidate solicita um novo frame para redesenhar a UI.
func (aw *AppWindow) Invalidate() {
	aw.window.Invalidate()
}

// Execute executa uma função na thread principal da UI.
// Útil para atualizar o estado da UI a partir de goroutines.
func (aw *AppWindow) Execute(f func()) {
	aw.window.Execute(f)
}

// HandleLoginSuccess é chamado pela LoginPage após um login bem-sucedido.
func (aw *AppWindow) HandleLoginSuccess(sessionID string, userData *models.UserPublic) {
	appLogger.Infof("Login bem-sucedido recebido pela AppWindow. SessionID: %s..., User: %s", sessionID[:8], userData.Username)
	auth.SetCurrentSessionID(sessionID) // Define a sessão global

	// TODO: Armazenar userData ou a sessão completa no AppWindow se necessário globalmente
	// ou no SessionManager (GetCurrentSession já faz isso).

	aw.router.NavigateTo(PageMain, userData) // Navega para a página principal, passando dados do usuário
	aw.Invalidate()
}

// HandleLogout é chamado para deslogar o usuário.
func (aw *AppWindow) HandleLogout() {
	appLogger.Info("Logout em progresso na AppWindow...")
	sessionID := auth.GetCurrentSessionID()
	if sessionID != "" {
		if err := aw.authenticator.LogoutUser(sessionID); err != nil {
			appLogger.Errorf("Erro durante o logout do usuário (Authenticator): %v", err)
			// Continuar com o logout na UI mesmo assim
		}
	}
	auth.SetCurrentSessionID("")                                     // Limpa sessão global
	aw.router.NavigateTo(PageLogin, "Logout realizado com sucesso.") // Passa mensagem opcional para LoginPage
	aw.Invalidate()
}

// StartGlobalLoading ativa o spinner de carregamento global.
func (aw *AppWindow) StartGlobalLoading() {
	aw.isGlobalLoading = true
	aw.globalSpinner.Start(aw.Context()) // Precisa de um gtx válido
	aw.Invalidate()
}

// StopGlobalLoading desativa o spinner de carregamento global.
func (aw *AppWindow) StopGlobalLoading() {
	aw.isGlobalLoading = false
	aw.globalSpinner.Stop(aw.Context()) // Precisa de um gtx válido
	aw.Invalidate()
}

// ShowGlobalMessage exibe uma mensagem no topo da janela.
// Type pode ser "info", "error", "success".
func (aw *AppWindow) ShowGlobalMessage(message, msgType string, autoHideDelay time.Duration) {
	aw.globalMessage = message
	aw.globalMessageType = strings.ToLower(msgType)
	aw.showGlobalMessage = true
	aw.Invalidate()

	if autoHideDelay > 0 {
		time.AfterFunc(autoHideDelay, func() {
			aw.Execute(func() { // Executa na thread da UI
				aw.showGlobalMessage = false
				aw.globalMessage = ""
				aw.Invalidate()
			})
		})
	}
}

// TODO: Adicionar método para abrir diálogos de arquivo nativos se necessário,
// que seriam chamados por páginas como ImportPage.
// Ex: func (aw *AppWindow) OpenFileDialog(filters []string) <-chan string { ... }
