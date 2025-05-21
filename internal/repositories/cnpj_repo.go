package repositories

import (
	"errors"
	"fmt"
	"maps"
	"strings"

	"gorm.io/gorm"

	appErrors "github.com/seu_usuario/riograndense_gio/internal/core/errors"
	appLogger "github.com/seu_usuario/riograndense_gio/internal/core/logger"
	"github.com/seu_usuario/riograndense_gio/internal/data/models"
	// Para IsValidCNPJ (a ser criado)
)

// CNPJRepository define a interface para operações no repositório de CNPJs.
type CNPJRepository interface {
	Add(cnpjData models.CNPJCreate) (*models.DBCNPJ, error)
	GetByID(cnpjID uint64) (*models.DBCNPJ, error)
	GetByCNPJ(cnpjNumber string) (*models.DBCNPJ, error)
	Update(cnpjID uint64, cnpjUpdateData models.CNPJUpdate) (*models.DBCNPJ, error)
	Delete(cnpjID uint64) error
	GetAll(includeInactive bool) ([]models.DBCNPJ, error)
	GetByNetworkID(networkID uint64, includeInactive bool) ([]models.DBCNPJ, error)
}

// gormCNPJRepository é a implementação GORM de CNPJRepository.
type gormCNPJRepository struct {
	db *gorm.DB
}

// NewGormCNPJRepository cria uma nova instância de gormCNPJRepository.
func NewGormCNPJRepository(db *gorm.DB) CNPJRepository {
	if db == nil {
		appLogger.Fatalf("gorm.DB não pode ser nil para NewGormCNPJRepository")
	}
	return &gormCNPJRepository{db: db}
}

// Add insere um novo CNPJ no banco de dados.
// Espera que cnpjData.CNPJ já tenha sido validado e limpo pelo serviço.
func (r *gormCNPJRepository) Add(cnpjData models.CNPJCreate) (*models.DBCNPJ, error) {
	// O serviço deve chamar CleanAndValidateCNPJ em cnpjData antes daqui
	cleanedCNPJ, err := cnpjData.CleanAndValidateCNPJ() // Assume que este método limpa e valida
	if err != nil {
		// O CleanAndValidateCNPJ deve retornar um appErrors.ValidationError
		appLogger.Warnf("Dados de criação de CNPJ inválidos: %v", err)
		return nil, err
	}

	// Verificar se o CNPJ já existe
	var existing models.DBCNPJ
	// Usar FirstOrInit ou FirstOrCreate pode ser uma opção, mas verificar explicitamente dá mais controle sobre o erro.
	err = r.db.Where("cnpj = ?", cleanedCNPJ).First(&existing).Error
	if err == nil { // Encontrou um existente
		appLogger.Warnf("Tentativa de adicionar CNPJ já existente: %s", cleanedCNPJ)
		return nil, fmt.Errorf("%w: CNPJ %s já cadastrado", appErrors.ErrConflict, cleanedCNPJ)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) { // Erro diferente de "não encontrado"
		appLogger.Errorf("Erro ao verificar existência do CNPJ %s: %v", cleanedCNPJ, err)
		return nil, appErrors.WrapErrorf(err, "falha ao verificar existência do CNPJ (GORM)")
	}
	// Se chegou aqui, é gorm.ErrRecordNotFound, o que é bom.

	dbCNPJ := models.DBCNPJ{
		CNPJ:      cleanedCNPJ,
		NetworkID: cnpjData.NetworkID,
		Active:    true, // Novo CNPJ é ativo por padrão
		// RegistrationDate é default:now()
	}

	result := r.db.Create(&dbCNPJ)
	if result.Error != nil {
		appLogger.Errorf("Erro ao adicionar CNPJ %s para network ID %d: %v", cleanedCNPJ, cnpjData.NetworkID, result.Error)
		// Verificar erro de FK para NetworkID
		if strings.Contains(strings.ToLower(result.Error.Error()), "foreign key constraint") {
			return nil, fmt.Errorf("%w: rede com ID %d não encontrada", appErrors.ErrNotFound, cnpjData.NetworkID)
		}
		return nil, appErrors.WrapErrorf(result.Error, "falha ao cadastrar CNPJ (GORM)")
	}

	appLogger.Infof("Novo CNPJ cadastrado: %s para rede ID %d (ID no DB: %d)", dbCNPJ.CNPJ, dbCNPJ.NetworkID, dbCNPJ.ID)
	return &dbCNPJ, nil
}

