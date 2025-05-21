package services

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/auth"
	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/errors"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/models"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/repositories"
)

// RoleService define a interface para o serviço de Role.
type RoleService interface {
	// CreateRole cria um novo role. `roleData.Name` deve ser normalizado.
	CreateRole(roleData models.RoleCreate, currentUserSession *auth.SessionData) (*models.RolePublic, error)

	GetRoleByID(roleID uint64, currentUserSession *auth.SessionData) (*models.RolePublic, error)
	GetRoleByName(name string, currentUserSession *auth.SessionData) (*models.RolePublic, error) // `name` deve ser normalizado.
	GetAllRoles(currentUserSession *auth.SessionData) ([]*models.RolePublic, error)

	// UpdateRole atualiza um role. Campos em `roleData` devem ser normalizados.
	UpdateRole(roleID uint64, roleData models.RoleUpdate, currentUserSession *auth.SessionData) (*models.RolePublic, error)

	// DeleteRole exclui um role.
	DeleteRole(roleID uint64, currentUserSession *auth.SessionData) error

	GetRolePermissions(roleID uint64, currentUserSession *auth.SessionData) ([]string, error)
	// SetRolePermissions define/sobrescreve todas as permissões para um role.
	// `permissionNames` devem ser validados pelo serviço.
	// SetRolePermissions(roleID uint64, permissionNames []string, currentUserSession *auth.SessionData) error
}

// roleServiceImpl é a implementação de RoleService.
type roleServiceImpl struct {
	repo            repositories.RoleRepository
	auditLogService AuditLogService
	permManager     *auth.PermissionManager // Usado para validar nomes de permissão e verificar permissões de gerenciamento.
}

// NewRoleService cria uma nova instância de RoleService.
func NewRoleService(
	repo repositories.RoleRepository,
	auditLog AuditLogService,
	pm *auth.PermissionManager, // PermissionManager agora é uma dependência explícita.
) RoleService {
	if repo == nil || auditLog == nil || pm == nil {
		appLogger.Fatalf("Dependências nulas fornecidas para NewRoleService (repo, auditLog, permManager)")
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
		return nil // Nenhuma permissão para validar é aceitável.
	}
	invalidNames := []string{}
	uniquePermNames := make(map[string]bool) // Para remover duplicatas antes de validar.

	for _, pName := range permissionNames {
		trimmedPermName := strings.TrimSpace(pName)
		if trimmedPermName == "" {
			continue // Ignora nomes vazios.
		}
		// A validação deve usar o tipo `auth.Permission`.
		if !s.permManager.IsPermissionDefined(auth.Permission(trimmedPermName)) {
			if !uniquePermNames[trimmedPermName] { // Adiciona à lista de inválidos apenas uma vez.
				invalidNames = append(invalidNames, trimmedPermName)
				uniquePermNames[trimmedPermName] = true
			}
		}
	}
	if len(invalidNames) > 0 {
		return appErrors.NewValidationError(
			fmt.Sprintf("Permissões inválidas ou não definidas no sistema: %s", strings.Join(invalidNames, ", ")),
			map[string]string{"permission_names": "Contém permissões inválidas: " + strings.Join(invalidNames, ", ")},
		)
	}
	return nil
}

