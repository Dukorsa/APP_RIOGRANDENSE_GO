package ui

import (
	"fmt"
	"time"

	"gioui.org/app"
	"gioui.org/font/gofont" // Registra a coleção de fontes Go padrão.
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/auth"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/models"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/services"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui/components"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui/pages"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui/theme"
)

// AppWindow gerencia a janela principal da aplicação, o tema e o roteamento de páginas.
type AppWindow struct {
	window *app.Window     // A janela nativa do sistema operacional.
	th     *material.Theme // O tema Material Design usado em toda a aplicação.
	cfg    *core.Config    // Configurações globais da aplicação.
	router *Router         // Gerenciador de navegação entre páginas.

	// Serviços que a AppWindow ou suas páginas podem precisar acessar.
	authenticator  auth.AuthenticatorInterface
	sessionManager *auth.SessionManager
	userService    services.UserService
	roleService    services.RoleService
	networkService services.NetworkService
	cnpjService    services.CNPJService
	importService  services.ImportService
	auditService   services.AuditLogService

	// Estado global da UI gerenciado pela AppWindow.
	globalSpinner   *components.LoadingSpinner // Spinner de carregamento global.
	isGlobalLoading bool                       // Controla a visibilidade do spinner global.

	// Para mensagens/notificações globais exibidas no topo ou rodapé da janela.
	globalMessage         string      // Texto da mensagem global.
	globalMessageType     string      // Tipo da mensagem: "info", "error", "success", "warning".
	showGlobalMessage     bool        // Controla a visibilidade da mensagem global.
	globalMessageAutoHide *time.Timer // Timer para esconder automaticamente a mensagem global.
}

// NewAppWindow cria e inicializa a janela principal da aplicação e seus componentes.
func NewAppWindow(
	th *material.Theme, // Tema Material (pode ser nil para usar o padrão).
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
	gofont.Register() // Garante que as fontes Go padrão estejam registradas.
	if th == nil {
		th = theme.NewAppTheme() // Usa o tema customizado da aplicação.
	}

	// Cria a janela nativa com título e dimensões padrão.
	win := app.NewWindow(
		app.Title(fmt.Sprintf("%s v%s", cfg.AppName, cfg.AppVersion)),
		app.Size(theme.WindowDefaultWidth, theme.WindowDefaultHeight),
		app.MinSize(theme.WindowMinWidth, theme.WindowMinHeight),
	)

	aw := &AppWindow{
		window:         win,
		th:             th,
		cfg:            cfg,
		authenticator:  authN,
		sessionManager: sessMan,
		userService:    userSvc,
		roleService:    roleSvc,
		networkService: netSvc,
		cnpjService:    cnpjSvc,
		importService:  importSvc,
		auditService:   auditSvc,
		globalSpinner:  components.NewLoadingSpinner(theme.Colors.Primary), // Spinner global com cor primária.
	}

	// Inicializa o Router, passando `aw` (para callbacks e acesso a serviços/tema)
	// e todas as dependências de serviço que as páginas podem precisar.
	// O PermissionManager é obtido globalmente pelo router.
	aw.router = NewRouter(th, cfg, aw, userSvc, roleSvc, netSvc, cnpjSvc, importSvc, auditSvc, authN, sessMan, auth.GetPermissionManager())

	// Registra as páginas de nível superior no router.
	// As páginas recebem o router para navegação e acesso a serviços.
	aw.router.Register(PageLogin, pages.NewLoginPage(aw.router, cfg, authN))
	aw.router.Register(PageRegistration, pages.NewRegistrationPage(aw.router, cfg, userSvc, nil)) // nil para adminSession, pois é auto-registro.
	aw.router.Register(PageForgotPassword, pages.NewForgotPasswordPage(aw.router, cfg, userSvc))

	// MainAppLayout é a página principal que contém a sidebar e a área de conteúdo dos módulos.
	// Ela também recebe as dependências de serviço para passar para suas sub-páginas/módulos.
	mainAppLayout := pages.NewMainAppLayout(aw.router, cfg, userSvc, roleSvc, netSvc, cnpjSvc, importSvc, auth.GetPermissionManager(), sessMan)
	aw.router.Register(PageMain, mainAppLayout)

	// Verifica se há uma sessão ativa ao iniciar a aplicação.
	// Se houver, navega para a página principal. Caso contrário, para a página de login.
	if currentSess, _ := sessMan.GetCurrentSession(); currentSess != nil {
		// É importante passar os dados do usuário para MainAppLayout para que ela possa
		// exibir informações do usuário e filtrar módulos da sidebar com base nas permissões.
		// O Authenticator já retorna UserPublicData.
		userPublicData := &models.UserPublic{ /* Preencher com dados da currentSess ou buscar do userService */
			ID:       currentSess.UserID,
			Username: currentSess.Username,
			Roles:    currentSess.Roles,
			// Outros campos podem ser buscados se necessário, ou o Authenticator já os provê.
		}
		aw.router.NavigateTo(PageMain, userPublicData)
	} else {
		aw.router.NavigateTo(PageLogin, nil)
	}

	return aw
}

