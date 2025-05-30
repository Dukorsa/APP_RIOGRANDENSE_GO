// Em internal/auth/authenticator.go
package auth

import (
	// "database/sql" // Mantido para referência no código original, mas não usado ativamente para *time.Time
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/config"
	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/errors"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/models"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/navigation" // Para navigation.PageID
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/repositories"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/services"
	"github.com/sirupsen/logrus"
)

// AuthResult encapsula o resultado de uma operação de autenticação.
type AuthResult struct {
	Success    bool
	Message    string
	SessionID  string
	UserData   *models.UserPublic
	RedirectTo navigation.PageID // Opcional: para onde redirecionar na UI
}

// AuthenticatorInterface define a interface para operações de autenticação.
type AuthenticatorInterface interface {
	AuthenticateUser(username, password, ipAddress, userAgent string) (*AuthResult, error)
	LogoutUser(sessionID string) error
}

// authenticatorImpl implementa AuthenticatorInterface.
type authenticatorImpl struct {
	cfg             *config.Config // Corrigido para usar o alias correto se necessário, ou o nome do pacote.
	userRepo        repositories.UserRepository
	sessionManager  *SessionManager
	auditLogService services.AuditLogService
}

// NewAuthenticator cria uma nova instância do Authenticator.
func NewAuthenticator(
	cfg *config.Config, // Corrigido
	db *gorm.DB,
	sessionManager *SessionManager,
	auditLogService services.AuditLogService,
) AuthenticatorInterface {
	userRepo := repositories.NewGormUserRepository(db)

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
		appLogger.Warnf("Erro inesperado durante verifyPassword (bcrypt.CompareHashAndPassword): %v. Hash recebido: '%s', Comprimento da Senha: %d", err, hashedPassword, len(plainPassword))
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

	user, err := a.userRepo.GetByUsernameOrEmail(normalizedInput)
	if err != nil {
		if errors.Is(err, appErrors.ErrNotFound) {
			logCtx.Warn("Usuário não encontrado no banco de dados.")
			a.auditLogService.LogAction(models.AuditLogEntry{
				Action:      "LOGIN_FAILED_USER_NOT_FOUND",
				Description: fmt.Sprintf("Tentativa de login para usuário inexistente: %s", normalizedInput),
				Severity:    "WARNING",
				Username:    normalizedInput,
				IPAddress:   &ipAddress,
				Metadata:    map[string]interface{}{"input": normalizedInput, "ip": ipAddress, "agent": userAgent},
			}, nil) // Passa nil, pois não há sessão de usuário ainda.
			return &AuthResult{Success: false, Message: "Usuário ou Senha inválidos."}, nil
		}
		logCtx.Errorf("Erro ao buscar usuário: %v", err)
		return nil, fmt.Errorf("%w: falha ao verificar usuário", appErrors.ErrDatabase)
	}
	logCtx = logCtx.WithField("userID", user.ID.String())

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
		}, nil)
		return &AuthResult{Success: false, Message: "Conta de usuário desativada."}, nil
	}

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
			}, nil)
			return &AuthResult{Success: false, Message: fmt.Sprintf("Conta temporariamente bloqueada. Tente novamente em %d minutos.", int(remainingLockout.Minutes())+1)}, nil
		}
		logCtx.Info("Bloqueio de conta expirado. Resetando tentativas.")
		user.FailedAttempts = 0
		user.LastFailedLogin = nil
		if err := a.userRepo.UpdateLoginAttempts(user.ID, user.FailedAttempts, user.LastFailedLogin, nil); err != nil {
			logCtx.Errorf("Erro (não fatal) ao resetar tentativas no desbloqueio: %v", err)
		}
	}

	if !VerifyPassword(password, user.PasswordHash) {
		user.FailedAttempts++
		failedLoginTime := now
		user.LastFailedLogin = &failedLoginTime

		if err := a.userRepo.UpdateLoginAttempts(user.ID, user.FailedAttempts, user.LastFailedLogin, nil); err != nil {
			logCtx.Errorf("Erro (não fatal) ao atualizar tentativas de login falhas: %v", err)
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
		}, nil)

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
			}, nil)
			return &AuthResult{Success: false, Message: "Senha incorreta. Conta bloqueada após múltiplas tentativas."}, nil
		}
		return &AuthResult{Success: false, Message: fmt.Sprintf("Usuário ou Senha inválidos. Tentativas restantes: %d", a.cfg.MaxLoginAttempts-user.FailedAttempts)}, nil
	}

	logCtx.Info("Login bem-sucedido.")
	lastLoginTime := now
	user.LastLogin = &lastLoginTime
	user.FailedAttempts = 0
	user.LastFailedLogin = nil

	if err := a.userRepo.UpdateLoginAttempts(user.ID, user.FailedAttempts, user.LastFailedLogin, user.LastLogin); err != nil {
		logCtx.Errorf("Erro (não fatal) ao atualizar último login e resetar falhas: %v", err)
	}

	roleNames := make([]string, len(user.Roles))
	for i, role := range user.Roles {
		roleNames[i] = role.Name
	}

	sessionData := SessionData{ // Esta é a struct auth.SessionData
		UserID:       user.ID,
		Username:     user.Username,
		Roles:        roleNames,
		IPAddress:    ipAddress,
		UserAgent:    userAgent,
		CreatedAt:    now,
		LastActivity: now,
		ExpiresAt:    now.Add(a.cfg.SessionTimeout),
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
			IPAddress:   &ipAddress,
			Metadata:    map[string]interface{}{"user_id": user.ID.String(), "error": err.Error()},
		}, nil)
		return nil, fmt.Errorf("%w: erro interno ao iniciar sessão do usuário", appErrors.ErrInternal)
	}
	logCtx.Infof("Sessão criada com ID: %s", sessionID)

	currentLoginSessionDataForLog := sessionData // Cria uma cópia valor
	currentLoginSessionDataForLog.ID = sessionID // Adiciona o ID da sessão

	// Passa o endereço de currentLoginSessionDataForLog, pois *SessionData implementa LoggableSession.
	a.auditLogService.LogAction(models.AuditLogEntry{
		Action:      "LOGIN_SUCCESS",
		Description: fmt.Sprintf("Usuário %s logado com sucesso.", user.Username),
		Severity:    "INFO",
		Username:    user.Username,
		UserID:      &user.ID,
		IPAddress:   &ipAddress,
		Metadata:    map[string]interface{}{"user_id": user.ID.String(), "session_id_prefix": sessionID[:8]},
	}, &currentLoginSessionDataForLog)

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
	}, nil
}