// CreateRole cria um novo role.
func (s *roleServiceImpl) CreateRole(roleData models.RoleCreate, currentUserSession *auth.SessionData) (*models.RolePublic, error) {
	// 1. Verificar Permissão para gerenciar roles.
	if err := s.permManager.CheckPermission(currentUserSession, auth.PermRoleManage, nil); err != nil {
		return nil, err
	}

	// 2. Validar e Limpar Dados de Entrada (formato, normalização).
	// `CleanAndValidate` normaliza `Name` para minúsculas e `PermissionNames`.
	if err := roleData.CleanAndValidate(); err != nil {
		appLogger.Warnf("Dados de criação de role inválidos para '%s': %v", roleData.Name, err)
		return nil, err // Retorna o ValidationError.
	}

	// 3. Validar se os nomes de permissão existem no sistema.
	if err := s._validatePermissionNames(roleData.PermissionNames); err != nil {
		appLogger.Warnf("Nomes de permissão inválidos ao criar role '%s': %v", roleData.Name, err)
		return nil, err
	}

	// 4. Verificar Unicidade do Nome do Role.
	// O repositório também fará essa checagem, mas verificar no serviço permite melhor feedback.
	// `roleData.Name` já está normalizado (minúsculas).
	if _, err := s.repo.GetByName(roleData.Name); err == nil {
		return nil, fmt.Errorf("%w: já existe um perfil (role) com o nome '%s'", appErrors.ErrConflict, roleData.Name)
	} else if !errors.Is(err, appErrors.ErrNotFound) {
		return nil, appErrors.WrapErrorf(err, "erro ao verificar unicidade do nome do role '%s'", roleData.Name)
	}
	// Se ErrNotFound, o nome está disponível.

	// 5. Chamar Repositório para criar o role.
	// `isSystemRole` é `false` para roles criados pela UI/API.
	dbRole, err := s.repo.Create(roleData, false)
	if err != nil {
		return nil, err // Erro já logado e formatado pelo repo.
	}

	// 6. Log de Auditoria
	logEntry := models.AuditLogEntry{
		Action:      "ROLE_CREATE",
		Description: fmt.Sprintf("Perfil (role) '%s' criado.", dbRole.Name),
		Severity:    "INFO",
		Metadata:    map[string]interface{}{"role_id": dbRole.ID, "role_name": dbRole.Name, "permissions_count": len(dbRole.Permissions)},
	}
	if logErr := s.auditLogService.LogAction(logEntry, currentUserSession); logErr != nil {
		appLogger.Warnf("Falha ao registrar log de auditoria para criação do role '%s': %v", dbRole.Name, logErr)
	}

	return models.ToRolePublic(dbRole), nil
}

// GetRoleByID busca um role pelo ID.
func (s *roleServiceImpl) GetRoleByID(roleID uint64, currentUserSession *auth.SessionData) (*models.RolePublic, error) {
	// A permissão `PermRoleManage` geralmente cobre visualização. Se houver `PermRoleRead`, usar essa.
	if err := s.permManager.CheckPermission(currentUserSession, auth.PermRoleManage, nil); err != nil {
		return nil, err
	}
	dbRole, err := s.repo.GetByID(roleID)
	if err != nil {
		return nil, err // Repo trata ErrNotFound.
	}
	return models.ToRolePublic(dbRole), nil
}

// GetRoleByName busca um role pelo nome.
func (s *roleServiceImpl) GetRoleByName(name string, currentUserSession *auth.SessionData) (*models.RolePublic, error) {
	if err := s.permManager.CheckPermission(currentUserSession, auth.PermRoleManage, nil); err != nil {
		return nil, err
	}
	normalizedName := strings.ToLower(strings.TrimSpace(name))
	if normalizedName == "" {
		return nil, fmt.Errorf("%w: nome do role para busca não pode ser vazio", appErrors.ErrInvalidInput)
	}
	dbRole, err := s.repo.GetByName(normalizedName) // Repo espera nome normalizado.
	if err != nil {
		return nil, err
	}
	return models.ToRolePublic(dbRole), nil
}

