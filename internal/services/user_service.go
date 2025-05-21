package services

import (
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/auth"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core"
	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/errors"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/models"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/repositories"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/utils" // Para SecurityValidator
	"github.com/google/uuid"
	"gorm.io/gorm"
	// Para comparar hash no serviço, embora authenticator faça isso
)

// UserService define a interface para o serviço de usuário.
type UserService interface {
	CreateUser(userData models.UserCreate, currentUserSession *auth.SessionData) (*models.UserPublic, error)
	UpdateUser(userIDToUpdate uuid.UUID, updateData models.UserUpdate, currentUserSession *auth.SessionData) (*models.UserPublic, error)
	DeactivateUser(userIDToDeactivate uuid.UUID, currentUserSession *auth.SessionData) error
	GetUserByID(userID uuid.UUID, currentUserSession *auth.SessionData) (*models.UserPublic, error)
	// GetUserByUsername(username string, currentUserSession *auth.SessionData) (*models.UserPublic, error) // Pode ser útil
	// GetUserByEmail(email string, currentUserSession *auth.SessionData) (*models.UserPublic, error)      // Pode ser útil
	ListUsers(includeInactive bool, currentUserSession *auth.SessionData) ([]*models.UserPublic, error)

	ChangePassword(userID uuid.UUID, oldPassword, newPassword string, currentUserSession *auth.SessionData) error
	AdminResetPassword(userIDToReset uuid.UUID, newPassword string, currentUserSession *auth.SessionData) error
	InitiatePasswordReset(email, ipAddress string) error
	ConfirmPasswordReset(email, resetToken, newPassword string) error
	// HandleFailedLogin(usernameOrEmail string) error // Agora está no Authenticator
	UnlockUser(userIDToUnlock uuid.UUID, currentUserSession *auth.SessionData) error
}

// userServiceImpl é a implementação de UserService.
type userServiceImpl struct {
	cfg             *core.Config
	userRepo        repositories.UserRepository
	roleRepo        repositories.RoleRepository
	auditLogService AuditLogService
	emailService    EmailService // Pode ser nil
	authenticator   auth.AuthenticatorInterface
	permManager     *auth.PermissionManager
	sessionManager  *auth.SessionManager // Para logout de todas as sessões se necessário
}

// NewUserService cria uma nova instância de UserService.
func NewUserService(
	dbOrTx interface{}, // *gorm.DB ou *sql.DB
	auditLog AuditLogService,
	emailService EmailService, // Pode ser nil
	cfg *core.Config,
	authN auth.AuthenticatorInterface,
	sm *auth.SessionManager,
	// pm *auth.PermissionManager, // PermissionManager pode ser obtido globalmente
) UserService {
	// Os repositórios são instanciados aqui, recebendo a conexão/transação
	// Esta é uma abordagem. Alternativamente, os repositórios podem ser passados como dependências.
	var userRepo repositories.UserRepository
	var roleRepo repositories.RoleRepository

	switch conn := dbOrTx.(type) {
	case *gorm.DB:
		userRepo = repositories.NewGormUserRepository(conn)
		roleRepo = repositories.NewGormRoleRepository(conn)
	// case *sql.DB:
	//  userRepo = repositories.NewSQLUserRepository(conn)
	//  roleRepo = repositories.NewSQLRoleRepository(conn)
	default:
		appLogger.Fatalf("Tipo de conexão de banco de dados não suportado para NewUserService: %T", dbOrTx)
	}

	if auditLog == nil || cfg == nil || authN == nil || sm == nil {
		appLogger.Fatalf("Dependências nulas (auditLog, cfg, authenticator, sessionManager) fornecidas para NewUserService")
	}
	// Obter o PermissionManager global
	permManager := auth.GetPermissionManager()
	if permManager == nil {
		appLogger.Fatalf("PermissionManager global não inicializado antes de NewUserService")
	}

	return &userServiceImpl{
		cfg:             cfg,
		userRepo:        userRepo,
		roleRepo:        roleRepo,
		auditLogService: auditLog,
		emailService:    emailService,
		authenticator:   authN,
		permManager:     permManager,
		sessionManager:  sm,
	}
}