// Run inicia o loop de eventos da janela da aplicação.
// Esta função bloqueará até que a janela seja fechada.
func (aw *AppWindow) Run() error {
	var ops op.Ops // Pool de operações de desenho, reutilizado a cada frame.
	for {
		// Espera pelo próximo evento do sistema (ex: mouse, teclado, redesenho).
		evt := aw.window.Event()
		switch e := evt.(type) {
		case app.DestroyEvent: // Evento de fechamento da janela.
			appLogger.Info("AppWindow: Evento Destroy recebido, encerrando aplicação.")
			// Limpeza de recursos (ex: parar goroutines, fechar conexões) deve ser feita
			// usando `defer` onde os recursos são inicializados (ex: `sessionManager.Shutdown()`).
			aw.globalSpinner.Destroy() // Garante que a goroutine do spinner global pare.
			return e.Err               // Retorna o erro do evento, se houver, para encerrar app.Main().
		case app.FrameEvent: // Evento para desenhar um novo frame na janela.
			gtx := app.NewContext(&ops, e) // Contexto gráfico para o frame atual.

			aw.Layout(gtx) // Chama o método de layout principal da AppWindow.

			e.Frame(gtx.Ops) // Envia as operações de desenho acumuladas para a janela.

		case key.Event: // Eventos de teclado.
			// Pode lidar com atalhos de teclado globais aqui.
			// Ex: if e.Name == key.NameEscape && e.State == key.Press { aw.window.Perform(system.ActionClose) }
		}
	}
}

// Layout é o método de desenho principal para a AppWindow.
// Ele organiza o layout do router (página atual), spinner global e mensagens globais.
func (aw *AppWindow) Layout(gtx layout.Context) layout.Dimensions {
	// Define uma cor de fundo padrão para a janela inteira.
	paint.FillShape(gtx.Ops, theme.Colors.Background, clip.Rect{Max: gtx.Constraints.Max}.Op())

	// Layout em Pilha (Stack) para permitir sobreposições:
	// 1. Conteúdo da página atual (do router).
	// 2. Mensagem global (se visível).
	// 3. Spinner global (se ativo).
	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		// Camada 1: Conteúdo da Página Atual (do Router)
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			if aw.router != nil {
				return aw.router.Layout(gtx) // Desenha a página atual gerenciada pelo router.
			}
			// Fallback se o router não estiver pronto (não deveria acontecer após NewAppWindow).
			return material.Body1(aw.th, "Erro crítico: Router não inicializado.").Layout(gtx)
		}),

		// Camada 2: Mensagem Global (sobrepõe o conteúdo da página)
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			if aw.showGlobalMessage {
				return aw.layoutGlobalMessage(gtx)
			}
			return layout.Dimensions{} // Nenhum espaço se não houver mensagem.
		}),

		// Camada 3: Spinner Global (sobrepõe tudo se ativo)
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			if aw.isGlobalLoading {
				// Centraliza o spinner na janela.
				return layout.Center.Layout(gtx, aw.globalSpinner.Layout)
			}
			return layout.Dimensions{}
		}),
	)
}

