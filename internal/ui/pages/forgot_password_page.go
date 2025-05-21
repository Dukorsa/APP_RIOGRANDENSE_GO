package pages

import (
	"errors"
	"fmt"
	"image/color"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/config"
	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/errors"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/services"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui/components"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/utils"
)

// forgotPasswordState define os diferentes estágios do processo de recuperação de senha.
type forgotPasswordState int

const (
	fpStateRequestToken         forgotPasswordState = iota // Estado inicial: solicitar token por e-mail.
	fpStateEnterCodeAndPassword                            // Estado secundário: inserir token e nova senha.
)

// ForgotPasswordPage gerencia a UI para recuperação de senha.
type ForgotPasswordPage struct {
	router      *ui.Router
	cfg         *core.Config
	userService services.UserService

	currentState  forgotPasswordState // Controla qual parte do formulário é exibida.
	isLoading     bool                // True se uma operação de backend estiver em andamento.
	statusMessage string              // Para exibir mensagens de erro ou sucesso.
	messageColor  color.NRGBA         // Cor da `statusMessage`.

	// Campos de entrada e botões.
	emailInput           widget.Editor
	codeInput            widget.Editor
	newPasswordInput     *components.PasswordInput // Componente customizado para senha.
	confirmPasswordInput *components.PasswordInput // Para confirmação da nova senha.

	requestCodeBtn  widget.Clickable // Botão para solicitar o token.
	confirmResetBtn widget.Clickable // Botão para confirmar a redefinição de senha.
	backToLoginBtn  widget.Clickable // Botão para voltar à tela de login.

	// Feedback para os campos de entrada (mensagens de erro de validação).
	emailInputFeedback      string
	codeInputFeedback       string
	newPasswordFeedback     string
	confirmPasswordFeedback string

	// targetEmail armazena o e-mail para o qual o token foi solicitado,
	// usado na segunda etapa para chamar o serviço de confirmação.
	targetEmail string

	spinner *components.LoadingSpinner // Spinner de carregamento.
}

// NewForgotPasswordPage cria uma nova instância da página de recuperação de senha.
func NewForgotPasswordPage(
	router *ui.Router,
	cfg *core.Config,
	userSvc services.UserService,
) *ForgotPasswordPage {
	th := router.GetAppWindow().Theme() // Obtém o tema da AppWindow.

	p := &ForgotPasswordPage{
		router:       router,
		cfg:          cfg,
		userService:  userSvc,
		currentState: fpStateRequestToken, // Estado inicial.
		spinner:      components.NewLoadingSpinner(theme.Colors.Primary),
	}

	p.emailInput.SingleLine = true
	p.emailInput.Hint = "Seu e-mail cadastrado"

	p.codeInput.SingleLine = true
	p.codeInput.Hint = "Token recebido por e-mail"
	p.codeInput.Filter = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ" // Filtro para token

	// Inicializa os componentes PasswordInput com o tema e configuração.
	p.newPasswordInput = components.NewPasswordInput(th, cfg)
	p.newPasswordInput.SetHint(fmt.Sprintf("Nova senha (mín. %d caracteres)", cfg.PasswordMinLength))
	p.confirmPasswordInput = components.NewPasswordInput(th, cfg)
	p.confirmPasswordInput.SetHint("Confirme a nova senha")
	p.confirmPasswordInput.ShowStrengthBar(false) // Não mostra barra de força para confirmação.

	return p
}

// OnNavigatedTo é chamado quando a página se torna ativa.
func (p *ForgotPasswordPage) OnNavigatedTo(params interface{}) {
	appLogger.Info("Navegou para ForgotPasswordPage")
	p.resetFormAndState() // Reseta o estado da página para o inicial.
}

// OnNavigatedFrom é chamado quando o router navega para fora desta página.
func (p *ForgotPasswordPage) OnNavigatedFrom() {
	appLogger.Info("Navegando para fora da ForgotPasswordPage")
	p.isLoading = false                               // Garante que `isLoading` seja false.
	p.spinner.Stop(p.router.GetAppWindow().Context()) // Para o spinner, se estiver ativo.
}