// _validateAndGetRolesForUser traduz nomes de roles para objetos DBRole e valida.
func (s *userServiceImpl) _validateAndGetDBRoles(roleNames []string) ([]*models.DBRole, error) {
	if len(roleNames) == 0 {
		return []*models.DBRole{}, nil // Retorna slice vazio, não nil
	}

	dbRoles := make([]*models.DBRole, 0, len(roleNames))
	var notFoundNames []string

	for _, name := range roleNames {
		normalizedName := strings.ToLower(strings.TrimSpace(name))
		if normalizedName == "" {
			continue // Ignora nomes de role vazios
		}
		role, err := s.roleRepo.GetByName(normalizedName) // GetByName já deve ser case-insensitive
		if err != nil {
			if errors.Is(err, appErrors.ErrNotFound) {
				notFoundNames = append(notFoundNames, name)
				continue
			}
			return nil, appErrors.WrapErrorf(err, "erro ao buscar role '%s'", name)
		}
		dbRoles = append(dbRoles, role)
	}

	if len(notFoundNames) > 0 {
		return nil, appErrors.NewValidationError(
			fmt.Sprintf("Perfis (roles) não encontrados: %s", strings.Join(notFoundNames, ", ")),
			map[string]string{"role_names": "Contém roles inválidos ou não existentes"},
		)
	}
	return dbRoles, nil
}

// CreateUser cria um novo usuário.
func (s *userServiceImpl) CreateUser(userData models.UserCreate, currentUserSession *auth.SessionData) (*models.UserPublic, error) {
	isSelfRegistration := currentUserSession == nil
	var creatorUsername string

	if isSelfRegistration {
		creatorUsername = "system (self-registration)"
		// Para auto-registro, não há verificação de permissão 'user:create'
	} else {
		if err := s.permManager.CheckPermission(currentUserSession, auth.PermUserCreate, nil); err != nil {
			return nil, err
		}
		creatorUsername = currentUserSession.Username
	}

	// Validar e limpar dados da struct de entrada
	if err := userData.CleanAndValidate(); err != nil {
		appLogger.Warnf("Dados de criação de usuário inválidos para '%s': %v", userData.Username, err)
		return nil, err
	}

	// Validar força da senha
	strengthValidation := utils.ValidatePasswordStrength(userData.Password, s.cfg.PasswordMinLength) // Supondo que PasswordMinLength está em cfg
	if !strengthValidation.IsValid {
		var errorDetails []string
		if !strengthValidation.Length {
			errorDetails = append(errorDetails, "comprimento insuficiente")
		}
		if !strengthValidation.Uppercase {
			errorDetails = append(errorDetails, "falta maiúscula")
		}
		// ... adicionar outras falhas
		return nil, appErrors.NewValidationError(
			fmt.Sprintf("Senha fornecida é inválida ou fraca. Falhas: %s", strings.Join(errorDetails, ", ")),
			map[string]string{"password": "Senha fraca ou inválida"},
		)
	}

	// Hash da senha
	hashedPassword, err := auth.HashPassword(userData.Password)
	if err != nil {
		return nil, err // Erro já logado por HashPassword
	}

	// Validar e obter os DBRoles
	var initialDBRoles []*models.DBRole
	if len(userData.RoleNames) > 0 {
		initialDBRoles, err = s._validateAndGetDBRoles(userData.RoleNames)
		if err != nil {
			return nil, err // Erro de validação de roles
		}
	} else { // Se nenhum role for fornecido, atribuir "user" por padrão
		userRole, errRole := s.roleRepo.GetByName("user")
		if errRole != nil {
			appLogger.Errorf("Role 'user' padrão não encontrado para novo usuário '%s': %v", userData.Username, errRole)
			return nil, appErrors.WrapErrorf(errRole, "role 'user' padrão não encontrado")
		}
		initialDBRoles = []*models.DBRole{userRole}
	}

	// Chamar repositório para criar
	// O repositório é responsável por verificar duplicidade de username/email
	dbUser, err := s.userRepo.CreateUser(userData, hashedPassword, initialDBRoles)
	if err != nil {
		return nil, err
	}

	// Log de Auditoria
	logAction := "USER_SELF_REGISTER"
	if !isSelfRegistration {
		logAction = "ADMIN_USER_CREATE"
	}
	roleNamesForLog := make([]string, len(dbUser.Roles))
	for i, r := range dbUser.Roles {
		roleNamesForLog[i] = r.Name
	}

	logEntry := models.AuditLogEntry{
		Action:      logAction,
		Description: fmt.Sprintf("Novo usuário '%s' criado por %s.", dbUser.Username, creatorUsername),
		Severity:    "INFO",
		Metadata:    map[string]interface{}{"new_user_id": dbUser.ID.String(), "creator": creatorUsername, "initial_roles": roleNamesForLog},
	}
	if logErr := s.auditLogService.LogAction(logEntry, currentUserSession); logErr != nil {
		appLogger.Warnf("Falha ao registrar log de auditoria para criação do usuário %s: %v", dbUser.Username, logErr)
	}

	// Enviar e-mail de boas-vindas (se EmailService estiver configurado)
	if s.emailService != nil {
		go func(email, username string) { // Envia em goroutine para não bloquear
			errMail := s.emailService.SendWelcomeEmail(email, username, map[string]interface{}{"email": email})
			if errMail != nil {
				appLogger.Errorf("Falha (não fatal) ao enviar e-mail de boas-vindas para %s: %v", email, errMail)
			}
		}(dbUser.Email, dbUser.Username)
	}

	return models.ToUserPublic(dbUser), nil
}

