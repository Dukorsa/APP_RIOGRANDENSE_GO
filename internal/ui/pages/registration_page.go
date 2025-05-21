package pages

import (
	"errors"
	"fmt"
	"image/color"
	"regexp"
	"strings"

	// "time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/auth" // Para SessionData (se admin estiver criando)
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core"
	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/errors"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/models"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/services"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui/components"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui/theme"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/utils" // Para validadores
)

// RegistrationPage gerencia a UI para cadastro de novos usuários.
type RegistrationPage struct {
	router      *ui.Router
	cfg         *core.Config
	userService services.UserService
	// Se admin puder criar usuários aqui, precisaria da sessão do admin
	adminSession *auth.SessionData // nil se for auto-registro

	isLoading     bool
	statusMessage string
	messageColor  color.NRGBA

	// Campos de entrada
	usernameInput        widget.Editor
	emailInput           widget.Editor
	passwordInput        *components.PasswordInput
	confirmPasswordInput *components.PasswordInput

	// Feedback para inputs
	usernameFeedback        string
	emailFeedback           string
	passwordFeedback        string // Para força da senha
	confirmPasswordFeedback string // Para "senhas não coincidem"

	// Botões
	registerBtn widget.Clickable
	cancelBtn   widget.Clickable // Ou "Voltar para Login"

	spinner *components.LoadingSpinner
}

// NewRegistrationPage cria uma nova instância da página de cadastro.
func NewRegistrationPage(
	router *ui.Router,
	cfg *core.Config,
	userSvc services.UserService,
	adminSess *auth.SessionData, // Pode ser nil para auto-registro
) *RegistrationPage {
	th := router.GetAppWindow().Theme() // Assumindo que AppWindow tem Theme()
	p := &RegistrationPage{
		router:       router,
		cfg:          cfg,
		userService:  userSvc,
		adminSession: adminSess,
		spinner:      components.NewLoadingSpinner(),
	}

	p.usernameInput.SingleLine = true
	p.usernameInput.Hint = "Nome de usuário (login)"

	p.emailInput.SingleLine = true
	p.emailInput.Hint = "Seu endereço de e-mail"

	p.passwordInput = components.NewPasswordInput(th, cfg)
	p.passwordInput.SetHint(fmt.Sprintf("Senha (mín. %d caracteres)", cfg.PasswordMinLength))

	p.confirmPasswordInput = components.NewPasswordInput(th, cfg)
	p.confirmPasswordInput.SetHint("Confirme a senha")
	// TODO: Adicionar método ao PasswordInput para esconder a barra de força
	// p.confirmPasswordInput.ShowStrengthBar(false)

	return p
}

func (p *RegistrationPage) OnNavigatedTo(params interface{}) {
	appLogger.Info("Navegou para RegistrationPage")
	p.resetForm()
	// Se adminSession for passado via params, pode ser usado
	if sess, ok := params.(*auth.SessionData); ok {
		p.adminSession = sess
	}
}

func (p.RegistrationPage) OnNavigatedFrom() {
	appLogger.Info("Navegando para fora da RegistrationPage")
	p.isLoading = false
	p.spinner.Stop(p.router.GetAppWindow().Context())
}

func (p *RegistrationPage) resetForm() {
	p.isLoading = false
	p.statusMessage = ""
	p.usernameInput.SetText("")
	p.emailInput.SetText("")
	p.passwordInput.SetText("")
	p.confirmPasswordInput.SetText("")
	p.usernameFeedback = ""
	p.emailFeedback = ""
	p.passwordFeedback = ""
	p.confirmPasswordFeedback = ""
	p.spinner.Stop(p.router.GetAppWindow().Context())
}