// resetFormAndState limpa todos os campos e reseta o estado da página.
func (p *ForgotPasswordPage) resetFormAndState() {
	p.currentState = fpStateRequestToken
	p.isLoading = false
	p.statusMessage = ""
	p.emailInput.SetText("")
	p.codeInput.SetText("")
	p.newPasswordInput.Clear() // Usa o método Clear do componente.
	p.confirmPasswordInput.Clear()
	p.clearAllFeedbacks()
	p.targetEmail = ""
	p.spinner.Stop(p.router.GetAppWindow().Context()) // Para o spinner.
}

// clearAllFeedbacks limpa todas as mensagens de feedback dos inputs.
func (p *ForgotPasswordPage) clearAllFeedbacks() {
	p.emailInputFeedback = ""
	p.codeInputFeedback = ""
	p.newPasswordFeedback = ""
	p.confirmPasswordFeedback = ""
}

// Layout é o método principal de desenho da página.
func (p *ForgotPasswordPage) Layout(gtx layout.Context) layout.Dimensions {
	th := p.router.GetAppWindow().Theme()

	// Processar eventos dos inputs para limpar feedback ao digitar.
	if p.emailInput.Update(gtx) {
		p.emailInputFeedback = ""
		p.statusMessage = ""
	}
	if p.codeInput.Update(gtx) {
		p.codeInputFeedback = ""
		p.statusMessage = ""
	}
	// Para PasswordInput, usar OnChange se implementado, ou validar no submit.
	// Se PasswordInput tivesse um callback OnChange:
	// p.newPasswordInput.OnChange = func(s string) { p.newPasswordFeedback = ""; p.statusMessage = "" }
	// p.confirmPasswordInput.OnChange = func(s string) { p.confirmPasswordFeedback = ""; p.statusMessage = "" }

	// Processar cliques nos botões.
	if p.requestCodeBtn.Clicked(gtx) && !p.isLoading {
		p.handleRequestCode()
	}
	if p.confirmResetBtn.Clicked(gtx) && !p.isLoading {
		p.handleConfirmReset()
	}
	if p.backToLoginBtn.Clicked(gtx) && !p.isLoading {
		p.router.NavigateTo(ui.PageLogin, nil)
	}

	// Layout centralizado na tela.
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		// Container com padding e largura máxima para o formulário.
		return layout.UniformInset(unit.Dp(20)).Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				maxWidth := gtx.Dp(unit.Dp(400)) // Largura máxima do formulário.
				gtx.Constraints.Max.X = maxWidth
				// Força a largura mínima a ser igual à máxima se houver espaço.
				if gtx.Constraints.Max.X > gtx.Constraints.Min.X {
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
				}
				return p.layoutFormContent(gtx, th)
			})
	})
}