// GetByID busca um CNPJ pelo seu ID no banco de dados.
func (r *gormCNPJRepository) GetByID(cnpjID uint64) (*models.DBCNPJ, error) {
	var dbCNPJ models.DBCNPJ
	result := r.db.First(&dbCNPJ, cnpjID)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: CNPJ com ID %d não encontrado", appErrors.ErrNotFound, cnpjID)
		}
		appLogger.Errorf("Erro ao buscar CNPJ por ID %d: %v", cnpjID, result.Error)
		return nil, appErrors.WrapErrorf(result.Error, "falha ao buscar CNPJ por ID (GORM)")
	}
	return &dbCNPJ, nil
}

// GetByCNPJ busca um CNPJ pelo seu número (limpo, apenas dígitos).
func (r *gormCNPJRepository) GetByCNPJ(cnpjNumber string) (*models.DBCNPJ, error) {
	cleanedCNPJ := models.CleanCNPJ(cnpjNumber) // Usa helper do models
	if len(cleanedCNPJ) != 14 {
		// appLogger.Warnf("Tentativa de buscar CNPJ com formato inválido (após limpeza): '%s'", cleanedCNPJ)
		// Poderia retornar um erro de validação aqui, ou deixar o DB não encontrar.
		// Por consistência, é melhor retornar ErrNotFound se o formato já é inválido para busca.
		return nil, fmt.Errorf("%w: formato de CNPJ inválido para busca '%s'", appErrors.ErrInvalidInput, cnpjNumber)
	}

	var dbCNPJ models.DBCNPJ
	result := r.db.Where("cnpj = ?", cleanedCNPJ).First(&dbCNPJ)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: CNPJ '%s' não encontrado", appErrors.ErrNotFound, cleanedCNPJ)
		}
		appLogger.Errorf("Erro ao buscar CNPJ pelo número '%s': %v", cleanedCNPJ, result.Error)
		return nil, appErrors.WrapErrorf(result.Error, "falha ao buscar CNPJ pelo número (GORM)")
	}
	return &dbCNPJ, nil
}

// Update atualiza os dados de um CNPJ existente.
func (r *gormCNPJRepository) Update(cnpjID uint64, cnpjUpdateData models.CNPJUpdate) (*models.DBCNPJ, error) {
	dbCNPJ, err := r.GetByID(cnpjID) // Reutiliza GetByID para verificar se existe
	if err != nil {
		return nil, err // GetByID já formata o erro (ErrNotFound ou DB error)
	}

	// Monta um mapa dos campos a serem atualizados
	// GORM lida bem com atualização de campos `nil` em structs se usar `Updates` com struct
	// ou `Update` com map[string]interface{}.
	// Para usar ponteiros de CNPJUpdate, é mais seguro construir um mapa.
	updates := make(map[string]interface{})
	changed := false

	if cnpjUpdateData.NetworkID != nil {
		if dbCNPJ.NetworkID != *cnpjUpdateData.NetworkID {
			updates["network_id"] = *cnpjUpdateData.NetworkID
			changed = true
		}
	}
	if cnpjUpdateData.Active != nil {
		if dbCNPJ.Active != *cnpjUpdateData.Active {
			updates["active"] = *cnpjUpdateData.Active
			changed = true
		}
	}

	if !changed {
		appLogger.Debugf("Nenhuma alteração detectada para CNPJ ID %d.", cnpjID)
		return dbCNPJ, nil // Retorna o objeto existente se nada mudou
	}

	// Adiciona UpdatedAt manualmente se não usar autoUpdateTime no GORM
	// updates["updated_at"] = time.Now().UTC()

	result := r.db.Model(&dbCNPJ).Updates(updates)
	if result.Error != nil {
		appLogger.Errorf("Erro ao atualizar CNPJ ID %d: %v", cnpjID, result.Error)
		if strings.Contains(strings.ToLower(result.Error.Error()), "foreign key constraint") && cnpjUpdateData.NetworkID != nil {
			return nil, fmt.Errorf("%w: rede com ID %d não encontrada para atualização do CNPJ", appErrors.ErrNotFound, *cnpjUpdateData.NetworkID)
		}
		return nil, appErrors.WrapErrorf(result.Error, "falha ao atualizar CNPJ (GORM)")
	}

	if result.RowsAffected == 0 && changed { // Se algo deveria ter mudado mas não afetou linhas
		appLogger.Warnf("Atualização do CNPJ ID %d não afetou nenhuma linha, mas mudanças eram esperadas.", cnpjID)
		// Isso pode acontecer se o registro foi deletado entre o GetByID e o Update,
		// ou se os valores eram os mesmos (mas o 'changed' flag deveria pegar isso).
	}

	appLogger.Infof("CNPJ ID %d atualizado. Campos alterados: %v", cnpjID, maps.Keys(updates)) // Go 1.21+ maps.Keys

	// Retorna o objeto atualizado (dbCNPJ foi atualizado in-place pelo Updates)
	return dbCNPJ, nil
}