// UpdateUser atualiza dados de um usuário e/ou seus roles.
func (s *userServiceImpl) UpdateUser(userIDToUpdate uuid.UUID, updateData models.UserUpdate, currentUserSession *auth.SessionData) (*models.UserPublic, error) {
	// 1. Verificar Permissão
	isEditingSelf := currentUserSession.UserID == userIDToUpdate
	if !isEditingSelf {
		if err := s.permManager.CheckPermission(currentUserSession, auth.PermUserUpdate, nil); err != nil { // Permissão para editar outros
			return nil, err
		}
	}
	// Permissão para gerenciar roles é separada se roles estão sendo alterados
	if updateData.RoleNames != nil && !isEditingSelf { // Só pode mudar roles de outros se tiver permissão
		if err := s.permManager.CheckPermission(currentUserSession, auth.PermUserManageRoles, nil); err != nil {
			return nil, err
		}
	}

	// 2. Buscar usuário a ser atualizado
	userToUpdate, err := s.userRepo.GetByID(userIDToUpdate) // GetByID já pré-carrega roles
	if err != nil {
		return nil, err // Trata ErrNotFound
	}

	// 3. Restrições de auto-edição
	if isEditingSelf {
		if updateData.Active != nil && *updateData.Active != userToUpdate.Active {
			return nil, fmt.Errorf("%w: não é permitido alterar o próprio status de ativação", appErrors.ErrPermissionDenied)
		}
		if updateData.RoleNames != nil { // Usuário não pode mudar seus próprios roles diretamente aqui
			// Comparar se os roles realmente mudaram
			currentRoleNamesSet := make(map[string]bool)
			for _, r := range userToUpdate.Roles {
				currentRoleNamesSet[strings.ToLower(r.Name)] = true
			}

			newRoleNamesSet := make(map[string]bool)
			if updateData.RoleNames != nil {
				for _, rn := range *updateData.RoleNames {
					newRoleNamesSet[strings.ToLower(rn)] = true
				}
			}

			if len(currentRoleNamesSet) != len(newRoleNamesSet) || !mapsEqual(currentRoleNamesSet, newRoleNamesSet) {
				return nil, fmt.Errorf("%w: não é permitido alterar os próprios perfis (roles) através desta função", appErrors.ErrPermissionDenied)
			}
			// Se não mudou, não precisa validar ou passar para o repo
			updateData.RoleNames = nil
		}
	}

	// 4. Validar e Limpar Dados de Entrada
	if err := updateData.CleanAndValidate(); err != nil {
		appLogger.Warnf("Dados de atualização de usuário inválidos para ID %s: %v", userIDToUpdate, err)
		return nil, err
	}

	// 5. Validar e obter os DBRoles se RoleNames foram fornecidos para update
	var newDBRoles []*models.DBRole // Será nil se updateData.RoleNames for nil
	if updateData.RoleNames != nil {
		newDBRoles, err = s._validateAndGetDBRoles(*updateData.RoleNames)
		if err != nil {
			return nil, err
		}
		// Verificar se está tentando remover o último admin
		if !isEditingSelf { // Só se um admin estiver editando outro
			isRemovingAdminRole := false
			newRolesMap := make(map[string]bool)
			for _, r := range newDBRoles {
				newRolesMap[strings.ToLower(r.Name)] = true
			}
			for _, currentRole := range userToUpdate.Roles {
				if strings.ToLower(currentRole.Name) == "admin" && !newRolesMap["admin"] {
					isRemovingAdminRole = true
					break
				}
			}
			if isRemovingAdminRole && s._isLastActiveAdmin(userIDToUpdate) {
				return nil, fmt.Errorf("%w: não é possível remover o perfil 'admin' do último administrador ativo do sistema", appErrors.ErrConflict)
			}
		}
	}

	// 6. Preparar mapa de atualização para o repositório
	// O repositório espera um map[string]interface{} para atualizações parciais de campos simples.
	// O campo Roles é tratado separadamente pelo repo se newDBRoles for passado.
	updateFields := make(map[string]interface{})
	if updateData.Email != nil {
		updateFields["email"] = *updateData.Email
	}
	if updateData.FullName != nil {
		updateFields["full_name"] = *updateData.FullName
	}
	if updateData.Active != nil {
		updateFields["active"] = *updateData.Active
	}
	// Adicionar outros campos se UserUpdate os tiver (ex: IsSuperuser)

	// 7. Chamar Repositório
	// O repositório verificará duplicidade de email/username se forem alterados.
	updatedUser, err := s.userRepo.UpdateUser(userIDToUpdate, updateFields, newDBRoles)
	if err != nil {
		return nil, err
	}

	// 8. Log de Auditoria
	finalRoleNames := make([]string, len(updatedUser.Roles))
	for i, r := range updatedUser.Roles {
		finalRoleNames[i] = r.Name
	}

	logEntry := models.AuditLogEntry{
		Action:      "USER_UPDATE",
		Description: fmt.Sprintf("Usuário ID %s ('%s') atualizado.", updatedUser.ID, updatedUser.Username),
		Severity:    "INFO",
		Metadata:    map[string]interface{}{"updated_user_id": updatedUser.ID.String(), "updated_fields": maps.Keys(updateFields), "final_roles": finalRoleNames},
	}
	if logErr := s.auditLogService.LogAction(logEntry, currentUserSession); logErr != nil {
		appLogger.Warnf("Falha ao registrar log de auditoria para atualização do usuário ID %s: %v", updatedUser.ID, logErr)
	}

	return models.ToUserPublic(updatedUser), nil
}