// layoutFormContent desenha o conteúdo interno do formulário de recuperação de senha.
func (p *ForgotPasswordPage) layoutFormContent(gtx layout.Context, th *material.Theme) layout.Dimensions {
	title := "Recuperar Senha"
	var instruction string
	if p.currentState == fpStateRequestToken {
		instruction = "Digite seu e-mail cadastrado para receber as instruções e o token de recuperação."
	} else { // fpStateEnterCodeAndPassword
		validityMinutes := p.cfg.PasswordResetTimeout.Minutes()
		// Usar HTML simulado com \n para quebras de linha, pois material.Body1 não renderiza HTML.
		instruction = fmt.Sprintf("Um token foi enviado para %s.\nInsira-o abaixo e crie uma nova senha.\nO token expira em %.0f minutos.", p.targetEmail, validityMinutes)
	}
	if p.isLoading {
		instruction = "Processando sua solicitação, por favor aguarde..."
	}

	titleWidget := material.H5(th, title)
	titleWidget.Font.Weight = font.Bold
	titleWidget.Alignment = text.Middle // Centraliza o título.

	instructionLabel := material.Body1(th, instruction)
	instructionLabel.Alignment = text.Start // Alinha à esquerda (ou text.Middle para centralizar).

	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle, Spacing: layout.SpaceSides}.Layout(gtx,
		layout.Rigid(titleWidget.Layout),
		layout.Rigid(layout.Spacer{Height: theme.DefaultVSpacer}.Layout),
		layout.Rigid(instructionLabel.Layout),
		layout.Rigid(layout.Spacer{Height: theme.LargeVSpacer}.Layout),

		// Campo de E-mail (sempre visível, mas interatividade controlada).
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			emailEditorWidget := material.Editor(th, &p.emailInput, p.emailInput.Hint)
			// Para "desabilitar" visualmente e interativamente se não for a etapa de request code:
			// A interatividade é controlada por não processar eventos se `p.currentState != fpStateRequestToken`.
			// Visualmente, poderia mudar a cor do texto/borda.
			isEmailEditable := (p.currentState == fpStateRequestToken && !p.isLoading)
			// Gio não tem um `SetEnabled`. A interatividade depende de como os eventos são tratados.
			// Para um editor, se não estiver focado e não processar `Editor.Events`, ele não será editável.
			// Se estiver desabilitado, o foco não deve ir para ele.
			return p.labeledInput(gtx, th, "E-mail:*", emailEditorWidget.Layout, p.emailInputFeedback, isEmailEditable)
		}),
		layout.Rigid(layout.Spacer{Height: theme.DefaultVSpacer}.Layout),

		// Campos para Etapa 2 (Token e Nova Senha).
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.currentState != fpStateEnterCodeAndPassword {
				return layout.Dimensions{} // Não desenha se não estiver nesta etapa.
			}
			isStep2Editable := !p.isLoading
			return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceSides}.Layout(gtx,
				layout.Rigid(func(gtx C) D { // Code Input
					codeInputWidget := material.Editor(th, &p.codeInput, p.codeInput.Hint)
					return p.labeledInput(gtx, th, "Token de Segurança:*", codeInputWidget.Layout, p.codeInputFeedback, isStep2Editable)
				}),
				layout.Rigid(layout.Spacer{Height: theme.DefaultVSpacer}.Layout),
				layout.Rigid(func(gtx C) D { // New Password
					return p.labeledInput(gtx, th, "Nova Senha:*", p.newPasswordInput.Layout(gtx, th), p.newPasswordFeedback, isStep2Editable)
				}),
				layout.Rigid(layout.Spacer{Height: theme.DefaultVSpacer}.Layout),
				layout.Rigid(func(gtx C) D { // Confirm New Password
					return p.labeledInput(gtx, th, "Confirmar Nova Senha:*", p.confirmPasswordInput.Layout(gtx, th), p.confirmPasswordFeedback, isStep2Editable)
				}),
			)
		}),
		layout.Rigid(layout.Spacer{Height: theme.LargeVSpacer}.Layout),

		// Botões de Ação (Solicitar Token ou Redefinir Senha).
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.isLoading { // Mostra o spinner no lugar dos botões de ação.
				return layout.Center.Layout(gtx, p.spinner.Layout)
			}
			var actionButton layout.Widget
			if p.currentState == fpStateRequestToken {
				btn := material.Button(th, &p.requestCodeBtn, "Solicitar Token de Recuperação")
				btn.Background = theme.Colors.Primary
				actionButton = btn.Layout
			} else { // fpStateEnterCodeAndPassword
				btn := material.Button(th, &p.confirmResetBtn, "Redefinir Senha")
				btn.Background = theme.Colors.Primary
				actionButton = btn.Layout
			}
			return actionButton(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: theme.DefaultVSpacer}.Layout),
		// Botão "Voltar para Login" (sempre visível e ativo, a menos que `isLoading`).
		layout.Rigid(func(gtx C) D {
			if p.isLoading {
				return D{}
			} // Esconde se carregando
			btn := material.Button(th, &p.backToLoginBtn, "Voltar para Login")
			// Estilo secundário (ex: texto ou outline)
			btn.Background = color.NRGBA{} // Transparente para botão de texto
			btn.Color = theme.Colors.Primary
			return btn.Layout(gtx)
		}),

		// Mensagem de Status.
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.statusMessage != "" && !p.isLoading { // Só mostra se não estiver carregando
				lbl := material.Body2(th, p.statusMessage)
				lbl.Color = p.messageColor
				lbl.Alignment = text.Middle
				return layout.Inset{Top: theme.DefaultVSpacer}.Layout(gtx, lbl.Layout)
			}
			return layout.Dimensions{}
		}),
	)
}

