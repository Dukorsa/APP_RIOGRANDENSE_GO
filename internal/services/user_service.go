package services

import (
	"errors"
	"fmt"
	"maps" // Requer Go 1.21+
	"strings"
	"time"

	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/auth"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core"
	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/models"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/repositories"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/utils" // Para validadores
	"github.com/google/uuid"
)

// UserService define a interface para o serviço de usuário.
type UserService interface {
	CreateUser(userData models.UserCreate, currentUserSession *auth.SessionData) (*models.UserPublic, error)
	UpdateUser(userIDToUpdate uuid.UUID, updateData models.UserUpdate, currentUserSession *auth.SessionData) (*models.UserPublic, error)
	DeactivateUser(userIDToDeactivate uuid.UUID, currentUserSession *auth.SessionData) error
	GetUserByID(userID uuid.UUID, currentUserSession *auth.SessionData) (*models.UserPublic, error)
	ListUsers(includeInactive bool, currentUserSession *auth.SessionData) ([]*models.UserPublic, error)

	ChangePassword(userID uuid.UUID, oldPassword, newPassword string, currentUserSession *auth.SessionData) error
	AdminResetPassword(userIDToReset uuid.UUID, newPassword string, currentUserSession *auth.SessionData) error
	InitiatePasswordReset(email, ipAddress string) error // ipAddress é do solicitante
	ConfirmPasswordReset(email, resetToken, newPassword string) error
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
	sessionManager  *auth.SessionManager
}

