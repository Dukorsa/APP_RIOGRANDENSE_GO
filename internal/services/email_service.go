// Em internal/services/email_service.go
package services

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"html/template" // Para templates HTML
	"net"
	"net/smtp"
	"path/filepath" // Mantido para filepath.Base se necessário, mas ParseFS lida com isso.
	"strings"
	texttemplate "text/template" // Para templates de texto plano
	"time"

	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/config"
	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/errors"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/appassets"      // <<< IMPORTADO O NOVO PACOTE
)

// EmailService define a interface para o serviço de e-mail.
type EmailService interface {
	SendEmail(to, subject, htmlBody, textBody, fromName string) error
	SendWelcomeEmail(to, username string, contextData map[string]interface{}) error
	SendPasswordResetCode(to, resetCode, requestIP string) error
	SendNotificationEmail(to, message, title, actionURL, actionText string) error
}

// emailServiceImpl implementa EmailService.
type emailServiceImpl struct {
	cfg                 *core.Config
	htmlTemplates       *template.Template     // Cache para templates HTML parseados
	textTemplates       *texttemplate.Template // Cache para templates de texto plano
	emailTemplateColors map[string]string
}

// NewEmailService cria uma nova instância de EmailService.
// Retorna um erro se a configuração de e-mail estiver incompleta.
func NewEmailService(cfg *core.Config) (EmailService, error) { // Assinatura original mantida
	if cfg.EmailSMTPServer == "" || cfg.EmailUser == "" || cfg.EmailPassword == "" || cfg.SupportEmail == "" {
		return nil, fmt.Errorf("%w: configuração de SMTP incompleta (servidor, usuário, senha ou e-mail de suporte faltando)", appErrors.ErrConfiguration)
	}

	// Parsear todos os templates HTML e de texto na inicialização usando appassets.EmailTemplatesFS
	// Os arquivos "notification.html", etc., estão na raiz do FS embutido.
	htmlTmpl, err := template.New("html").ParseFS(appassets.EmailTemplatesFS, "*.html")
	if err != nil {
		return nil, fmt.Errorf("falha ao parsear templates HTML de email: %w", err)
	}
	textTmpl, err := texttemplate.New("text").ParseFS(appassets.EmailTemplatesFS, "*.txt")
	if err != nil {
		return nil, fmt.Errorf("falha ao parsear templates de texto de email: %w", err)
	}

	// Cores para os templates de e-mail
	emailColors := map[string]string{
		"Primary":      "#1A659E",
		"PrimaryDark":  "#0F4C7B",
		"PrimaryLight": "#4D8DBC",
		"Grey50":       "#F8F9FA",
		"Grey100":      "#f1f3f5",
		"Grey300":      "#DEE2E6",
		"Border":       "#DEE2E6",
		"Text":         "#212529",
		"TextMuted":    "#6C757D",
	}

	appLogger.Info("EmailService inicializado e templates carregados de appassets.")
	return &emailServiceImpl{
		cfg:                 cfg,
		htmlTemplates:       htmlTmpl,
		textTemplates:       textTmpl,
		emailTemplateColors: emailColors,
	}, nil
}

