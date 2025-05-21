package pages

import (
	"errors"
	"fmt"
	"image/color"
	"regexp"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/auth" // Para SessionData (se admin estiver criando)
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/config"
	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/errors"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/models"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/services"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui/components"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/utils"
)

// RegistrationPage gerencia a UI para cadastro de novos usuários.
type RegistrationPage struct {
	router      *ui.Router
	cfg         *core.Config
	userService services.UserService
	// Se um administrador estiver criando o usuário, a sessão do admin é passada.
	// Para auto-registro, `adminSession` será nil.
	adminSession *auth.SessionData

	isLoading     bool        // True se uma operação de backend estiver em andamento.
	statusMessage string      // Para exibir mensagens de erro ou sucesso globais da página.
	messageColor  color.NRGBA // Cor da `statusMessage`.

	// Campos de entrada do formulário.
	usernameInput        widget.Editor
	emailInput           widget.Editor
	fullNameInput        widget.Editor             // Adicionado para nome completo.
	passwordInput        *components.PasswordInput // Componente customizado.
	confirmPasswordInput *components.PasswordInput // Para confirmação da nova senha.

	// Feedback para os campos de entrada (mensagens de erro de validação).
	usernameFeedback        string
	emailFeedback           string
	fullNameFeedback        string // Adicionado
	passwordFeedback        string // Para força da senha (geralmente gerenciado pelo PasswordInput).
	confirmPasswordFeedback string // Para "senhas não coincidem".

	// Botões de ação.
	registerBtn widget.Clickable
	cancelBtn   widget.Clickable // Ou "Voltar para Login".

	spinner *components.LoadingSpinner // Spinner de carregamento.
}

// NewRegistrationPage cria uma nova instância da página de cadastro.
func NewRegistrationPage(
	router *ui.Router,
	cfg *core.Config,
	userSvc services.UserService,
	adminSess *auth.SessionData, // Pode ser nil para auto-registro.
) *RegistrationPage {
	th := router.GetAppWindow().Theme() // Obtém o tema da AppWindow.

	p := &RegistrationPage{
		router:       router,
		cfg:          cfg,
		userService:  userSvc,
		adminSession: adminSess,
		spinner:      components.NewLoadingSpinner(theme.Colors.Primary),
	}

	p.usernameInput.SingleLine = true
	p.usernameInput.Hint = "Nome de usuário (para login)"

	p.emailInput.SingleLine = true
	p.emailInput.Hint = "Seu endereço de e-mail"

	p.fullNameInput.SingleLine = true
	p.fullNameInput.Hint = "Nome Completo (opcional)"

	p.passwordInput = components.NewPasswordInput(th, cfg)
	p.passwordInput.SetHint(fmt.Sprintf("Senha (mín. %d caracteres)", cfg.PasswordMinLength))

	p.confirmPasswordInput = components.NewPasswordInput(th, cfg)
	p.confirmPasswordInput.SetHint("Confirme a senha")
	p.confirmPasswordInput.ShowStrengthBar(false) // Não mostra barra de força para confirmação.

	return p
}

// OnNavigatedTo é chamado quando a página se torna ativa.
func (p *RegistrationPage) OnNavigatedTo(params interface{}) {
	appLogger.Info("Navegou para RegistrationPage")
	// Verifica se `params` é a sessão do admin, se esperado.
	if sess, ok := params.(*auth.SessionData); ok {
		p.adminSession = sess // Atualiza se admin está navegando para cá.
	} else if params != nil {
		// Se params não for nil e não for SessionData, pode ser um erro de navegação.
		appLogger.Warnf("RegistrationPage recebeu parâmetros inesperados: %T", params)
	}
	p.resetForm() // Reseta o formulário para um estado limpo.
}

// OnNavigatedFrom é chamado quando o router navega para fora desta página.
func (p *RegistrationPage) OnNavigatedFrom() {
	appLogger.Info("Navegando para fora da RegistrationPage")
	p.isLoading = false
	p.spinner.Stop(p.router.GetAppWindow().Context()) // Para o spinner.
}

