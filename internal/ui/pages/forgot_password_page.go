package pages

import (
	"errors"
	"fmt"
	"image/color"
	"strings"

	// "time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	// Para SessionManager, se necessário obter IP (não diretamente aqui)
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core"
	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/errors"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"

	// "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/models"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/services"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui/components"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui/theme"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/utils" // Para ValidateEmail
)

type forgotPasswordState int

const (
	fpStateRequestToken forgotPasswordState = iota
	fpStateEnterCodeAndPassword
	// fpStateFinished // Pode ser apenas navegar para login
)

// ForgotPasswordPage gerencia a UI para recuperação de senha.
type ForgotPasswordPage struct {
	router      *ui.Router
	cfg         *core.Config
	userService services.UserService
	// emailService é usado indiretamente via userService.InitiatePasswordReset

	currentState  forgotPasswordState
	isLoading     bool
	statusMessage string // Para erros ou mensagens de sucesso
	messageColor  color.NRGBA

	// Campos de entrada e botões
	emailInput           widget.Editor
	codeInput            widget.Editor
	newPasswordInput     *components.PasswordInput // Usando o componente customizado
	confirmPasswordInput *components.PasswordInput // Para confirmação da nova senha

	requestCodeBtn  widget.Clickable
	confirmResetBtn widget.Clickable
	backToLoginBtn  widget.Clickable // Para voltar à tela de login

	// Feedback para inputs
	emailInputFeedback      string
	codeInputFeedback       string
	newPasswordFeedback     string
	confirmPasswordFeedback string

	targetEmail string // Armazena o e-mail para o qual o token foi solicitado

	spinner *components.LoadingSpinner
}

// NewForgotPasswordPage cria uma nova instância da página de recuperação de senha.
func NewForgotPasswordPage(
	router *ui.Router,
	cfg *core.Config,
	userSvc services.UserService,
) *ForgotPasswordPage {
	p := &ForgotPasswordPage{
		router:       router,
		cfg:          cfg,
		userService:  userSvc,
		currentState: fpStateRequestToken,
		spinner:      components.NewLoadingSpinner(),
	}
	p.emailInput.SingleLine = true
	p.emailInput.Hint = "Seu e-mail cadastrado"
	p.codeInput.SingleLine = true
	p.codeInput.Hint = "Token recebido por e-mail"

	// Passar o tema para os PasswordInputs
	// Idealmente, o tema é acessado via router.GetAppWindow().th
	th := router.GetAppWindow().Theme() // Supondo que AppWindow tem um método Theme()

	p.newPasswordInput = components.NewPasswordInput(th, cfg)
	p.newPasswordInput.SetHint(fmt.Sprintf("Nova senha (mín. %d caracteres)", cfg.PasswordMinLength))
	p.confirmPasswordInput = components.NewPasswordInput(th, cfg)
	p.confirmPasswordInput.SetHint("Confirme a nova senha")
	// Esconder barra de força para o campo de confirmação
	// (Precisa de um método no PasswordInput para controlar visibilidade da barra)
	// p.confirmPasswordInput.ShowStrengthBar(false)

	return p
}

func (p *ForgotPasswordPage) OnNavigatedTo(params interface{}) {
	appLogger.Info("Navegou para ForgotPasswordPage")
	p.resetForm()
}

func (p *ForgotPasswordPage) OnNavigatedFrom() {
	appLogger.Info("Navegando para fora da ForgotPasswordPage")
	// Não precisa fazer muito aqui, o estado é resetado em OnNavigatedTo
}

func (p *ForgotPasswordPage) resetForm() {
	p.currentState = fpStateRequestToken
	p.isLoading = false
	p.statusMessage = ""
	p.emailInput.SetText("")
	p.codeInput.SetText("")
	p.newPasswordInput.SetText("")
	p.confirmPasswordInput.SetText("")
	p.emailInputFeedback = ""
	p.codeInputFeedback = ""
	p.newPasswordFeedback = ""
	p.confirmPasswordFeedback = ""
	p.targetEmail = ""
	p.spinner.Stop(p.router.GetAppWindow().Context()) // Garante que o spinner pare
}

