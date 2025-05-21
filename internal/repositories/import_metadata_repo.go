package repositories

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause" // Para Upsert (OnConflict)

	appErrors "github.com/seu_usuario/riograndense_gio/internal/core/errors"
	appLogger "github.com/seu_usuario/riograndense_gio/internal/core/logger"
	"github.com/seu_usuario/riograndense_gio/internal/data/models"
)

// ImportMetadataRepository define a interface para operações no repositório de metadados de importação.
type ImportMetadataRepository interface {
	GetByFileType(fileType string) (*models.DBImportMetadata, error)
	GetAll() ([]models.DBImportMetadata, error)
	// Upsert atualiza metadados existentes ou cria novos se não existirem para o fileType.
	// O campo LastUpdatedAt é sempre definido para o tempo atual.
	Upsert(fileType string, originalFilename *string, recordCount *int, importedBy *string) (*models.DBImportMetadata, error)
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
	if fileType == "" {
		return nil, fmt.Errorf("%w: tipo de arquivo não pode ser vazio", appErrors.ErrInvalidInput)
	}

	var metadata models.DBImportMetadata
	// Usar LOWER ou ILIKE dependendo do DB para busca case-insensitive
	// GORM pode precisar de uma função específica do DB para case-insensitivity em Where.
	// Para PostgreSQL: result := r.db.Where("file_type ILIKE ?", fileType).First(&metadata)
	// Para outros ou abordagem genérica:
	result := r.db.Where("LOWER(file_type) = LOWER(?)", fileType).First(&metadata)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: metadados de importação para tipo '%s' não encontrados", appErrors.ErrNotFound, fileType)
		}
		appLogger.Errorf("Erro ao buscar metadados de importação para tipo '%s': %v", fileType, result.Error)
		return nil, appErrors.WrapErrorf(result.Error, "falha ao buscar metadados de importação (GORM)")
	}
	return &metadata, nil
}

// GetAll busca todos os metadados de importação, ordenados por tipo de arquivo.
func (r *gormImportMetadataRepository) GetAll() ([]models.DBImportMetadata, error) {
	var metadatas []models.DBImportMetadata
	if err := r.db.Order("file_type ASC").Find(&metadatas).Error; err != nil {
		appLogger.Errorf("Erro ao buscar todos os metadados de importação: %v", err)
		return nil, appErrors.WrapErrorf(err, "falha ao buscar lista de metadados de importação (GORM)")
	}
	return metadatas, nil
}

// Upsert atualiza ou cria metadados para um tipo de arquivo.
// LastUpdatedAt é sempre definido para o tempo atual (UTC).
func (r *gormImportMetadataRepository) Upsert(
	fileType string,
	originalFilename *string,
	recordCount *int,
	importedBy *string,
) (*models.DBImportMetadata, error) {
	if fileType == "" {
		return nil, fmt.Errorf("%w: tipo de arquivo não pode ser vazio para upsert", appErrors.ErrInvalidInput)
	}

	// Padroniza para maiúsculas para consistência no banco, já que é uniqueIndex
	normalizedFileType := strings.ToUpper(fileType)
	nowUTC := time.Now().UTC()

	metadata := models.DBImportMetadata{
		FileType:         normalizedFileType,
		LastUpdatedAt:    nowUTC, // Sempre atualiza este campo
		OriginalFilename: originalFilename,
		RecordCount:      recordCount,
		ImportedBy:       importedBy,
		// ID será gerenciado pelo GORM (autoincremento na criação, ou usado no OnConflict)
	}

	// GORM Upsert:
	// OnConflict especifica a constraint de conflito (neste caso, a coluna 'file_type' que é uniqueIndex).
	// DoUpdates especifica quais colunas devem ser atualizadas se um conflito ocorrer.
	// clause.AssignmentColumns pode ser usado para especificar explicitamente as colunas de atualização.
	// Ou clause.Associations para atualizar associações (não aplicável aqui).
	// Usar clause. всех except "id", "file_type", "created_at" (se existisse)
	// GORM v2: Para `DoUpdates`, você pode usar `clause.AssignmentColumns` para especificar colunas
	// ou `clause.AssignmentValues` para valores específicos, ou `clause.DoUpdates(clause.AssignmentColumns([]string{"last_updated_at", ...}))`

	// Se o ID não for PK ou não for usado para identificar o conflito,
	// e FileType for a constraint UNIQUE que causa o conflito:
	result := r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "file_type"}}, // A coluna que tem a constraint UNIQUE
		DoUpdates: clause.AssignmentColumns([]string{"last_updated_at", "original_filename", "record_count", "imported_by"}),
		// Alternativamente, para atualizar todos os campos exceto 'id' e 'file_type':
		// DoUpdates: clause. 모든 except ID e FileType
		// Ou, se você quiser atualizar com os valores da struct `metadata` que está sendo inserida:
		// DoUpdates: clause.Set("last_updated_at = EXCLUDED.last_updated_at, original_filename = EXCLUDED.original_filename, ..."),
	}).Create(&metadata) // Create tentará inserir; se houver conflito em file_type, fará o update.

	if result.Error != nil {
		appLogger.Errorf("Erro durante upsert de metadados para tipo '%s': %v", normalizedFileType, result.Error)
		return nil, appErrors.WrapErrorf(result.Error, "falha ao atualizar/criar metadados de importação (GORM)")
	}

	// O objeto 'metadata' deve ser atualizado com o ID (se criado) ou refletir os updates.
	// Se foi um update, o ID já existia. Se foi create, GORM preenche o ID.
	// Para garantir que temos o estado mais recente, especialmente se o DB tiver defaults/triggers não cobertos pelo GORM:
	// A cláusula .Create com OnConflict já deve retornar o objeto atualizado ou criado.
	// Se não, uma busca separada seria necessária:
	// var updatedMetadata models.DBImportMetadata
	// if err := r.db.Where("file_type = ?", normalizedFileType).First(&updatedMetadata).Error; err != nil {
	// 	  appLogger.Errorf("Erro ao buscar metadados após upsert para '%s': %v", normalizedFileType, err)
	//    return nil, appErrors.WrapErrorf(err, "falha ao buscar metadados após upsert")
	// }
	// return &updatedMetadata, nil

	appLogger.Infof("Metadados de importação para tipo '%s' atualizados/criados (ID: %d).", normalizedFileType, metadata.ID)
	return &metadata, nil
}
