package repositories

import (
	"errors"
	"fmt"
	"maps" // Requer Go 1.21+
	"strings"

	"gorm.io/gorm"
	// "gorm.io/gorm/clause" // Para OnConflict, se fosse usar upsert

	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/models"
)

// NetworkRepository define a interface para operações no repositório de redes.
type NetworkRepository interface {
	GetAll(includeInactive bool) ([]models.DBNetwork, error)
	Search(term string, buyer *string, includeInactive bool) ([]models.DBNetwork, error)
	GetByID(networkID uint64) (*models.DBNetwork, error)
	GetByName(name string) (*models.DBNetwork, error) // name deve ser em minúsculas para busca
	Create(networkData models.NetworkCreate, createdByUsername string) (*models.DBNetwork, error)
	Update(networkID uint64, networkUpdateData models.NetworkUpdate, updatedByUsername string) (*models.DBNetwork, error)
	ToggleStatus(networkID uint64, updatedByUsername string) (*models.DBNetwork, error)
	// BulkDelete realiza exclusão física. Retorna o número de redes efetivamente excluídas.
	BulkDelete(ids []uint64) (deletedCount int64, err error)
}

// gormNetworkRepository é a implementação GORM de NetworkRepository.
type gormNetworkRepository struct {
	db *gorm.DB
}

// NewGormNetworkRepository cria uma nova instância de gormNetworkRepository.
func NewGormNetworkRepository(db *gorm.DB) NetworkRepository {
	if db == nil {
		appLogger.Fatalf("gorm.DB não pode ser nil para NewGormNetworkRepository")
	}
	return &gormNetworkRepository{db: db}
}

// GetAll busca todas as redes, opcionalmente incluindo inativas.
// Ordena por nome (ASC).
func (r *gormNetworkRepository) GetAll(includeInactive bool) ([]models.DBNetwork, error) {
	var networks []models.DBNetwork
	query := r.db.Order("name ASC") // Nome já é minúsculo no DB, então a ordem é case-insensitive.

	if !includeInactive {
		query = query.Where("status = ?", true)
	}

	if err := query.Find(&networks).Error; err != nil {
		appLogger.Errorf("Erro ao buscar todas as redes (includeInactive: %t): %v", includeInactive, err)
		return nil, appErrors.WrapErrorf(err, "falha na recuperação da lista de redes (GORM)")
	}
	return networks, nil
}

// Search busca redes por termo (nome ou comprador) e opcionalmente por comprador.
// A busca é case-insensitive. Ordena por nome (ASC).
func (r *gormNetworkRepository) Search(term string, buyer *string, includeInactive bool) ([]models.DBNetwork, error) {
	var networks []models.DBNetwork
	query := r.db.Order("name ASC")

	// Termo de busca é convertido para minúsculas para LIKE case-insensitive.
	// O campo `name` no DB já está em minúsculas.
	// Para `buyer`, que está em Title Case, usamos LOWER() do DB.
	searchTerm := "%" + strings.ToLower(strings.TrimSpace(term)) + "%"

	if term != "" { // Só aplica o filtro de termo se não for vazio
		query = query.Where("name LIKE ? OR LOWER(buyer) LIKE ?", searchTerm, searchTerm)
	}

	if buyer != nil && *buyer != "" {
		searchBuyer := "%" + strings.ToLower(strings.TrimSpace(*buyer)) + "%"
		query = query.Where("LOWER(buyer) LIKE ?", searchBuyer)
	}

	if !includeInactive {
		query = query.Where("status = ?", true)
	}

	if err := query.Find(&networks).Error; err != nil {
		appLogger.Errorf("Erro na pesquisa de redes (termo='%s', comprador='%v', includeInactive: %t): %v", term, buyer, includeInactive, err)
		return nil, appErrors.WrapErrorf(err, "falha na operação de pesquisa de redes (GORM)")
	}
	return networks, nil
}

// GetByID busca uma rede específica pelo ID.
func (r *gormNetworkRepository) GetByID(networkID uint64) (*models.DBNetwork, error) {
	var network models.DBNetwork
	result := r.db.First(&network, networkID)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: rede com ID %d não encontrada", appErrors.ErrNotFound, networkID)
		}
		appLogger.Errorf("Erro ao buscar rede por ID %d: %v", networkID, result.Error)
		return nil, appErrors.WrapErrorf(result.Error, "falha ao buscar rede por ID (GORM)")
	}
	return &network, nil
}