// labeledInput é um helper para criar um Label + Widget de Input + FeedbackLabel.
// `enabled` é um booleano para controlar a "editabilidade" (foco, processamento de eventos).
func (p *ForgotPasswordPage) labeledInput(gtx layout.Context, th *material.Theme, labelText string, inputWidgetlayout layout.Widget, feedbackText string, enabled bool) layout.Dimensions {
	// Se não estiver habilitado, pode-se alterar a cor do label para indicar.
	label := material.Body1(th, labelText)
	if !enabled {
		label.Color = theme.Colors.TextMuted
	}

	// O inputWidget em si não é diretamente desabilitado aqui,
	// mas a lógica de eventos (Editor.Events, Clickable.Clicked)
	// deve verificar `enabled` ou `p.isLoading` antes de processar.

	return layout.Flex{Axis: layout.Vertical, Spacing: layout.Tight}.Layout(gtx,
		layout.Rigid(label.Layout),
		layout.Rigid(inputWidgetlayout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { // Feedback de erro
			if feedbackText != "" {
				feedbackLabel := material.Body2(th, feedbackText)
				feedbackLabel.Color = theme.Colors.Danger
				return layout.Inset{Top: unit.Dp(2)}.Layout(gtx, feedbackLabel.Layout)
			}
			return layout.Dimensions{}
		}),
	)
}

// --- Lógica de Ações ---

// handleRequestCode lida com a solicitação de um token de recuperação de senha.
func (p *ForgotPasswordPage) handleRequestCode() {
	p.clearAllFeedbacks()
	p.statusMessage = ""

	email := strings.TrimSpace(strings.ToLower(p.emailInput.Text()))
	if errVal := utils.ValidateEmail(email); errVal != nil {
		p.emailInputFeedback = errVal.Error() // Assume que ValidateEmail retorna um erro simples.
		p.router.GetAppWindow().Invalidate()
		return
	}

	p.isLoading = true
	p.statusMessage = "Enviando token de recuperação..."
	p.messageColor = theme.Colors.TextMuted
	p.spinner.Start(p.router.GetAppWindow().Context())
	p.router.GetAppWindow().Invalidate()

	go func(emailAddr string) {
		var opErr error
		// O IP do cliente é difícil de obter em um app desktop puro sem chamadas externas.
		// "DESKTOP_APP_IP_NA" é um placeholder.
		err := p.userService.InitiatePasswordReset(emailAddr, "DESKTOP_APP_IP_NA")
		if err != nil {
			opErr = err
		}

		p.router.GetAppWindow().Execute(func() {
			p.isLoading = false
			p.spinner.Stop(p.router.GetAppWindow().Context())
			if opErr != nil {
				p.statusMessage = fmt.Sprintf("Erro ao solicitar token: %v", opErr)
				p.messageColor = theme.Colors.Danger
				if errors.Is(opErr, appErrors.ErrConfiguration) {
					p.emailInputFeedback = "Serviço de e-mail temporariamente indisponível."
				} else {
					// Não dar feedback específico sobre se o email existe ou não, por segurança.
					p.emailInputFeedback = "Falha ao processar a solicitação. Tente novamente."
				}
			} else {
				// Mensagem genérica para não confirmar a existência do e-mail.
				p.statusMessage = "Se o e-mail fornecido estiver cadastrado, você receberá um token de recuperação. Verifique sua caixa de entrada (e pasta de spam)."
				p.messageColor = theme.Colors.Success
				p.targetEmail = emailAddr // Guarda o e-mail para a próxima etapa.
				p.currentState = fpStateEnterCodeAndPassword
				p.emailInput.SetText("") // Limpa o campo de e-mail para evitar reenvio acidental.
			}
			p.router.GetAppWindow().Invalidate()
		})
	}(email)
}