// Helper para verificar se é o último admin ativo
func (s *userServiceImpl) _isLastActiveAdmin(userIDBeingChecked uuid.UUID) bool {
	users, err := s.userRepo.GetAllUsers(true) // Pega todos, incluindo inativos, com roles
	if err != nil {
		appLogger.Errorf("Erro ao buscar usuários para checagem _isLastActiveAdmin: %v", err)
		return false // Assume que não é o último se não conseguir verificar
	}

	activeAdminCount := 0
	for _, u := range users {
		if u.Active {
			for _, r := range u.Roles {
				if strings.ToLower(r.Name) == "admin" {
					activeAdminCount++
					break // Próximo usuário
				}
			}
		}
	}
	// Se o usuário que está sendo verificado é um admin ativo,
	// e o total de admins ativos é 1, então ele é o último.
	isCheckingUserAdminActive := false
	for _, u := range users {
		if u.ID == userIDBeingChecked && u.Active {
			for _, r := range u.Roles {
				if strings.ToLower(r.Name) == "admin" {
					isCheckingUserAdminActive = true
					break
				}
			}
			break
		}
	}
	return isCheckingUserAdminActive && activeAdminCount <= 1
}

// DeactivateUser "deleta" logicamente um usuário.
func (s *userServiceImpl) DeactivateUser(userIDToDeactivate uuid.UUID, currentUserSession *auth.SessionData) error {
	if err := s.permManager.CheckPermission(currentUserSession, auth.PermUserDelete, nil); err != nil {
		return err
	}

	userToDeactivate, err := s.userRepo.GetByID(userIDToDeactivate)
	if err != nil {
		return err // Trata ErrNotFound
	}

	if currentUserSession.UserID == userIDToDeactivate {
		return fmt.Errorf("%w: não é possível desativar a própria conta", appErrors.ErrPermissionDenied)
	}

	// Verificar se está tentando desativar o último admin ativo
	isLastAdmin := false
	if userToDeactivate.Active {
		for _, r := range userToDeactivate.Roles {
			if strings.ToLower(r.Name) == "admin" {
				if s._isLastActiveAdmin(userIDToDeactivate) {
					isLastAdmin = true
				}
				break
			}
		}
	}
	if isLastAdmin {
		return fmt.Errorf("%w: não é possível desativar o último administrador ativo do sistema", appErrors.ErrConflict)
	}

	if !userToDeactivate.Active {
		appLogger.Infof("Usuário ID %s já está inativo.", userIDToDeactivate)
		return nil // Nenhuma ação necessária
	}

	if err := s.userRepo.DeactivateUser(userIDToDeactivate); err != nil {
		return err
	}

	// Log de Auditoria
	logEntry := models.AuditLogEntry{
		Action:      "USER_DEACTIVATE",
		Description: fmt.Sprintf("Usuário '%s' (ID: %s) desativado.", userToDeactivate.Username, userIDToDeactivate),
		Severity:    "WARNING",
		Metadata:    map[string]interface{}{"deactivated_user_id": userIDToDeactivate.String()},
	}
	if logErr := s.auditLogService.LogAction(logEntry, currentUserSession); logErr != nil {
		appLogger.Warnf("Falha ao registrar log de auditoria para desativação do usuário ID %s: %v", userIDToDeactivate, logErr)
	}

	// Opcional: Invalidar todas as sessões ativas do usuário desativado
	if removedCount, sessErr := s.sessionManager.DeleteAllUserSessions(userIDToDeactivate); sessErr != nil {
		appLogger.Errorf("Erro ao tentar invalidar sessões do usuário desativado ID %s: %v", userIDToDeactivate, sessErr)
	} else if removedCount > 0 {
		appLogger.Infof("%d sessões ativas invalidadas para o usuário desativado ID %s.", removedCount, userIDToDeactivate)
	}

	return nil
}