// GetByName busca uma rede específica pelo nome (minúsculo).
// O nome da rede é armazenado em minúsculas no banco, então a busca é direta.
func (r *gormNetworkRepository) GetByName(name string) (*models.DBNetwork, error) {
	// Assume-se que `name` já foi normalizado para minúsculas pelo serviço antes de chamar este método.
	if name == "" {
		return nil, fmt.Errorf("%w: nome da rede não pode ser vazio para busca", appErrors.ErrInvalidInput)
	}
	var network models.DBNetwork
	result := r.db.Where("name = ?", name).First(&network) // Busca exata, case-sensitive no nome já em minúsculas
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: rede com nome '%s' não encontrada", appErrors.ErrNotFound, name)
		}
		appLogger.Errorf("Erro ao buscar rede por nome '%s': %v", name, result.Error)
		return nil, appErrors.WrapErrorf(result.Error, "falha ao buscar rede por nome (GORM)")
	}
	return &network, nil
}

// Create cria uma nova rede no banco de dados.
// `networkData.Name` já deve estar em minúsculas e `networkData.Buyer` em Title Case, conforme `CleanAndValidate`.
func (r *gormNetworkRepository) Create(networkData models.NetworkCreate, createdByUsername string) (*models.DBNetwork, error) {
	// Validação de formato e limpeza já devem ter sido feitas pelo serviço.
	// A unicidade do nome (case-insensitive) também deve ser verificada pelo serviço antes de chamar Create.
	// No entanto, a constraint unique no DB é a garantia final.

	// Verificar novamente a existência do nome (que já está em minúsculas em networkData.Name)
	// para tratar condições de corrida, embora a constraint do DB seja a principal.
	_, err := r.GetByName(networkData.Name) // networkData.Name já está normalizado
	if err == nil {
		appLogger.Warnf("Tentativa de criar rede com nome já existente (detectado no repo): '%s'", networkData.Name)
		return nil, fmt.Errorf("%w: já existe uma rede cadastrada com o nome '%s'", appErrors.ErrConflict, networkData.Name)
	}
	if !errors.Is(err, appErrors.ErrNotFound) && err != nil {
		appLogger.Errorf("Erro ao verificar existência da rede '%s' antes de criar: %v", networkData.Name, err)
		return nil, appErrors.WrapErrorf(err, "falha ao verificar existência da rede antes de criar (GORM)")
	}
	// Se ErrNotFound, podemos prosseguir.

	dbNetwork := models.DBNetwork{
		Name:      networkData.Name,  // Já em minúsculas
		Buyer:     networkData.Buyer, // Já em Title Case
		Status:    true,              // Novas redes são ativas por padrão
		CreatedBy: &createdByUsername,
		UpdatedBy: &createdByUsername, // No momento da criação, UpdatedBy é o mesmo que CreatedBy
		// CreatedAt e UpdatedAt são gerenciados por `autoCreateTime` e `autoUpdateTime` do GORM.
	}

	result := r.db.Create(&dbNetwork)
	if result.Error != nil {
		appLogger.Errorf("Erro ao criar rede '%s': %v", networkData.Name, result.Error)
		// Verificar erro de constraint UNIQUE do DB.
		if strings.Contains(strings.ToLower(result.Error.Error()), "unique constraint") ||
			strings.Contains(strings.ToLower(result.Error.Error()), "duplicate key value violates unique constraint") {
			return nil, fmt.Errorf("%w: já existe uma rede cadastrada com o nome '%s' (conflito no DB)", appErrors.ErrConflict, networkData.Name)
		}
		return nil, appErrors.WrapErrorf(result.Error, "falha ao criar registro de rede (GORM)")
	}

	appLogger.Infof("Nova rede criada: '%s' (Comprador: %s) por %s (ID: %d)", dbNetwork.Name, dbNetwork.Buyer, createdByUsername, dbNetwork.ID)
	return &dbNetwork, nil
}