// handleConfirmReset lida com a confirmação da redefinição de senha.
func (p *ForgotPasswordPage) handleConfirmReset() {
	p.clearAllFeedbacks()
	p.statusMessage = ""

	token := strings.TrimSpace(p.codeInput.Text())
	newPassword := p.newPasswordInput.Text() // Não trim aqui, senha pode ter espaços.
	confirmNewPassword := p.confirmPasswordInput.Text()

	// Validações de UI.
	allValid := true
	if token == "" {
		p.codeInputFeedback = "Token de segurança é obrigatório."
		allValid = false
	}
	if newPassword == "" {
		p.newPasswordFeedback = "Nova senha é obrigatória."
		allValid = false
	}
	if confirmNewPassword == "" {
		p.confirmPasswordFeedback = "Confirmação da nova senha é obrigatória."
		allValid = false
	}
	if newPassword != "" && newPassword != confirmNewPassword {
		p.confirmPasswordFeedback = "As senhas não coincidem."
		p.newPasswordFeedback = "As senhas não coincidem." // Também no campo de nova senha
		allValid = false
	}
	// Validação de força da nova senha.
	if newPassword != "" {
		strength := utils.ValidatePasswordStrength(newPassword, p.cfg.PasswordMinLength)
		if !strength.IsValid {
			p.newPasswordFeedback = fmt.Sprintf("Senha fraca ou inválida: %s", strings.Join(strength.GetErrorDetailsList(), ", "))
			allValid = false
		}
	}

	if !allValid {
		p.router.GetAppWindow().Invalidate()
		return
	}

	p.isLoading = true
	p.statusMessage = "Redefinindo sua senha..."
	p.messageColor = theme.Colors.TextMuted
	p.spinner.Start(p.router.GetAppWindow().Context())
	p.router.GetAppWindow().Invalidate()

	go func(emailForReset, tkn, newPass string) {
		var opErr error
		err := p.userService.ConfirmPasswordReset(emailForReset, tkn, newPass)
		if err != nil {
			opErr = err
		}

		p.router.GetAppWindow().Execute(func() {
			p.isLoading = false
			p.spinner.Stop(p.router.GetAppWindow().Context())
			if opErr != nil {
				p.statusMessage = fmt.Sprintf("Erro ao redefinir senha: %v", opErr)
				p.messageColor = theme.Colors.Danger
				// Atribuir erro ao campo específico se possível.
				if errors.Is(opErr, appErrors.ErrInvalidCredentials) || errors.Is(opErr, appErrors.ErrTokenExpired) {
					p.codeInputFeedback = opErr.Error()
				} else if valErr, ok := opErr.(*appErrors.ValidationError); ok {
					if msg, found := valErr.Fields["new_password"]; found {
						p.newPasswordFeedback = msg
					} else { // Mensagem genérica do ValidationError se não for específica do campo.
						p.newPasswordFeedback = opErr.Error()
					}
				}
			} else {
				appLogger.Infof("Senha redefinida com sucesso para %s", emailForReset)
				// Navega para login com mensagem de sucesso.
				// AppWindow pode ter um método para mostrar uma mensagem global.
				p.router.GetAppWindow().ShowGlobalMessage(
					"Sucesso!",
					"Sua senha foi redefinida com sucesso. Por favor, faça login com sua nova senha.",
					true,          // Sucesso
					5*time.Second, // Auto-esconder após 5 segundos
				)
				p.router.NavigateTo(ui.PageLogin, nil) // Params podem ser usados para passar a mensagem se ShowGlobalMessage não existir.
			}
			p.router.GetAppWindow().Invalidate()
		})
	}(p.targetEmail, token, newPassword) // Usa p.targetEmail que foi guardado.
}