// GetUserByID busca um usuário pelo ID.
func (s *userServiceImpl) GetUserByID(userID uuid.UUID, currentUserSession *auth.SessionData) (*models.UserPublic, error) {
	// Permissão para ler outros usuários. Se for o próprio usuário, geralmente permite.
	isSelf := currentUserSession.UserID == userID
	if !isSelf {
		if err := s.permManager.CheckPermission(currentUserSession, auth.PermUserRead, nil); err != nil {
			return nil, err
		}
	}

	dbUser, err := s.userRepo.GetByID(userID)
	if err != nil {
		return nil, err
	}
	return models.ToUserPublic(dbUser), nil
}

// ListUsers lista todos os usuários.
func (s *userServiceImpl) ListUsers(includeInactive bool, currentUserSession *auth.SessionData) ([]*models.UserPublic, error) {
	if err := s.permManager.CheckPermission(currentUserSession, auth.PermUserRead, nil); err != nil {
		return nil, err
	}
	dbUsers, err := s.userRepo.GetAllUsers(includeInactive)
	if err != nil {
		return nil, err
	}
	return models.ToUserPublicList(dbUsers), nil
}

// --- Métodos de Gerenciamento de Senha ---

// ChangePassword permite ao próprio usuário alterar sua senha.
func (s *userServiceImpl) ChangePassword(userID uuid.UUID, oldPassword, newPassword string, currentUserSession *auth.SessionData) error {
	if currentUserSession == nil || currentUserSession.UserID != userID {
		// Se currentUserSession for nil, indica que não há sessão ativa (ex: processo de recuperação)
		// Mas esta função é para *usuário logado* mudando a *própria* senha.
		return fmt.Errorf("%w: tentativa de alterar senha de outro usuário ou sem sessão válida", appErrors.ErrPermissionDenied)
	}

	user, err := s.userRepo.GetByID(userID) // GetByID carrega o UserInDB (com hash)
	if err != nil {
		return err // NotFound ou DB error
	}

	if !auth.VerifyPassword(oldPassword, user.PasswordHash) {
		// Log de tentativa falha de mudança de senha (sem incrementar contador de login)
		logEntry := models.AuditLogEntry{
			Action: "PASSWORD_CHANGE_FAILED_OLD_MISMATCH", Description: fmt.Sprintf("Tentativa de alterar senha para usuário '%s' falhou (senha atual incorreta).", user.Username),
			Severity: "WARNING", Metadata: map[string]interface{}{"user_id": user.ID.String()},
		}
		s.auditLogService.LogAction(logEntry, currentUserSession)
		return fmt.Errorf("%w: senha atual incorreta", appErrors.ErrInvalidCredentials)
	}

	if auth.VerifyPassword(newPassword, user.PasswordHash) {
		return appErrors.NewValidationError("A nova senha deve ser diferente da senha atual.", map[string]string{"new_password": "Deve ser diferente da atual"})
	}

	strength := utils.ValidatePasswordStrength(newPassword, s.cfg.PasswordMinLength)
	if !strength.IsValid {
		return appErrors.NewValidationError("Nova senha inválida ou fraca.", strength.GetErrorDetails()) // Supondo GetErrorDetails() em PasswordStrengthResult
	}

	newHash, err := auth.HashPassword(newPassword)
	if err != nil {
		return err
	}

	if err := s.userRepo.UpdatePasswordHash(userID, newHash); err != nil {
		return err
	}

	// Log de Auditoria
	logEntry := models.AuditLogEntry{
		Action: "PASSWORD_CHANGE_SUCCESS", Description: fmt.Sprintf("Senha alterada com sucesso para usuário '%s'.", user.Username),
		Severity: "INFO", Metadata: map[string]interface{}{"user_id": user.ID.String()},
	}
	s.auditLogService.LogAction(logEntry, currentUserSession)
	return nil
}

