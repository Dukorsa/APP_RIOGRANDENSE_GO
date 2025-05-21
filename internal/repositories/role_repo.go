package repositories

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
	// "gorm.io/gorm/clause" // Para OnConflict, se fosse usar upsert de roles

	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/models"
)

// RoleRepository define a interface para operações no repositório de roles.
type RoleRepository interface {
	// Create cria um novo role e associa suas permissões.
	// `roleData.Name` deve estar normalizado (minúsculas).
	// `isSystemRole` indica se o role é um role do sistema.
	Create(roleData models.RoleCreate, isSystemRole bool) (*models.DBRole, error)

	GetByID(roleID uint64) (*models.DBRole, error)
	GetByName(name string) (*models.DBRole, error) // `name` deve estar normalizado (minúsculas).
	GetAll() ([]models.DBRole, error)

	// Update atualiza um role existente e/ou suas permissões.
	// `roleUpdateData` campos devem estar normalizados.
	Update(roleID uint64, roleUpdateData models.RoleUpdate) (*models.DBRole, error)

	// Delete remove um role e suas associações.
	// O serviço deve verificar se é um system role antes de chamar.
	Delete(roleID uint64) error

	// GetPermissionsForRole busca os nomes das permissões associadas a um roleID.
	GetPermissionsForRole(roleID uint64) ([]string, error)

	// SetRolePermissions define/sobrescreve todas as permissões para um role.
	// `permissionNames` deve conter nomes de permissões válidos e normalizados.
	SetRolePermissions(roleID uint64, permissionNames []string) error
}

// gormRoleRepository é a implementação GORM de RoleRepository.
type gormRoleRepository struct {
	db *gorm.DB
}

// NewGormRoleRepository cria uma nova instância de gormRoleRepository.
func NewGormRoleRepository(db *gorm.DB) RoleRepository {
	if db == nil {
		appLogger.Fatalf("gorm.DB não pode ser nil para NewGormRoleRepository")
	}
	return &gormRoleRepository{db: db}
}

// loadRolePermissions busca e anexa os nomes das permissões a um DBRole.
// Esta é uma função helper interna para reutilização.
func (r *gormRoleRepository) loadRolePermissions(dbRole *models.DBRole) error {
	if dbRole == nil || dbRole.ID == 0 {
		dbRole.Permissions = []string{} // Garante que seja um slice vazio, não nil
		return nil
	}
	var rolePerms []models.DBRolePermission
	// Busca todas as entradas na tabela de junção para o roleID.
	if err := r.db.Where("role_id = ?", dbRole.ID).Find(&rolePerms).Error; err != nil {
		appLogger.Errorf("Erro ao buscar permissões para role ID %d ('%s'): %v", dbRole.ID, dbRole.Name, err)
		return appErrors.WrapErrorf(err, "falha ao buscar permissões do role (GORM)")
	}

	dbRole.Permissions = make([]string, len(rolePerms))
	for i, rp := range rolePerms {
		dbRole.Permissions[i] = rp.PermissionName
	}
	return nil
}