// renderTemplate renderiza um template específico (HTML ou texto).
func (s *emailServiceImpl) renderTemplate(templateName string, data map[string]interface{}, isHTML bool) (string, error) {
	var tplBuffer bytes.Buffer
	var err error

	if data == nil {
		data = make(map[string]interface{})
	}

	if _, exists := data["AppName"]; !exists {
		data["AppName"] = s.cfg.AppName
	}
	if _, exists := data["SupportEmail"]; !exists {
		data["SupportEmail"] = s.cfg.SupportEmail
	}
	if _, exists := data["Year"]; !exists {
		data["Year"] = time.Now().Year()
	}
	if _, exists := data["Colors"]; !exists {
		data["Colors"] = s.emailTemplateColors
	}

	// O nome do template para ExecuteTemplate é o nome base do arquivo (ex: "notification.html").
	// ParseFS já registra os templates com seus nomes base.
	baseTemplateName := templateName // filepath.Base não é mais estritamente necessário aqui se templateName já é o base.

	if isHTML {
		if s.htmlTemplates == nil {
			return "", errors.New("templates HTML de email não inicializados")
		}
		if tmpl := s.htmlTemplates.Lookup(baseTemplateName); tmpl == nil {
			return "", fmt.Errorf("template HTML '%s' não encontrado no conjunto parseado", baseTemplateName)
		}
		err = s.htmlTemplates.ExecuteTemplate(&tplBuffer, baseTemplateName, data)
	} else {
		if s.textTemplates == nil {
			return "", errors.New("templates de texto de email não inicializados")
		}
		if tmpl := s.textTemplates.Lookup(baseTemplateName); tmpl == nil {
			return "", fmt.Errorf("template de texto '%s' não encontrado no conjunto parseado", baseTemplateName)
		}
		err = s.textTemplates.ExecuteTemplate(&tplBuffer, baseTemplateName, data)
	}

	if err != nil {
		appLogger.Errorf("Erro ao executar template '%s' (HTML: %t): %v", baseTemplateName, isHTML, err)
		return "", fmt.Errorf("%w: falha ao renderizar template '%s': %v", appErrors.ErrInternal, baseTemplateName, err)
	}
	return tplBuffer.String(), nil
}

// SendEmail envia um e-mail usando SMTP.
func (s *emailServiceImpl) SendEmail(to, subject, htmlBody, textBody, fromName string) error {
	if s.cfg.AppDebug {
		logMsg := fmt.Sprintf("\n--- SIMULAÇÃO DE E-MAIL ---\n"+
			"Para: %s\n"+
			"De: %s <%s>\n"+
			"Assunto: %s\n"+
			"--- Conteúdo HTML (início) ---\n%s\n"+
			"--- Conteúdo Texto (início) ---\n%s\n"+
			"--- FIM SIMULAÇÃO ---",
			to, fromName, s.cfg.EmailUser, subject,
			truncateForLog(htmlBody, 500),
			truncateForLog(textBody, 500))
		appLogger.Debug(logMsg)
		return nil
	}

	if s.cfg.EmailSMTPServer == "" || s.cfg.EmailUser == "" || s.cfg.EmailPassword == "" {
		return fmt.Errorf("%w: serviço de e-mail não configurado corretamente (faltam credenciais SMTP)", appErrors.ErrConfiguration)
	}

	smtpHost := s.cfg.EmailSMTPServer
	smtpPort := s.cfg.EmailPort
	smtpAuth := smtp.PlainAuth("", s.cfg.EmailUser, s.cfg.EmailPassword, smtpHost)

	fromHeaderValue := s.cfg.EmailUser
	if strings.TrimSpace(fromName) != "" {
		fromHeaderValue = fmt.Sprintf("%s <%s>", fromName, s.cfg.EmailUser)
	}

	var body strings.Builder
	body.WriteString(fmt.Sprintf("From: %s\r\n", fromHeaderValue))
	body.WriteString(fmt.Sprintf("To: %s\r\n", to))
	body.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	body.WriteString("MIME-Version: 1.0\r\n")

	if textBody != "" && htmlBody != "" {
		boundary := "----=_NextPart_GoAppRiograndense" + time.Now().Format("20060102150405.999999")
		body.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=\"%s\"\r\n\r\n", boundary))

		body.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		body.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
		body.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		body.WriteString(quotedPrintableEncode(textBody) + "\r\n\r\n")

		body.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		body.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
		body.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		body.WriteString(quotedPrintableEncode(htmlBody) + "\r\n\r\n")

		body.WriteString(fmt.Sprintf("--%s--\r\n", boundary))
	} else if htmlBody != "" {
		body.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
		body.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		body.WriteString(quotedPrintableEncode(htmlBody))
	} else if textBody != "" {
		body.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
		body.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		body.WriteString(quotedPrintableEncode(textBody))
	} else {
		return fmt.Errorf("%w: corpo do e-mail (HTML ou texto) não pode ser vazio", appErrors.ErrInvalidInput)
	}

	addr := fmt.Sprintf("%s:%d", smtpHost, smtpPort)
	var err error
	connTimeout := 15 * time.Second

	if s.cfg.EmailUseTLS && smtpPort != 465 { // STARTTLS
		conn, dialErr := net.DialTimeout("tcp", addr, connTimeout)
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
		defer client.Quit()

		tlsConfig := &tls.Config{ServerName: smtpHost}
		if err = client.StartTLS(tlsConfig); err != nil {
			appLogger.Errorf("Erro ao iniciar STARTTLS com %s: %v", smtpHost, err)
			return fmt.Errorf("%w: falha ao iniciar STARTTLS: %v", appErrors.ErrEmail, err)
		}
		if err = client.Auth(smtpAuth); err != nil {
			appLogger.Errorf("Erro de autenticação SMTP (STARTTLS) com %s: %v", smtpHost, err)
			return fmt.Errorf("%w: falha na autenticação SMTP (STARTTLS): %v", appErrors.ErrAuthentication, err)
		}
		err = sendMailUsingClient(client, s.cfg.EmailUser, to, []byte(body.String()))

	} else if s.cfg.EmailUseTLS && smtpPort == 465 { // SSL/TLS direto
		tlsConfig := &tls.Config{ServerName: smtpHost}
		tlsConn, dialErr := tls.DialWithDialer(&net.Dialer{Timeout: connTimeout}, "tcp", addr, tlsConfig)
		if dialErr != nil {
			appLogger.Errorf("Erro ao discar TLS para servidor SMTP (SSL/TLS) %s: %v", addr, dialErr)
			return fmt.Errorf("%w: falha ao conectar ao servidor SMTP (SSL/TLS): %v", appErrors.ErrEmail, dialErr)
		}
		defer tlsConn.Close()

		client, newClientErr := smtp.NewClient(tlsConn, smtpHost)
		if newClientErr != nil {
			appLogger.Errorf("Erro ao criar cliente SMTP (SSL/TLS) para %s: %v", smtpHost, newClientErr)
			return fmt.Errorf("%w: falha ao criar cliente SMTP (SSL/TLS): %v", appErrors.ErrEmail, newClientErr)
		}
		defer client.Quit()

		if err = client.Auth(smtpAuth); err != nil {
			appLogger.Errorf("Erro de autenticação SMTP (SSL/TLS) com %s: %v", smtpHost, err)
			return fmt.Errorf("%w: falha na autenticação SMTP (SSL/TLS): %v", appErrors.ErrAuthentication, err)
		}
		err = sendMailUsingClient(client, s.cfg.EmailUser, to, []byte(body.String()))

	} else { // SMTP não seguro
		err = smtp.SendMail(addr, smtpAuth, s.cfg.EmailUser, []string{to}, []byte(body.String()))
	}

	if err != nil {
		appLogger.Errorf("Erro ao enviar e-mail para %s via %s: %v", to, addr, err)
		if strings.Contains(strings.ToLower(err.Error()), "authentication") {
			return fmt.Errorf("%w: %v", appErrors.ErrAuthentication, err)
		}
		return fmt.Errorf("%w: falha ao enviar e-mail: %v", appErrors.ErrEmail, err)
	}

	appLogger.Infof("E-mail enviado com sucesso para %s (Assunto: %s)", to, subject)
	return nil
}

