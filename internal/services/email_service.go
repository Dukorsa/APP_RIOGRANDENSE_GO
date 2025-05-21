package services

import (
	"bytes"
	"crypto/tls"
	"embed" // Para embutir templates
	"errors"
	"fmt"
	"html/template" // Para templates HTML
	"net"
	"net/smtp"
	"path/filepath"
	"strings"
	texttemplate "text/template" // Para templates de texto plano
	"time"

	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core"
	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/errors"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	// "github.com/vanng822/go-premailer/premailer" // Opcional: para inlining de CSS
	// "github.com/jaytaylor/html2text" // Opcional: para converter HTML para texto plano
)

//go:embed assets_email_templates/*.html assets_email_templates/*.txt
var emailTemplatesFS embed.FS // Embutir os templates

const emailTemplatesDir = "assets_email_templates" // Nome do diretório dentro do embed.FS

// EmailService define a interface para o serviço de e-mail.
type EmailService interface {
	SendEmail(to, subject, htmlBody, textBody, fromName string) error
	SendWelcomeEmail(to, username string, context map[string]interface{}) error
	SendPasswordResetCode(to, resetCode, requestIP string) error
	SendNotificationEmail(to, message, title, actionURL, actionText string) error
}

// emailServiceImpl implementa EmailService.
type emailServiceImpl struct {
	cfg           *core.Config
	htmlTemplates *template.Template     // Cache para templates HTML parseados
	textTemplates *texttemplate.Template // Cache para templates de texto parseados
}

// NewEmailService cria uma nova instância de EmailService.
// Retorna um erro se a configuração de e-mail estiver incompleta.
func NewEmailService(cfg *core.Config) (*emailServiceImpl, error) {
	if cfg.EmailSMTPServer == "" || cfg.EmailUser == "" || cfg.EmailPassword == "" || cfg.SupportEmail == "" {
		return nil, fmt.Errorf("%w: configuração de SMTP incompleta (servidor, usuário, senha ou e-mail de suporte faltando)", appErrors.ErrConfiguration)
	}

	// Parsear todos os templates HTML e de texto na inicialização
	htmlTmpl, err := template.ParseFS(emailTemplatesFS, filepath.Join(emailTemplatesDir, "*.html"))
	if err != nil {
		return nil, fmt.Errorf("falha ao parsear templates HTML de email: %w", err)
	}
	textTmpl, err := texttemplate.ParseFS(emailTemplatesFS, filepath.Join(emailTemplatesDir, "*.txt"))
	if err != nil {
		return nil, fmt.Errorf("falha ao parsear templates de texto de email: %w", err)
	}

	appLogger.Info("EmailService inicializado e templates carregados.")
	return &emailServiceImpl{
		cfg:           cfg,
		htmlTemplates: htmlTmpl,
		textTemplates: textTmpl,
	}, nil
}

// renderTemplate renderiza um template específico (HTML ou texto).
func (s *emailServiceImpl) renderTemplate(templateName string, data interface{}, isHTML bool) (string, error) {
	var tplBuffer bytes.Buffer
	var err error

	// Adiciona dados globais/comuns a todos os templates
	// No Python, isso era feito com `COLORS`, `settings`, `app_name`, `support_email`, `now`
	// Aqui, podemos criar um wrapper ou adicionar diretamente ao `data`
	// Por simplicidade, vamos assumir que `data` (um map[string]interface{}) já os contém ou não são necessários.
	// Para `now` e `app_name`, podemos adicionar aqui se o chamador não fornecer.

	contextData, ok := data.(map[string]interface{})
	if !ok {
		// Se data não for um mapa, criar um novo e tentar colocar data nele.
		// Isso é uma simplificação; idealmente, data sempre seria map[string]interface{}.
		contextData = make(map[string]interface{})
		if data != nil {
			contextData["Data"] = data // Coloca o dado original sob uma chave "Data"
		}
	}

	if _, exists := contextData["AppName"]; !exists {
		contextData["AppName"] = s.cfg.AppName
	}
	if _, exists := contextData["SupportEmail"]; !exists {
		contextData["SupportEmail"] = s.cfg.SupportEmail
	}
	if _, exists := contextData["Year"]; !exists {
		contextData["Year"] = time.Now().Year()
	}
	// TODO: Mapear `COLORS` do Python para uma struct/map Go e passá-lo aqui se os templates precisarem
	// contextData["Colors"] = theme.EmailColors // Supondo que exista `theme.EmailColors`

	if isHTML {
		if s.htmlTemplates == nil {
			return "", errors.New("templates HTML não inicializados")
		}
		err = s.htmlTemplates.ExecuteTemplate(&tplBuffer, templateName, contextData)
	} else {
		if s.textTemplates == nil {
			return "", errors.New("templates de texto não inicializados")
		}
		err = s.textTemplates.ExecuteTemplate(&tplBuffer, templateName, contextData)
	}

	if err != nil {
		appLogger.Errorf("Erro ao executar template '%s' (HTML: %t): %v", templateName, isHTML, err)
		return "", fmt.Errorf("%w: falha ao renderizar template '%s': %v", appErrors.ErrInternal, templateName, err)
	}
	return tplBuffer.String(), nil
}