// NewUserService cria uma nova instância de UserService.
// Os repositórios são injetados diretamente.
func NewUserService(
	cfg *core.Config,
	userRepo repositories.UserRepository,
	roleRepo repositories.RoleRepository,
	auditLog AuditLogService,
	emailService EmailService, // Pode ser nil
	authN auth.AuthenticatorInterface,
	sm *auth.SessionManager,
) UserService {
	if cfg == nil || userRepo == nil || roleRepo == nil || auditLog == nil || authN == nil || sm == nil {
		appLogger.Fatalf("Dependências nulas (cfg, userRepo, roleRepo, auditLog, authenticator, sessionManager) fornecidas para NewUserService")
	}
	permManager := auth.GetPermissionManager() // Obtém a instância global do PermissionManager
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

// _validateAndGetDBRoles traduz nomes de roles para objetos DBRole e valida sua existência.
func (s *userServiceImpl) _validateAndGetDBRoles(roleNames []string) ([]*models.DBRole, error) {
	if len(roleNames) == 0 {
		return []*models.DBRole{}, nil // Retorna slice vazio, não nil.
	}

	dbRoles := make([]*models.DBRole, 0, len(roleNames))
	var notFoundNames []string
	processedNames := make(map[string]bool) // Para evitar processar/retornar duplicatas de nomes não encontrados

	for _, name := range roleNames {
		normalizedName := strings.ToLower(strings.TrimSpace(name))
		if normalizedName == "" || processedNames[normalizedName] {
			continue // Ignora nomes de role vazios ou já processados (para erro).
		}
		role, err := s.roleRepo.GetByName(normalizedName) // GetByName já deve ser case-insensitive.
		if err != nil {
			if errors.Is(err, appErrors.ErrNotFound) {
				notFoundNames = append(notFoundNames, name)
				processedNames[normalizedName] = true
				continue
			}
			return nil, appErrors.WrapErrorf(err, "erro ao buscar role '%s'", name)
		}
		dbRoles = append(dbRoles, role)
		processedNames[normalizedName] = true
	}

	if len(notFoundNames) > 0 {
		return nil, appErrors.NewValidationError(
			fmt.Sprintf("Perfis (roles) não encontrados: %s", strings.Join(notFoundNames, ", ")),
			map[string]string{"role_names": "Contém roles inválidos ou não existentes: " + strings.Join(notFoundNames, ", ")},
		)
	}
	return dbRoles, nil
}

// CreateUser cria um novo usuário.
func (s *userServiceImpl) CreateUser(userData models.UserCreate, currentUserSession *auth.SessionData) (*models.UserPublic, error) {
	isSelfRegistration := currentUserSession == nil
	var creatorUsername string
	var logUserSession *auth.SessionData // Sessão para log de auditoria

	if isSelfRegistration {
		creatorUsername = "system (auto-registro)"
		// Para auto-registro, não há verificação de permissão `user:create`.
		// No entanto, pode haver uma configuração global para permitir ou não auto-registro.
	} else {
		if err := s.permManager.CheckPermission(currentUserSession, auth.PermUserCreate, nil); err != nil {
			return nil, err
		}
		creatorUsername = currentUserSession.Username
		logUserSession = currentUserSession
	}

	// Validar e limpar dados da struct de entrada.
	if err := userData.CleanAndValidate(); err != nil {
		appLogger.Warnf("Dados de criação de usuário inválidos para '%s': %v", userData.Username, err)
		return nil, err
	}

	// Validar força da senha.
	strengthValidation := utils.ValidatePasswordStrength(userData.Password, s.cfg.PasswordMinLength)
	if !strengthValidation.IsValid {
		var errorDetails []string
		if !strengthValidation.Length {
			errorDetails = append(errorDetails, fmt.Sprintf("comprimento mínimo de %d caracteres", s.cfg.PasswordMinLength))
		}
		if !strengthValidation.Uppercase {
			errorDetails = append(errorDetails, "letra maiúscula")
		}
		if !strengthValidation.Lowercase {
			errorDetails = append(errorDetails, "letra minúscula")
		}
		if !strengthValidation.Digit {
			errorDetails = append(errorDetails, "número")
		}
		if !strengthValidation.SpecialChar {
			errorDetails = append(errorDetails, "caractere especial")
		}
		if !strengthValidation.NotCommonPassword {
			errorDetails = append(errorDetails, "senha muito comum")
		}
		return nil, appErrors.NewValidationError(
			fmt.Sprintf("Senha fornecida é inválida ou fraca. Falhas: %s", strings.Join(errorDetails, ", ")),
			map[string]string{"password": "Senha fraca ou inválida: " + strings.Join(errorDetails, ", ")},
		)
	}

	// Hash da senha.
	hashedPassword, errHash := auth.HashPassword(userData.Password)
	if errHash != nil {
		return nil, errHash // Erro já logado por HashPassword.
	}

	// Validar e obter os DBRoles.
	// Se `userData.RoleNames` for vazio e for auto-registro, atribuir "user" por padrão.
	// Se admin estiver criando e não fornecer roles, pode ser um erro ou um default diferente.
	var initialDBRoles []*models.DBRole
	if len(userData.RoleNames) == 0 {
		if isSelfRegistration { // Auto-registro sempre ganha role "user".
			userRole, errRole := s.roleRepo.GetByName("user")
			if errRole != nil {
				appLogger.Errorf("Role 'user' padrão não encontrado para novo usuário (auto-registro) '%s': %v", userData.Username, errRole)
				return nil, appErrors.WrapErrorf(errRole, "role 'user' padrão não encontrado")
			}
			initialDBRoles = []*models.DBRole{userRole}
		} else { // Admin criando, e não especificou roles.
			// Política: Requerer que admin especifique roles ou ter um default diferente?
			// Por agora, se admin não especificar, não atribuir roles (ou atribuir "user").
			// Vamos atribuir "user" se nada for passado, para consistência.
			userRole, errRole := s.roleRepo.GetByName("user")
			if errRole != nil {
				appLogger.Errorf("Role 'user' padrão não encontrado para novo usuário (admin) '%s': %v", userData.Username, errRole)
				return nil, appErrors.WrapErrorf(errRole, "role 'user' padrão não encontrado")
			}
			initialDBRoles = []*models.DBRole{userRole}
			userData.RoleNames = []string{"user"} // Atualiza para log
		}
	} else { // Roles foram fornecidos
		var errRoles error
		initialDBRoles, errRoles = s._validateAndGetDBRoles(userData.RoleNames)
		if errRoles != nil {
			return nil, errRoles // Erro de validação de roles.
		}
	}

	// Chamar repositório para criar. O repo verifica duplicidade de username/email (conflito no DB).
	dbUser, err := s.userRepo.CreateUser(userData, hashedPassword, initialDBRoles)
	if err != nil {
		return nil, err
	}

	// Log de Auditoria.
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
		Description: fmt.Sprintf("Novo usuário '%s' (Email: %s) criado por %s.", dbUser.Username, dbUser.Email, creatorUsername),
		Severity:    "INFO",
		Metadata:    map[string]interface{}{"new_user_id": dbUser.ID.String(), "new_username": dbUser.Username, "new_email": dbUser.Email, "creator": creatorUsername, "initial_roles": roleNamesForLog},
	}
	if logErr := s.auditLogService.LogAction(logEntry, logUserSession); logErr != nil {
		appLogger.Warnf("Falha ao registrar log de auditoria para criação do usuário '%s': %v", dbUser.Username, logErr)
	}

	// Enviar e-mail de boas-vindas (se EmailService estiver configurado).
	if s.emailService != nil && isSelfRegistration { // Envia e-mail apenas para auto-registro.
		go func(email, username string) { // Envia em goroutine para não bloquear.
			// O contexto para o template de e-mail pode ser preparado aqui.
			// Os templates usam {{.Username}}, {{.Email}}, {{.AppName}}, etc.
			emailContext := map[string]interface{}{
				"Username": username,
				"Email":    email,
				// AppURL pode ser uma configuração se a aplicação tiver um frontend web.
				// "AppURL": s.cfg.AppURL,
			}
			errMail := s.emailService.SendWelcomeEmail(email, username, emailContext)
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
	if !isEditingSelf { // Editando outro usuário
		if err := s.permManager.CheckPermission(currentUserSession, auth.PermUserUpdate, nil); err != nil {
			return nil, err
		}
	}
	// Permissão para gerenciar roles é separada se roles estão sendo alterados.
	if updateData.RoleNames != nil { // Se a intenção é alterar roles
		if isEditingSelf {
			// Usuário não pode alterar seus próprios roles diretamente por esta função.
			// Isso deve ser uma operação de admin ou um fluxo específico.
			// Se os roles em `updateData` forem os mesmos que os atuais, permite.
			currentUser, errSelf := s.userRepo.GetByID(userIDToUpdate)
			if errSelf != nil {
				return nil, errSelf
			}

			currentRoleNamesSet := make(map[string]bool)
			for _, r := range currentUser.Roles {
				currentRoleNamesSet[strings.ToLower(r.Name)] = true
			}

			newRoleNamesSet := make(map[string]bool)
			for _, rn := range *updateData.RoleNames {
				newRoleNamesSet[strings.ToLower(rn)] = true
			}

			if len(currentRoleNamesSet) != len(newRoleNamesSet) || !maps.Equal(currentRoleNamesSet, newRoleNamesSet) {
				return nil, fmt.Errorf("%w: não é permitido alterar os próprios perfis (roles) através desta função. Solicite a um administrador.", appErrors.ErrPermissionDenied)
			}
			// Se não houve mudança nos roles, define RoleNames como nil para não processar.
			updateData.RoleNames = nil
		} else { // Admin alterando roles de outro usuário.
			if err := s.permManager.CheckPermission(currentUserSession, auth.PermUserManageRoles, nil); err != nil {
				return nil, err
			}
		}
	}

	// 2. Buscar usuário a ser atualizado.
	userToUpdate, err := s.userRepo.GetByID(userIDToUpdate)
	if err != nil {
		return nil, err // Trata ErrNotFound.
	}

	// 3. Restrições de auto-edição (além de roles).
	if isEditingSelf {
		if updateData.Active != nil && *updateData.Active != userToUpdate.Active {
			return nil, fmt.Errorf("%w: não é permitido alterar o próprio status de ativação", appErrors.ErrPermissionDenied)
		}
		// Outras restrições de auto-edição podem ser adicionadas aqui.
	}

	// 4. Validar e Limpar Dados de Entrada.
	if err := updateData.CleanAndValidate(); err != nil {
		appLogger.Warnf("Dados de atualização de usuário inválidos para ID %s: %v", userIDToUpdate, err)
		return nil, err
	}

	// 5. Validar e obter os DBRoles se `updateData.RoleNames` foram fornecidos e são para serem alterados.
	var newDBRoles []*models.DBRole // Será nil se `updateData.RoleNames` for nil.
	if updateData.RoleNames != nil {
		var errRoles error
		newDBRoles, errRoles = s._validateAndGetDBRoles(*updateData.RoleNames)
		if errRoles != nil {
			return nil, errRoles
		}
		// Verificar se está tentando remover o último admin (apenas se admin estiver editando outro).
		if !isEditingSelf {
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

	// 6. Preparar mapa de atualização para o repositório.
	updateFields := make(map[string]interface{})
	changed := false // Flag para verificar se algo realmente mudou nos campos básicos.
	if updateData.Email != nil && *updateData.Email != userToUpdate.Email {
		// Verificar unicidade do novo email.
		if _, errCheckEmail := s.userRepo.GetByEmail(*updateData.Email); errCheckEmail == nil {
			return nil, fmt.Errorf("%w: o e-mail '%s' já está em uso por outro usuário", appErrors.ErrConflict, *updateData.Email)
		} else if !errors.Is(errCheckEmail, appErrors.ErrNotFound) {
			return nil, appErrors.WrapErrorf(errCheckEmail, "erro ao verificar novo e-mail para atualização")
		}
		updateFields["email"] = *updateData.Email
		changed = true
	}
	if updateData.FullName != nil && (userToUpdate.FullName == nil || *updateData.FullName != *userToUpdate.FullName) {
		updateFields["full_name"] = updateData.FullName // Pode ser nil para limpar
		changed = true
	}
	if updateData.Active != nil && *updateData.Active != userToUpdate.Active {
		// Se for desativar, verificar se é o último admin.
		if !(*updateData.Active) && s._isLastActiveAdmin(userIDToUpdate) {
			return nil, fmt.Errorf("%w: não é possível desativar o último administrador ativo do sistema", appErrors.ErrConflict)
		}
		updateFields["active"] = *updateData.Active
		changed = true
	}
	// Adicionar outros campos se UserUpdate os tiver (ex: IsSuperuser).

	// 7. Chamar Repositório se houver mudanças nos campos básicos ou nos roles.
	if !changed && newDBRoles == nil { // Se nem campos básicos nem roles mudaram.
		appLogger.Infof("Nenhuma alteração detectada para o usuário ID %s.", userIDToUpdate)
		return models.ToUserPublic(userToUpdate), nil
	}

	updatedUser, err := s.userRepo.UpdateUser(userIDToUpdate, updateFields, newDBRoles)
	if err != nil {
		return nil, err
	}

	// 8. Log de Auditoria.
	finalRoleNames := make([]string, len(updatedUser.Roles))
	for i, r := range updatedUser.Roles {
		finalRoleNames[i] = r.Name
	}

	logDescParts := []string{}
	if changed {
		logDescParts = append(logDescParts, fmt.Sprintf("campos básicos (%s)", strings.Join(maps.Keys(updateFields), ", ")))
	}
	if newDBRoles != nil {
		logDescParts = append(logDescParts, fmt.Sprintf("perfis para [%s]", strings.Join(finalRoleNames, ", ")))
	}

	logEntry := models.AuditLogEntry{
		Action:      "USER_UPDATE",
		Description: fmt.Sprintf("Usuário ID %s ('%s') atualizado por %s. Alterações: %s.", updatedUser.ID, updatedUser.Username, currentUserSession.Username, strings.Join(logDescParts, "; ")),
		Severity:    "INFO",
		Metadata:    map[string]interface{}{"updated_user_id": updatedUser.ID.String(), "updated_by": currentUserSession.UserID.String(), "updated_fields": maps.Keys(updateFields), "final_roles": finalRoleNames},
	}
	if logErr := s.auditLogService.LogAction(logEntry, currentUserSession); logErr != nil {
		appLogger.Warnf("Falha ao registrar log de auditoria para atualização do usuário ID %s: %v", updatedUser.ID, logErr)
	}

	// Se o usuário foi desativado, invalidar suas sessões.
	if val, ok := updateFields["active"]; ok && !val.(bool) {
		if removedCount, sessErr := s.sessionManager.DeleteAllUserSessions(userIDToUpdate); sessErr != nil {
			appLogger.Errorf("Erro ao tentar invalidar sessões do usuário (ID: %s) que foi desativado: %v", userIDToUpdate, sessErr)
		} else if removedCount > 0 {
			appLogger.Infof("%d sessões ativas invalidadas para o usuário (ID: %s) que foi desativado.", removedCount, userIDToUpdate)
		}
	}

	return models.ToUserPublic(updatedUser), nil
}

// _isLastActiveAdmin verifica se o usuário fornecido é o último administrador ativo.
func (s *userServiceImpl) _isLastActiveAdmin(userIDBeingChecked uuid.UUID) bool {
	// Esta implementação busca todos os usuários. Otimizar com query específica no repo se necessário.
	users, err := s.userRepo.GetAllUsers(true) // Pega todos (ativos e inativos) com roles.
	if err != nil {
		appLogger.Errorf("Erro ao buscar usuários para checagem _isLastActiveAdmin: %v", err)
		return false // Assume que não é o último se não conseguir verificar (fail-safe).
	}

	activeAdminCount := 0
	isUserBeingCheckedAnActiveAdmin := false

	for _, u := range users {
		if u.Active { // Considera apenas usuários ativos.
			isCurrentLoopUserAdmin := false
			for _, r := range u.Roles {
				if strings.ToLower(r.Name) == "admin" {
					isCurrentLoopUserAdmin = true
					break
				}
			}
			if isCurrentLoopUserAdmin {
				activeAdminCount++
				if u.ID == userIDBeingChecked {
					isUserBeingCheckedAnActiveAdmin = true
				}
			}
		}
	}
	return isUserBeingCheckedAnActiveAdmin && activeAdminCount <= 1
}

// DeactivateUser "deleta" logicamente um usuário.
func (s *userServiceImpl) DeactivateUser(userIDToDeactivate uuid.UUID, currentUserSession *auth.SessionData) error {
	if err := s.permManager.CheckPermission(currentUserSession, auth.PermUserDelete, nil); err != nil {
		return err
	}

	userToDeactivate, err := s.userRepo.GetByID(userIDToDeactivate)
	if err != nil {
		return err // Trata ErrNotFound.
	}

	if currentUserSession.UserID == userIDToDeactivate {
		return fmt.Errorf("%w: não é possível desativar a própria conta", appErrors.ErrPermissionDenied)
	}

	// Verificar se está tentando desativar o último admin ativo.
	if userToDeactivate.Active && s._isLastActiveAdmin(userIDToDeactivate) {
		return fmt.Errorf("%w: não é possível desativar o último administrador ativo do sistema", appErrors.ErrConflict)
	}

	if !userToDeactivate.Active {
		appLogger.Infof("Usuário ID %s ('%s') já está inativo.", userIDToDeactivate, userToDeactivate.Username)
		return nil // Nenhuma ação necessária.
	}

	if err := s.userRepo.DeactivateUser(userIDToDeactivate); err != nil {
		return err
	}

	// Log de Auditoria.
	logEntry := models.AuditLogEntry{
		Action:      "USER_DEACTIVATE",
		Description: fmt.Sprintf("Usuário '%s' (ID: %s) desativado por %s.", userToDeactivate.Username, userIDToDeactivate, currentUserSession.Username),
		Severity:    "WARNING",
		Metadata:    map[string]interface{}{"deactivated_user_id": userIDToDeactivate.String(), "deactivated_by_user_id": currentUserSession.UserID.String()},
	}
	if logErr := s.auditLogService.LogAction(logEntry, currentUserSession); logErr != nil {
		appLogger.Warnf("Falha ao registrar log de auditoria para desativação do usuário ID %s: %v", userIDToDeactivate, logErr)
	}

	// Invalidar todas as sessões ativas do usuário desativado.
	if removedCount, sessErr := s.sessionManager.DeleteAllUserSessions(userIDToDeactivate); sessErr != nil {
		appLogger.Errorf("Erro ao tentar invalidar sessões do usuário desativado ID %s: %v", userIDToDeactivate, sessErr)
	} else if removedCount > 0 {
		appLogger.Infof("%d sessões ativas invalidadas para o usuário desativado ID %s.", removedCount, userIDToDeactivate)
	}

	return nil
}

// GetUserByID busca um usuário pelo ID.
func (s *userServiceImpl) GetUserByID(userID uuid.UUID, currentUserSession *auth.SessionData) (*models.UserPublic, error) {
	isSelf := currentUserSession.UserID == userID
	if !isSelf { // Se não estiver buscando a si mesmo, requer permissão de leitura.
		if err := s.permManager.CheckPermission(currentUserSession, auth.PermUserRead, nil); err != nil {
			return nil, err
		}
	}
	// Se for `isSelf`, permite buscar os próprios dados sem `PermUserRead` explícito.

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
		return fmt.Errorf("%w: não é permitido alterar a senha de outro usuário ou sem uma sessão válida para si mesmo", appErrors.ErrPermissionDenied)
	}

	user, err := s.userRepo.GetByID(userID) // GetByID do repo já carrega o hash.
	if err != nil {
		return err // NotFound ou DB error.
	}

	if !auth.VerifyPassword(oldPassword, user.PasswordHash) {
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
		return appErrors.NewValidationError("Nova senha inválida ou fraca.", strength.GetErrorDetailsList())
	}

	newHash, errHash := auth.HashPassword(newPassword)
	if errHash != nil {
		return errHash
	}

	if err := s.userRepo.UpdatePasswordHash(userID, newHash); err != nil {
		return err
	}

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
		return fmt.Errorf("%w: administradores devem usar a opção 'Alterar Minha Senha' para sua própria conta, não o reset administrativo", appErrors.ErrPermissionDenied)
	}

	strength := utils.ValidatePasswordStrength(newPassword, s.cfg.PasswordMinLength)
	if !strength.IsValid {
		return appErrors.NewValidationError("Nova senha inválida ou fraca.", strength.GetErrorDetailsList())
	}

	if auth.VerifyPassword(newPassword, userToReset.PasswordHash) {
		return appErrors.NewValidationError("A nova senha deve ser diferente da senha atual do usuário.", map[string]string{"new_password": "Deve ser diferente da atual"})
	}

	newHash, errHash := auth.HashPassword(newPassword)
	if errHash != nil {
		return errHash
	}

	if err := s.userRepo.UpdatePasswordHash(userIDToReset, newHash); err != nil {
		return err
	}

	logEntry := models.AuditLogEntry{
		Action: "PASSWORD_RESET_ADMIN", Description: fmt.Sprintf("Senha do usuário '%s' (ID: %s) redefinida por %s.", userToReset.Username, userIDToReset, currentUserSession.Username),
		Severity: "WARNING", Metadata: map[string]interface{}{"reset_user_id": userIDToReset.String(), "admin_user_id": currentUserSession.UserID.String()},
	}
	s.auditLogService.LogAction(logEntry, currentUserSession)
	return nil
}

// InitiatePasswordReset inicia o processo de recuperação de senha pelo usuário (envia token por e-mail).
func (s *userServiceImpl) InitiatePasswordReset(email, ipAddress string) error {
	if s.emailService == nil {
		return fmt.Errorf("%w: serviço de e-mail não configurado, recuperação de senha indisponível", appErrors.ErrConfiguration)
	}

	normalizedEmail := strings.ToLower(strings.TrimSpace(email))
	if errVal := utils.ValidateEmail(normalizedEmail); errVal != nil { // Valida formato do email
		appLogger.Warnf("Tentativa de reset de senha com e-mail em formato inválido: '%s'", email)
		// Não retorna erro detalhado para o usuário para evitar enumeração de e-mails.
		return nil
	}

	user, err := s.userRepo.GetByEmail(normalizedEmail)
	if err != nil {
		if errors.Is(err, appErrors.ErrNotFound) {
			appLogger.Warnf("Tentativa de reset de senha para e-mail não cadastrado: %s", normalizedEmail)
			s.auditLogService.LogAction(models.AuditLogEntry{
				Action: "PASSWORD_RESET_INIT_EMAIL_NOT_FOUND", Description: fmt.Sprintf("Tentativa de iniciar reset para e-mail não existente: %s", normalizedEmail),
				Severity: "INFO", IPAddress: &ipAddress, Metadata: map[string]interface{}{"email_attempted": normalizedEmail},
			}, nil)
			return nil // Não revela se o e-mail existe.
		}
		return err // Outro erro de DB.
	}

	if !user.Active {
		appLogger.Warnf("Tentativa de reset de senha para usuário inativo: %s (Email: %s)", user.Username, normalizedEmail)
		s.auditLogService.LogAction(models.AuditLogEntry{
			Action: "PASSWORD_RESET_INIT_INACTIVE_USER", Description: fmt.Sprintf("Tentativa de iniciar reset para usuário inativo '%s'", user.Username),
			Severity: "WARNING", UserID: &user.ID, IPAddress: &ipAddress, Metadata: map[string]interface{}{"user_id": user.ID.String()},
		}, nil)
		return nil // Não envia e-mail para inativos.
	}

	resetTokenPlain := utils.GenerateSecureRandomToken(32)
	tokenHash, errHash := auth.HashPassword(resetTokenPlain)
	if errHash != nil {
		return errHash
	}
	expiresAt := time.Now().UTC().Add(s.cfg.PasswordResetTimeout)

	if err := s.userRepo.UpdatePasswordResetToken(user.ID, &tokenHash, &expiresAt); err != nil {
		return err
	}

	if err := s.emailService.SendPasswordResetCode(user.Email, resetTokenPlain, ipAddress); err != nil {
		appLogger.Errorf("Falha ao enviar e-mail de reset de senha para %s: %v. Revertendo token no DB.", user.Email, err)
		// Reverter token se o e-mail falhar é uma boa prática.
		if errClearToken := s.userRepo.UpdatePasswordResetToken(user.ID, nil, nil); errClearToken != nil {
			appLogger.Errorf("Falha crítica ao tentar limpar token de reset após falha no envio de email para %s: %v", user.Email, errClearToken)
		}
		return err // Retorna o EmailError.
	}

	s.auditLogService.LogAction(models.AuditLogEntry{
		Action: "PASSWORD_RESET_INIT_SUCCESS", Description: fmt.Sprintf("Reset de senha iniciado para usuário '%s'. E-mail enviado para %s.", user.Username, user.Email),
		Severity: "INFO", UserID: &user.ID, IPAddress: &ipAddress, Metadata: map[string]interface{}{"user_id": user.ID.String(), "email": user.Email},
	}, nil)
	return nil
}

// ConfirmPasswordReset confirma o reset usando o token e define a nova senha.
func (s *userServiceImpl) ConfirmPasswordReset(email, resetTokenPlain, newPassword string) error {
	normalizedEmail := strings.ToLower(strings.TrimSpace(email))
	user, err := s.userRepo.GetByEmail(normalizedEmail)
	if err != nil {
		if errors.Is(err, appErrors.ErrNotFound) {
			return fmt.Errorf("%w: token de reset inválido ou expirado (usuário não encontrado para e-mail '%s')", appErrors.ErrInvalidCredentials, normalizedEmail)
		}
		return err
	}

	if user.PasswordResetToken == nil || *user.PasswordResetToken == "" || user.PasswordResetExpires == nil {
		return fmt.Errorf("%w: token de reset inválido ou não solicitado para este usuário", appErrors.ErrInvalidCredentials)
	}
	if time.Now().UTC().After(*user.PasswordResetExpires) {
		s.userRepo.UpdatePasswordResetToken(user.ID, nil, nil) // Limpa token expirado.
		return fmt.Errorf("%w: token de reset expirado", appErrors.ErrTokenExpired)
	}

	if !auth.VerifyPassword(resetTokenPlain, *user.PasswordResetToken) {
		// Logar tentativa falha de uso de token (sem incrementar falhas de login).
		// Pode-se adicionar um contador de falhas de token de reset para mitigar ataques.
		return fmt.Errorf("%w: token de reset inválido", appErrors.ErrInvalidCredentials)
	}

	strength := utils.ValidatePasswordStrength(newPassword, s.cfg.PasswordMinLength)
	if !strength.IsValid {
		return appErrors.NewValidationError("Nova senha inválida ou fraca.", strength.GetErrorDetailsList())
	}
	if auth.VerifyPassword(newPassword, user.PasswordHash) {
		return appErrors.NewValidationError("A nova senha deve ser diferente da senha atual.", map[string]string{"new_password": "Deve ser diferente da atual"})
	}

	newHash, errHash := auth.HashPassword(newPassword)
	if errHash != nil {
		return errHash
	}

	// `UpdatePasswordHash` também limpa os campos de reset e tentativas de login.
	if err := s.userRepo.UpdatePasswordHash(user.ID, newHash); err != nil {
		return err
	}

	s.auditLogService.LogAction(models.AuditLogEntry{
		Action: "PASSWORD_RESET_CONFIRM_SUCCESS", Description: fmt.Sprintf("Senha redefinida com sucesso via recuperação para usuário '%s'.", user.Username),
		Severity: "INFO", UserID: &user.ID, Metadata: map[string]interface{}{"user_id": user.ID.String(), "email": user.Email},
	}, nil)
	return nil
}

// UnlockUser desbloqueia uma conta que foi bloqueada por excesso de tentativas de login.
func (s *userServiceImpl) UnlockUser(userIDToUnlock uuid.UUID, currentUserSession *auth.SessionData) error {
	if err := s.permManager.CheckPermission(currentUserSession, auth.PermUserUnlock, nil); err != nil {
		return err
	}

	userToUnlock, err := s.userRepo.GetByID(userIDToUnlock)
	if err != nil {
		return err
	}

	if userToUnlock.FailedAttempts == 0 && userToUnlock.LastFailedLogin == nil {
		appLogger.Infof("Usuário ID %s ('%s') já estava desbloqueado (ou nunca foi bloqueado).", userIDToUnlock, userToUnlock.Username)
		return nil // Nenhuma ação necessária.
	}

	// Reseta tentativas de login falhas e o timestamp da última falha.
	// `LastLogin` não é alterado aqui, pois não é um login.
	if err := s.userRepo.UpdateLoginAttempts(userIDToUnlock, 0, nil, userToUnlock.LastLogin); err != nil {
		return err
	}

	logEntry := models.AuditLogEntry{
		Action: "ACCOUNT_UNLOCK_ADMIN", Description: fmt.Sprintf("Conta do usuário '%s' (ID: %s) desbloqueada por %s.", userToUnlock.Username, userIDToUnlock, currentUserSession.Username),
		Severity: "INFO", UserID: &userToUnlock.ID, Metadata: map[string]interface{}{"unlocked_user_id": userIDToUnlock.String(), "admin_user_id": currentUserSession.UserID.String()},
	}
	s.auditLogService.LogAction(logEntry, currentUserSession)
	return nil
}
