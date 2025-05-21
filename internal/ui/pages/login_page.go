package pages

import (
	"time"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	appLogger "github.com/seu_usuario/riograndense_gio/internal/core/logger"
	"github.com/seu_usuario/riograndense_gio/internal/ui" // Para acesso ao Router ou AppWindow
	"github.com/seu_usuario/riograndense_gio/internal/ui/components"
	"github.com/seu_usuario/riograndense_gio/internal/ui/theme"
	// TODO: Importar serviço de autenticação
)

type LoginPage struct {
	router *ui.Router // Para navegação
	// authService services.AuthService // Exemplo

	usernameEdit widget.Editor
	passwordEdit components.PasswordInput // Usando o componente customizado
	loginButton  widget.Clickable
	forgotButton widget.Clickable
	createButton widget.Clickable

	isLoading bool
	errorText string

	// TODO: Adicionar spinner
	// spinner *components.LoadingSpinner
}

// NewLoginPage cria uma nova instância da LoginPage
func NewLoginPage(router *ui.Router /*, authService services.AuthService */) *LoginPage {
	lp := &LoginPage{
		router: router,
		// authService: authService,
	}
	lp.usernameEdit.SingleLine = true
	lp.usernameEdit.Hint = "Usuário ou E-mail"
	// Inicializa PasswordInput
	lp.passwordEdit = components.NewPasswordInput(router.GetAppWindow().th) // Passa o tema
	lp.passwordEdit.SetHint("Senha")

	// lp.spinner = components.NewLoadingSpinner(theme.Colors.Primary)
	return lp
}

func (lp *LoginPage) OnNavigatedTo(params interface{}) {
	appLogger.Info("Navegou para LoginPage")
	lp.isLoading = false
	lp.errorText = ""
	lp.usernameEdit.SetText("")
	lp.passwordEdit.SetText("")
	// TODO: Resetar estado do spinner se necessário
}

func (lp *LoginPage) OnNavigatedFrom() {
	appLogger.Info("Navegando para fora da LoginPage")
	// Limpar campos ou estado se necessário
}

// Layout define a UI da página de login
func (lp *LoginPage) Layout(gtx layout.Context) layout.Dimensions {
	th := lp.router.GetAppWindow().th // Acessa o tema da AppWindow através do router

	for lp.loginButton.Clicked(gtx) {
		lp.handleLogin()
	}
	for lp.forgotButton.Clicked(gtx) {
		lp.router.NavigateTo(ui.PageForgotPassword, nil)
	}
	for lp.createButton.Clicked(gtx) {
		lp.router.NavigateTo(ui.PageRegistration, nil)
	}

	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle, Spacing: layout.SpaceSides}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, material.H3(th, lp.router.GetAppWindow().cfg.AppName).Layout)
		}),
		layout.Rigid(layout.Spacer{Height: theme.LargeVSpacer}.Layout), // Espaçador

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return material.Editor(th, &lp.usernameEdit, lp.usernameEdit.Hint).Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: theme.DefaultVSpacer}.Layout),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return lp.passwordEdit.Layout(gtx, th)
		}),
		layout.Rigid(layout.Spacer{Height: theme.LargeVSpacer}.Layout),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if lp.isLoading {
				// TODO: Mostrar spinner
				// return lp.spinner.Layout(gtx)
				return material.Button(th, &widget.Clickable{}, "Autenticando...").Layout(gtx) // Placeholder
			}
			btn := material.Button(th, &lp.loginButton, "Entrar")
			btn.Background = theme.Colors.Primary
			return btn.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: theme.DefaultVSpacer}.Layout),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(gtx,
				layout.Rigid(material.ButtonLayoutStyle{Button: &lp.forgotButton}.Layout(gtx, material.Body2(th, "Esqueceu a senha?").Layout)),
				layout.Rigid(material.ButtonLayoutStyle{Button: &lp.createButton}.Layout(gtx, material.Body2(th, "Criar conta").Layout)),
			)
		}),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if lp.errorText != "" {
				errorLabel := material.Body2(th, lp.errorText)
				errorLabel.Color = theme.Colors.Danger // Vermelho para erro
				return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, errorLabel.Layout)
			}
			return layout.Dimensions{}
		}),
	)
}

func (lp *LoginPage) handleLogin() {
	if lp.isLoading {
		return
	}
	lp.isLoading = true
	lp.errorText = ""
	// Força redesenho para mostrar spinner/estado de loading
	lp.router.GetAppWindow().window.Invalidate()

	username := lp.usernameEdit.Text()
	password := lp.passwordEdit.Text()

	appLogger.Infof("Tentativa de login com usuário: %s", username)

	// Simulação de chamada de serviço
	go func() {
		// TODO: Chamar o serviço de autenticação real aqui
		// result, err := lp.authService.Login(username, password)
		time.Sleep(1 * time.Second) // Simula delay de rede/processamento

		// Simulação de resultado
		var loginSuccessful bool = (username == "admin" && password == "admin") // Lógica de teste
		var sessionID string = "fake-session-id-123"
		var errMsg string
		if !loginSuccessful {
			errMsg = "Usuário ou senha inválidos."
		}

		// Atualiza UI na goroutine principal do Gio
		lp.router.GetAppWindow().window.Execute(func() {
			lp.isLoading = false
			if loginSuccessful {
				appLogger.Info("Login bem sucedido (simulado). Navegando para Main Page.")
				lp.router.GetAppWindow().HandleLoginSuccess(sessionID /*, result.User */)
			} else {
				appLogger.Warnf("Falha no login (simulado): %s", errMsg)
				lp.errorText = errMsg
			}
			lp.router.GetAppWindow().window.Invalidate() // Redesenha para mostrar erro ou mudar de página
		})
	}()
}
