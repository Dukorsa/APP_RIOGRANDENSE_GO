package pages

import (
	"errors"
	"fmt"
	"strings"

	// Para usar theme.Colors.Danger
	// "time" // Descomentado se simular delay

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/text" // Para text.Alignment
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/auth"           // Para AuthenticatorInterface
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/config"
	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/errors" // Para verificar ErrInvalidCredentials
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui/components"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui"
)

// LoginPage gerencia a UI para a tela de login.
type LoginPage struct {
	router        *ui.Router
	cfg           *core.Config                // Para acessar AppName, etc.
	authenticator auth.AuthenticatorInterface // Serviço de autenticação

	usernameEdit widget.Editor
	passwordEdit *components.PasswordInput // Usando o componente PasswordInput customizado
	loginButton  widget.Clickable
	forgotButton widget.Clickable
	createButton widget.Clickable

	isLoading bool   // True se a autenticação estiver em andamento
	errorText string // Mensagem de erro a ser exibida

	spinner *components.LoadingSpinner // Spinner de carregamento

	// Para exibir mensagens de sucesso passadas por outras páginas (ex: após cadastro ou reset de senha)
	successMessage string
}

// NewLoginPage cria uma nova instância da LoginPage.
func NewLoginPage(router *ui.Router, cfg *core.Config, authN auth.AuthenticatorInterface) *LoginPage {
	th := router.GetAppWindow().Theme() // Obtém o tema da AppWindow
	lp := &LoginPage{
		router:        router,
		cfg:           cfg,
		authenticator: authN,
		spinner:       components.NewLoadingSpinner(theme.Colors.Primary), // Usa a cor primária do tema
	}
	lp.usernameEdit.SingleLine = true
	lp.usernameEdit.Hint = "Usuário ou E-mail"

	lp.passwordEdit = components.NewPasswordInput(th, cfg) // Passa o tema e config
	lp.passwordEdit.SetHint("Senha")
	// A barra de força da senha pode ser mostrada ou não por padrão no PasswordInput.
	// lp.passwordEdit.ShowStrengthBar(false) // Exemplo para esconder se não quiser aqui.

	return lp
}

// OnNavigatedTo é chamado quando a página se torna ativa.
// `params` pode ser usado para passar dados (ex: mensagem de sucesso).
func (lp *LoginPage) OnNavigatedTo(params interface{}) {
	appLogger.Info("Navegou para LoginPage")
	lp.isLoading = false
	lp.errorText = ""
	lp.successMessage = "" // Limpa mensagem de sucesso anterior

	// Limpa campos, exceto se houver uma política para manter o username.
	// lp.usernameEdit.SetText("") // Descomentar para limpar username sempre.
	lp.passwordEdit.Clear() // Limpa a senha.

	// Se params for uma string, assume que é uma mensagem de sucesso.
	if msg, ok := params.(string); ok && msg != "" {
		lp.successMessage = msg
	}

	// Focar no campo de username ao carregar a página.
	// A AppWindow precisaria de um método para solicitar foco em um widget específico,
	// ou o Editor precisaria de uma forma de solicitar foco ao ser exibido.
	// Por agora, o usuário precisará clicar.
	// lp.usernameEdit.Focus() // Gio não tem um Focus() direto que sempre funciona fora do evento de layout
	lp.router.GetAppWindow().Invalidate() // Garante redesenho
}

// OnNavigatedFrom é chamado quando o router navega para fora desta página.
func (lp *LoginPage) OnNavigatedFrom() {
	appLogger.Info("Navegando para fora da LoginPage")
	lp.isLoading = false                                // Garante que `isLoading` seja false.
	lp.spinner.Stop(lp.router.GetAppWindow().Context()) // Para o spinner, se estiver ativo.
	// Limpar campos pode ser feito aqui ou em OnNavigatedTo da próxima página.
}

