package repositories

import (
	"errors"
	"fmt"
	"maps" // Requer Go 1.21+; para versões anteriores, use um helper.
	"strings"

	// "time" // Não diretamente usado para campos de UpdatedAt aqui, pois GORM pode gerenciar

	"gorm.io/gorm"
	"gorm.io/gorm/clause" // Para OnConflict em upserts, se necessário

	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/models"
	// "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/utils" // Para IsValidCNPJ se a validação fosse aqui
)

// CNPJRepository define a interface para operações no repositório de CNPJs.
type CNPJRepository interface {
	// Add cadastra um novo CNPJ. Espera que cnpjData.CNPJ já tenha sido validado e limpo.
	Add(cnpjData models.CNPJCreate) (*models.DBCNPJ, error)

	GetByID(cnpjID uint64) (*models.DBCNPJ, error)
	GetByCNPJ(cnpjNumber string) (*models.DBCNPJ, error) // cnpjNumber deve ser apenas dígitos.
	Update(cnpjID uint64, cnpjUpdateData models.CNPJUpdate) (*models.DBCNPJ, error)
	Delete(cnpjID uint64) error
	GetAll(includeInactive bool) ([]models.DBCNPJ, error)
	GetByNetworkID(networkID uint64, includeInactive bool) ([]models.DBCNPJ, error)
	// UpsertCNPJ insere ou atualiza um CNPJ. Útil se a lógica de negócio permitir.
	// UpsertCNPJ(cnpjData models.CNPJCreate) (*models.DBCNPJ, error)
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
// cnpjData.CNPJ já deve estar limpo (apenas dígitos) e validado (formato e dígitos verificadores) pelo serviço.
func (r *gormCNPJRepository) Add(cnpjData models.CNPJCreate) (*models.DBCNPJ, error) {
	// A validação de formato e dígitos verificadores do CNPJ deve ocorrer no serviço.
	// Aqui, assumimos que `cnpjData.CNPJ` contém o CNPJ limpo (14 dígitos).
	if len(cnpjData.CNPJ) != 14 { // Checagem de segurança
		appLogger.Warnf("Tentativa de adicionar CNPJ com formato inválido (não 14 dígitos): '%s'", cnpjData.CNPJ)
		return nil, fmt.Errorf("%w: CNPJ fornecido ao repositório deve ter 14 dígitos", appErrors.ErrInvalidInput)
	}

	// Verificar se o CNPJ já existe (a constraint unique no DB também faria isso, mas verificar antes é melhor para o erro).
	var existing models.DBCNPJ
	err := r.db.Where("cnpj = ?", cnpjData.CNPJ).First(&existing).Error
	if err == nil { // Encontrou um existente
		appLogger.Warnf("Tentativa de adicionar CNPJ já existente: %s", cnpjData.CNPJ)
		return nil, fmt.Errorf("%w: CNPJ %s já cadastrado", appErrors.ErrConflict, cnpjData.CNPJ)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) { // Erro diferente de "não encontrado"
		appLogger.Errorf("Erro ao verificar existência do CNPJ %s: %v", cnpjData.CNPJ, err)
		return nil, appErrors.WrapErrorf(err, "falha ao verificar existência do CNPJ (GORM)")
	}
	// Se gorm.ErrRecordNotFound, o CNPJ não existe, podemos prosseguir.

	dbCNPJ := models.DBCNPJ{
		CNPJ:      cnpjData.CNPJ, // Já limpo
		NetworkID: cnpjData.NetworkID,
		Active:    true, // Novo CNPJ é ativo por padrão.
		// RegistrationDate é `default:now()` ou `autoCreateTime` pelo GORM.
	}

	result := r.db.Create(&dbCNPJ)
	if result.Error != nil {
		appLogger.Errorf("Erro ao adicionar CNPJ %s para network ID %d: %v", cnpjData.CNPJ, cnpjData.NetworkID, result.Error)
		// Verificar erro de FK para NetworkID
		if strings.Contains(strings.ToLower(result.Error.Error()), "foreign key constraint") ||
			strings.Contains(strings.ToLower(result.Error.Error()), "foreign key violation") {
			return nil, fmt.Errorf("%w: rede com ID %d não encontrada ao tentar cadastrar CNPJ", appErrors.ErrNotFound, cnpjData.NetworkID)
		}
		return nil, appErrors.WrapErrorf(result.Error, "falha ao cadastrar CNPJ (GORM)")
	}

	appLogger.Infof("Novo CNPJ cadastrado: %s para rede ID %d (ID no DB: %d)", dbCNPJ.CNPJ, dbCNPJ.NetworkID, dbCNPJ.ID)
	return &dbCNPJ, nil
}

// GetByID busca um CNPJ pelo seu ID no banco de dados.
func (r *gormCNPJRepository) GetByID(cnpjID uint64) (*models.DBCNPJ, error) {
	var dbCNPJ models.DBCNPJ
	result := r.db.First(&dbCNPJ, cnpjID) // GORM busca pela chave primária
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
	// O serviço deve garantir que cnpjNumber seja limpo e tenha 14 dígitos.
	if len(cnpjNumber) != 14 {
		return nil, fmt.Errorf("%w: formato de CNPJ inválido para busca '%s' (esperado 14 dígitos)", appErrors.ErrInvalidInput, cnpjNumber)
	}

	var dbCNPJ models.DBCNPJ
	result := r.db.Where("cnpj = ?", cnpjNumber).First(&dbCNPJ)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: CNPJ '%s' não encontrado", appErrors.ErrNotFound, cnpjNumber)
		}
		appLogger.Errorf("Erro ao buscar CNPJ pelo número '%s': %v", cnpjNumber, result.Error)
		return nil, appErrors.WrapErrorf(result.Error, "falha ao buscar CNPJ pelo número (GORM)")
	}
	return &dbCNPJ, nil
}

