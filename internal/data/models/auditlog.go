package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	// Se usar GORM para JSON: "gorm.io/datatypes"
)

// JSONMetadata é um tipo customizado para lidar com o campo metadata que é um JSON no banco.
// Ele implementa as interfaces sql.Scanner e driver.Valuer.
type JSONMetadata map[string]interface{}

// Value implementa a interface driver.Valuer.
// Converte JSONMetadata para uma string JSON para ser salva no banco.
func (jm JSONMetadata) Value() (driver.Value, error) {
	if jm == nil {
		return nil, nil
	}
	return json.Marshal(jm)
}

// Scan implementa a interface sql.Scanner.
// Converte uma string JSON do banco para JSONMetadata.
func (jm *JSONMetadata) Scan(value interface{}) error {
	if value == nil {
		*jm = nil
		return nil
	}
	b, ok := value.([]byte) // O driver geralmente retorna []byte para TEXT/JSONB
	if !ok {
		// Tentar converter string também, caso o driver retorne string
		s, okStr := value.(string)
		if !okStr {
			return errors.New("tipo de valor inválido para JSONMetadata scan, esperado []byte ou string")
		}
		b = []byte(s)
	}
	if len(b) == 0 { // Tratar string vazia como JSON nulo ou objeto vazio
		*jm = make(JSONMetadata) // Ou *jm = nil se preferir
		return nil
	}
	return json.Unmarshal(b, jm)
}

// AuditLogEntry representa uma entrada de log de auditoria no banco de dados.
// Esta struct é usada tanto para o ORM (com tags gorm, por exemplo)
// quanto para transferência de dados, similar ao LogEntry do Pydantic.
type AuditLogEntry struct {
	ID          uint64     `gorm:"primaryKey;autoIncrement"` // Usando uint64 para ID autoincrementado
	Timestamp   time.Time  `gorm:"not null;index;default:now()"`
	Action      string     `gorm:"type:varchar(100);not null;index"`
	Description string     `gorm:"type:text;not null"`
	Severity    string     `gorm:"type:varchar(10);not null;index"` // DEBUG, INFO, WARNING, ERROR, CRITICAL
	Username    string     `gorm:"type:varchar(50);not null;index"` // Username que realizou a ação
	UserID      *uuid.UUID `gorm:"type:uuid;index"`                 // ID do usuário (opcional, pode ser sistema)
	Roles       *string    `gorm:"type:varchar(255)"`               // Roles do usuário no momento da ação (string CSV ou JSON)
	IPAddress   *string    `gorm:"type:varchar(45)"`                // Endereço IP da origem da ação

	// Metadata armazena dados adicionais relevantes para a ação, como um JSON.
	// Se usar GORM: Metadata datatypes.JSON `gorm:"type:jsonb"` (para PostgreSQL) ou `gorm:"type:text"` (para SQLite)
	// Se usar database/sql puro ou sqlx, você usará o tipo JSONMetadata definido acima.
	Metadata JSONMetadata `gorm:"type:jsonb"` // Exemplo para GORM com PostgreSQL
	// Para SQLite com GORM, ou sqlx/database/sql:
	// Metadata JSONMetadata `gorm:"type:text"`
}

// TableName especifica o nome da tabela para GORM (opcional se seguir convenções).
func (AuditLogEntry) TableName() string {
	return "audit_logs"
}

// --- Validação (Exemplo usando tags, mas a validação real seria feita no serviço) ---
// Para validação real, você usaria uma struct separada para entrada,
// similar ao LogEntry do Pydantic, e a validaria no serviço antes de criar AuditLogEntry.
// Exemplo de struct de entrada para o serviço (não é o modelo de DB):
/*
type AuditLogInput struct {
	Action      string                 `validate:"required,min=1,max=100"`
	Description string                 `validate:"required,min=1,max=4000"`
	Severity    string                 `validate:"required,oneof=DEBUG INFO WARNING ERROR CRITICAL"`
	Username    string                 `validate:"required,min=1,max=50"` // Ou obtido da sessão
	UserID      *uuid.UUID             `validate:"omitempty,uuid"`
	Roles       *string                `validate:"omitempty,max=255"`
	IPAddress   *string                `validate:"omitempty,ip|cidrv4|cidrv6"`
	Metadata    map[string]interface{} `validate:"omitempty"`
}

// Helper para criar AuditLogEntry a partir do input e da sessão
func NewAuditLogEntryFromInput(input AuditLogInput, session *auth.SessionData) AuditLogEntry {
    entry := AuditLogEntry{
        Action: input.Action,
        Description: input.Description,
        Severity: input.Severity,
        Metadata: input.Metadata,
		// Timestamp é default
    }
    if session != nil {
        entry.Username = session.Username
        entry.UserID = &session.UserID // Supondo que SessionData tem UserID
        if len(session.Roles) > 0 {
            rolesStr := strings.Join(session.Roles, ",")
            entry.Roles = &rolesStr
        }
        entry.IPAddress = &session.IPAddress
    } else {
		entry.Username = input.Username // Se sessão não disponível, usa o que foi passado
		entry.UserID = input.UserID
		entry.Roles = input.Roles
		entry.IPAddress = input.IPAddress
	}
    return entry
}
*/

// ValidSeverities define os níveis de severidade válidos.
var ValidSeverities = map[string]bool{
	"DEBUG":    true,
	"INFO":     true,
	"WARNING":  true,
	"ERROR":    true,
	"CRITICAL": true,
}