// Layout define a UI da página de login.
func (lp *LoginPage) Layout(gtx layout.Context) layout.Dimensions {
	th := lp.router.GetAppWindow().Theme()

	// Processar eventos dos inputs para limpar erros/sucessos ao digitar.
	if lp.usernameEdit.Update(gtx) {
		lp.errorText = ""
		lp.successMessage = ""
	}
	// O PasswordInput pode ter um callback OnChange para limpar o errorText também.
	// lp.passwordEdit.OnChange = func(text string) { lp.errorText = ""; lp.successMessage = "" }

	// Processar cliques nos botões.
	// A verificação `!lp.isLoading` impede múltiplas submissões.
	if lp.loginButton.Clicked(gtx) && !lp.isLoading {
		lp.handleLogin(gtx)
	}
	if lp.forgotButton.Clicked(gtx) && !lp.isLoading {
		lp.router.NavigateTo(ui.PageForgotPassword, nil)
	}
	if lp.createButton.Clicked(gtx) && !lp.isLoading {
		lp.router.NavigateTo(ui.PageRegistration, nil)
	}

	// Layout centralizado na tela.
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		// Container com padding e largura máxima para o formulário.
		return layout.UniformInset(unit.Dp(20)).Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				maxWidth := gtx.Dp(unit.Dp(350)) // Largura máxima do formulário de login.
				gtx.Constraints.Max.X = maxWidth
				if gtx.Constraints.Max.X > gtx.Constraints.Min.X {
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
				}

				return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle, Spacing: layout.SpaceSides}.Layout(gtx,
					// Título da Aplicação
					layout.Rigid(func(gtx C) D {
						title := material.H4(th, lp.cfg.AppName)
						title.Font.Weight = font.Bold
						title.Alignment = text.Middle
						return layout.Inset{Bottom: theme.LargeVSpacer * 2}.Layout(gtx, title.Layout)
					}),

					// Campo Username
					layout.Rigid(func(gtx C) D {
						editor := material.Editor(th, &lp.usernameEdit, lp.usernameEdit.Hint)
						editor.TextSize = unit.Sp(16)
						// Adicionar um ícone de usuário (opcional)
						// return components.InputWithIcon(th, icons.User, editor).Layout(gtx)
						return editor.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: theme.DefaultVSpacer}.Layout),

					// Campo Password (usando o componente PasswordInput)
					layout.Rigid(func(gtx C) D {
						return lp.passwordEdit.Layout(gtx, th)
					}),
					layout.Rigid(layout.Spacer{Height: theme.LargeVSpacer}.Layout),

					// Botão de Login ou Spinner
					layout.Rigid(func(gtx C) D {
						if lp.isLoading {
							// Centraliza o spinner.
							return layout.Center.Layout(gtx, lp.spinner.Layout)
						}
						btnLogin := material.Button(th, &lp.loginButton, "Entrar")
						btnLogin.Background = theme.Colors.Primary
						btnLogin.Color = theme.Colors.PrimaryText
						btnLogin.CornerRadius = theme.CornerRadius
						// btnLogin.Inset = layout.UniformInset(unit.Dp(12)) // Padding interno do botão
						return layout.Sizer{Width: unit.Dp(float32(maxWidth))}.Layout(gtx, btnLogin.Layout) // Botão com largura total
					}),
					layout.Rigid(layout.Spacer{Height: theme.DefaultVSpacer}.Layout),

					// Links (Esqueceu a senha?, Criar conta)
					layout.Rigid(func(gtx C) D {
						return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(gtx,
							layout.Rigid(material.ButtonLayoutStyle{Button: &lp.forgotButton}.Layout(gtx,
								func(gtx C) D {
									lbl := material.Body2(th, "Esqueceu a senha?")
									lbl.Color = theme.Colors.Primary
									return lbl.Layout(gtx)
								})),
							layout.Rigid(material.ButtonLayoutStyle{Button: &lp.createButton}.Layout(gtx,
								func(gtx C) D {
									lbl := material.Body2(th, "Criar nova conta")
									lbl.Color = theme.Colors.Primary
									return lbl.Layout(gtx)
								})),
						)
					}),

					// Mensagem de Erro ou Sucesso
					layout.Rigid(func(gtx C) D {
						message := ""
						msgColor := theme.Colors.Text // Cor padrão
						if lp.errorText != "" {
							message = lp.errorText
							msgColor = theme.Colors.Danger
						} else if lp.successMessage != "" {
							message = lp.successMessage
							msgColor = theme.Colors.Success
						}

						if message != "" {
							msgLabel := material.Body2(th, message)
							msgLabel.Color = msgColor
							msgLabel.Alignment = text.Middle
							return layout.Inset{Top: theme.DefaultVSpacer}.Layout(gtx, msgLabel.Layout)
						}
						return layout.Dimensions{}
					}),
				)
			})
	})
}