// Create cria um novo role e associa suas permissões.
// `roleData.Name` já deve estar normalizado (minúsculas) pelo serviço.
// `roleData.PermissionNames` já devem ser validados pelo serviço.
func (r *gormRoleRepository) Create(roleData models.RoleCreate, isSystemRole bool) (*models.DBRole, error) {
	// O serviço já deve ter chamado CleanAndValidate em roleData.
	// `roleData.Name` já está em minúsculas.
	// O serviço também já validou se os `roleData.PermissionNames` existem no sistema.

	// Verificar se já existe um role com o mesmo nome (case-insensitive já tratado pela normalização).
	_, err := r.GetByName(roleData.Name) // `roleData.Name` já está normalizado
	if err == nil {                      // Encontrou um existente
		appLogger.Warnf("Tentativa de criar role com nome já existente: '%s'", roleData.Name)
		return nil, fmt.Errorf("%w: role com nome '%s' já existe", appErrors.ErrConflict, roleData.Name)
	}
	if !errors.Is(err, appErrors.ErrNotFound) && err != nil { // Erro diferente de não encontrado
		appLogger.Errorf("Erro ao verificar existência do role '%s' antes de criar: %v", roleData.Name, err)
		return nil, appErrors.WrapErrorf(err, "falha ao verificar existência do role antes de criar (GORM)")
	}
	// Se ErrNotFound, podemos prosseguir.

	dbRole := models.DBRole{
		Name:         roleData.Name,
		Description:  roleData.Description,
		IsSystemRole: isSystemRole,
		// CreatedAt e UpdatedAt são gerenciados por GORM autoCreateTime/autoUpdateTime
	}

	// Usar transação para garantir atomicidade na criação do role e suas permissões.
	txErr := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&dbRole).Error; err != nil {
			// A constraint UNIQUE no GORM/DB deve pegar nomes duplicados.
			if strings.Contains(strings.ToLower(err.Error()), "unique constraint") ||
				strings.Contains(strings.ToLower(err.Error()), "duplicate key value violates unique constraint") {
				return fmt.Errorf("%w: role com nome '%s' já existe (conflito no DB)", appErrors.ErrConflict, roleData.Name)
			}
			return appErrors.WrapErrorf(err, "falha ao criar role (GORM)")
		}

		// Associar permissões.
		if len(roleData.PermissionNames) > 0 {
			rolePerms := make([]models.DBRolePermission, len(roleData.PermissionNames))
			for i, permName := range roleData.PermissionNames {
				// O serviço já validou que `permName` existe no PermissionManager.
				rolePerms[i] = models.DBRolePermission{RoleID: dbRole.ID, PermissionName: permName}
			}
			if err := tx.Create(&rolePerms).Error; err != nil { // Cria as entradas na tabela de junção.
				return appErrors.WrapErrorf(err, "falha ao associar permissões ao novo role (GORM)")
			}
		}
		return nil // Commit da transação.
	})

	if txErr != nil {
		appLogger.Errorf("Erro na transação de criação do role '%s': %v", roleData.Name, txErr)
		return nil, txErr // Retorna o erro da transação.
	}

	// Preenche o campo DBRole.Permissions para o objeto retornado.
	// É importante fazer isso fora da transação, pois a transação já foi commitada ou rollbackada.
	if errLoad := r.loadRolePermissions(&dbRole); errLoad != nil {
		appLogger.Warnf("Role '%s' criado, mas houve erro ao recarregar suas permissões para o objeto retornado: %v", dbRole.Name, errLoad)
		// Não retorna erro fatal aqui, o role foi criado.
	}

	appLogger.Infof("Novo Role criado: '%s' (ID: %d) com %d permissões.", dbRole.Name, dbRole.ID, len(dbRole.Permissions))
	return &dbRole, nil
}

// GetByID busca um role pelo ID, incluindo suas permissões.
func (r *gormRoleRepository) GetByID(roleID uint64) (*models.DBRole, error) {
	var dbRole models.DBRole
	if err := r.db.First(&dbRole, roleID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: role com ID %d não encontrado", appErrors.ErrNotFound, roleID)
		}
		appLogger.Errorf("Erro ao buscar role por ID %d: %v", roleID, err)
		return nil, appErrors.WrapErrorf(err, "falha ao buscar role por ID (GORM)")
	}
	if err := r.loadRolePermissions(&dbRole); err != nil {
		// Se não conseguir carregar as permissões, o objeto DBRole está incompleto.
		return nil, err
	}
	return &dbRole, nil
}