// AdminResetPassword permite a um admin resetar a senha de outro usuário.
func (s *userServiceImpl) AdminResetPassword(userIDToReset uuid.UUID, newPassword string, currentUserSession *auth.SessionData) error {
	if err := s.permManager.CheckPermission(currentUserSession, auth.PermUserResetPassword, nil); err != nil {
		return err
	}

	userToReset, err := s.userRepo.GetByID(userIDToReset)
	if err != nil {
		return err
	}

	if currentUserSession.UserID == userIDToReset {
		return fmt.Errorf("%w: use a opção 'Alterar Senha' para sua própria conta", appErrors.ErrPermissionDenied)
	}

	strength := utils.ValidatePasswordStrength(newPassword, s.cfg.PasswordMinLength)
	if !strength.IsValid {
		return appErrors.NewValidationError("Nova senha inválida ou fraca.", strength.GetErrorDetails())
	}

	if auth.VerifyPassword(newPassword, userToReset.PasswordHash) {
		return appErrors.NewValidationError("A nova senha deve ser diferente da senha atual do usuário.", map[string]string{"new_password": "Deve ser diferente da atual"})
	}

	newHash, err := auth.HashPassword(newPassword)
	if err != nil {
		return err
	}

	if err := s.userRepo.UpdatePasswordHash(userIDToReset, newHash); err != nil { // UpdatePasswordHash também limpa tokens de reset
		return err
	}

	// Log de Auditoria
	logEntry := models.AuditLogEntry{
		Action: "PASSWORD_RESET_ADMIN", Description: fmt.Sprintf("Senha do usuário '%s' (ID: %s) redefinida por %s.", userToReset.Username, userIDToReset, currentUserSession.Username),
		Severity: "WARNING", Metadata: map[string]interface{}{"reset_user_id": userIDToReset.String(), "admin_user_id": currentUserSession.UserID.String()},
	}
	s.auditLogService.LogAction(logEntry, currentUserSession)
	return nil
}