func (p *ForgotPasswordPage) Layout(gtx layout.Context) layout.Dimensions {
	th := p.router.GetAppWindow().Theme()

	// Processar eventos dos inputs
	for _, e := range p.emailInput.Events(gtx) {
		if _, ok := e.(widget.ChangeEvent); ok {
			p.emailInputFeedback = ""
			p.statusMessage = ""
		}
	}
	for _, e := range p.codeInput.Events(gtx) {
		if _, ok := e.(widget.ChangeEvent); ok {
			p.codeInputFeedback = ""
			p.statusMessage = ""
		}
	}
	// Para PasswordInput, precisaria de um callback TextChanged ou verificar eventos aqui
	// Supondo que PasswordInput tenha um callback TextChanged
	// p.newPasswordInput.TextChanged = func(s string) { p.newPasswordFeedback = ""; p.statusMessage = "" }
	// p.confirmPasswordInput.TextChanged = func(s string) { p.confirmPasswordFeedback = ""; p.statusMessage = "" }

	// Processar cliques nos botões
	if p.requestCodeBtn.Clicked(gtx) {
		p.handleRequestCode()
	}
	if p.confirmResetBtn.Clicked(gtx) {
		p.handleConfirmReset()
	}
	if p.backToLoginBtn.Clicked(gtx) {
		p.router.NavigateTo(ui.PageLogin, nil)
	}

	// Layout da página
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions { // Centraliza todo o conteúdo
		return layout.UniformInset(unit.Dp(20)).Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				// Limitar a largura do conteúdo
				return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Max.X = gtx.Dp(400) // Largura máxima do formulário
						if gtx.Constraints.Max.X > gtx.Constraints.Min.X {
							gtx.Constraints.Min.X = gtx.Constraints.Max.X // Força a largura
						}
						return p.layoutFormContent(gtx, th)
					}),
				)
			})
	})
}