// layoutGlobalMessage desenha a mensagem global na tela.
func (aw *AppWindow) layoutGlobalMessage(gtx layout.Context) layout.Dimensions {
	// Define cores com base no tipo da mensagem.
	bgColor := theme.Colors.InfoBg     // Cor de fundo padrão para "info".
	textColor := theme.Colors.InfoText // Cor de texto padrão.
	borderColor := theme.Colors.InfoBorder

	switch aw.globalMessageType {
	case "error":
		bgColor = theme.Colors.DangerBg
		textColor = theme.Colors.DangerText
		borderColor = theme.Colors.DangerBorder
	case "success":
		bgColor = theme.Colors.SuccessBg
		textColor = theme.Colors.SuccessText
		borderColor = theme.Colors.SuccessBorder
	case "warning":
		bgColor = theme.Colors.WarningBg
		textColor = theme.Colors.WarningText
		borderColor = theme.Colors.WarningBorder
	}

	// Layout da mensagem (ex: no topo da tela).
	// `layout.Inset` para padding, `layout.Background` para cor de fundo.
	// `material.Label` para o texto. Um botão de fechar pode ser adicionado.
	return layout.Align(layout.Center).Layout(gtx, // Alinha o container da mensagem (ex: topo central)
		func(gtx C) D {
			// Limitar largura da mensagem
			// gtx.Constraints.Max.X = gtx.Dp(unit.Dp(600))
			// if gtx.Constraints.Max.X > gtx.Constraints.Min.X {
			// 	gtx.Constraints.Min.X = gtx.Constraints.Max.X
			// }

			// Card para a mensagem
			return material.Card(aw.th, bgColor, theme.ElevationLarge, layout.UniformInset(unit.Dp(16)),
				func(gtx C) D {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
						layout.Flexed(1, func(gtx C) D {
							label := material.Body1(aw.th, aw.globalMessage)
							label.Color = textColor
							return label.Layout(gtx)
						}),
						// Opcional: Botão de fechar a mensagem
						// layout.Rigid(func(gtx C)D {
						//  // ... botão de fechar aqui ...
						//  // if closeBtn.Clicked(gtx) { aw.hideGlobalMessage() }
						// })
					)
				}).Layout(gtx)
		})
}

// hideGlobalMessage esconde a mensagem global.
func (aw *AppWindow) hideGlobalMessage() {
	if aw.globalMessageAutoHide != nil {
		aw.globalMessageAutoHide.Stop() // Para o timer se estiver ativo.
		aw.globalMessageAutoHide = nil
	}
	aw.showGlobalMessage = false
	aw.globalMessage = "" // Limpa o texto para não reaparecer se Invalidate for chamado.
	aw.Invalidate()
}

// --- Métodos de Callback e Interação com Páginas ---

// Theme retorna o tema Material Design atual da aplicação.
func (aw *AppWindow) Theme() *material.Theme {
	return aw.th
}

// Config retorna as configurações da aplicação.
func (aw *AppWindow) Config() *core.Config {
	return aw.cfg
}

// Invalidate solicita um novo frame para redesenhar a UI.
// Deve ser chamado sempre que o estado da UI mudar e precisar ser refletido visualmente.
func (aw *AppWindow) Invalidate() {
	aw.window.Invalidate()
}

// Execute executa uma função `f` na thread principal da UI.
// Útil para atualizar o estado da UI a partir de goroutines (ex: após chamadas de serviço).
func (aw *AppWindow) Execute(f func()) {
	aw.window.Execute(f)
}

// HandleLoginSuccess é chamado pela LoginPage após um login bem-sucedido.
// `sessionID` é o ID da sessão criada. `userData` contém os dados públicos do usuário.
func (aw *AppWindow) HandleLoginSuccess(sessionID string, userData *models.UserPublic) {
	if userData == nil {
		appLogger.Error("HandleLoginSuccess chamado com userData nil. Login abortado.")
		aw.ShowGlobalMessage("Erro de Login", "Falha ao obter dados do usuário após login.", true, 5*time.Second)
		return
	}
	appLogger.Infof("Login bem-sucedido recebido pela AppWindow. SessionID: %s..., Usuário: %s", sessionID[:min(8, len(sessionID))], userData.Username)
	auth.SetCurrentSessionID(sessionID) // Define a sessão globalmente.

	// Navega para a página principal (MainAppLayout), passando os dados do usuário.
	// MainAppLayout usará esses dados para configurar a sidebar e exibir infos.
	aw.router.NavigateTo(PageMain, userData)
	aw.Invalidate()
}