// LogoutUser invalida uma sessão de usuário.
func (a *authenticatorImpl) LogoutUser(sessionID string) error {
	session, err := a.sessionManager.GetSession(sessionID) // Retorna *SessionData
	if err != nil {
		if errors.Is(err, appErrors.ErrNotFound) || errors.Is(err, appErrors.ErrSessionExpired) {
			appLogger.Warnf("Tentativa de logout para sessão inexistente/expirada: %s", sessionID)
			_ = a.sessionManager.DeleteSession(sessionID)
			return nil
		}
		appLogger.Errorf("Erro ao obter sessão para logout (%s): %v", sessionID, err)
		return fmt.Errorf("%w: falha ao validar sessão para logout", appErrors.ErrInternal)
	}

	// 'session' é *SessionData, que implementa types.LoggableSession.
	a.auditLogService.LogAction(models.AuditLogEntry{
		Action:      "LOGOUT",
		Description: fmt.Sprintf("Logout para sessão %s (usuário %s).", sessionID[:8], session.Username),
		Severity:    "INFO",
		Username:    session.Username,
		UserID:      &session.UserID,
		IPAddress:   &session.IPAddress,
		Roles:       func() *string { s := strings.Join(session.Roles, ","); return &s }(),
		Metadata:    map[string]interface{}{"session_id_prefix": sessionID[:8]},
	}, session)

	err = a.sessionManager.DeleteSession(sessionID)
	if err != nil {
		appLogger.Errorf("Erro ao deletar sessão (%s) durante logout: %v", sessionID, err)
		// Não retorna erro aqui, pois o logout do ponto de vista do usuário deve prosseguir.
	}

	appLogger.Infof("Sessão %s (Usuário: %s) removida (logout).", sessionID, session.Username)
	return nil
}
