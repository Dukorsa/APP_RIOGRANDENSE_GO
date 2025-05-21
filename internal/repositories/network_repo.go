package repositories

import (
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	"gorm.io/gorm"
	// Para preloading seletivo ou outras cláusulas
	appErrors "github.com/seu_usuario/riograndense_gio/internal/core/errors"
	appLogger "github.com/seu_usuario/riograndense_gio/internal/core/logger"
	"github.com/seu_usuario/riograndense_gio/internal/data/models"
)

// NetworkRepository define a interface para operações no repositório de redes.
type NetworkRepository interface {
	GetAll(includeInactive bool) ([]models.DBNetwork, error)
	Search(term string, buyer *string, includeInactive bool) ([]models.DBNetwork, error)
	GetByID(networkID uint64) (*models.DBNetwork, error)
	GetByName(name string) (*models.DBNetwork, error)
	Create(networkData models.NetworkCreate, createdByUsername string) (*models.DBNetwork, error)
	Update(networkID uint64, networkUpdateData models.NetworkUpdate, updatedByUsername string) (*models.DBNetwork, error)
	ToggleStatus(networkID uint64, updatedByUsername string) (*models.DBNetwork, error)
	BulkDelete(ids []uint64) (int64, error) // Retorna o número de redes excluídas
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
func (r *gormNetworkRepository) GetAll(includeInactive bool) ([]models.DBNetwork, error) {
	var networks []models.DBNetwork
	query := r.db.Order("name ASC")

	if !includeInactive {
		query = query.Where("status = ?", true)
	}

	if err := query.Find(&networks).Error; err != nil {
		appLogger.Errorf("Erro ao buscar todas as redes: %v", err)
		return nil, appErrors.WrapErrorf(err, "falha na recuperação da lista de redes (GORM)")
	}
	return networks, nil
}

// Search busca redes por termo (nome ou comprador) e opcionalmente por comprador.
func (r *gormNetworkRepository) Search(term string, buyer *string, includeInactive bool) ([]models.DBNetwork, error) {
	var networks []models.DBNetwork
	query := r.db.Order("name ASC")

	searchTerm := "%" + strings.ToLower(term) + "%" // Para busca case-insensitive com LIKE

	// Usar funções LOWER do DB para busca case-insensitive
	query = query.Where("LOWER(name) LIKE ? OR LOWER(buyer) LIKE ?", searchTerm, searchTerm)

	if buyer != nil && *buyer != "" {
		searchBuyer := "%" + strings.ToLower(*buyer) + "%"
		query = query.Where("LOWER(buyer) LIKE ?", searchBuyer)
	}

	if !includeInactive {
		query = query.Where("status = ?", true)
	}

	if err := query.Find(&networks).Error; err != nil {
		appLogger.Errorf("Erro na pesquisa de redes (termo='%s', comprador='%v'): %v", term, buyer, err)
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

// GetByName busca uma rede específica pelo nome (case-insensitive).
func (r *gormNetworkRepository) GetByName(name string) (*models.DBNetwork, error) {
	var network models.DBNetwork
	// GORM Where com string geralmente é case-sensitive por padrão em alguns DBs (como PostgreSQL).
	// Para case-insensitive, usar funções do DB.
	result := r.db.Where("LOWER(name) = LOWER(?)", name).First(&network)
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
// Espera que networkData já tenha sido limpa e validada pelo serviço.
func (r *gormNetworkRepository) Create(networkData models.NetworkCreate, createdByUsername string) (*models.DBNetwork, error) {
	// O serviço já deve ter chamado CleanAndValidate em networkData.
	// O nome da rede em networkData.Name já deve estar em minúsculas, e Buyer em Title Case.

	// Verificar se já existe uma rede com o mesmo nome (case-insensitive)
	// GetByName já faz a busca case-insensitive.
	_, err := r.GetByName(networkData.Name)
	if err == nil { // Encontrou uma existente
		appLogger.Warnf("Tentativa de criar rede com nome já existente: '%s'", networkData.Name)
		return nil, fmt.Errorf("%w: já existe uma rede cadastrada com o nome '%s'", appErrors.ErrConflict, networkData.Name)
	}
	if !errors.Is(err, appErrors.ErrNotFound) && err != nil { // Erro diferente de não encontrado
		appLogger.Errorf("Erro ao verificar existência da rede '%s' antes de criar: %v", networkData.Name, err)
		return nil, appErrors.WrapErrorf(err, "falha ao verificar existência da rede antes de criar (GORM)")
	}
	// Se chegou aqui, é appErrors.ErrNotFound, o que é bom.

	now := time.Now().UTC()
	dbNetwork := models.DBNetwork{
		Name:      networkData.Name,  // Já em minúsculas
		Buyer:     networkData.Buyer, // Já em Title Case
		Status:    true,              // Novas redes são ativas por padrão
		CreatedBy: &createdByUsername,
		UpdatedBy: &createdByUsername,
		CreatedAt: now, // GORM também pode setar com default:now()
		UpdatedAt: now, // GORM também pode setar com autoUpdateTime
	}

	result := r.db.Create(&dbNetwork)
	if result.Error != nil {
		appLogger.Errorf("Erro ao criar rede '%s': %v", networkData.Name, result.Error)
		// A constraint UNIQUE no GORM/DB deve pegar nomes duplicados se a verificação acima falhar por race condition.
		if strings.Contains(strings.ToLower(result.Error.Error()), "unique constraint") ||
			strings.Contains(strings.ToLower(result.Error.Error()), "duplicate key value violates unique constraint") {
			return nil, fmt.Errorf("%w: já existe uma rede cadastrada com o nome '%s'", appErrors.ErrConflict, networkData.Name)
		}
		return nil, appErrors.WrapErrorf(result.Error, "falha ao criar registro de rede (GORM)")
	}

	appLogger.Infof("Nova rede criada: '%s' por %s (ID: %d)", dbNetwork.Name, createdByUsername, dbNetwork.ID)
	return &dbNetwork, nil
}

// Update atualiza uma rede existente.
// Espera que networkUpdateData já tenha sido limpa e validada pelo serviço.
func (r *gormNetworkRepository) Update(networkID uint64, networkUpdateData models.NetworkUpdate, updatedByUsername string) (*models.DBNetwork, error) {
	dbNetwork, err := r.GetByID(networkID)
	if err != nil {
		return nil, err // GetByID já formata ErrNotFound ou DB error
	}

	updates := make(map[string]interface{})
	changed := false

	if networkUpdateData.Name != nil {
		// O serviço já deve ter chamado CleanAndValidate em networkUpdateData.
		// O nome em *networkUpdateData.Name já deve estar em minúsculas.
		if dbNetwork.Name != *networkUpdateData.Name {
			// Verificar se o novo nome já está em uso por OUTRA rede
			existingByName, errGet := r.GetByName(*networkUpdateData.Name)
			if errGet == nil && existingByName.ID != networkID { // Encontrou e é de outra rede
				appLogger.Warnf("Tentativa de atualizar rede ID %d para nome '%s' que já pertence à rede ID %d.", networkID, *networkUpdateData.Name, existingByName.ID)
				return nil, fmt.Errorf("%w: já existe outra rede com o nome '%s'", appErrors.ErrConflict, *networkUpdateData.Name)
			}
			if errGet != nil && !errors.Is(errGet, appErrors.ErrNotFound) { // Erro ao verificar
				appLogger.Errorf("Erro ao verificar nome '%s' para atualização da rede ID %d: %v", *networkUpdateData.Name, networkID, errGet)
				return nil, appErrors.WrapErrorf(errGet, "falha ao verificar nome para atualização de rede (GORM)")
			}
			updates["name"] = *networkUpdateData.Name
			changed = true
		}
	}
	if networkUpdateData.Buyer != nil {
		// O serviço já deve ter chamado CleanAndValidate em networkUpdateData.
		// O comprador em *networkUpdateData.Buyer já deve estar em Title Case.
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
	// updates["updated_at"] = time.Now().UTC() // GORM autoUpdateTime deve cuidar disso

	result := r.db.Model(&dbNetwork).Updates(updates)
	if result.Error != nil {
		appLogger.Errorf("Erro ao atualizar rede ID %d: %v", networkID, result.Error)
		if strings.Contains(strings.ToLower(result.Error.Error()), "unique constraint") && networkUpdateData.Name != nil {
			return nil, fmt.Errorf("%w: já existe outra rede com o nome '%s'", appErrors.ErrConflict, *networkUpdateData.Name)
		}
		return nil, appErrors.WrapErrorf(result.Error, "falha na atualização do registro da rede (GORM)")
	}

	appLogger.Infof("Rede ID %d ('%s') atualizada por %s. Campos: %v", networkID, dbNetwork.Name, updatedByUsername, maps.Keys(updates)) // Go 1.21+ maps.Keys
	return dbNetwork, nil
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
		// "updated_at": time.Now().UTC(), // GORM autoUpdateTime
	}

	result := r.db.Model(&dbNetwork).Updates(updates)
	if result.Error != nil {
		appLogger.Errorf("Erro ao alterar status da rede ID %d: %v", networkID, result.Error)
		return nil, appErrors.WrapErrorf(result.Error, "falha na alteração de status da rede (GORM)")
	}

	statusStr := "Ativo"
	if !newStatus {
		statusStr = "Inativo"
	}
	appLogger.Infof("Status da rede ID %d ('%s') alterado para %s por %s.", networkID, dbNetwork.Name, statusStr, updatedByUsername)
	return dbNetwork, nil
}

// BulkDelete exclui redes em massa pelo ID (Exclusão FÍSICA).
// Retorna o número de redes efetivamente excluídas.
func (r *gormNetworkRepository) BulkDelete(ids []uint64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}

	// GORM usa Delete com uma slice de chaves primárias para exclusão em massa.
	// Ou db.Where("id IN ?", ids).Delete(&models.DBNetwork{})
	result := r.db.Delete(&models.DBNetwork{}, ids)

	if result.Error != nil {
		appLogger.Errorf("Erro na exclusão em massa de redes (IDs: %v): %v", ids, result.Error)
		// Verificar erro de FK (ex: redes com CNPJs associados)
		if strings.Contains(strings.ToLower(result.Error.Error()), "foreign key constraint") ||
			strings.Contains(strings.ToLower(result.Error.Error()), "violates foreign key constraint") {
			return 0, fmt.Errorf("%w: não é possível excluir redes que possuem dados relacionados (ex: CNPJs)", appErrors.ErrConflict)
		}
		return 0, appErrors.WrapErrorf(result.Error, "falha na exclusão em massa de redes (GORM)")
	}

	deletedCount := result.RowsAffected
	if deletedCount > 0 {
		appLogger.Infof("%d redes excluídas fisicamente (IDs: %v).", deletedCount, ids)
	} else {
		appLogger.Warnf("Nenhuma rede encontrada para exclusão em massa com IDs: %v.", ids)
	}
	return deletedCount, nil
}