// resetForm limpa todos os campos e mensagens de feedback da página.
func (p *RegistrationPage) resetForm() {
	p.isLoading = false
	p.statusMessage = ""
	p.usernameInput.SetText("")
	p.emailInput.SetText("")
	p.fullNameInput.SetText("")
	p.passwordInput.Clear()
	p.confirmPasswordInput.Clear()
	p.clearAllFeedbacks()
	p.spinner.Stop(p.router.GetAppWindow().Context())
}

// clearAllFeedbacks limpa as mensagens de erro dos campos de input.
func (p *RegistrationPage) clearAllFeedbacks() {
	p.usernameFeedback = ""
	p.emailFeedback = ""
	p.fullNameFeedback = ""
	p.passwordFeedback = "" // O PasswordInput pode gerenciar seu próprio feedback de força.
	p.confirmPasswordFeedback = ""
	p.statusMessage = "" // Limpa mensagem global da página também.
}

// Layout é o método principal de desenho da página.
func (p *RegistrationPage) Layout(gtx layout.Context) layout.Dimensions {
	th := p.router.GetAppWindow().Theme()

	// Processar eventos dos inputs para limpar feedback ao digitar.
	if p.usernameInput.Update(gtx) {
		p.usernameFeedback = ""
		p.statusMessage = ""
	}
	if p.emailInput.Update(gtx) {
		p.emailFeedback = ""
		p.statusMessage = ""
	}
	if p.fullNameInput.Update(gtx) {
		p.fullNameFeedback = ""
		p.statusMessage = ""
	}
	// O PasswordInput tem seu próprio Layout que lida com eventos internos.
	// Para limpar `passwordFeedback` ou `confirmPasswordFeedback` ao digitar neles,
	// seria necessário que PasswordInput emitisse um evento OnChange.

	// Processar cliques nos botões.
	if p.registerBtn.Clicked(gtx) && !p.isLoading {
		p.handleRegister()
	}
	if p.cancelBtn.Clicked(gtx) && !p.isLoading {
		if p.adminSession != nil { // Se admin está criando, volta para a página de admin.
			p.router.NavigateTo(ui.PageAdminPermissions, nil)
		} else { // Se auto-registro, volta para Login.
			p.router.NavigateTo(ui.PageLogin, nil)
		}
	}

	titleText := "Criar Nova Conta de Usuário"
	if p.adminSession != nil {
		titleText = "Cadastrar Novo Usuário (Admin)"
	}
	titleWidget := material.H5(th, titleText)
	titleWidget.Font.Weight = font.Bold
	titleWidget.Alignment = text.Middle

	// Layout centralizado na tela.
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(20)).Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				maxWidth := gtx.Dp(unit.Dp(450)) // Largura máxima do formulário.
				gtx.Constraints.Max.X = maxWidth
				if gtx.Constraints.Max.X > gtx.Constraints.Min.X {
					gtx.Constraints.Min.X = maxWidth
				}
				return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle, Spacing: layout.SpaceSides}.Layout(gtx,
					layout.Rigid(titleWidget.Layout),
					layout.Rigid(layout.Spacer{Height: theme.LargeVSpacer}.Layout),

					layout.Rigid(p.labeledInput(gtx, th, "Usuário (para login):*", material.Editor(th, &p.usernameInput, p.usernameInput.Hint).Layout, p.usernameFeedback)),
					layout.Rigid(layout.Spacer{Height: theme.DefaultVSpacer}.Layout),
					layout.Rigid(p.labeledInput(gtx, th, "E-mail:*", material.Editor(th, &p.emailInput, p.emailInput.Hint).Layout, p.emailFeedback)),
					layout.Rigid(layout.Spacer{Height: theme.DefaultVSpacer}.Layout),
					layout.Rigid(p.labeledInput(gtx, th, "Nome Completo (Opcional):", material.Editor(th, &p.fullNameInput, p.fullNameInput.Hint).Layout, p.fullNameFeedback)),
					layout.Rigid(layout.Spacer{Height: theme.DefaultVSpacer}.Layout),
					layout.Rigid(p.labeledInput(gtx, th, "Senha:*", p.passwordInput.Layout(gtx, th), p.passwordFeedback)), // PasswordInput já tem seu layout.
					layout.Rigid(layout.Spacer{Height: theme.DefaultVSpacer}.Layout),
					layout.Rigid(p.labeledInput(gtx, th, "Confirmar Senha:*", p.confirmPasswordInput.Layout(gtx, th), p.confirmPasswordFeedback)),

					layout.Rigid(layout.Spacer{Height: theme.LargeVSpacer}.Layout),

					// Botões ou Spinner.
					layout.Rigid(func(gtx C) D {
						if p.isLoading {
							return layout.Center.Layout(gtx, p.spinner.Layout)
						}
						btnReg := material.Button(th, &p.registerBtn, "Cadastrar Usuário")
						btnReg.Background = theme.Colors.Primary
						btnReg.Color = theme.Colors.PrimaryText
						btnReg.CornerRadius = theme.CornerRadius

						btnCan := material.Button(th, &p.cancelBtn, "Cancelar")
						// Estilo secundário para o botão Cancelar.
						btnCan.Background = color.NRGBA{} // Transparente
						btnCan.Color = theme.Colors.Primary

						return layout.Flex{Spacing: layout.SpaceBetween}.Layout(gtx,
							layout.Flexed(1, btnCan.Layout),
							layout.Flexed(1, btnReg.Layout),
						)
					}),
					// Mensagem de Status Global da página.
					layout.Rigid(func(gtx C) D {
						if p.statusMessage != "" && !p.isLoading {
							lbl := material.Body2(th, p.statusMessage)
							lbl.Color = p.messageColor
							lbl.Alignment = text.Middle
							return layout.Inset{Top: theme.DefaultVSpacer}.Layout(gtx, lbl.Layout)
						}
						return layout.Dimensions{}
					}),
				)
			})
	})
}

