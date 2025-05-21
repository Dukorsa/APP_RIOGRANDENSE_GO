package repositories

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
	// Para Upsert em tabelas de junção ou preloading
	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/errors"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/models"
)

// RoleRepository define a interface para operações no repositório de roles.
type RoleRepository interface {
	Create(roleData models.RoleCreate, isSystemRole bool) (*models.DBRole, error)
	GetByID(roleID uint64) (*models.DBRole, error)
	GetByName(name string) (*models.DBRole, error)
	GetAll() ([]models.DBRole, error)
	Update(roleID uint64, roleUpdateData models.RoleUpdate) (*models.DBRole, error)
	Delete(roleID uint64) error
	GetPermissionsForRole(roleID uint64) ([]string, error)
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

// loadRolePermissions busca e anexa as permissões a um DBRole.
// Esta é uma função helper interna.
func (r *gormRoleRepository) loadRolePermissions(dbRole *models.DBRole) error {
	if dbRole == nil || dbRole.ID == 0 {
		return nil // Nada a fazer se o role for nil ou não tiver ID
	}
	var rolePerms []models.DBRolePermission
	if err := r.db.Where("role_id = ?", dbRole.ID).Find(&rolePerms).Error; err != nil {
		appLogger.Errorf("Erro ao buscar permissões para role ID %d: %v", dbRole.ID, err)
		return appErrors.WrapErrorf(err, "falha ao buscar permissões do role (GORM)")
	}
	dbRole.Permissions = make([]string, len(rolePerms))
	for i, rp := range rolePerms {
		dbRole.Permissions[i] = rp.PermissionName
	}
	return nil
}

// Create cria um novo role e associa suas permissões.
// Espera que roleData já tenha sido limpa e validada pelo serviço (exceto existência de permissões).
func (r *gormRoleRepository) Create(roleData models.RoleCreate, isSystemRole bool) (*models.DBRole, error) {
	// O serviço já deve ter chamado CleanAndValidate em roleData.
	// roleData.Name já deve estar em minúsculas.

	// Verificar se já existe um role com o mesmo nome
	_, err := r.GetByName(roleData.Name)
	if err == nil { // Encontrou um existente
		appLogger.Warnf("Tentativa de criar role com nome já existente: '%s'", roleData.Name)
		return nil, fmt.Errorf("%w: role com nome '%s' já existe", appErrors.ErrConflict, roleData.Name)
	}
	if !errors.Is(err, appErrors.ErrNotFound) && err != nil { // Erro diferente de não encontrado
		appLogger.Errorf("Erro ao verificar existência do role '%s' antes de criar: %v", roleData.Name, err)
		return nil, appErrors.WrapErrorf(err, "falha ao verificar existência do role antes de criar (GORM)")
	}
	// Se chegou aqui, é appErrors.ErrNotFound, o que é bom.

	dbRole := models.DBRole{
		Name:         roleData.Name,
		Description:  roleData.Description,
		IsSystemRole: isSystemRole,
	}

	// Usar transação para garantir atomicidade na criação do role e suas permissões
	txErr := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&dbRole).Error; err != nil {
			// A constraint UNIQUE no GORM/DB deve pegar nomes duplicados
			if strings.Contains(strings.ToLower(err.Error()), "unique constraint") ||
				strings.Contains(strings.ToLower(err.Error()), "duplicate key value violates unique constraint") {
				return fmt.Errorf("%w: role com nome '%s' já existe (concorrência?)", appErrors.ErrConflict, roleData.Name)
			}
			return appErrors.WrapErrorf(err, "falha ao criar role (GORM)")
		}

		// Associar permissões
		if len(roleData.PermissionNames) > 0 {
			rolePerms := make([]models.DBRolePermission, len(roleData.PermissionNames))
			for i, permName := range roleData.PermissionNames {
				// TODO: O serviço deveria ter validado se permName existe no PermissionManager.
				// Aqui, apenas assumimos que são válidos e os inserimos.
				rolePerms[i] = models.DBRolePermission{RoleID: dbRole.ID, PermissionName: permName}
			}
			if err := tx.Create(&rolePerms).Error; err != nil {
				return appErrors.WrapErrorf(err, "falha ao associar permissões ao novo role (GORM)")
			}
		}
		return nil // Commit
	})

	if txErr != nil {
		appLogger.Errorf("Erro na transação de criação do role '%s': %v", roleData.Name, txErr)
		return nil, txErr // Retorna o erro da transação (que já pode ser um appError)
	}

	// Preenche o campo DBRole.Permissions para o objeto retornado
	// dbRole.Permissions já foi definido pela associação acima dentro da transação
	// ou, se SetRolePermissions fosse chamado aqui:
	if err := r.loadRolePermissions(&dbRole); err != nil {
		// Não fatal para a criação, mas loga o aviso
		appLogger.Warnf("Role '%s' criado, mas houve erro ao recarregar suas permissões para o objeto retornado: %v", dbRole.Name, err)
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
		return nil, err // Erro ao carregar permissões é considerado falha na busca completa
	}
	return &dbRole, nil
}