// sendMailUsingClient é um helper para o fluxo de envio após conexão e auth.
func sendMailUsingClient(client *smtp.Client, from, to string, msg []byte) error {
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("erro no comando MAIL FROM: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("erro no comando RCPT TO: %w", err)
	}
	w, errData := client.Data()
	if errData != nil {
		return fmt.Errorf("erro no comando DATA: %w", errData)
	}
	defer w.Close() // Certifique-se de fechar o writer de dados
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("erro ao escrever corpo do email: %w", err)
	}
	return nil
}

// SendWelcomeEmail envia um e-mail de boas-vindas.
func (s *emailServiceImpl) SendWelcomeEmail(to, username string, contextData map[string]interface{}) error {
	if contextData == nil {
		contextData = make(map[string]interface{})
	}
	contextData["Username"] = username
	contextData["Email"] = to

	htmlBody, err := s.renderTemplate("welcome_email.html", contextData, true)
	if err != nil {
		return err
	}
	textBody, err := s.renderTemplate("welcome_email.txt", contextData, false)
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("Bem-vindo(a) ao %s!", s.cfg.AppName)
	return s.SendEmail(to, subject, htmlBody, textBody, fmt.Sprintf("Equipe %s", s.cfg.AppName))
}

// SendPasswordResetCode envia o código de redefinição de senha.
func (s *emailServiceImpl) SendPasswordResetCode(to, resetCode, requestIP string) error {
	contextData := map[string]interface{}{
		"ResetCode":    resetCode,
		"CodeValidity": s.cfg.PasswordResetTimeout.Minutes(),
		"RequestIP":    requestIP,
		"Timestamp":    time.Now().Format("02/01/2006 15:04:05 MST"),
	}

	htmlBody, err := s.renderTemplate("password_reset.html", contextData, true)
	if err != nil {
		// Verificar se é porque o template "password_reset.html" não foi encontrado.
		// O arquivo de texto fornecido para `assets/emails/password_reset.html` continha código Go,
		// e não HTML. Isso causaria um erro no `template.New("html").ParseFS(appassets.EmailTemplatesFS, "*.html")`
		// ou no `renderTemplate` se o template for encontrado mas não for HTML válido.
		// Se o erro for de template não encontrado, significa que o ParseFS inicial falhou para este template.
		appLogger.Errorf("Erro ao renderizar password_reset.html: %v. Verifique se o template existe e é HTML válido.", err)
		// Por enquanto, retornamos o erro. Pode-se tentar enviar apenas a versão em texto como fallback.
		return err
	}
	textBody, err := s.renderTemplate("password_reset.txt", contextData, false)
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("Código de Redefinição de Senha - %s", s.cfg.AppName)
	return s.SendEmail(to, subject, htmlBody, textBody, fmt.Sprintf("Segurança %s", s.cfg.AppName))
}

