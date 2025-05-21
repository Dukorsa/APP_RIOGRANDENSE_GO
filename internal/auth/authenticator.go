package auth

import (
	"database/sql" // Para sql.NullTime se usar ponteiros para time.Time
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core"
	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/errors" // Alias para evitar conflito
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/models"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/repositories"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/services"
	"github.com/sirupsen/logrus"
)

// AuthResult encapsula o resultado de uma operação de autenticação.
type AuthResult struct {
	Success    bool
	Message    string
	SessionID  string             // ID da sessão criada em caso de sucesso
	UserData   *models.UserPublic // Dados públicos do usuário
	RedirectTo ui.PageID          // Opcional: para onde redirecionar na UI
}

// Authenticator define a interface para operações de autenticação.
// Isso permite testar e mockar mais facilmente.
type AuthenticatorInterface interface {
	AuthenticateUser(username, password, ipAddress, userAgent string) (*AuthResult, error)
	LogoutUser(sessionID string) error
	// Outros métodos relacionados à autenticação poderiam ir aqui
}

// authenticatorImpl implementa AuthenticatorInterface.
type authenticatorImpl struct {
	cfg             *core.Config
	userRepo        repositories.UserRepository // Interface do repositório de usuário
	sessionManager  *SessionManager             // Gerenciador de sessão concreto
	auditLogService services.AuditLogService    // Interface do serviço de log de auditoria
}

// NewAuthenticator cria uma nova instância do Authenticator.
// Nota: db *sql.DB é passado aqui para que o authenticator possa criar transações se necessário,
// ou pode ser que os repositórios e o sessionManager já lidem com suas próprias conexões/transações.
// Se você estiver usando um ORM, você passaria a instância do ORM (ex: *gorm.DB).
func NewAuthenticator(
	cfg *core.Config,
	db *sql.DB, // Ou seu tipo de conexão ORM
	sessionManager *SessionManager,
	auditLogService services.AuditLogService,
) AuthenticatorInterface {
	// Dentro do NewAuthenticator, você instancia os repositórios concretos
	userRepo := repositories.NewSQLUserRepository(db) // Ou NewGORMUserRepository(db), etc.

	return &authenticatorImpl{
		cfg:             cfg,
		userRepo:        userRepo,
		sessionManager:  sessionManager,
		auditLogService: auditLogService,
	}
}

// HashPassword gera um hash bcrypt de uma senha.
func HashPassword(password string) (string, error) {
	if password == "" {
		return "", errors.New("senha não pode estar vazia")
	}
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		appLogger.Errorf("Erro ao gerar hash da senha: %v", err)
		return "", fmt.Errorf("%w: falha ao processar senha", appErrors.ErrInternal)
	}
	return string(hashedBytes), nil
}

// VerifyPassword compara uma senha em texto plano com um hash bcrypt.
func VerifyPassword(plainPassword, hashedPassword string) bool {
	if plainPassword == "" || hashedPassword == "" {
		return false
	}
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(plainPassword))
	if err == nil {
		appLogger.Debug("Verificação de senha (bcrypt.CompareHashAndPassword) bem-sucedida.")
	} else if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		appLogger.Debug("Verificação de senha (bcrypt.CompareHashAndPassword) falhou: hash e senha não correspondem.")
	} else {
		appLogger.Warnf("Erro inesperado durante verifyPassword (bcrypt.CompareHashAndPassword): %v. Hash recebido: '%s'", err, hashedPassword)
	}
	return err == nil
}