// GetByName busca um role pelo nome (normalizado para minúsculas), incluindo suas permissões.
func (r *gormRoleRepository) GetByName(name string) (*models.DBRole, error) {
	// Assume-se que `name` já foi normalizado para minúsculas pelo serviço.
	if name == "" {
		return nil, fmt.Errorf("%w: nome do role não pode ser vazio para busca", appErrors.ErrInvalidInput)
	}
	var dbRole models.DBRole
	// `name` no DB já está em minúsculas.
	if err := r.db.Where("name = ?", name).First(&dbRole).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: role com nome '%s' não encontrado", appErrors.ErrNotFound, name)
		}
		appLogger.Errorf("Erro ao buscar role por nome '%s': %v", name, err)
		return nil, appErrors.WrapErrorf(err, "falha ao buscar role por nome (GORM)")
	}
	if err := r.loadRolePermissions(&dbRole); err != nil {
		return nil, err
	}
	return &dbRole, nil
}

// GetAll busca todos os roles, incluindo suas permissões. Ordena por nome.
func (r *gormRoleRepository) GetAll() ([]models.DBRole, error) {
	var dbRoles []models.DBRole
	if err := r.db.Order("name ASC").Find(&dbRoles).Error; err != nil {
		appLogger.Errorf("Erro ao buscar todos os roles: %v", err)
		return nil, appErrors.WrapErrorf(err, "falha ao buscar lista de roles (GORM)")
	}

	// Carregar permissões para cada role.
	// Otimização para evitar N+1 queries:
	if len(dbRoles) > 0 {
		roleIDs := make([]uint64, len(dbRoles))
		for i, role := range dbRoles {
			roleIDs[i] = role.ID
		}
		var allRolePerms []models.DBRolePermission
		// Busca todas as permissões para os roles listados de uma vez.
		if err := r.db.Where("role_id IN ?", roleIDs).Find(&allRolePerms).Error; err != nil {
			appLogger.Errorf("Erro ao buscar todas as permissões para os roles listados: %v", err)
			return nil, appErrors.WrapErrorf(err, "falha ao buscar permissões dos roles (GORM)")
		}

		// Mapeia as permissões para seus respectivos roles.
		permsMap := make(map[uint64][]string)
		for _, rp := range allRolePerms {
			permsMap[rp.RoleID] = append(permsMap[rp.RoleID], rp.PermissionName)
		}

		for i := range dbRoles { // Usa índice para modificar o slice original.
			if perms, ok := permsMap[dbRoles[i].ID]; ok {
				dbRoles[i].Permissions = perms
			} else {
				dbRoles[i].Permissions = []string{} // Garante slice vazio, não nil.
			}
		}
	}
	return dbRoles, nil
}