// SendEmail envia um e-mail usando SMTP.
func (s *emailServiceImpl) SendEmail(to, subject, htmlBody, textBody, fromName string) error {
	if s.cfg.AppDebug {
		appLogger.Debugf("--- SIMULAÇÃO DE E-MAIL ---")
		appLogger.Debugf("Para: %s", to)
		appLogger.Debugf("De: %s <%s>", fromName, s.cfg.EmailUser)
		appLogger.Debugf("Assunto: %s", subject)
		appLogger.Debugf("--- Conteúdo HTML (início) ---")
		if len(htmlBody) > 500 {
			appLogger.Debugf("%s...", htmlBody[:500])
		} else {
			appLogger.Debug(htmlBody)
		}
		appLogger.Debugf("--- Conteúdo Texto (início) ---")
		if len(textBody) > 500 {
			appLogger.Debugf("%s...", textBody[:500])
		} else {
			appLogger.Debug(textBody)
		}
		appLogger.Debugf("--- FIM SIMULAÇÃO ---")
		return nil
	}

	if s.cfg.EmailSMTPServer == "" || s.cfg.EmailUser == "" || s.cfg.EmailPassword == "" {
		return fmt.Errorf("%w: serviço de e-mail não configurado corretamente (faltam credenciais SMTP)", appErrors.ErrConfiguration)
	}

	smtpHost := s.cfg.EmailSMTPServer
	smtpPort := s.cfg.EmailPort
	auth := smtp.PlainAuth("", s.cfg.EmailUser, s.cfg.EmailPassword, smtpHost)

	// From header
	fromHeader := s.cfg.EmailUser
	if fromName != "" {
		fromHeader = fmt.Sprintf("%s <%s>", fromName, s.cfg.EmailUser)
	}

	// Montar a mensagem MIME
	// A ordem das partes é importante para clientes de e-mail (texto primeiro, depois HTML)
	var body strings.Builder
	body.WriteString(fmt.Sprintf("From: %s\r\n", fromHeader))
	body.WriteString(fmt.Sprintf("To: %s\r\n", to))
	body.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	body.WriteString("MIME-Version: 1.0\r\n")

	if textBody != "" && htmlBody != "" {
		boundary := "===============BOUNDARYGOEMAILSERVICE=="
		body.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=%s\r\n\r\n", boundary))

		// Parte Texto
		body.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		body.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
		body.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n") // ou quoted-printable
		body.WriteString(textBody + "\r\n\r\n")

		// Parte HTML
		body.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		body.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
		body.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n") // ou quoted-printable
		body.WriteString(htmlBody + "\r\n\r\n")

		body.WriteString(fmt.Sprintf("--%s--\r\n", boundary))

	} else if htmlBody != "" {
		body.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
		body.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
		body.WriteString(htmlBody + "\r\n")
	} else if textBody != "" {
		body.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
		body.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
		body.WriteString(textBody + "\r\n")
	} else {
		return fmt.Errorf("%w: corpo do e-mail (HTML ou texto) não pode ser vazio", appErrors.ErrInvalidInput)
	}

	addr := fmt.Sprintf("%s:%d", smtpHost, smtpPort)

	var err error
	if s.cfg.EmailUseTLS && smtpPort != 465 { // STARTTLS
		// Conexão TCP normal
		conn, dialErr := net.DialTimeout("tcp", addr, 10*time.Second) // Timeout para conexão
		if dialErr != nil {
			appLogger.Errorf("Erro ao discar para servidor SMTP (STARTTLS) %s: %v", addr, dialErr)
			return fmt.Errorf("%w: falha ao conectar ao servidor SMTP (STARTTLS): %v", appErrors.ErrEmail, dialErr)
		}
		defer conn.Close()

		client, newClientErr := smtp.NewClient(conn, smtpHost)
		if newClientErr != nil {
			appLogger.Errorf("Erro ao criar cliente SMTP (STARTTLS) para %s: %v", smtpHost, newClientErr)
			return fmt.Errorf("%w: falha ao criar cliente SMTP (STARTTLS): %v", appErrors.ErrEmail, newClientErr)
		}
		defer client.Close() // Garante que Quit() seja chamado

		// Configuração TLS
		tlsConfig := &tls.Config{
			ServerName: smtpHost,
			// InsecureSkipVerify: true, // NÃO use em produção, a menos que estritamente necessário e compreendido
		}
		if err = client.StartTLS(tlsConfig); err != nil {
			appLogger.Errorf("Erro ao iniciar STARTTLS com %s: %v", smtpHost, err)
			return fmt.Errorf("%w: falha ao iniciar STARTTLS: %v", appErrors.ErrEmail, err)
		}
		// Autenticar após STARTTLS
		if err = client.Auth(auth); err != nil {
			appLogger.Errorf("Erro de autenticação SMTP (STARTTLS) com %s: %v", smtpHost, err)
			return fmt.Errorf("%w: falha na autenticação SMTP (STARTTLS): %v", appErrors.ErrAuthentication, err)
		}
		// Enviar e-mail usando o cliente SMTP
		if err = client.Mail(s.cfg.EmailUser); err != nil {
			return fmt.Errorf("%w: erro no comando MAIL: %v", appErrors.ErrEmail, err)
		}
		if err = client.Rcpt(to); err != nil {
			return fmt.Errorf("%w: erro no comando RCPT: %v", appErrors.ErrEmail, err)
		}
		w, errData := client.Data()
		if errData != nil {
			return fmt.Errorf("%w: erro no comando DATA: %v", appErrors.ErrEmail, errData)
		}
		_, err = w.Write([]byte(body.String()))
		if err != nil {
			return fmt.Errorf("%w: erro ao escrever corpo do email: %v", appErrors.ErrEmail, err)
		}
		err = w.Close()
		if err != nil {
			return fmt.Errorf("%w: erro ao fechar data writer: %v", appErrors.ErrEmail, err)
		}
		// client.Quit() é chamado pelo defer client.Close()
	} else { // SSL/TLS direto (geralmente porta 465)
		err = smtp.SendMail(addr, auth, s.cfg.EmailUser, []string{to}, []byte(body.String()))
	}

	if err != nil {
		appLogger.Errorf("Erro ao enviar e-mail para %s via %s: %v", to, addr, err)
		// Mapear erros SMTP específicos se necessário
		if strings.Contains(err.Error(), "authentication failed") || strings.Contains(err.Error(), "Username and Password not accepted") {
			return fmt.Errorf("%w: %v", appErrors.ErrAuthentication, err)
		}
		return fmt.Errorf("%w: falha ao enviar e-mail: %v", appErrors.ErrEmail, err)
	}

	appLogger.Infof("E-mail enviado com sucesso para %s (Assunto: %s)", to, subject)
	return nil
}