// HandleLogout é chamado para deslogar o usuário da aplicação.
func (aw *AppWindow) HandleLogout() {
	appLogger.Info("Logout em progresso na AppWindow...")
	sessionID := auth.GetCurrentSessionID()
	if sessionID != "" {
		// Chama o Authenticator para invalidar a sessão no backend (se houver lógica de backend).
		if err := aw.authenticator.LogoutUser(sessionID); err != nil {
			appLogger.Errorf("Erro durante o logout do usuário no Authenticator (sessão ID: %s...): %v", sessionID[:min(8, len(sessionID))], err)
			// Continuar com o logout na UI mesmo se houver erro no backend.
		}
	}
	auth.SetCurrentSessionID("") // Limpa o ID da sessão global.
	// Navega para a página de login, opcionalmente com uma mensagem.
	aw.router.NavigateTo(PageLogin, "Logout realizado com sucesso.")
	aw.Invalidate()
}

// StartGlobalLoading ativa o spinner de carregamento global.
// Requer um `layout.Context` para iniciar a animação do spinner (para `gtx.Now`).
func (aw *AppWindow) StartGlobalLoading(gtx layout.Context) {
	if !aw.isGlobalLoading {
		aw.isGlobalLoading = true
		aw.globalSpinner.Start(gtx) // Passa o contexto para o spinner.
		aw.Invalidate()
	}
}

// StopGlobalLoading desativa o spinner de carregamento global.
func (aw *AppWindow) StopGlobalLoading(gtx layout.Context) {
	if aw.isGlobalLoading {
		aw.isGlobalLoading = false
		aw.globalSpinner.Stop(gtx)
		aw.Invalidate()
	}
}

// ShowGlobalMessage exibe uma mensagem no topo (ou outra posição) da janela.
// `messageType` pode ser "info", "error", "success", "warning".
// `isError` é um booleano para simplificar a coloração (true para erro/warning, false para info/sucesso).
// `autoHideDelay` (se > 0) define um tempo para a mensagem desaparecer automaticamente.
func (aw *AppWindow) ShowGlobalMessage(title, message string, isError bool, autoHideDuration time.Duration) {
	// Uma implementação mais robusta poderia usar um tipo enum para `messageType`.
	// Por agora, `isError` controla a cor primária da mensagem.
	// O `layoutGlobalMessage` pode ser expandido para usar `title` e mais tipos.

	// Cancela timer anterior se houver, para evitar múltiplos auto-hides.
	if aw.globalMessageAutoHide != nil {
		aw.globalMessageAutoHide.Stop()
	}

	aw.globalMessage = message // Usar `title + ": " + message` se quiser incluir o título.
	if isError {
		aw.globalMessageType = "error"
	} else {
		aw.globalMessageType = "success" // Ou "info"
	}
	aw.showGlobalMessage = true
	aw.Invalidate()

	if autoHideDuration > 0 {
		aw.globalMessageAutoHide = time.AfterFunc(autoHideDuration, func() {
			aw.Execute(aw.hideGlobalMessage) // Executa na thread da UI.
		})
	}
}

// Helper min para evitar pânico com slices.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ConfirmDialog (Exemplo de como poderia ser - requer um estado de diálogo na AppWindow)
// type ConfirmDialogAction func(confirmed bool)
// var (
// 	showConfirmDialog bool
// 	confirmDialogTitle string
// 	confirmDialogMessage string
// 	confirmDialogYesBtn widget.Clickable
// 	confirmDialogNoBtn widget.Clickable
// 	confirmDialogCallback ConfirmDialogAction
// )
// func (aw *AppWindow) ShowConfirmDialog(title, message string, callback ConfirmDialogAction) {
// 	showConfirmDialog = true
// 	confirmDialogTitle = title
// 	confirmDialogMessage = message
// 	confirmDialogCallback = callback
// 	aw.Invalidate()
// }
// No Layout da AppWindow, se showConfirmDialog, desenhar o diálogo.
// if showConfirmDialog {
// 	// ... layout do diálogo ...
// 	if confirmDialogYesBtn.Clicked(gtx) { confirmDialogCallback(true); showConfirmDialog = false }
// 	if confirmDialogNoBtn.Clicked(gtx) { confirmDialogCallback(false); showConfirmDialog = false }
// }