// AuthenticateUser autentica um usuário com base em username/email e senha.
func (a *authenticatorImpl) AuthenticateUser(usernameOrEmail, password, ipAddress, userAgent string) (*AuthResult, error) {
	normalizedInput := strings.ToLower(strings.TrimSpace(usernameOrEmail))
	logCtx := appLogger.WithFields(logrus.Fields{
		"input":     normalizedInput,
		"ipAddress": ipAddress,
		"userAgent": userAgent,
	})
	logCtx.Infof("Iniciando autenticação para: %s", normalizedInput)

	if normalizedInput == "" || password == "" {
		logCtx.Warn("Tentativa de login com nome de usuário/email ou senha vazios.")
		return &AuthResult{Success: false, Message: "Usuário/Email e senha são obrigatórios."}, nil
	}

	// 1. Buscar usuário
	user, err := a.userRepo.GetByUsernameOrEmail(normalizedInput) // Este método precisa ser implementado no repo
	if err != nil {
		if errors.Is(err, appErrors.ErrNotFound) {
			logCtx.Warn("Usuário não encontrado no banco de dados.")
			a.auditLogService.LogAction(models.AuditLogEntry{ // Supondo que LogAction aceite um struct
				Action:      "LOGIN_FAILED_USER_NOT_FOUND",
				Description: fmt.Sprintf("Tentativa de login para usuário inexistente: %s", normalizedInput),
				Severity:    "WARNING",
				Username:    "system", // Ou o próprio input se quiser registrar
				IPAddress:   &ipAddress,
				Metadata:    map[string]interface{}{"input": normalizedInput, "ip": ipAddress, "agent": userAgent},
			})
			return &AuthResult{Success: false, Message: "Usuário ou Senha inválidos."}, nil // Mensagem genérica
		}
		logCtx.Errorf("Erro ao buscar usuário: %v", err)
		return nil, fmt.Errorf("%w: falha ao verificar usuário", appErrors.ErrDatabase)
	}
	logCtx = logCtx.WithField("userID", user.ID.String()) // Adiciona userID ao contexto do log

	// 2. Verificar se a conta está ativa
	if !user.Active {
		logCtx.Warn("Tentativa de login em conta inativa.")
		a.auditLogService.LogAction(models.AuditLogEntry{
			Action:      "LOGIN_FAILED_INACTIVE",
			Description: fmt.Sprintf("Tentativa de login para conta inativa: %s (ID: %s)", user.Username, user.ID),
			Severity:    "WARNING",
			Username:    user.Username,
			UserID:      &user.ID,
			IPAddress:   &ipAddress,
			Metadata:    map[string]interface{}{"user_id": user.ID.String(), "ip": ipAddress, "agent": userAgent},
		})
		return &AuthResult{Success: false, Message: "Conta de usuário desativada."}, nil
	}

	// 3. Verificar bloqueio de conta
	now := time.Now().UTC()
	if user.FailedAttempts >= a.cfg.MaxLoginAttempts && user.LastFailedLogin != nil {
		lockExpiryTime := user.LastFailedLogin.Add(a.cfg.AccountLockoutTime)
		if now.Before(lockExpiryTime) {
			remainingLockout := lockExpiryTime.Sub(now)
			logCtx.Warnf("Conta bloqueada. Tempo restante: %v", remainingLockout)
			a.auditLogService.LogAction(models.AuditLogEntry{
				Action:      "LOGIN_FAILED_LOCKED",
				Description: fmt.Sprintf("Tentativa de login para conta bloqueada: %s (ID: %s)", user.Username, user.ID),
				Severity:    "WARNING",
				Username:    user.Username,
				UserID:      &user.ID,
				IPAddress:   &ipAddress,
				Metadata:    map[string]interface{}{"user_id": user.ID.String(), "remaining_lockout_sec": remainingLockout.Seconds()},
			})
			return &AuthResult{Success: false, Message: fmt.Sprintf("Conta temporariamente bloqueada. Tente novamente em %d minutos.", int(remainingLockout.Minutes())+1)}, nil
		}
		// Bloqueio expirou, resetar tentativas
		logCtx.Info("Bloqueio de conta expirado. Resetando tentativas.")
		user.FailedAttempts = 0
		user.LastFailedLogin = nil // sql.NullTime{Valid: false}
		if err := a.userRepo.UpdateLoginAttempts(user.ID, user.FailedAttempts, user.LastFailedLogin, nil); err != nil {
			logCtx.Errorf("Erro (não fatal) ao resetar tentativas no desbloqueio: %v", err)
			// Continuar mesmo assim
		}
	}

	// 4. Verificar senha
	if !VerifyPassword(password, user.PasswordHash) {
		user.FailedAttempts++
		failedLoginTime := now // sql.NullTime{Time: now, Valid: true}
		user.LastFailedLogin = &failedLoginTime

		if err := a.userRepo.UpdateLoginAttempts(user.ID, user.FailedAttempts, user.LastFailedLogin, nil); err != nil {
			logCtx.Errorf("Erro (não fatal) ao atualizar tentativas de login falhas: %v", err)
			// Considerar se isso deve impedir o login ou apenas ser logado
		}

		logCtx.Warnf("Senha inválida. Tentativa %d/%d.", user.FailedAttempts, a.cfg.MaxLoginAttempts)
		a.auditLogService.LogAction(models.AuditLogEntry{
			Action:      "LOGIN_FAILED_PASSWORD",
			Description: fmt.Sprintf("Senha inválida para %s. Tentativa %d/%d.", user.Username, user.FailedAttempts, a.cfg.MaxLoginAttempts),
			Severity:    "WARNING",
			Username:    user.Username,
			UserID:      &user.ID,
			IPAddress:   &ipAddress,
			Metadata:    map[string]interface{}{"user_id": user.ID.String(), "attempt": user.FailedAttempts},
		})

		if user.FailedAttempts >= a.cfg.MaxLoginAttempts {
			logCtx.Warn("Conta bloqueada após múltiplas tentativas falhas.")
			a.auditLogService.LogAction(models.AuditLogEntry{
				Action:      "ACCOUNT_LOCKED",
				Description: fmt.Sprintf("Conta %s bloqueada após %d tentativas.", user.Username, user.FailedAttempts),
				Severity:    "WARNING",
				Username:    user.Username,
				UserID:      &user.ID,
				IPAddress:   &ipAddress,
				Metadata:    map[string]interface{}{"user_id": user.ID.String(), "attempts": user.FailedAttempts},
			})
			return &AuthResult{Success: false, Message: "Senha incorreta. Conta bloqueada após múltiplas tentativas."}, nil
		}
		return &AuthResult{Success: false, Message: fmt.Sprintf("Usuário ou Senha inválidos. Tentativas restantes: %d", a.cfg.MaxLoginAttempts-user.FailedAttempts)}, nil
	}

	// 5. Login bem-sucedido
	logCtx.Info("Login bem-sucedido.")
	lastLoginTime := now //sql.NullTime{Time: now, Valid: true}
	user.LastLogin = &lastLoginTime
	user.FailedAttempts = 0
	user.LastFailedLogin = nil //sql.NullTime{Valid: false}

	if err := a.userRepo.UpdateLoginAttempts(user.ID, user.FailedAttempts, user.LastFailedLogin, user.LastLogin); err != nil {
		logCtx.Errorf("Erro (não fatal) ao atualizar último login e resetar falhas: %v", err)
		// Não impedir o login por isso, mas é importante logar.
	}

	// 6. Criar sessão
	// Os roles devem ser carregados pelo userRepo.GetByUsernameOrEmail ou por uma chamada separada aqui.
	// Supondo que user.Roles (tipo []*models.DBRole) já está populado.
	roleNames := make([]string, len(user.Roles))
	for i, role := range user.Roles {
		roleNames[i] = role.Name
	}

	sessionData := SessionData{
		UserID:       user.ID,
		Username:     user.Username,
		Roles:        roleNames,
		IPAddress:    ipAddress,
		UserAgent:    userAgent,
		CreatedAt:    now,
		LastActivity: now,
		ExpiresAt:    now.Add(a.cfg.SessionTimeout),
		// Metadata:     make(map[string]interface{}), // Inicializar se necessário
	}
	sessionID, err := a.sessionManager.CreateSession(sessionData)
	if err != nil {
		logCtx.Errorf("Erro CRÍTICO ao criar sessão: %v", err)
		a.auditLogService.LogAction(models.AuditLogEntry{
			Action:      "LOGIN_FAILED_SESSION_CREATE",
			Description: fmt.Sprintf("Falha crítica ao criar sessão para %s após senha correta.", user.Username),
			Severity:    "CRITICAL",
			Username:    user.Username,
			UserID:      &user.ID,
			Metadata:    map[string]interface{}{"user_id": user.ID.String(), "error": err.Error()},
		})
		return nil, fmt.Errorf("%w: erro interno ao iniciar sessão do usuário", appErrors.ErrInternal)
	}
	logCtx.Infof("Sessão criada com ID: %s", sessionID)

	a.auditLogService.LogAction(models.AuditLogEntry{
		Action:      "LOGIN_SUCCESS",
		Description: fmt.Sprintf("Usuário %s logado com sucesso.", user.Username),
		Severity:    "INFO",
		Username:    user.Username,
		UserID:      &user.ID,
		IPAddress:   &ipAddress,
		Metadata:    map[string]interface{}{"user_id": user.ID.String(), "session_id_prefix": sessionID[:8]},
	})

	// Preparar dados públicos do usuário
	userPublicData := &models.UserPublic{
		ID:        user.ID,
		Username:  user.Username,
		Email:     user.Email,
		FullName:  user.FullName,
		Active:    user.Active,
		Roles:     roleNames,
		CreatedAt: user.CreatedAt,
		LastLogin: user.LastLogin,
	}

	return &AuthResult{
		Success:   true,
		Message:   "Autenticação bem-sucedida.",
		SessionID: sessionID,
		UserData:  userPublicData,
		// RedirectTo: ui.PageMain, // Opcional: Para UI saber para onde ir
	}, nil
}