func (p *RegistrationPage) Layout(gtx layout.Context) layout.Dimensions {
	th := p.router.GetAppWindow().Theme()

	// Processar eventos dos inputs para validação inline
	for _, e := range p.usernameInput.Events(gtx) {
		if _, ok := e.(widget.ChangeEvent); ok {
			p.validateUsernameUI()
			p.statusMessage = ""
		}
	}
	for _, e := range p.emailInput.Events(gtx) {
		if _, ok := e.(widget.ChangeEvent); ok {
			p.validateEmailUI()
			p.statusMessage = ""
		}
	}
	// Para PasswordInput, a validação de força é interna. Apenas a de "match" é aqui.
	// E o feedback da força vem do próprio PasswordInput.
	// Precisaríamos de um callback TextChanged no PasswordInput ou verificar eventos.
	// Exemplo: p.passwordInput.TextChanged = func(s string) { p.validatePasswordMatchUI(); p.statusMessage = "" }
	//          p.confirmPasswordInput.TextChanged = func(s string) { p.validatePasswordMatchUI(); p.statusMessage = "" }
	// Ou, para simplificar, validar tudo no submit.

	// Processar cliques nos botões
	if p.registerBtn.Clicked(gtx) {
		p.handleRegister()
	}
	if p.cancelBtn.Clicked(gtx) {
		if p.adminSession != nil { // Se admin está criando, talvez volte para Admin Page
			p.router.NavigateTo(ui.PageAdminPermissions, nil)
		} else { // Se auto-registro, volta para Login
			p.router.NavigateTo(ui.PageLogin, nil)
		}
	}

	titleText := "Criar Nova Conta"
	if p.adminSession != nil {
		titleText = "Cadastrar Novo Usuário (Admin)"
	}
	titleWidget := material.H5(th, titleText)
	titleWidget.Font.Weight = font.Bold

	// Layout da página
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(20)).Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Max.X = gtx.Dp(450) // Limita largura do formulário
				if gtx.Constraints.Max.X > gtx.Constraints.Min.X {
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
				}
				return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle, Spacing: layout.SpaceSides}.Layout(gtx,
					layout.Rigid(func(gtx C) D { // Logo
						// TODO: Adicionar o logo como no LoginPage
						return titleWidget.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: theme.LargeVSpacer}.Layout),

					layout.Rigid(p.labeledEditor(gtx, th, "Usuário (login):*", material.Editor(th, &p.usernameInput, p.usernameInput.Hint).Layout, p.usernameFeedback)),
					layout.Rigid(layout.Spacer{Height: theme.DefaultVSpacer}.Layout),
					layout.Rigid(p.labeledEditor(gtx, th, "E-mail:*", material.Editor(th, &p.emailInput, p.emailInput.Hint).Layout, p.emailFeedback)),
					layout.Rigid(layout.Spacer{Height: theme.DefaultVSpacer}.Layout),
					layout.Rigid(p.labeledEditor(gtx, th, "Senha:*", p.passwordInput.Layout(gtx, th), p.passwordFeedback)),
					layout.Rigid(layout.Spacer{Height: theme.DefaultVSpacer}.Layout),
					layout.Rigid(p.labeledEditor(gtx, th, "Confirmar Senha:*", p.confirmPasswordInput.Layout(gtx, th), p.confirmPasswordFeedback)),

					layout.Rigid(layout.Spacer{Height: theme.LargeVSpacer}.Layout),

					// Botões
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						btnReg := material.Button(th, &p.registerBtn, "Cadastrar")
						btnReg.Background = theme.Colors.Primary
						// btnReg.Enabled = !p.isLoading // Controlar habilitação

						btnCan := material.Button(th, &p.cancelBtn, "Cancelar")
						// btnCan.Enabled = !p.isLoading

						return layout.Flex{Spacing: layout.SpaceBetween}.Layout(gtx,
							layout.Flexed(1, btnCan.Layout),
							layout.Flexed(1, btnReg.Layout),
						)
					}),
					// Mensagem de Status Global
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if p.statusMessage != "" {
							lbl := material.Body2(th, p.statusMessage)
							lbl.Color = p.messageColor
							return layout.Inset{Top: theme.DefaultVSpacer}.Layout(gtx, layout.Center.Layout(gtx, lbl.Layout))
						}
						return layout.Dimensions{}
					}),
					// Spinner
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if p.isLoading {
							// return p.spinner.Layout(gtx) // Precisa de layout.Stack
						}
						return layout.Dimensions{}
					}),
				)
			})
	})
}

