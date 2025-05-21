package services

import (
	"fmt"
	"strings"

	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/auth" // Para SessionData e PermissionManager/Permission
	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/errors"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/models"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/repositories"
)

// RoleService define a interface para o serviço de Role.
type RoleService interface {
	CreateRole(roleData models.RoleCreate, currentUserSession *auth.SessionData) (*models.RolePublic, error)
	GetRoleByID(roleID uint64, currentUserSession *auth.SessionData) (*models.RolePublic, error)
	GetRoleByName(name string, currentUserSession *auth.SessionData) (*models.RolePublic, error)
	GetAllRoles(currentUserSession *auth.SessionData) ([]*models.RolePublic, error)
	UpdateRole(roleID uint64, roleData models.RoleUpdate, currentUserSession *auth.SessionData) (*models.RolePublic, error)
	DeleteRole(roleID uint64, currentUserSession *auth.SessionData) error
	GetRolePermissions(roleID uint64, currentUserSession *auth.SessionData) ([]string, error)
	// SetRolePermissions(roleID uint64, permissionNames []string, currentUserSession *auth.SessionData) error // Opcional, se UpdateRole não for suficiente
}

// roleServiceImpl é a implementação de RoleService.
type roleServiceImpl struct {
	repo            repositories.RoleRepository
	auditLogService AuditLogService
	permManager     *auth.PermissionManager // Usado para validar nomes de permissão
}

// NewRoleService cria uma nova instância de RoleService.
func NewRoleService(
	repo repositories.RoleRepository,
	auditLog AuditLogService,
	pm *auth.PermissionManager,
) RoleService {
	if repo == nil || auditLog == nil || pm == nil {
		appLogger.Fatalf("Dependências nulas fornecidas para NewRoleService")
	}
	return &roleServiceImpl{
		repo:            repo,
		auditLogService: auditLog,
		permManager:     pm,
	}
}

// _validatePermissionNames verifica se todos os nomes de permissão fornecidos são válidos e definidos no sistema.
func (s *roleServiceImpl) _validatePermissionNames(permissionNames []string) error {
	if len(permissionNames) == 0 {
		return nil // Nenhuma permissão para validar
	}
	invalidNames := []string{}
	for _, pName := range permissionNames {
		if !s.permManager.IsPermissionDefined(auth.Permission(pName)) { // Converte para auth.Permission
			invalidNames = append(invalidNames, pName)
		}
	}
	if len(invalidNames) > 0 {
		return appErrors.NewValidationError(
			fmt.Sprintf("Permissões inválidas ou não definidas: %s", strings.Join(invalidNames, ", ")),
			map[string]string{"permission_names": "Contém permissões inválidas"},
		)
	}
	return nil
}

// CreateRole cria um novo role.
func (s *roleServiceImpl) CreateRole(roleData models.RoleCreate, currentUserSession *auth.SessionData) (*models.RolePublic, error) {
	// 1. Verificar Permissão
	if err := s.permManager.CheckPermission(currentUserSession, auth.PermRoleManage, nil); err != nil {
		return nil, err
	}

	// 2. Validar e Limpar Dados de Entrada
	if err := roleData.CleanAndValidate(); err != nil { // Chama método do modelo
		appLogger.Warnf("Dados de criação de role inválidos para '%s': %v", roleData.Name, err)
		return nil, err
	}
	if err := s._validatePermissionNames(roleData.PermissionNames); err != nil {
		appLogger.Warnf("Nomes de permissão inválidos ao criar role '%s': %v", roleData.Name, err)
		return nil, err
	}

	// 3. Chamar Repositório
	// O repositório verificará a unicidade do nome.
	// Passamos isSystemRole=false para roles criados pelo usuário/admin.
	dbRole, err := s.repo.Create(roleData, false)
	if err != nil {
		return nil, err
	}

	// 4. Log de Auditoria
	logEntry := models.AuditLogEntry{
		Action:      "ROLE_CREATE",
		Description: fmt.Sprintf("Role '%s' criado.", dbRole.Name),
		Severity:    "INFO",
		Metadata:    map[string]interface{}{"role_id": dbRole.ID, "role_name": dbRole.Name, "permissions_count": len(dbRole.Permissions)},
	}
	if logErr := s.auditLogService.LogAction(logEntry, currentUserSession); logErr != nil {
		appLogger.Warnf("Falha ao registrar log de auditoria para criação do role %s: %v", dbRole.Name, logErr)
	}

	return models.ToRolePublic(dbRole), nil
}

// GetRoleByID busca um role pelo ID.
func (s *roleServiceImpl) GetRoleByID(roleID uint64, currentUserSession *auth.SessionData) (*models.RolePublic, error) {
	if err := s.permManager.CheckPermission(currentUserSession, auth.PermRoleManage, nil); err != nil {
		return nil, err
	}
	dbRole, err := s.repo.GetByID(roleID)
	if err != nil {
		return nil, err
	}
	return models.ToRolePublic(dbRole), nil
}

// GetRoleByName busca um role pelo nome.
func (s *roleServiceImpl) GetRoleByName(name string, currentUserSession *auth.SessionData) (*models.RolePublic, error) {
	if err := s.permManager.CheckPermission(currentUserSession, auth.PermRoleManage, nil); err != nil {
		return nil, err
	}
	dbRole, err := s.repo.GetByName(name)
	if err != nil {
		return nil, err
	}
	return models.ToRolePublic(dbRole), nil
}