// LogoutUser invalida uma sessão de usuário.
func (a *authenticatorImpl) LogoutUser(sessionID string) error {
	session, err := a.sessionManager.GetSession(sessionID)
	if err != nil {
		if errors.Is(err, appErrors.ErrNotFound) {
			appLogger.Warnf("Tentativa de logout para sessão inexistente/expirada: %s", sessionID)
			// Ainda tenta remover, caso exista algum resquício
			_ = a.sessionManager.DeleteSession(sessionID)
			return nil // Não é um erro crítico para o chamador
		}
		appLogger.Errorf("Erro ao obter sessão para logout (%s): %v", sessionID, err)
		return fmt.Errorf("%w: falha ao validar sessão para logout", appErrors.ErrInternal)
	}

	err = a.sessionManager.DeleteSession(sessionID)
	if err != nil {
		appLogger.Errorf("Erro ao deletar sessão (%s) durante logout: %v", sessionID, err)
		// Logar falha, mas continuar. O principal é que a sessão não seja mais validada.
	}

	appLogger.Infof("Sessão %s (Usuário: %s) removida (logout).", sessionID, session.Username)
	userID := session.UserID // Assume que UserID está na SessionData
	a.auditLogService.LogAction(models.AuditLogEntry{
		Action:      "LOGOUT",
		Description: fmt.Sprintf("Logout para sessão %s (usuário %s).", sessionID[:8], session.Username),
		Severity:    "INFO",
		Username:    session.Username,
		UserID:      &userID,
		Metadata:    map[string]interface{}{"session_id_prefix": sessionID[:8]},
	})
	return nil
}