func (p *ForgotPasswordPage) layoutFormContent(gtx layout.Context, th *material.Theme) layout.Dimensions {
	title := "Recuperar Senha"
	instruction := "Digite seu e-mail para receber as instruções e o token de recuperação."
	if p.currentState == fpStateEnterCodeAndPassword {
		validity := p.cfg.PasswordResetTimeout.Minutes()
		instruction = fmt.Sprintf("Insira o token recebido em <b>%s</b> e crie uma nova senha.<br>O token expira em %.0f minutos.", p.targetEmail, validity)
	}
	if p.isLoading {
		instruction = "Processando sua solicitação..."
	}

	// Styling para o título
	titleWidget := material.H5(th, title)
	titleWidget.Font.Weight = font.Bold

	// Styling para a instrução (material.Body1 não suporta HTML diretamente)
	// Você precisaria de um widget de texto rico ou parsear o HTML.
	// Por simplicidade, vamos usar texto plano.
	instructionText := strings.ReplaceAll(strings.ReplaceAll(instruction, "<b>", ""), "</b>", "")
	instructionText = strings.ReplaceAll(instructionText, "<br>", "\n")
	instructionLabel := material.Body1(th, instructionText)
	instructionLabel.Alignment = text.Start // Alinha à esquerda

	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle, Spacing: layout.SpaceSides}.Layout(gtx,
		layout.Rigid(titleWidget.Layout),
		layout.Rigid(layout.Spacer{Height: theme.DefaultVSpacer}.Layout),
		layout.Rigid(instructionLabel.Layout),
		layout.Rigid(layout.Spacer{Height: theme.LargeVSpacer}.Layout),

		// Email Input (sempre visível, mas habilitado/desabilitado)
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			ed := material.Editor(th, &p.emailInput, p.emailInput.Hint)
			// Desabilitar visualmente se não for a etapa de request code
			if p.currentState != fpStateRequestToken || p.isLoading {
				// ed.Fg = theme.Colors.TextMuted // Simula desabilitado
				// Editor do Gio não tem um "disabled" state visual fácil,
				// a interatividade é controlada pelo FocusPolicy e se ele processa eventos.
				// Para desabilitar, não adicione eventos para ele ou coloque um overlay.
			}
			return p.labeledEditor(gtx, th, "E-mail:", ed.Layout, p.emailInputFeedback)
		}),
		layout.Rigid(layout.Spacer{Height: theme.DefaultVSpacer}.Layout),

		// Campos para Etapa 2 (Token e Nova Senha)
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.currentState != fpStateEnterCodeAndPassword {
				return layout.Dimensions{} // Não desenha se não estiver nesta etapa
			}
			return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceSides}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { // Code Input
					ed := material.Editor(th, &p.codeInput, p.codeInput.Hint)
					return p.labeledEditor(gtx, th, "Token de Segurança:", ed.Layout, p.codeInputFeedback)
				}),
				layout.Rigid(layout.Spacer{Height: theme.DefaultVSpacer}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { // New Password
					return p.labeledEditor(gtx, th, "Nova Senha:", p.newPasswordInput.Layout(gtx, th), p.newPasswordFeedback)
				}),
				layout.Rigid(layout.Spacer{Height: theme.DefaultVSpacer}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { // Confirm New Password
					return p.labeledEditor(gtx, th, "Confirmar Nova Senha:", p.confirmPasswordInput.Layout(gtx, th), p.confirmPasswordFeedback)
				}),
			)
		}),
		layout.Rigid(layout.Spacer{Height: theme.LargeVSpacer}.Layout),

		// Botões
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			btnReq := material.Button(th, &p.requestCodeBtn, "Solicitar Token")
			btnConf := material.Button(th, &p.confirmResetBtn, "Redefinir Senha")
			btnBack := material.Button(th, &p.backToLoginBtn, "Voltar para Login")
			btnReq.Background = theme.Colors.Primary
			btnConf.Background = theme.Colors.Primary
			// TODO: Estilizar btnBack como secundário

			if p.isLoading { // Desabilita botões durante o carregamento
				// Para desabilitar em Gio, não processe o clique.
				// Visualmente, pode-se mudar a cor.
				// return layout.Dimensions{} // Ou mostrar botões desabilitados
			}

			if p.currentState == fpStateRequestToken {
				return btnReq.Layout(gtx)
			}
			return btnConf.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: theme.DefaultVSpacer}.Layout),
		layout.Rigid(material.Button(th, &p.backToLoginBtn, "Voltar para Login").Layout), // Sempre visível

		// Mensagem de Status
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.statusMessage != "" {
				lbl := material.Body2(th, p.statusMessage)
				lbl.Color = p.messageColor
				return layout.Inset{Top: theme.DefaultVSpacer}.Layout(gtx, layout.Center.Layout(gtx, lbl.Layout))
			}
			return layout.Dimensions{}
		}),
		// Spinner (precisa ser empilhado para sobrepor)
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.isLoading {
				// return p.spinner.Layout(gtx) // Precisa de layout.Stack
			}
			return layout.Dimensions{}
		}),
	)
}

func (p *ForgotPasswordPage) labeledEditor(gtx layout.Context, th *material.Theme, labelText string, editorWidget layout.Widget, feedbackText string) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical, Spacing: layout.Tight}.Layout(gtx,
		layout.Rigid(material.Body1(th, labelText).Layout),
		layout.Rigid(editorWidget),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if feedbackText != "" {
				lbl := material.Body2(th, feedbackText)
				lbl.Color = theme.Colors.Danger
				return layout.Inset{Top: unit.Dp(2)}.Layout(gtx, lbl.Layout)
			}
			return layout.Dimensions{}
		}),
	)
}

// --- Lógica de Ações ---