// Delete remove um CNPJ do banco de dados.
func (r *gormCNPJRepository) Delete(cnpjID uint64) error {
	// Primeiro, verifica se o CNPJ existe para fornecer uma mensagem de log melhor
	// e para potencialmente retornar ErrNotFound se não existir.
	var dbCNPJ models.DBCNPJ
	if err := r.db.Select("id", "cnpj").First(&dbCNPJ, cnpjID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			appLogger.Warnf("CNPJ com ID %d não encontrado para exclusão.", cnpjID)
			return fmt.Errorf("%w: CNPJ com ID %d não encontrado para exclusão", appErrors.ErrNotFound, cnpjID)
		}
		appLogger.Errorf("Erro ao verificar CNPJ ID %d antes da exclusão: %v", cnpjID, err)
		return appErrors.WrapErrorf(err, "falha ao verificar CNPJ antes da exclusão (GORM)")
	}

	result := r.db.Delete(&models.DBCNPJ{}, cnpjID)
	if result.Error != nil {
		appLogger.Errorf("Erro ao excluir CNPJ ID %d: %v", cnpjID, result.Error)
		// TODO: Verificar se o erro é de FK (ex: CNPJ usado em outra tabela)
		// e retornar appErrors.ErrConflict se for o caso.
		return appErrors.WrapErrorf(result.Error, "falha ao excluir CNPJ (GORM)")
	}

	if result.RowsAffected == 0 {
		// Isso não deveria acontecer se o First acima encontrou o registro,
		// mas é uma checagem de segurança.
		appLogger.Warnf("Exclusão do CNPJ ID %d não afetou nenhuma linha (já excluído?).", cnpjID)
		return fmt.Errorf("%w: CNPJ com ID %d não encontrado durante a operação de exclusão (ou já excluído)", appErrors.ErrNotFound, cnpjID)
	}

	appLogger.Infof("CNPJ %s (ID: %d) excluído.", dbCNPJ.CNPJ, cnpjID)
	return nil
}

// GetAll busca todos os CNPJs, com opção de incluir inativos.
func (r *gormCNPJRepository) GetAll(includeInactive bool) ([]models.DBCNPJ, error) {
	var cnpjs []models.DBCNPJ
	query := r.db.Order("registration_date DESC") // Mais recentes primeiro

	if !includeInactive {
		query = query.Where("active = ?", true)
	}

	if err := query.Find(&cnpjs).Error; err != nil {
		appLogger.Errorf("Erro ao buscar todos os CNPJs: %v", err)
		return nil, appErrors.WrapErrorf(err, "falha ao buscar lista de CNPJs (GORM)")
	}
	return cnpjs, nil
}

// GetByNetworkID busca CNPJs associados a um NetworkID específico.
func (r *gormCNPJRepository) GetByNetworkID(networkID uint64, includeInactive bool) ([]models.DBCNPJ, error) {
	var cnpjs []models.DBCNPJ
	query := r.db.Where("network_id = ?", networkID).Order("cnpj ASC")

	if !includeInactive {
		query = query.Where("active = ?", true)
	}

	if err := query.Find(&cnpjs).Error; err != nil {
		appLogger.Errorf("Erro ao buscar CNPJs para network ID %d: %v", networkID, err)
		return nil, appErrors.WrapErrorf(err, fmt.Sprintf("falha ao buscar CNPJs para a rede %d (GORM)", networkID))
	}
	return cnpjs, nil
}

// Helper para extrair chaves de mapa (para log em Go < 1.21)
// Em Go 1.21+, use maps.Keys()
// Se estiver em Go < 1.21, descomente e use esta função, ou atualize Go.
/*
func getMapKeys(m map[string]interface{}) []string {
    keys := make([]string, 0, len(m))
    for k := range m {
        keys = append(keys, k)
    }
    return keys
}
*/