// Update atualiza um role existente e/ou suas permissões.
// Campos em `roleUpdateData` já devem estar normalizados pelo serviço.
func (r *gormRoleRepository) Update(roleID uint64, roleUpdateData models.RoleUpdate) (*models.DBRole, error) {
	// O serviço deve ter buscado o role e verificado se é system role antes de permitir alteração de nome.
	// O serviço também valida os nomes de permissão em `roleUpdateData.PermissionNames`.

	updatesMap := make(map[string]interface{})
	changedBasicFields := false

	if roleUpdateData.Name != nil {
		// O serviço deve ter verificado conflito de nome.
		updatesMap["name"] = *roleUpdateData.Name // Nome já normalizado
		changedBasicFields = true
	}
	if roleUpdateData.Description != nil { // Permite limpar descrição passando um ponteiro para string vazia (após trim)
		updatesMap["description"] = roleUpdateData.Description // Pode ser nil para limpar descrição
		changedBasicFields = true
	}
	// UpdatedAt será gerenciado pelo GORM autoUpdateTime.

	txErr := r.db.Transaction(func(tx *gorm.DB) error {
		if changedBasicFields {
			if err := tx.Model(&models.DBRole{}).Where("id = ?", roleID).Updates(updatesMap).Error; err != nil {
				if strings.Contains(strings.ToLower(err.Error()), "unique constraint") && roleUpdateData.Name != nil {
					return fmt.Errorf("%w: já existe outro role com o nome '%s' (conflito no DB)", appErrors.ErrConflict, *roleUpdateData.Name)
				}
				return appErrors.WrapErrorf(err, "falha ao atualizar campos do role (GORM)")
			}
		}

		// Se `PermissionNames` for fornecido (não nil), substitui todas as permissões.
		if roleUpdateData.PermissionNames != nil {
			// 1. Deletar permissões antigas do role.
			if err := tx.Where("role_id = ?", roleID).Delete(&models.DBRolePermission{}).Error; err != nil {
				return appErrors.WrapErrorf(err, "falha ao limpar permissões antigas do role (GORM)")
			}
			// 2. Adicionar novas permissões.
			if len(*roleUpdateData.PermissionNames) > 0 {
				newPerms := make([]models.DBRolePermission, len(*roleUpdateData.PermissionNames))
				for i, permName := range *roleUpdateData.PermissionNames {
					// O serviço já validou que `permName` existe.
					newPerms[i] = models.DBRolePermission{RoleID: roleID, PermissionName: permName}
				}
				if err := tx.Create(&newPerms).Error; err != nil {
					return appErrors.WrapErrorf(err, "falha ao associar novas permissões ao role (GORM)")
				}
			}
		}
		return nil // Commit.
	})

	if txErr != nil {
		appLogger.Errorf("Erro na transação de atualização do role ID %d: %v", roleID, txErr)
		return nil, txErr
	}

	// Recarregar o role atualizado com suas permissões.
	updatedRole, err := r.GetByID(roleID)
	if err != nil {
		appLogger.Errorf("Falha ao recarregar role ID %d após update: %v", roleID, err)
		return nil, fmt.Errorf("falha ao recarregar role após atualização: %w", err)
	}

	appLogger.Infof("Role ID %d ('%s') atualizado.", roleID, updatedRole.Name)
	return updatedRole, nil
}

// Delete remove um role e suas associações.
// O serviço deve ter verificado se o role é `IsSystemRole` antes de chamar este método.
func (r *gormRoleRepository) Delete(roleID uint64) error {
	// Opcional: buscar o role primeiro para logar o nome ou verificar IsSystemRole novamente.
	var roleToDelete models.DBRole
	if err := r.db.Select("name", "is_system_role").First(&roleToDelete, roleID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: role com ID %d não encontrado para exclusão", appErrors.ErrNotFound, roleID)
		}
		return appErrors.WrapErrorf(err, "falha ao verificar role antes da exclusão")
	}
	if roleToDelete.IsSystemRole { // Dupla checagem, embora o serviço deva ter feito isso.
		return fmt.Errorf("%w: role de sistema '%s' não pode ser excluído pelo repositório", appErrors.ErrPermissionDenied, roleToDelete.Name)
	}

	txErr := r.db.Transaction(func(tx *gorm.DB) error {
		// 1. Remover associações de permissões (tabela `role_permissions`).
		if err := tx.Where("role_id = ?", roleID).Delete(&models.DBRolePermission{}).Error; err != nil {
			return appErrors.WrapErrorf(err, "falha ao remover associações de permissões do role (GORM)")
		}

		// 2. Remover associações de usuários (tabela `user_roles`).
		//    O GORM com `many2many` e `constraint:OnDelete:CASCADE` no modelo `DBUser` para `Roles`
		//    poderia cuidar disso automaticamente na exclusão do role.
		//    Se não houver CASCADE, ou para ser explícito:
		if err := tx.Where("role_id = ?", roleID).Delete(&models.DBUserRole{}).Error; err != nil {
			// Este erro pode indicar que usuários ainda estão usando este role, e não há CASCADE.
			// Dependendo da política, pode-se retornar ErrConflict aqui.
			appLogger.Warnf("Erro (ou nenhuma linha afetada) ao remover associações de usuários para role ID %d. Pode indicar que não havia usuários ou um problema de constraint. Erro: %v", roleID, err)
			// Não retornar erro fatal aqui, a menos que seja uma violação de constraint que impeça a exclusão do role.
		}

		// 3. Deletar o role da tabela `roles`.
		result := tx.Delete(&models.DBRole{}, roleID)
		if result.Error != nil {
			// Verificar se o erro é de FK (se algum usuário ainda está referenciando este role
			// e não há ON DELETE SET NULL/CASCADE na tabela `user_roles`).
			if strings.Contains(strings.ToLower(result.Error.Error()), "foreign key constraint") {
				return fmt.Errorf("%w: não foi possível excluir o role ID %d pois ele pode estar em uso por usuários e as constraints impedem", appErrors.ErrConflict, roleID)
			}
			return appErrors.WrapErrorf(result.Error, "falha ao excluir o role (GORM)")
		}
		if result.RowsAffected == 0 {
			// Se o role foi deletado entre a verificação e este ponto.
			return fmt.Errorf("%w: role com ID %d não encontrado durante a operação de exclusão (ou já excluído)", appErrors.ErrNotFound, roleID)
		}
		return nil // Commit.
	})

	if txErr != nil {
		appLogger.Errorf("Erro na transação de exclusão do role ID %d ('%s'): %v", roleID, roleToDelete.Name, txErr)
		return txErr
	}

	appLogger.Infof("Role '%s' (ID: %d) e suas associações de permissão/usuário foram excluídos.", roleToDelete.Name, roleID)
	return nil
}

