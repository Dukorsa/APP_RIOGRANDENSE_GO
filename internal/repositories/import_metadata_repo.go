package repositories

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause" // Para Upsert (OnConflict)

	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/config"
"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/errors"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/models"
)

// ImportMetadataRepository define a interface para operações no repositório de metadados de importação.
type ImportMetadataRepository interface {
	// GetByFileType busca metadados de importação por tipo de arquivo.
	// O tipo de arquivo é case-insensitive na busca.
	GetByFileType(fileType string) (*models.DBImportMetadata, error)

	// GetAll busca todos os metadados de importação, ordenados por tipo de arquivo.
	GetAll() ([]models.DBImportMetadata, error)

	// Upsert atualiza metadados existentes para o fileType ou cria um novo se não existir.
	// fileType é normalizado para maiúsculas.
	// LastUpdatedAt é sempre definido para o tempo atual (UTC).
	Upsert(upsertData models.ImportMetadataUpsert) (*models.DBImportMetadata, error)
}

// gormImportMetadataRepository é a implementação GORM de ImportMetadataRepository.
type gormImportMetadataRepository struct {
	db *gorm.DB
}

// NewGormImportMetadataRepository cria uma nova instância de gormImportMetadataRepository.
func NewGormImportMetadataRepository(db *gorm.DB) ImportMetadataRepository {
	if db == nil {
		appLogger.Fatalf("gorm.DB não pode ser nil para NewGormImportMetadataRepository")
	}
	return &gormImportMetadataRepository{db: db}
}

// GetByFileType busca metadados de importação por tipo de arquivo (case-insensitive).
func (r *gormImportMetadataRepository) GetByFileType(fileType string) (*models.DBImportMetadata, error) {
	trimmedFileType := strings.TrimSpace(fileType)
	if trimmedFileType == "" {
		return nil, fmt.Errorf("%w: tipo de arquivo não pode ser vazio para GetByFileType", appErrors.ErrInvalidInput)
	}

	var metadata models.DBImportMetadata
	// Busca case-insensitive usando LOWER() ou ILIKE (dependendo do DB).
	// GORM lida bem com `LOWER(column) = LOWER(?)` para a maioria dos bancos.
	result := r.db.Where("LOWER(file_type) = LOWER(?)", trimmedFileType).First(&metadata)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: metadados de importação para tipo '%s' não encontrados", appErrors.ErrNotFound, trimmedFileType)
		}
		appLogger.Errorf("Erro ao buscar metadados de importação para tipo '%s': %v", trimmedFileType, result.Error)
		return nil, appErrors.WrapErrorf(result.Error, "falha ao buscar metadados de importação (GORM)")
	}
	return &metadata, nil
}

// GetAll busca todos os metadados de importação, ordenados por tipo de arquivo.
func (r *gormImportMetadataRepository) GetAll() ([]models.DBImportMetadata, error) {
	var metadatas []models.DBImportMetadata
	// Ordena por file_type para consistência.
	if err := r.db.Order("file_type ASC").Find(&metadatas).Error; err != nil {
		appLogger.Errorf("Erro ao buscar todos os metadados de importação: %v", err)
		return nil, appErrors.WrapErrorf(err, "falha ao buscar lista de metadados de importação (GORM)")
	}
	return metadatas, nil
}

// Upsert atualiza ou cria metadados para um tipo de arquivo.
// LastUpdatedAt é sempre definido para o tempo atual (UTC).
func (r *gormImportMetadataRepository) Upsert(upsertData models.ImportMetadataUpsert) (*models.DBImportMetadata, error) {
	upsertData.Normalize() // Garante que FileType esteja em maiúsculas e trim.

	if upsertData.FileType == "" {
		return nil, fmt.Errorf("%w: tipo de arquivo não pode ser vazio para upsert de metadados", appErrors.ErrInvalidInput)
	}

	nowUTC := time.Now().UTC()

	// O struct `DBImportMetadata` é usado para a operação de Create/Update.
	// O GORM preencherá o ID na criação ou usará o ID existente na atualização (se encontrado pelo OnConflict).
	metadataToPersist := models.DBImportMetadata{
		FileType:         upsertData.FileType, // Já normalizado
		LastUpdatedAt:    nowUTC,              // Sempre atualiza este campo
		OriginalFilename: upsertData.OriginalFilename,
		RecordCount:      upsertData.RecordCount,
		ImportedBy:       upsertData.ImportedBy,
	}

	// GORM Upsert:
	// - `clause.OnConflict` especifica a constraint de conflito (neste caso, a coluna `file_type` que é `uniqueIndex`).
	// - `DoUpdates: clause.AssignmentColumns(...)` especifica quais colunas devem ser atualizadas se um conflito ocorrer.
	//   Exclui `id` e `file_type` da atualização, pois `file_type` é a chave de conflito e `id` é auto-incrementado/PK.
	//   `created_at` (se existisse no modelo e fosse `autoCreateTime`) também seria implicitamente excluído da atualização.
	result := r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "file_type"}}, // A coluna que tem a constraint UNIQUE
		DoUpdates: clause.AssignmentColumns([]string{"last_updated_at", "original_filename", "record_count", "imported_by"}),
	}).Create(&metadataToPersist) // `Create` tentará inserir; se houver conflito em `file_type`, fará o update.

	if result.Error != nil {
		appLogger.Errorf("Erro durante upsert de metadados para tipo '%s': %v", upsertData.FileType, result.Error)
		return nil, appErrors.WrapErrorf(result.Error, "falha ao atualizar/criar metadados de importação (GORM)")
	}

	// `metadataToPersist` é atualizado com o ID (se criado) ou reflete os updates.
	// Se o GORM não preencher o ID corretamente em um cenário de update complexo ou
	// se houver triggers no DB que modificam o registro de formas não refletidas pelo GORM,
	// uma busca explícita (`GetByFileType`) poderia ser feita aqui para retornar o estado mais recente.
	// No entanto, para `Create` com `OnConflict`, o GORM geralmente preenche o ID corretamente.

	appLogger.Infof("Metadados de importação para tipo '%s' atualizados/criados (ID: %d).", metadataToPersist.FileType, metadataToPersist.ID)
	return &metadataToPersist, nil
}