// SendWelcomeEmail envia um e-mail de boas-vindas.
func (s *emailServiceImpl) SendWelcomeEmail(to, username string, context map[string]interface{}) error {
	if context == nil {
		context = make(map[string]interface{})
	}
	context["username"] = username
	context["email"] = to // O template Python esperava 'email'

	htmlBody, err := s.renderTemplate("welcome_email.html", context, true)
	if err != nil {
		return err // Erro já logado por renderTemplate
	}
	textBody, err := s.renderTemplate("welcome_email.txt", context, false)
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("Bem-vindo ao %s!", s.cfg.AppName)
	return s.SendEmail(to, subject, htmlBody, textBody, fmt.Sprintf("Equipe %s", s.cfg.AppName))
}

// SendPasswordResetCode envia o código de redefinição de senha.
func (s *emailServiceImpl) SendPasswordResetCode(to, resetCode, requestIP string) error {
	context := map[string]interface{}{
		"reset_code":    resetCode,
		"code_validity": s.cfg.PasswordResetTimeout.Minutes(), // Em minutos
		"request_ip":    requestIP,
		"timestamp":     time.Now().Format("02/01/2006 15:04:05"),
	}

	htmlBody, err := s.renderTemplate("password_reset.html", context, true)
	if err != nil {
		return err
	}
	textBody, err := s.renderTemplate("password_reset.txt", context, false)
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("Código de Redefinição de Senha - %s", s.cfg.AppName)
	return s.SendEmail(to, subject, htmlBody, textBody, fmt.Sprintf("Segurança %s", s.cfg.AppName))
}

// SendNotificationEmail envia um e-mail de notificação genérico.
func (s *emailServiceImpl) SendNotificationEmail(to, message, title, actionURL, actionText string) error {
	context := map[string]interface{}{
		"title":       title,
		"message":     template.HTML(message), // Permitir HTML na mensagem, o template deve usar {{.message}}
		"action_url":  actionURL,
		"action_text": actionText,
	}
	// Para o template de texto, precisamos de uma versão sem HTML da mensagem
	// textMessage := message // Simplificação, idealmente converter HTML para texto aqui se message tiver HTML
	// if html2text != nil { textMessage = html2text.HTML2Text(message) }

	htmlBody, err := s.renderTemplate("notification.html", context, true)
	if err != nil {
		return err
	}
	// Para o texto, usar uma renderização mais simples
	var textBodyBuffer bytes.Buffer
	textBodyBuffer.WriteString(title + "\n\n")
	textBodyBuffer.WriteString(message + "\n\n") // Aqui 'message' ainda pode ter HTML. Ideal seria converter.
	if actionURL != "" {
		linkText := "Acessar"
		if actionText != "" {
			linkText = actionText
		}
		textBodyBuffer.WriteString(fmt.Sprintf("%s: %s\n", linkText, actionURL))
	}
	textBody := textBodyBuffer.String()

	subject := fmt.Sprintf("%s - %s", title, s.cfg.AppName)
	return s.SendEmail(to, subject, htmlBody, textBody, s.cfg.AppName) // Remetente genérico
}
