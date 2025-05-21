package models

import (
	"time"
	// "gorm.io/gorm" // Se estiver usando GORM
)

// DBImportMetadata representa os metadados de uma importação de arquivo no banco de dados.
type DBImportMetadata struct {
	ID       uint64 `gorm:"primaryKey;autoIncrement"`              // ID único do registro de metadados
	FileType string `gorm:"type:varchar(50);uniqueIndex;not null"` // Tipo do arquivo importado (ex: "DIREITOS", "OBRIGACOES")

	// LastUpdatedAt armazena quando este tipo de arquivo foi atualizado pela última vez.
	// GORM pode usar `gorm:"autoUpdateTime"` ou o banco de dados pode ter um trigger/default.
	// Para consistência com o Python que usava server_default=func.now(), podemos definir na aplicação.
	LastUpdatedAt time.Time `gorm:"not null"`

	OriginalFilename *string `gorm:"type:varchar(255)"` // Nome original do arquivo importado (opcional)
	RecordCount      *int    `gorm:"type:integer"`      // Número de registros processados na última importação (opcional)
	ImportedBy       *string `gorm:"type:varchar(50)"`  // Username de quem realizou a importação (opcional)

	// CreatedAt pode ser útil para saber quando o metadado foi registrado pela primeira vez
	// CreatedAt time.Time `gorm:"not null;default:now()"`
}

// TableName especifica o nome da tabela para GORM.
func (DBImportMetadata) TableName() string {
	return "import_metadata"
}

// --- Struct para Transferência de Dados (se diferente do modelo de DB) ---

// ImportMetadataPublic representa os dados de metadados de importação para a UI ou API.
// Neste caso, é muito similar ao DBImportMetadata, mas podemos ser explícitos.
type ImportMetadataPublic struct {
	ID               uint64    `json:"id"`
	FileType         string    `json:"file_type"`
	LastUpdatedAt    time.Time `json:"last_updated_at"`
	OriginalFilename *string   `json:"original_filename,omitempty"`
	RecordCount      *int      `json:"record_count,omitempty"`
	ImportedBy       *string   `json:"imported_by,omitempty"`
}

// ToImportMetadataPublic converte um DBImportMetadata para ImportMetadataPublic.
func ToImportMetadataPublic(dbMeta *DBImportMetadata) *ImportMetadataPublic {
	if dbMeta == nil {
		return nil
	}
	return &ImportMetadataPublic{
		ID:               dbMeta.ID,
		FileType:         dbMeta.FileType,
		LastUpdatedAt:    dbMeta.LastUpdatedAt,
		OriginalFilename: dbMeta.OriginalFilename,
		RecordCount:      dbMeta.RecordCount,
		ImportedBy:       dbMeta.ImportedBy,
	}
}

// ToImportMetadataPublicList converte uma lista de DBImportMetadata para uma lista de ImportMetadataPublic.
func ToImportMetadataPublicList(dbMetas []*DBImportMetadata) []*ImportMetadataPublic {
	publicList := make([]*ImportMetadataPublic, len(dbMetas))
	for i, dbMeta := range dbMetas {
		publicList[i] = ToImportMetadataPublic(dbMeta)
	}
	return publicList
}

// --- Estrutura para Atualização (usada pelo serviço/repositório) ---
// ImportMetadataUpdate define os campos que podem ser atualizados para um metadado de importação.
// Usar ponteiros permite distinguir entre um valor não fornecido e um valor zero/string vazia.
type ImportMetadataUpdate struct {
	OriginalFilename *string
	RecordCount      *int
	ImportedBy       *string
	// LastUpdatedAt será sempre definido como time.Now().UTC() no momento da atualização.
}