// SendNotificationEmail envia um e-mail de notificação genérico.
func (s *emailServiceImpl) SendNotificationEmail(to, message, title, actionURL, actionText string) error {
	contextData := map[string]interface{}{
		"Title":      title,
		"Message":    template.HTML(message), // Permite HTML na mensagem.
		"ActionURL":  actionURL,
		"ActionText": actionText,
	}

	htmlBody, err := s.renderTemplate("notification.html", contextData, true)
	if err != nil {
		return err
	}

	// Tenta renderizar "notification.txt". Se não existir ou falhar, usa um fallback.
	textBody, errTxt := s.renderTemplate("notification.txt", contextData, false)
	if errTxt != nil {
		appLogger.Warnf("Template notification.txt não encontrado ou falhou ao renderizar: %v. Usando fallback para texto.", errTxt)
		var textBodyFallback strings.Builder
		textBodyFallback.WriteString(title + "\n\n")
		textBodyFallback.WriteString(message + "\n\n")
		if actionURL != "" {
			linkText := "Acessar"
			if actionText != "" {
				linkText = actionText
			}
			textBodyFallback.WriteString(fmt.Sprintf("%s: %s\n", linkText, actionURL))
		}
		textBodyFallback.WriteString(fmt.Sprintf("\n\nAtenciosamente,\nEquipe %s", s.cfg.AppName))
		textBody = textBodyFallback.String()
	}

	subject := fmt.Sprintf("%s - %s", title, s.cfg.AppName)
	return s.SendEmail(to, subject, htmlBody, textBody, s.cfg.AppName)
}

// truncateForLog trunca uma string para um tamanho máximo para logging.
func truncateForLog(s string, maxLength int) string {
	if len(s) > maxLength {
		return s[:maxLength-3] + "..."
	}
	return s
}

// quotedPrintableEncode simula uma codificação quoted-printable básica.
func quotedPrintableEncode(body string) string {
	var encoded strings.Builder
	lineLen := 0
	for _, r := range body {
		if r == '=' || r > '~' || (r < ' ' && r != '\n' && r != '\r' && r != '\t') {
			encoded.WriteString(fmt.Sprintf("=%02X", byte(r)))
			lineLen += 3
		} else {
			encoded.WriteRune(r)
			lineLen++
		}
		if lineLen >= 72 && r != '\n' && r != '\r' {
			encoded.WriteString("=\r\n")
			lineLen = 0
		}
		if r == '\n' || r == '\r' {
			lineLen = 0
		}
	}
	return encoded.String()
}