// InitiatePasswordReset inicia o processo de recuperação de senha pelo usuário.
func (s *userServiceImpl) InitiatePasswordReset(email, ipAddress string) error {
	if s.emailService == nil {
		return fmt.Errorf("%w: serviço de e-mail não configurado para recuperação de senha", appErrors.ErrConfiguration)
	}

	user, err := s.userRepo.GetByEmail(email) // Case-insensitive
	if err != nil {
		if errors.Is(err, appErrors.ErrNotFound) {
			appLogger.Warnf("Tentativa de reset de senha para e-mail não cadastrado: %s", email)
			// Não retorna erro para o usuário, para não revelar se um email existe ou não.
			// Mas loga a tentativa.
			s.auditLogService.LogAction(models.AuditLogEntry{
				Action: "PASSWORD_RESET_INIT_EMAIL_NOT_FOUND", Description: fmt.Sprintf("Tentativa de iniciar reset para e-mail não existente: %s", email),
				Severity: "INFO", IPAddress: &ipAddress, Metadata: map[string]interface{}{"email_attempted": email},
			}, nil) // Sem sessão de usuário aqui
			return nil
		}
		return err // Outro erro de DB
	}

	if !user.Active {
		appLogger.Warnf("Tentativa de reset de senha para usuário inativo: %s (Email: %s)", user.Username, email)
		s.auditLogService.LogAction(models.AuditLogEntry{
			Action: "PASSWORD_RESET_INIT_INACTIVE_USER", Description: fmt.Sprintf("Tentativa de iniciar reset para usuário inativo '%s'", user.Username),
			Severity: "WARNING", UserID: &user.ID, IPAddress: &ipAddress, Metadata: map[string]interface{}{"user_id": user.ID.String()},
		}, nil)
		return nil // Não envia e-mail para inativos
	}

	resetTokenPlain := utils.GenerateSecureRandomToken(32) // utils precisa ter esta função
	tokenHash, err := auth.HashPassword(resetTokenPlain)
	if err != nil {
		return err
	}
	expiresAt := time.Now().UTC().Add(s.cfg.PasswordResetTimeout)

	if err := s.userRepo.UpdatePasswordResetToken(user.ID, &tokenHash, &expiresAt); err != nil {
		return err
	}

	// Enviar e-mail com resetTokenPlain
	if err := s.emailService.SendPasswordResetCode(user.Email, resetTokenPlain, ipAddress); err != nil {
		// Se o envio falhar, reverter o token no DB? Ou deixar expirar?
		// Por agora, logamos e retornamos o erro de e-mail.
		appLogger.Errorf("Falha ao enviar e-mail de reset de senha para %s: %v", user.Email, err)
		// Limpar o token se o email falhou é uma boa prática de segurança
		s.userRepo.UpdatePasswordResetToken(user.ID, nil, nil)
		return err // Retorna o EmailError
	}

	s.auditLogService.LogAction(models.AuditLogEntry{
		Action: "PASSWORD_RESET_INIT_SUCCESS", Description: fmt.Sprintf("Reset de senha iniciado para usuário %s. E-mail enviado.", user.Username),
		Severity: "INFO", UserID: &user.ID, IPAddress: &ipAddress, Metadata: map[string]interface{}{"user_id": user.ID.String(), "email": user.Email},
	}, nil)
	return nil
}