// GetPermissionsForRole busca os nomes das permissões associadas a um roleID.
func (r *gormRoleRepository) GetPermissionsForRole(roleID uint64) ([]string, error) {
	var rolePerms []models.DBRolePermission
	if err := r.db.Where("role_id = ?", roleID).Find(&rolePerms).Error; err != nil {
		appLogger.Errorf("Erro ao buscar permissões para role ID %d: %v", roleID, err)
		return nil, appErrors.WrapErrorf(err, "falha ao buscar permissões do role (GORM)")
	}
	permissionNames := make([]string, len(rolePerms))
	for i, rp := range rolePerms {
		permissionNames[i] = rp.PermissionName
	}
	return permissionNames, nil
}

// SetRolePermissions define/sobrescreve todas as permissões para um role.
// `permissionNames` deve conter nomes de permissões válidos e normalizados.
func (r *gormRoleRepository) SetRolePermissions(roleID uint64, permissionNames []string) error {
	// Opcional: Verificar se o role existe primeiro.
	// var count int64
	// if err := r.db.Model(&models.DBRole{}).Where("id = ?", roleID).Count(&count).Error; err != nil {
	// 	return appErrors.WrapErrorf(err, "falha ao verificar existência do role ID %d antes de setar permissões", roleID)
	// }
	// if count == 0 {
	// 	return fmt.Errorf("%w: role com ID %d não encontrado para definir permissões", appErrors.ErrNotFound, roleID)
	// }

	txErr := r.db.Transaction(func(tx *gorm.DB) error {
		// Deletar permissões antigas.
		if err := tx.Where("role_id = ?", roleID).Delete(&models.DBRolePermission{}).Error; err != nil {
			return appErrors.WrapErrorf(err, "falha ao limpar permissões antigas do role (GORM)")
		}
		// Adicionar novas permissões, se houver.
		if len(permissionNames) > 0 {
			newPerms := make([]models.DBRolePermission, len(permissionNames))
			for i, permName := range permissionNames {
				// O serviço deve ter validado se permName existe no PermissionManager.
				newPerms[i] = models.DBRolePermission{RoleID: roleID, PermissionName: permName}
			}
			if err := tx.Create(&newPerms).Error; err != nil {
				return appErrors.WrapErrorf(err, "falha ao associar novas permissões ao role (GORM)")
			}
		}
		return nil // Commit.
	})

	if txErr != nil {
		appLogger.Errorf("Erro na transação de definir permissões para role ID %d: %v", roleID, txErr)
		return txErr
	}

	appLogger.Infof("Permissões atualizadas para Role ID %d. Total de permissões definidas: %d", roleID, len(permissionNames))
	return nil
}