// GetAllRoles busca todos os roles.
func (s *roleServiceImpl) GetAllRoles(currentUserSession *auth.SessionData) ([]*models.RolePublic, error) {
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

	// 2. Buscar role existente para verificações (ex: IsSystemRole).
	existingRole, err := s.repo.GetByID(roleID)
	if err != nil {
		return nil, err // Trata ErrNotFound.
	}

	// 3. Restrições para Roles do Sistema.
	if existingRole.IsSystemRole {
		if roleData.Name != nil && strings.ToLower(strings.TrimSpace(*roleData.Name)) != existingRole.Name {
			return nil, fmt.Errorf("%w: não é permitido renomear roles do sistema ('%s')", appErrors.ErrPermissionDenied, existingRole.Name)
		}
		// Poderia haver restrições para alterar permissões de system roles também, dependendo da política.
		// Ex: if roleData.PermissionNames != nil && existingRole.Name == "admin" { /* não permitir */ }
	}

	// 4. Validar e Limpar Dados de Entrada
	if err := roleData.CleanAndValidate(); err != nil {
		appLogger.Warnf("Dados de atualização de role inválidos para ID %d: %v", roleID, err)
		return nil, err
	}

	// 5. Validar Nomes de Permissão (se fornecidos para alteração).
	if roleData.PermissionNames != nil { // Só valida se `PermissionNames` não for nil.
		if err := s._validatePermissionNames(*roleData.PermissionNames); err != nil {
			appLogger.Warnf("Nomes de permissão inválidos ao atualizar role ID %d: %v", roleID, err)
			return nil, err
		}
	}

	// 6. Verificar Unicidade do Novo Nome (se estiver sendo alterado).
	if roleData.Name != nil && *roleData.Name != existingRole.Name {
		// `*roleData.Name` já está normalizado (minúsculas).
		if _, errCheckName := s.repo.GetByName(*roleData.Name); errCheckName == nil {
			return nil, fmt.Errorf("%w: já existe outro perfil (role) com o nome '%s'", appErrors.ErrConflict, *roleData.Name)
		} else if !errors.Is(errCheckName, appErrors.ErrNotFound) {
			return nil, appErrors.WrapErrorf(errCheckName, "erro ao verificar novo nome para atualização do role ID %d", roleID)
		}
	}

	// 7. Chamar Repositório
	dbRole, err := s.repo.Update(roleID, roleData)
	if err != nil {
		return nil, err
	}

	// 8. Log de Auditoria
	updatedFieldsLog := []string{}
	meta := map[string]interface{}{"role_id": dbRole.ID, "role_name_after_update": dbRole.Name}
	if roleData.Name != nil {
		updatedFieldsLog = append(updatedFieldsLog, fmt.Sprintf("nome para '%s'", *roleData.Name))
		meta["new_name"] = *roleData.Name
	}
	if roleData.Description != nil {
		descStr := "(limpo)"
		if roleData.Description != nil { // Se não for nil explicitamente
			descStr = fmt.Sprintf("'%s'", *roleData.Description)
		}
		updatedFieldsLog = append(updatedFieldsLog, fmt.Sprintf("descrição para %s", descStr))
		meta["new_description"] = roleData.Description // Pode ser nil
	}
	if roleData.PermissionNames != nil {
		updatedFieldsLog = append(updatedFieldsLog, fmt.Sprintf("permissões (total: %d)", len(*roleData.PermissionNames)))
		meta["new_permissions_count"] = len(*roleData.PermissionNames)
	}

	logEntry := models.AuditLogEntry{
		Action:      "ROLE_UPDATE",
		Description: fmt.Sprintf("Perfil (role) '%s' (ID: %d) atualizado. Campos: %s.", dbRole.Name, dbRole.ID, strings.Join(updatedFieldsLog, ", ")),
		Severity:    "INFO",
		Metadata:    meta,
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

	// 2. Buscar role para verificar se é `IsSystemRole` e para logar o nome.
	roleToDelete, err := s.repo.GetByID(roleID)
	if err != nil {
		return err // Trata ErrNotFound.
	}

	if roleToDelete.IsSystemRole {
		return fmt.Errorf("%w: role do sistema '%s' não pode ser excluído", appErrors.ErrPermissionDenied, roleToDelete.Name)
	}

	// 3. Chamar Repositório para exclusão.
	// O repositório deve lidar com a remoção de associações (role_permissions, user_roles).
	if err := s.repo.Delete(roleID); err != nil {
		// Erros como ErrConflict (role em uso por usuários e DB impede) são tratados pelo repo.
		return err
	}

	// 4. Log de Auditoria
	logEntry := models.AuditLogEntry{
		Action:      "ROLE_DELETE",
		Description: fmt.Sprintf("Perfil (role) '%s' (ID: %d) excluído.", roleToDelete.Name, roleID),
		Severity:    "WARNING", // Exclusão é geralmente um Warning.
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
	// Verificar se o role existe primeiro.
	if _, err := s.repo.GetByID(roleID); err != nil {
		return nil, err // Trata ErrNotFound.
	}
	return s.repo.GetPermissionsForRole(roleID)
}