func (p *RegistrationPage) labeledEditor(gtx layout.Context, th *material.Theme, labelText string, editorWidget layout.Widget, feedbackText string) layout.Dimensions {
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

// --- Lógica de Validação e Ação ---

func (p *RegistrationPage) validateAllFields() bool {
	p.clearAllFeedbacks()
	allValid := true

	if !p.validateUsernameUI() {
		allValid = false
	}
	if !p.validateEmailUI() {
		allValid = false
	}

	password := p.passwordInput.Text()
	confirmPassword := p.confirmPasswordInput.Text()

	if password == "" {
		p.passwordFeedback = "Senha é obrigatória."
		allValid = false
	} else {
		strength := utils.ValidatePasswordStrength(password, p.cfg.PasswordMinLength)
		if !strength.IsValid {
			p.passwordFeedback = fmt.Sprintf("Senha fraca: %s", strings.Join(strength.GetErrorDetailsList(), ", "))
			allValid = false
		}
	}

	if confirmPassword == "" {
		p.confirmPasswordFeedback = "Confirmação de senha é obrigatória."
		allValid = false
	} else if password != "" && password != confirmPassword {
		p.confirmPasswordFeedback = "As senhas não coincidem."
		allValid = false
	}

	if !allValid {
		p.router.GetAppWindow().Invalidate() // Para mostrar feedbacks
	}
	return allValid
}

func (p *RegistrationPage) validateUsernameUI() bool {
	username := strings.TrimSpace(p.usernameInput.Text())
	p.usernameFeedback = "" // Limpa antes
	if username == "" {
		p.usernameFeedback = "Nome de usuário é obrigatório."
		return false
	}
	// Regex do Python: ^[a-zA-Z0-9_-]{3,50}$
	// Go regex não tem \w para alfanumérico + underscore diretamente como Python
	// Use [a-zA-Z0-9_] ou \p{L}\p{N}_
	// Vamos usar uma regex mais simples para o exemplo.
	// O validador em `models.UserCreate.CleanAndValidate` e `utils` deve ser mais robusto.
	if matched, _ := regexp.MatchString(`^[a-zA-Z0-9_-]{3,50}$`, username); !matched {
		p.usernameFeedback = "Usuário deve ter 3-50 caracteres (letras, números, _, -)."
		return false
	}
	return true
}

func (p *RegistrationPage) validateEmailUI() bool {
	email := strings.TrimSpace(strings.ToLower(p.emailInput.Text()))
	p.emailFeedback = "" // Limpa antes
	if email == "" {
		p.emailFeedback = "E-mail é obrigatório."
		return false
	}
	if err := utils.ValidateEmail(email); err != nil { // Supondo que utils.ValidateEmail retorna erro
		p.emailFeedback = err.Error()
		return false
	}
	return true
}

func (p *RegistrationPage) clearAllFeedbacks() {
	p.usernameFeedback = ""
	p.emailFeedback = ""
	p.passwordFeedback = ""
	p.confirmPasswordFeedback = ""
	p.statusMessage = ""
}

func (p *RegistrationPage) handleRegister() {
	if p.isLoading {
		return
	}

	if !p.validateAllFields() {
		appLogger.Warn("Validação da UI de cadastro falhou.")
		return
	}

	p.isLoading = true
	p.statusMessage = "Cadastrando usuário..."
	p.messageColor = theme.Colors.TextMuted
	p.spinner.Start(p.router.GetAppWindow().Context())
	p.router.GetAppWindow().Invalidate()

	// Coleta os dados
	userData := models.UserCreate{
		Username: p.usernameInput.Text(), // CleanAndValidate do modelo fará trim e toLower
		Email:    p.emailInput.Text(),    // CleanAndValidate do modelo fará trim e toLower
		Password: p.passwordInput.Text(),
		// RoleNames será ["user"] por padrão se não alterado no modelo ou serviço
	}
	// Se admin estiver criando, poderia permitir seleção de roles
	// if p.adminSession != nil { userData.RoleNames = p.getSelectedRolesFromUI() }

	go func(ud models.UserCreate, adminSess *auth.SessionData) {
		var opErr error
		var successMsg string

		createdUser, err := p.userService.CreateUser(ud, adminSess)
		if err != nil {
			opErr = err
		} else {
			successMsg = fmt.Sprintf("Usuário '%s' cadastrado com sucesso!", createdUser.Username)
		}

		p.router.GetAppWindow().Execute(func() {
			p.isLoading = false
			p.spinner.Stop(p.router.GetAppWindow().Context())
			if opErr != nil {
				p.statusMessage = fmt.Sprintf("Erro no cadastro: %v", opErr)
				p.messageColor = theme.Colors.Danger
				appLogger.Errorf("Erro ao cadastrar usuário '%s': %v", ud.Username, opErr)
				// Tentar mostrar erro no campo específico, se possível
				if valErr, ok := opErr.(*appErrors.ValidationError); ok {
					if msg, found := valErr.Fields["username"]; found {
						p.usernameFeedback = msg
					}
					if msg, found := valErr.Fields["email"]; found {
						p.emailFeedback = msg
					}
					if msg, found := valErr.Fields["password"]; found {
						p.passwordFeedback = msg
					}
				} else if errors.Is(opErr, appErrors.ErrConflict) {
					if strings.Contains(opErr.Error(), "usuário") {
						p.usernameFeedback = opErr.Error()
					}
					if strings.Contains(opErr.Error(), "e-mail") {
						p.emailFeedback = opErr.Error()
					}
				}
			} else {
				appLogger.Infof(successMsg)
				//p.statusMessage = successMsg
				//p.messageColor = theme.Colors.Success
				//p.resetForm()
				// Navegar para login após sucesso
				p.router.GetAppWindow().ShowGlobalMessage("Cadastro Realizado", successMsg, false) // Supondo ShowGlobalMessage
				p.router.NavigateTo(ui.PageLogin, successMsg)
			}
			p.router.GetAppWindow().Invalidate()
		})
	}(userData, p.adminSession)
}