// labeledInput é um helper para criar um Label + Widget de Input + FeedbackLabel.
func (p *RegistrationPage) labeledInput(gtx layout.Context, th *material.Theme, labelText string, inputWidgetLayout layout.Widget, feedbackText string) layout.Dimensions {
	label := material.Body1(th, labelText)
	return layout.Flex{Axis: layout.Vertical, Spacing: layout.Tight}.Layout(gtx,
		layout.Rigid(label.Layout),
		layout.Rigid(inputWidgetLayout), // O widget de input (ex: material.Editor ou PasswordInput.Layout)
		layout.Rigid(func(gtx C) D { // Feedback de erro para o input
			if feedbackText != "" {
				feedbackLabel := material.Body2(th, feedbackText)
				feedbackLabel.Color = theme.Colors.Danger
				return layout.Inset{Top: unit.Dp(2)}.Layout(gtx, feedbackLabel.Layout)
			}
			return layout.Dimensions{}
		}),
	)
}

// --- Lógica de Validação e Ação ---

// validateAllFieldsUI valida todos os campos do formulário na UI e atualiza os feedbacks.
// Retorna true se todos os campos forem válidos.
func (p *RegistrationPage) validateAllFieldsUI() bool {
	p.clearAllFeedbacks() // Limpa feedbacks antigos antes de revalidar.
	allValid := true

	// Validar Username
	username := strings.TrimSpace(p.usernameInput.Text())
	if username == "" {
		p.usernameFeedback = "Nome de usuário é obrigatório."
		allValid = false
	} else if matched, _ := regexp.MatchString(`^[a-zA-Z0-9_-]{3,50}$`, username); !matched {
		// Usar regex do modelo UserCreate para consistência, ou utils.IsValidUsernameFormat
		p.usernameFeedback = "Usuário deve ter 3-50 caracteres (letras, números, _, -)."
		allValid = false
	}

	// Validar Email
	email := strings.TrimSpace(p.emailInput.Text())
	if email == "" {
		p.emailFeedback = "E-mail é obrigatório."
		allValid = false
	} else if errValEmail := utils.ValidateEmail(email); errValEmail != nil {
		p.emailFeedback = errValEmail.Error() // Assume que ValidateEmail retorna erro.
		allValid = false
	}

	// Validar FullName (opcional, mas com limite de tamanho se preenchido)
	fullName := strings.TrimSpace(p.fullNameInput.Text())
	if len(fullName) > 100 {
		p.fullNameFeedback = "Nome completo não pode exceder 100 caracteres."
		allValid = false
	}

	// Validar Senha
	password := p.passwordInput.Text() // Não trim aqui.
	if password == "" {
		p.passwordFeedback = "Senha é obrigatória." // PasswordInput pode já mostrar isso.
		allValid = false
	} else {
		strength := utils.ValidatePasswordStrength(password, p.cfg.PasswordMinLength)
		if !strength.IsValid {
			// PasswordInput já mostra a barra de força.
			// Este feedback é adicional, se necessário, ou pode ser omitido.
			p.passwordFeedback = fmt.Sprintf("Senha fraca ou inválida: %s", strings.Join(strength.GetErrorDetailsList(), ", "))
			allValid = false
		}
	}

	// Validar Confirmação de Senha
	confirmPassword := p.confirmPasswordInput.Text()
	if confirmPassword == "" {
		p.confirmPasswordFeedback = "Confirmação de senha é obrigatória."
		allValid = false
	} else if password != "" && password != confirmPassword {
		p.confirmPasswordFeedback = "As senhas não coincidem."
		p.passwordFeedback = "As senhas não coincidem." // Também no campo de senha
		allValid = false
	}

	if !allValid {
		p.router.GetAppWindow().Invalidate() // Atualiza a UI para mostrar todos os feedbacks.
	}
	return allValid
}