// GetByName busca um role pelo nome (case-insensitive), incluindo suas permissões.
func (r *gormRoleRepository) GetByName(name string) (*models.DBRole, error) {
	var dbRole models.DBRole
	if err := r.db.Where("LOWER(name) = LOWER(?)", name).First(&dbRole).Error; err != nil {
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

// GetAll busca todos os roles, incluindo suas permissões.
func (r *gormRoleRepository) GetAll() ([]models.DBRole, error) {
	var dbRoles []models.DBRole
	if err := r.db.Order("name ASC").Find(&dbRoles).Error; err != nil {
		appLogger.Errorf("Erro ao buscar todos os roles: %v", err)
		return nil, appErrors.WrapErrorf(err, "falha ao buscar lista de roles (GORM)")
	}

	// Carregar permissões para cada role (pode ser N+1, otimizar se necessário)
	// Otimização: Buscar todas as DBRolePermission de uma vez e mapear
	if len(dbRoles) > 0 {
		roleIDs := make([]uint64, len(dbRoles))
		for i, role := range dbRoles {
			roleIDs[i] = role.ID
		}
		var allRolePerms []models.DBRolePermission
		if err := r.db.Where("role_id IN ?", roleIDs).Find(&allRolePerms).Error; err != nil {
			appLogger.Errorf("Erro ao buscar todas as permissões para os roles listados: %v", err)
			return nil, appErrors.WrapErrorf(err, "falha ao buscar permissões dos roles (GORM)")
		}

		permsMap := make(map[uint64][]string)
		for _, rp := range allRolePerms {
			permsMap[rp.RoleID] = append(permsMap[rp.RoleID], rp.PermissionName)
		}

		for i := range dbRoles { // Usa índice para modificar o slice original
			dbRoles[i].Permissions = permsMap[dbRoles[i].ID]
			if dbRoles[i].Permissions == nil { // Garante que seja um slice vazio, não nil
				dbRoles[i].Permissions = []string{}
			}
		}
	}
	return dbRoles, nil
}

// Update atualiza um role existente e/ou suas permissões.
// Espera que roleUpdateData já tenha sido limpa e validada pelo serviço.
func (r *gormRoleRepository) Update(roleID uint64, roleUpdateData models.RoleUpdate) (*models.DBRole, error) {
	dbRole, err := r.GetByID(roleID) // GetByID já carrega permissões atuais
	if err != nil {
		return nil, err
	}

	// O serviço deve impedir a renomeação de roles do sistema.
	// Aqui, apenas aplicamos as mudanças.

	updates := make(map[string]interface{})
	changedBasicFields := false

	if roleUpdateData.Name != nil && strings.ToLower(*roleUpdateData.Name) != dbRole.Name {
		// Verificar se o novo nome já está em uso por OUTRO role
		cleanedNewName := strings.ToLower(*roleUpdateData.Name)
		existingByName, errGet := r.GetByName(cleanedNewName)
		if errGet == nil && existingByName.ID != roleID {
			return nil, fmt.Errorf("%w: já existe outro role com o nome '%s'", appErrors.ErrConflict, cleanedNewName)
		}
		if errGet != nil && !errors.Is(errGet, appErrors.ErrNotFound) {
			return nil, appErrors.WrapErrorf(errGet, "falha ao verificar novo nome para atualização de role (GORM)")
		}
		updates["name"] = cleanedNewName
		changedBasicFields = true
	}
	if roleUpdateData.Description != nil && (dbRole.Description == nil || *roleUpdateData.Description != *dbRole.Description) {
		updates["description"] = roleUpdateData.Description // Pode ser nil para limpar descrição
		changedBasicFields = true
	}

	// Usar transação para atualizar role e suas permissões atomicamente
	txErr := r.db.Transaction(func(tx *gorm.DB) error {
		if changedBasicFields {
			if err := tx.Model(&models.DBRole{}).Where("id = ?", roleID).Updates(updates).Error; err != nil {
				// Verificar erro de nome duplicado na atualização (concorrência)
				if strings.Contains(strings.ToLower(err.Error()), "unique constraint") && roleUpdateData.Name != nil {
					return fmt.Errorf("%w: já existe outro role com o nome '%s' (concorrência?)", appErrors.ErrConflict, *roleUpdateData.Name)
				}
				return appErrors.WrapErrorf(err, "falha ao atualizar campos do role (GORM)")
			}
		}

		// Se PermissionNames for fornecido, substitui todas as permissões
		if roleUpdateData.PermissionNames != nil {
			// Deletar permissões antigas
			if err := tx.Where("role_id = ?", roleID).Delete(&models.DBRolePermission{}).Error; err != nil {
				return appErrors.WrapErrorf(err, "falha ao limpar permissões antigas do role (GORM)")
			}
			// Adicionar novas permissões
			if len(*roleUpdateData.PermissionNames) > 0 {
				newPerms := make([]models.DBRolePermission, len(*roleUpdateData.PermissionNames))
				for i, permName := range *roleUpdateData.PermissionNames {
					// TODO: O serviço deve ter validado se permName existe no PermissionManager
					newPerms[i] = models.DBRolePermission{RoleID: roleID, PermissionName: permName}
				}
				if err := tx.Create(&newPerms).Error; err != nil {
					return appErrors.WrapErrorf(err, "falha ao associar novas permissões ao role (GORM)")
				}
			}
		}
		return nil // Commit
	})

	if txErr != nil {
		appLogger.Errorf("Erro na transação de atualização do role ID %d: %v", roleID, txErr)
		return nil, txErr
	}

	// Recarregar o role atualizado com suas permissões
	updatedRole, err := r.GetByID(roleID)
	if err != nil {
		appLogger.Errorf("Falha ao recarregar role ID %d após update: %v", roleID, err)
		return nil, err // Retorna erro se não conseguir recarregar
	}

	appLogger.Infof("Role ID %d ('%s') atualizado.", roleID, updatedRole.Name)
	return updatedRole, nil
}

// Delete remove um role e suas associações.
// O serviço deve verificar se é um system role antes de chamar.
func (r *gormRoleRepository) Delete(roleID uint64) error {
	// Verificar se o role existe
	dbRole, err := r.GetByID(roleID) // GetByID já retorna ErrNotFound
	if err != nil {
		return err
	}

	// O serviço deve ter impedido a exclusão de system roles
	if dbRole.IsSystemRole { // Dupla checagem
		return fmt.Errorf("%w: role de sistema '%s' não pode ser excluído pelo repositório", appErrors.ErrPermissionDenied, dbRole.Name)
	}

	txErr := r.db.Transaction(func(tx *gorm.DB) error {
		// 1. Remover associações de permissões
		if err := tx.Where("role_id = ?", roleID).Delete(&models.DBRolePermission{}).Error; err != nil {
			return appErrors.WrapErrorf(err, "falha ao remover associações de permissões do role (GORM)")
		}

		// 2. Remover associações de usuários (tabela user_roles)
		//    Se estiver usando GORM many2many para User-Role, ele pode lidar com isso
		//    automaticamente com `Select("Users").Delete(&dbRole)` ou limpando a associação
		//    nos usuários. Ou deletar manualmente da tabela de junção:
		if err := tx.Where("role_id = ?", roleID).Delete(&models.DBUserRole{}).Error; err != nil {
			// Se houver constraint FK de users para user_roles, isso pode não ser necessário
			// ou pode falhar se não for feito na ordem correta.
			// GORM com `Association("Users").Clear()` no objeto DBRole antes de deletar o role é uma opção.
			// Este delete manual é mais explícito se não confiar na mágica do ORM.
			appLogger.Warnf("Pode haver usuários ainda associados (não tratados por este Delete) ao role ID %d se não usar cascade ou hooks.", roleID)
			// return appErrors.WrapErrorf(err, "falha ao remover associações de usuários do role (GORM)")
		}

		// 3. Deletar o role
		if err := tx.Delete(&models.DBRole{}, roleID).Error; err != nil {
			return appErrors.WrapErrorf(err, "falha ao excluir o role (GORM)")
		}
		return nil // Commit
	})

	if txErr != nil {
		appLogger.Errorf("Erro na transação de exclusão do role ID %d: %v", roleID, txErr)
		// Verificar se o erro é de FK (usuários ainda associados)
		if strings.Contains(strings.ToLower(txErr.Error()), "foreign key constraint") {
			return fmt.Errorf("%w: não foi possível excluir o role ID %d pois ele pode estar em uso por usuários", appErrors.ErrConflict, roleID)
		}
		return txErr
	}

	appLogger.Infof("Role '%s' (ID: %d) excluído.", dbRole.Name, roleID)
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
func (r *gormRoleRepository) SetRolePermissions(roleID uint64, permissionNames []string) error {
	// Verificar se o role existe
	if _, err := r.GetByID(roleID); err != nil {
		return err // Retorna ErrNotFound se não existir
	}

	txErr := r.db.Transaction(func(tx *gorm.DB) error {
		// Deletar permissões antigas
		if err := tx.Where("role_id = ?", roleID).Delete(&models.DBRolePermission{}).Error; err != nil {
			return appErrors.WrapErrorf(err, "falha ao limpar permissões antigas do role (GORM)")
		}
		// Adicionar novas permissões
		if len(permissionNames) > 0 {
			newPerms := make([]models.DBRolePermission, len(permissionNames))
			for i, permName := range permissionNames {
				// TODO: Serviço deve validar se permName existe no PermissionManager
				newPerms[i] = models.DBRolePermission{RoleID: roleID, PermissionName: permName}
			}
			if err := tx.Create(&newPerms).Error; err != nil {
				return appErrors.WrapErrorf(err, "falha ao associar novas permissões ao role (GORM)")
			}
		}
		return nil // Commit
	})

	if txErr != nil {
		appLogger.Errorf("Erro na transação de definir permissões para role ID %d: %v", roleID, txErr)
		return txErr
	}

	appLogger.Infof("Permissões atualizadas para Role ID %d. Total: %d", roleID, len(permissionNames))
	return nil
}
