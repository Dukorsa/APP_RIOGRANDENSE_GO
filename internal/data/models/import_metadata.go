package models

import (
	"strings"
	"time"
	// "gorm.io/gorm" // Descomentado se GORM for usado diretamente aqui, mas geralmente não é.
)

// DBImportMetadata representa os metadados de uma importação de arquivo no banco de dados.
type DBImportMetadata struct {
	ID uint64 `gorm:"primaryKey;autoIncrement"` // ID único do registro de metadados

	// FileType identifica o tipo do arquivo importado (ex: "DIREITOS", "OBRIGACOES").
	// `uniqueIndex` garante que haja apenas uma entrada de metadados por tipo de arquivo.
	// É armazenado em maiúsculas para consistência.
	FileType string `gorm:"type:varchar(50);uniqueIndex;not null"`

	// LastUpdatedAt armazena quando este tipo de arquivo foi atualizado pela última vez.
	// GORM pode usar `gorm:"autoUpdateTime"` ou o banco de dados pode ter um trigger/default.
	// Se a aplicação gerencia este campo, ele deve ser definido explicitamente no momento da atualização.
	LastUpdatedAt time.Time `gorm:"not null"`

	// OriginalFilename é o nome original do arquivo que foi importado mais recentemente (opcional).
	OriginalFilename *string `gorm:"type:varchar(255)"`

	// RecordCount é o número de registros processados com sucesso na última importação (opcional).
	RecordCount *int `gorm:"type:integer"`

	// ImportedBy é o nome de usuário de quem realizou a última importação (opcional).
	ImportedBy *string `gorm:"type:varchar(50)"`

	// CreatedAt pode ser útil para saber quando o metadado foi registrado pela primeira vez.
	// GORM pode usar `gorm:"autoCreateTime"` ou o banco `default:now()`.
	// CreatedAt time.Time `gorm:"not null;autoCreateTime"`
}

// TableName especifica o nome da tabela para GORM.
func (DBImportMetadata) TableName() string {
	return "import_metadata"
}

// --- Struct para Transferência de Dados (DTO) ---

// ImportMetadataPublic representa os dados de metadados de importação para a UI ou API.
// Este DTO é usado para expor os dados de forma controlada.
type ImportMetadataPublic struct {
	ID               uint64    `json:"id"`
	FileType         string    `json:"file_type"`
	LastUpdatedAt    time.Time `json:"last_updated_at"` // Data e hora da última atualização bem-sucedida.
	OriginalFilename *string   `json:"original_filename,omitempty"`
	RecordCount      *int      `json:"record_count,omitempty"`
	ImportedBy       *string   `json:"imported_by,omitempty"`
}

// ToImportMetadataPublic converte um DBImportMetadata (modelo do banco) para ImportMetadataPublic (DTO).
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

// --- Estrutura para Criação/Atualização (usada pelo serviço/repositório) ---

// ImportMetadataUpsert define os campos que podem ser fornecidos ao criar ou atualizar
// um registro de metadados de importação.
// O FileType é obrigatório e usado como chave de conflito no Upsert.
// LastUpdatedAt será sempre definido como `time.Now().UTC()` no momento da operação.
type ImportMetadataUpsert struct {
	// FileType deve ser normalizado (ex: para maiúsculas) antes de ser usado.
	FileType         string
	OriginalFilename *string
	RecordCount      *int
	ImportedBy       *string
}

// Normalize garante que o FileType esteja em maiúsculas.
func (imu *ImportMetadataUpsert) Normalize() {
	if imu != nil {
		imu.FileType = strings.ToUpper(strings.TrimSpace(imu.FileType))
	}
}