func (p *ForgotPasswordPage) handleRequestCode() {
	if p.isLoading {
		return
	}
	p.clearAllFeedbacks()
	p.statusMessage = ""

	email := strings.TrimSpace(strings.ToLower(p.emailInput.Text()))
	if err := utils.ValidateEmail(email); err != nil { // Supondo que ValidateEmail retorne erro
		p.emailInputFeedback = err.Error()
		p.router.GetAppWindow().Invalidate()
		return
	}

	p.isLoading = true
	p.statusMessage = "Enviando token..."
	p.messageColor = theme.Colors.TextMuted
	p.spinner.Start(p.router.GetAppWindow().Context())
	p.router.GetAppWindow().Invalidate()

	go func(emailAddr string) {
		var opErr error
		// TODO: Obter IP do cliente se possível (mais complexo em app desktop)
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
				if errors.Is(opErr, appErrors.ErrConfiguration) { // Se for erro de config de email
					p.emailInputFeedback = "Serviço de email indisponível."
				} else {
					p.emailInputFeedback = "Falha ao enviar. Tente novamente."
				}
			} else {
				p.statusMessage = fmt.Sprintf("Se %s estiver cadastrado, um token foi enviado. Verifique seu e-mail (e spam).", emailAddr)
				p.messageColor = theme.Colors.Success
				p.targetEmail = emailAddr
				p.currentState = fpStateEnterCodeAndPassword
			}
			p.router.GetAppWindow().Invalidate()
		})
	}(email)
}

func (p *ForgotPasswordPage) handleConfirmReset() {
	if p.isLoading {
		return
	}
	p.clearAllFeedbacks()
	p.statusMessage = ""

	token := strings.TrimSpace(p.codeInput.Text())
	newPassword := p.newPasswordInput.Text()
	confirmNewPassword := p.confirmPasswordInput.Text()

	// Validações
	valid := true
	if token == "" {
		p.codeInputFeedback = "Token é obrigatório."
		valid = false
	}
	if newPassword == "" {
		p.newPasswordFeedback = "Nova senha é obrigatória."
		valid = false
	}
	if confirmNewPassword == "" {
		p.confirmPasswordFeedback = "Confirmação de senha é obrigatória."
		valid = false
	}
	if newPassword != "" && newPassword != confirmNewPassword {
		p.confirmPasswordFeedback = "As senhas não coincidem."
		valid = false
	}
	if newPassword != "" {
		strength := utils.ValidatePasswordStrength(newPassword, p.cfg.PasswordMinLength)
		if !strength.IsValid {
			p.newPasswordFeedback = fmt.Sprintf("Senha fraca: %s", strings.Join(strength.GetErrorDetailsList(), ", ")) // Supondo GetErrorDetailsList
			valid = false
		}
	}
	if !valid {
		p.router.GetAppWindow().Invalidate()
		return
	}

	p.isLoading = true
	p.statusMessage = "Redefinindo senha..."
	p.messageColor = theme.Colors.TextMuted
	p.spinner.Start(p.router.GetAppWindow().Context())
	p.router.GetAppWindow().Invalidate()

	go func(emailAddr, tkn, newPass string) {
		var opErr error
		err := p.userService.ConfirmPasswordReset(emailAddr, tkn, newPass)
		if err != nil {
			opErr = err
		}

		p.router.GetAppWindow().Execute(func() {
			p.isLoading = false
			p.spinner.Stop(p.router.GetAppWindow().Context())
			if opErr != nil {
				p.statusMessage = fmt.Sprintf("Erro ao redefinir senha: %v", opErr)
				p.messageColor = theme.Colors.Danger
				if errors.Is(opErr, appErrors.ErrInvalidCredentials) || errors.Is(opErr, appErrors.ErrTokenExpired) {
					p.codeInputFeedback = opErr.Error()
				} else if _, ok := opErr.(*appErrors.ValidationError); ok { // Se for ValidationError
					// Tentar extrair detalhes, mas o erro já é a mensagem principal
					p.newPasswordFeedback = opErr.Error()
				}
			} else {
				// Sucesso! Mostrar mensagem e navegar para login.
				// Idealmente, a AppWindow teria um método para mostrar um diálogo global de sucesso.
				appLogger.Infof("Senha redefinida com sucesso para %s", emailAddr)
				// p.router.GetAppWindow().ShowGlobalMessage("Sucesso", "Senha redefinida! Faça login com a nova senha.", false)
				p.router.NavigateTo(ui.PageLogin, "Senha redefinida com sucesso!") // Passa mensagem para LoginPage
			}
			p.router.GetAppWindow().Invalidate()
		})
	}(p.targetEmail, token, newPassword)
}

func (p *ForgotPasswordPage) clearAllFeedbacks() {
	p.emailInputFeedback = ""
	p.codeInputFeedback = ""
	p.newPasswordFeedback = ""
	p.confirmPasswordFeedback = ""
}