// Update atualiza os dados de um CNPJ existente.
func (r *gormCNPJRepository) Update(cnpjID uint64, cnpjUpdateData models.CNPJUpdate) (*models.DBCNPJ, error) {
	dbCNPJ, err := r.GetByID(cnpjID)
	if err != nil {
		return nil, err // GetByID já formata o erro (ErrNotFound ou DB error)
	}

	// Monta um mapa dos campos a serem atualizados para evitar atualizar campos não intencionais.
	// GORM `Updates` com struct atualiza apenas campos não-zero, a menos que `Select` seja usado.
	// Usar um mapa é mais explícito para atualizações parciais com ponteiros.
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
		return dbCNPJ, nil // Retorna o objeto existente se nada mudou.
	}

	// GORM atualiza `updated_at` automaticamente se o modelo tiver o campo com `autoUpdateTime`.
	// Se não, adicione manualmente: updates["updated_at"] = time.Now().UTC()

	// Usar `db.Model(&dbCNPJ).Updates(updates)` atualiza o objeto `dbCNPJ` in-place.
	result := r.db.Model(&dbCNPJ).Updates(updates)
	if result.Error != nil {
		appLogger.Errorf("Erro ao atualizar CNPJ ID %d: %v", cnpjID, result.Error)
		if strings.Contains(strings.ToLower(result.Error.Error()), "foreign key constraint") && cnpjUpdateData.NetworkID != nil {
			return nil, fmt.Errorf("%w: rede com ID %d não encontrada para atualização do CNPJ", appErrors.ErrNotFound, *cnpjUpdateData.NetworkID)
		}
		return nil, appErrors.WrapErrorf(result.Error, "falha ao atualizar CNPJ (GORM)")
	}

	if result.RowsAffected == 0 && changed {
		appLogger.Warnf("Atualização do CNPJ ID %d não afetou nenhuma linha, mas mudanças eram esperadas.", cnpjID)
	}

	appLogger.Infof("CNPJ ID %d atualizado. Campos alterados: %v", cnpjID, maps.Keys(updates))
	return dbCNPJ, nil // dbCNPJ foi atualizado in-place.
}

// Delete remove um CNPJ do banco de dados (exclusão física).
func (r *gormCNPJRepository) Delete(cnpjID uint64) error {
	// Opcional: buscar antes para logar ou verificar existência.
	var dbCNPJ models.DBCNPJ
	if err := r.db.Select("id", "cnpj").First(&dbCNPJ, cnpjID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: CNPJ com ID %d não encontrado para exclusão", appErrors.ErrNotFound, cnpjID)
		}
		appLogger.Errorf("Erro ao verificar CNPJ ID %d antes da exclusão: %v", cnpjID, err)
		return appErrors.WrapErrorf(err, "falha ao verificar CNPJ antes da exclusão (GORM)")
	}

	// GORM realiza exclusão física por padrão.
	result := r.db.Delete(&models.DBCNPJ{}, cnpjID)
	if result.Error != nil {
		appLogger.Errorf("Erro ao excluir CNPJ ID %d: %v", cnpjID, result.Error)
		// Verificar se o erro é de FK (ex: CNPJ usado em outra tabela que restringe a exclusão)
		if strings.Contains(strings.ToLower(result.Error.Error()), "foreign key constraint") {
			return fmt.Errorf("%w: não é possível excluir o CNPJ ID %d pois ele está referenciado em outros registros", appErrors.ErrConflict, cnpjID)
		}
		return appErrors.WrapErrorf(result.Error, "falha ao excluir CNPJ (GORM)")
	}

	if result.RowsAffected == 0 {
		// Não deveria acontecer se o First acima encontrou o registro.
		appLogger.Warnf("Exclusão do CNPJ ID %d não afetou nenhuma linha (já excluído ou condição de corrida?).", cnpjID)
		return fmt.Errorf("%w: CNPJ com ID %d não encontrado durante a operação de exclusão", appErrors.ErrNotFound, cnpjID)
	}

	appLogger.Infof("CNPJ %s (ID: %d) excluído.", dbCNPJ.CNPJ, cnpjID)
	return nil
}