// Update atualiza uma rede existente.
// `networkUpdateData` campos já devem estar limpos e normalizados pelo serviço.
func (r *gormNetworkRepository) Update(networkID uint64, networkUpdateData models.NetworkUpdate, updatedByUsername string) (*models.DBNetwork, error) {
	dbNetwork, err := r.GetByID(networkID)
	if err != nil {
		return nil, err // GetByID já formata ErrNotFound ou DB error
	}

	updates := make(map[string]interface{})
	changed := false

	if networkUpdateData.Name != nil {
		// `networkUpdateData.Name` já está em minúsculas.
		if dbNetwork.Name != *networkUpdateData.Name {
			// O serviço já deve ter verificado se o novo nome conflita com outra rede.
			// Aqui, apenas aplicamos. A constraint do DB pegaria conflitos de corrida.
			updates["name"] = *networkUpdateData.Name
			changed = true
		}
	}
	if networkUpdateData.Buyer != nil {
		// `networkUpdateData.Buyer` já está em Title Case.
		if dbNetwork.Buyer != *networkUpdateData.Buyer {
			updates["buyer"] = *networkUpdateData.Buyer
			changed = true
		}
	}
	if networkUpdateData.Status != nil {
		if dbNetwork.Status != *networkUpdateData.Status {
			updates["status"] = *networkUpdateData.Status
			changed = true
		}
	}

	if !changed {
		appLogger.Debugf("Nenhuma alteração detectada para rede ID %d.", networkID)
		return dbNetwork, nil
	}

	updates["updated_by"] = &updatedByUsername
	// `updated_at` é gerenciado por `autoUpdateTime` do GORM.

	result := r.db.Model(&dbNetwork).Updates(updates)
	if result.Error != nil {
		appLogger.Errorf("Erro ao atualizar rede ID %d: %v", networkID, result.Error)
		if strings.Contains(strings.ToLower(result.Error.Error()), "unique constraint") && networkUpdateData.Name != nil {
			return nil, fmt.Errorf("%w: já existe outra rede com o nome '%s' (conflito no DB)", appErrors.ErrConflict, *networkUpdateData.Name)
		}
		return nil, appErrors.WrapErrorf(result.Error, "falha na atualização do registro da rede (GORM)")
	}

	appLogger.Infof("Rede ID %d ('%s') atualizada por %s. Campos: %v", networkID, dbNetwork.Name, updatedByUsername, maps.Keys(updates))
	return dbNetwork, nil // dbNetwork foi atualizado in-place.
}

// ToggleStatus alterna o status (ativo/inativo) de uma rede.
func (r *gormNetworkRepository) ToggleStatus(networkID uint64, updatedByUsername string) (*models.DBNetwork, error) {
	dbNetwork, err := r.GetByID(networkID)
	if err != nil {
		return nil, err
	}

	newStatus := !dbNetwork.Status
	updates := map[string]interface{}{
		"status":     newStatus,
		"updated_by": &updatedByUsername,
		// `updated_at` é gerenciado por `autoUpdateTime` do GORM.
	}

	result := r.db.Model(&dbNetwork).Updates(updates)
	if result.Error != nil {
		appLogger.Errorf("Erro ao alterar status da rede ID %d: %v", networkID, result.Error)
		return nil, appErrors.WrapErrorf(result.Error, "falha na alteração de status da rede (GORM)")
	}

	statusStr := "Ativa"
	if !newStatus {
		statusStr = "Inativa"
	}
	appLogger.Infof("Status da rede ID %d ('%s') alterado para %s por %s.", networkID, dbNetwork.Name, statusStr, updatedByUsername)
	return dbNetwork, nil // dbNetwork foi atualizado in-place.
}

// BulkDelete exclui redes em massa pelo ID (Exclusão FÍSICA).
// Retorna o número de redes efetivamente excluídas.
func (r *gormNetworkRepository) BulkDelete(ids []uint64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil // Nenhuma ação se a lista de IDs estiver vazia.
	}

	// GORM `Delete` com uma slice de chaves primárias realiza exclusão em massa.
	// Ou `db.Where("id IN ?", ids).Delete(&models.DBNetwork{})`
	result := r.db.Delete(&models.DBNetwork{}, ids)

	if result.Error != nil {
		appLogger.Errorf("Erro na exclusão em massa de redes (IDs: %v): %v", ids, result.Error)
		// Verificar erro de FK (ex: redes com CNPJs associados que restringem a deleção).
		if strings.Contains(strings.ToLower(result.Error.Error()), "foreign key constraint") ||
			strings.Contains(strings.ToLower(result.Error.Error()), "violates foreign key constraint") {
			// Tentar identificar quais redes causaram o conflito pode ser complexo aqui.
			// O serviço pode precisar iterar e deletar individualmente para melhor feedback.
			return 0, fmt.Errorf("%w: não é possível excluir uma ou mais redes pois possuem dados relacionados (ex: CNPJs vinculados). Verifique as dependências.", appErrors.ErrConflict)
		}
		return 0, appErrors.WrapErrorf(result.Error, "falha na exclusão em massa de redes (GORM)")
	}

	deletedCount := result.RowsAffected
	if deletedCount > 0 {
		appLogger.Infof("%d redes excluídas fisicamente (IDs tentados: %v).", deletedCount, ids)
	} else {
		appLogger.Warnf("Nenhuma rede encontrada para exclusão em massa com os IDs fornecidos: %v (podem já ter sido excluídas).", ids)
	}
	return deletedCount, nil
}