// ConfirmPasswordReset confirma o reset usando o token e define a nova senha.
func (s *userServiceImpl) ConfirmPasswordReset(email, resetTokenPlain, newPassword string) error {
	user, err := s.userRepo.GetByEmail(email)
	if err != nil {
		if errors.Is(err, appErrors.ErrNotFound) {
			return fmt.Errorf("%w: token de reset inválido ou expirado (usuário não encontrado)", appErrors.ErrInvalidCredentials)
		}
		return err
	}

	if user.PasswordResetToken == nil || *user.PasswordResetToken == "" || user.PasswordResetExpires == nil {
		return fmt.Errorf("%w: token de reset inválido ou não solicitado", appErrors.ErrInvalidCredentials)
	}
	if time.Now().UTC().After(*user.PasswordResetExpires) {
		// Limpar token expirado
		s.userRepo.UpdatePasswordResetToken(user.ID, nil, nil)
		return fmt.Errorf("%w: token de reset expirado", appErrors.ErrTokenExpired)
	}

	if !auth.VerifyPassword(resetTokenPlain, *user.PasswordResetToken) {
		// Logar tentativa falha de uso de token (sem incrementar falhas de login)
		return fmt.Errorf("%w: token de reset inválido", appErrors.ErrInvalidCredentials)
	}

	// Validar nova senha
	strength := utils.ValidatePasswordStrength(newPassword, s.cfg.PasswordMinLength)
	if !strength.IsValid {
		return appErrors.NewValidationError("Nova senha inválida ou fraca.", strength.GetErrorDetails())
	}
	if auth.VerifyPassword(newPassword, user.PasswordHash) {
		return appErrors.NewValidationError("A nova senha deve ser diferente da senha atual.", map[string]string{"new_password": "Deve ser diferente da atual"})
	}

	newHash, err := auth.HashPassword(newPassword)
	if err != nil {
		return err
	}

	// Atualiza a senha e limpa os campos de reset e tentativas de login
	if err := s.userRepo.UpdatePasswordHash(user.ID, newHash); err != nil {
		return err
	}

	s.auditLogService.LogAction(models.AuditLogEntry{
		Action: "PASSWORD_RESET_CONFIRM_SUCCESS", Description: fmt.Sprintf("Senha redefinida com sucesso via recuperação para usuário %s.", user.Username),
		Severity: "INFO", UserID: &user.ID, Metadata: map[string]interface{}{"user_id": user.ID.String(), "email": user.Email},
	}, nil)
	return nil
}

// UnlockUser desbloqueia uma conta.
func (s *userServiceImpl) UnlockUser(userIDToUnlock uuid.UUID, currentUserSession *auth.SessionData) error {
	if err := s.permManager.CheckPermission(currentUserSession, auth.PermUserUnlock, nil); err != nil {
		return err
	}

	userToUnlock, err := s.userRepo.GetByID(userIDToUnlock)
	if err != nil {
		return err
	}

	if userToUnlock.FailedAttempts == 0 && userToUnlock.LastFailedLogin == nil {
		appLogger.Infof("Usuário ID %s ('%s') já estava desbloqueado.", userIDToUnlock, userToUnlock.Username)
		return nil // Nenhuma ação necessária
	}

	// Reseta tentativas de login falhas
	if err := s.userRepo.UpdateLoginAttempts(userIDToUnlock, 0, nil, userToUnlock.LastLogin); err != nil {
		return err
	}

	logEntry := models.AuditLogEntry{
		Action: "ACCOUNT_UNLOCK_ADMIN", Description: fmt.Sprintf("Conta '%s' (ID: %s) desbloqueada por %s.", userToUnlock.Username, userIDToUnlock, currentUserSession.Username),
		Severity: "INFO", UserID: &userToUnlock.ID, Metadata: map[string]interface{}{"unlocked_user_id": userIDToUnlock.String()},
	}
	s.auditLogService.LogAction(logEntry, currentUserSession)
	return nil
}

// Helper para comparar maps (usado para checar se roles mudaram)
func mapsEqual(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}