// GetAllRoles busca todos os roles.
func (s *roleServiceImpl) GetAllRoles(currentUserSession *auth.SessionData) ([]*models.RolePublic, error) {
	// A permissão 'role:manage' implicitamente permite listar roles.
	// Se houvesse uma permissão 'role:read', seria usada aqui.
	if err := s.permManager.CheckPermission(currentUserSession, auth.PermRoleManage, nil); err != nil {
		return nil, err
	}
	dbRoles, err := s.repo.GetAll()
	if err != nil {
		return nil, err
	}
	return models.ToRolePublicList(dbRoles), nil
}

// UpdateRole atualiza um role existente e/ou suas permissões.
func (s *roleServiceImpl) UpdateRole(roleID uint64, roleData models.RoleUpdate, currentUserSession *auth.SessionData) (*models.RolePublic, error) {
	// 1. Verificar Permissão
	if err := s.permManager.CheckPermission(currentUserSession, auth.PermRoleManage, nil); err != nil {
		return nil, err
	}

	// 2. Buscar role existente para verificações
	existingRole, err := s.repo.GetByID(roleID)
	if err != nil {
		return nil, err // Trata ErrNotFound
	}
	if existingRole.IsSystemRole && roleData.Name != nil && strings.ToLower(*roleData.Name) != existingRole.Name {
		return nil, fmt.Errorf("%w: não é permitido renomear roles do sistema ('%s')", appErrors.ErrPermissionDenied, existingRole.Name)
	}

	// 3. Validar e Limpar Dados de Entrada
	if err := roleData.CleanAndValidate(); err != nil { // Chama método do modelo
		appLogger.Warnf("Dados de atualização de role inválidos para ID %d: %v", roleID, err)
		return nil, err
	}
	if roleData.PermissionNames != nil { // Só valida se foi fornecido para alteração
		if err := s._validatePermissionNames(*roleData.PermissionNames); err != nil {
			appLogger.Warnf("Nomes de permissão inválidos ao atualizar role ID %d: %v", roleID, err)
			return nil, err
		}
	}

	// 4. Verificar se há algo para atualizar
	// (O repositório também pode fazer essa checagem, mas é bom ter no serviço também)
	// Esta checagem é mais complexa aqui porque `roleData` usa ponteiros.
	// O repositório Update já lida com não fazer nada se nada mudar.

	// 5. Chamar Repositório
	dbRole, err := s.repo.Update(roleID, roleData)
	if err != nil {
		return nil, err
	}

	// 6. Log de Auditoria
	updatedFields := []string{}
	if roleData.Name != nil {
		updatedFields = append(updatedFields, "name")
	}
	if roleData.Description != nil {
		updatedFields = append(updatedFields, "description")
	}
	if roleData.PermissionNames != nil {
		updatedFields = append(updatedFields, "permission_names")
	}

	logEntry := models.AuditLogEntry{
		Action:      "ROLE_UPDATE",
		Description: fmt.Sprintf("Role '%s' (ID: %d) atualizado.", dbRole.Name, dbRole.ID),
		Severity:    "INFO",
		Metadata:    map[string]interface{}{"role_id": dbRole.ID, "role_name": dbRole.Name, "updated_fields": updatedFields},
	}
	if logErr := s.auditLogService.LogAction(logEntry, currentUserSession); logErr != nil {
		appLogger.Warnf("Falha ao registrar log de auditoria para atualização do role ID %d: %v", dbRole.ID, logErr)
	}

	return models.ToRolePublic(dbRole), nil
}

// DeleteRole exclui um role.
func (s *roleServiceImpl) DeleteRole(roleID uint64, currentUserSession *auth.SessionData) error {
	// 1. Verificar Permissão
	if err := s.permManager.CheckPermission(currentUserSession, auth.PermRoleManage, nil); err != nil {
		return err
	}

	// 2. Buscar role para verificar se é system role e para log
	roleToDelete, err := s.repo.GetByID(roleID)
	if err != nil {
		return err // Trata ErrNotFound
	}

	if roleToDelete.IsSystemRole {
		return fmt.Errorf("%w: role do sistema '%s' não pode ser excluído", appErrors.ErrPermissionDenied, roleToDelete.Name)
	}

	// 3. Chamar Repositório
	if err := s.repo.Delete(roleID); err != nil {
		// Erros como ErrConflict (role em uso por usuários) são tratados pelo repo.
		return err
	}

	// 4. Log de Auditoria
	logEntry := models.AuditLogEntry{
		Action:      "ROLE_DELETE",
		Description: fmt.Sprintf("Role '%s' (ID: %d) excluído.", roleToDelete.Name, roleID),
		Severity:    "WARNING", // Exclusão é geralmente um Warning
		Metadata:    map[string]interface{}{"deleted_role_id": roleID, "deleted_role_name": roleToDelete.Name},
	}
	if logErr := s.auditLogService.LogAction(logEntry, currentUserSession); logErr != nil {
		appLogger.Warnf("Falha ao registrar log de auditoria para exclusão do role ID %d: %v", roleID, logErr)
	}

	return nil
}

// GetRolePermissions obtém as permissões de um role específico.
func (s *roleServiceImpl) GetRolePermissions(roleID uint64, currentUserSession *auth.SessionData) ([]string, error) {
	if err := s.permManager.CheckPermission(currentUserSession, auth.PermRoleManage, nil); err != nil {
		return nil, err
	}
	// Verificar se o role existe primeiro
	if _, err := s.repo.GetByID(roleID); err != nil {
		return nil, err
	}
	return s.repo.GetPermissionsForRole(roleID)
}