// handleRegister lida com a submissão do formulário de cadastro.
func (p *RegistrationPage) handleRegister() {
	if !p.validateAllFieldsUI() {
		appLogger.Warn("Validação da UI de cadastro falhou.")
		p.statusMessage = "Por favor, corrija os erros no formulário."
		p.messageColor = theme.Colors.Warning
		p.router.GetAppWindow().Invalidate()
		return
	}

	p.isLoading = true
	p.statusMessage = "Cadastrando usuário, por favor aguarde..."
	p.messageColor = theme.Colors.TextMuted
	p.spinner.Start(p.router.GetAppWindow().Context())
	p.router.GetAppWindow().Invalidate()

	// Coleta os dados para o serviço.
	// O método CleanAndValidate do modelo UserCreate fará a normalização final (ex: toLower).
	fullNameText := strings.TrimSpace(p.fullNameInput.Text())
	var fullNamePtr *string
	if fullNameText != "" {
		fullNamePtr = &fullNameText
	}

	userData := models.UserCreate{
		Username: strings.TrimSpace(p.usernameInput.Text()),
		Email:    strings.TrimSpace(p.emailInput.Text()),
		FullName: fullNamePtr,
		Password: p.passwordInput.Text(), // Senha bruta, será hasheada no serviço.
		// RoleNames: Por padrão, o serviço atribuirá ["user"] se for auto-registro
		// ou se admin não especificar roles.
		// Se admin puder especificar roles nesta página, eles seriam coletados aqui.
	}

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
				// Tenta atribuir erro ao campo específico se for ValidationError.
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
				} else if errors.Is(opErr, appErrors.ErrConflict) { // Conflito de username ou email
					if strings.Contains(strings.ToLower(opErr.Error()), "usuário") {
						p.usernameFeedback = opErr.Error()
					} else if strings.Contains(strings.ToLower(opErr.Error()), "e-mail") {
						p.emailFeedback = opErr.Error()
					} else {
						// Erro de conflito genérico
					}
				}
			} else {
				appLogger.Infof(successMsg)
				// Limpa formulário e navega para login com mensagem de sucesso.
				p.resetForm()
				targetPage := ui.PageLogin
				successParam := successMsg
				if adminSess != nil { // Se admin criou, volta para a página de admin e mostra mensagem lá.
					targetPage = ui.PageAdminPermissions
					p.router.GetAppWindow().ShowGlobalMessage("Sucesso", successMsg, true, 5*time.Second)
					successParam = "" // Não passa para AdminPermissionsPage
				} else {
					p.router.GetAppWindow().ShowGlobalMessage("Cadastro Realizado", successMsg, true, 5*time.Second)
				}
				p.router.NavigateTo(targetPage, successParam)
			}
			p.router.GetAppWindow().Invalidate()
		})
	}(userData, p.adminSession)
}