// handleLogin lida com a tentativa de login.
func (lp *LoginPage) handleLogin(gtx layout.Context) {
	lp.isLoading = true
	lp.errorText = ""
	lp.successMessage = ""                // Limpa mensagens ao tentar login
	lp.spinner.Start(gtx)                 // Inicia o spinner com o contexto atual
	lp.router.GetAppWindow().Invalidate() // Solicita redesenho para mostrar spinner/estado de loading

	username := strings.TrimSpace(lp.usernameEdit.Text())
	password := lp.passwordEdit.Text() // Senha não deve ter TrimSpace

	if username == "" || password == "" {
		lp.isLoading = false
		lp.spinner.Stop(gtx)
		lp.errorText = "Usuário e senha são obrigatórios."
		lp.router.GetAppWindow().Invalidate()
		return
	}

	appLogger.Infof("Tentativa de login com usuário/email: %s", username)

	go func(u, pword string) {
		// IP e UserAgent podem ser mais difíceis de obter em desktop puro.
		// Para IP, pode-se tentar obter o local, ou "N/A".
		// UserAgent seria o nome/versão da aplicação.
		ipAddress := "DESKTOP_APP_IP_NA" // Placeholder
		userAgent := fmt.Sprintf("%s/%s (Desktop)", lp.cfg.AppName, lp.cfg.AppVersion)

		authResult, err := lp.authenticator.AuthenticateUser(u, pword, ipAddress, userAgent)

		lp.router.GetAppWindow().Execute(func() { // Atualiza UI na thread principal do Gio
			lp.isLoading = false
			lp.spinner.Stop(gtx) // Para o spinner com o mesmo contexto usado para iniciar
			if err != nil {
				// Erros de validação ou DB já são logados pelo Authenticator ou camadas inferiores.
				appLogger.Errorf("Erro durante a autenticação para '%s': %v", u, err)
				// Tenta extrair uma mensagem mais amigável.
				if errors.Is(err, appErrors.ErrDatabase) {
					lp.errorText = "Erro interno ao tentar autenticar. Tente novamente mais tarde."
				} else {
					lp.errorText = fmt.Sprintf("Falha na autenticação: %v", err) // Pode ser muito técnico
				}
			} else if authResult != nil {
				if authResult.Success {
					appLogger.Infof("Login bem-sucedido para '%s'. Navegando para Main Page.", authResult.UserData.Username)
					lp.router.GetAppWindow().HandleLoginSuccess(authResult.SessionID, authResult.UserData)
				} else {
					appLogger.Warnf("Falha no login para '%s': %s", u, authResult.Message)
					lp.errorText = authResult.Message
				}
			} else { // Caso inesperado
				appLogger.Error("Resultado da autenticação inesperadamente nulo sem erro.")
				lp.errorText = "Ocorreu um erro inesperado durante o login."
			}
			lp.router.GetAppWindow().Invalidate() // Redesenha para mostrar erro ou mudar de página
		})
	}(username, password)
}