// GetAll busca todos os CNPJs, com opção de incluir inativos.
// Ordena por data de registro (mais recentes primeiro) e depois por CNPJ.
func (r *gormCNPJRepository) GetAll(includeInactive bool) ([]models.DBCNPJ, error) {
	var cnpjs []models.DBCNPJ
	query := r.db.Order("registration_date DESC, cnpj ASC")

	if !includeInactive {
		query = query.Where("active = ?", true)
	}

	if err := query.Find(&cnpjs).Error; err != nil {
		appLogger.Errorf("Erro ao buscar todos os CNPJs (includeInactive: %t): %v", includeInactive, err)
		return nil, appErrors.WrapErrorf(err, "falha ao buscar lista de CNPJs (GORM)")
	}
	return cnpjs, nil
}

// GetByNetworkID busca CNPJs associados a um NetworkID específico.
// Ordena por CNPJ.
func (r *gormCNPJRepository) GetByNetworkID(networkID uint64, includeInactive bool) ([]models.DBCNPJ, error) {
	var cnpjs []models.DBCNPJ
	query := r.db.Where("network_id = ?", networkID).Order("cnpj ASC")

	if !includeInactive {
		query = query.Where("active = ?", true)
	}

	if err := query.Find(&cnpjs).Error; err != nil {
		appLogger.Errorf("Erro ao buscar CNPJs para network ID %d (includeInactive: %t): %v", networkID, includeInactive, err)
		return nil, appErrors.WrapErrorf(err, fmt.Sprintf("falha ao buscar CNPJs para a rede %d (GORM)", networkID))
	}
	return cnpjs, nil
}

// UpsertCNPJ (Exemplo, não solicitado diretamente, mas útil)
// Cria um novo CNPJ ou atualiza um existente com base no número do CNPJ.
// `cnpjData` deve ter CNPJ já limpo.
// Este método é mais complexo pois precisa definir quais campos atualizar em caso de conflito.
func (r *gormCNPJRepository) UpsertCNPJ(cnpjData models.CNPJCreate) (*models.DBCNPJ, error) {
	if len(cnpjData.CNPJ) != 14 {
		return nil, fmt.Errorf("%w: CNPJ para upsert deve ter 14 dígitos", appErrors.ErrInvalidInput)
	}

	dbCNPJ := models.DBCNPJ{
		CNPJ:      cnpjData.CNPJ,
		NetworkID: cnpjData.NetworkID,
		Active:    true, // Assume ativo no upsert, ou pode ser parte do update.
		// RegistrationDate será definida pelo DB na inserção.
	}

	// Tenta inserir. Se houver conflito na coluna `cnpj` (que é uniqueIndex),
	// atualiza os campos especificados em `DoUpdates`.
	err := r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "cnpj"}},                                          // Coluna de conflito
		DoUpdates: clause.AssignmentColumns([]string{"network_id", "active", "updated_at"}), // Campos a atualizar
		// Se "updated_at" não for autoUpdateTime, precisa ser setado explicitamente:
		// DoUpdates: clause.Assignments(map[string]interface{}{
		// 	"network_id": dbCNPJ.NetworkID, // Usa o valor da tentativa de inserção
		// 	"active":     dbCNPJ.Active,
		// 	"updated_at": time.Now().UTC(),
		// }),
	}).Create(&dbCNPJ).Error

	if err != nil {
		appLogger.Errorf("Erro durante upsert do CNPJ %s: %v", cnpjData.CNPJ, err)
		// Tratar erros de FK e outros
		return nil, appErrors.WrapErrorf(err, "falha no upsert do CNPJ (GORM)")
	}

	appLogger.Infof("CNPJ %s (ID: %d) inserido/atualizado via upsert.", dbCNPJ.CNPJ, dbCNPJ.ID)
	return &dbCNPJ, nil
}
