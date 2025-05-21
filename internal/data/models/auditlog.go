package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	// "gorm.io/datatypes" // Descomente se usar gorm.io/datatypes.JSON para GORM
)

// JSONMetadata é um tipo customizado para lidar com o campo metadata que é um JSON no banco.
// Ele implementa as interfaces sql.Scanner e driver.Valuer.
type JSONMetadata map[string]interface{}

// Value implementa a interface driver.Valuer.
// Converte JSONMetadata para uma string JSON (ou []byte) para ser salva no banco.
func (jm JSONMetadata) Value() (driver.Value, error) {
	if jm == nil {
		// Retornar nil explicitamente se o mapa for nil, o que o DB pode interpretar como NULL.
		return nil, nil
	}
	// Se o mapa estiver vazio, mas não nil, serializar para "{}"
	if len(jm) == 0 {
		return json.Marshal(make(map[string]interface{}))
	}
	return json.Marshal(jm)
}

// Scan implementa a interface sql.Scanner.
// Converte um valor do banco (geralmente []byte para JSON/JSONB, ou string para TEXT) para JSONMetadata.
func (jm *JSONMetadata) Scan(value interface{}) error {
	if value == nil {
		*jm = nil // Se o valor do DB for NULL, o mapa será nil.
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return errors.New("tipo de valor inválido para JSONMetadata scan, esperado []byte ou string")
	}

	if len(bytes) == 0 {
		// Tratar string/byte slice vazio como um mapa vazio, não nil,
		// para consistência se um JSON vazio "{}" é esperado.
		// Se nil for preferível para entrada vazia, use: *jm = nil
		*jm = make(JSONMetadata)
		return nil
	}

	// Se o conteúdo for a string "null", deve ser interpretado como um JSONMetadata nil.
	if string(bytes) == "null" {
		*jm = nil
		return nil
	}

	// Tenta desempacotar em um mapa temporário para garantir que é um objeto JSON válido.
	var tempMap map[string]interface{}
	if err := json.Unmarshal(bytes, &tempMap); err != nil {
		return fmt.Errorf("falha ao desempacotar JSON para JSONMetadata: %w", err)
	}
	*jm = tempMap // Atribui o mapa desempacotado com sucesso.
	return nil
}

// AuditLogEntry representa uma entrada de log de auditoria no banco de dados.
type AuditLogEntry struct {
	ID          uint64     `gorm:"primaryKey;autoIncrement"`
	Timestamp   time.Time  `gorm:"not null;index;default:now()"`     // Momento em que o evento ocorreu.
	Action      string     `gorm:"type:varchar(100);not null;index"` // Tipo de ação (ex: "USER_LOGIN", "NETWORK_CREATE").
	Description string     `gorm:"type:text;not null"`               // Descrição detalhada da ação.
	Severity    string     `gorm:"type:varchar(10);not null;index"`  // Nível de severidade (DEBUG, INFO, WARNING, ERROR, CRITICAL).
	Username    string     `gorm:"type:varchar(50);not null;index"`  // Nome de usuário que realizou a ação (ou "system", "anonymous").
	UserID      *uuid.UUID `gorm:"type:uuid;index"`                  // ID do usuário (opcional, pode ser nulo para ações do sistema).
	Roles       *string    `gorm:"type:varchar(255)"`                // Roles do usuário no momento da ação (string CSV ou JSON).
	IPAddress   *string    `gorm:"type:varchar(45)"`                 // Endereço IP da origem da ação (opcional).

	// Metadata armazena dados adicionais relevantes para a ação, como um JSON.
	// GORM com PostgreSQL: `gorm:"type:jsonb"`
	// GORM com SQLite ou outros que não suportam JSON nativo: `gorm:"type:text"` (usando JSONMetadata.Scan/Value).
	Metadata JSONMetadata `gorm:"type:jsonb"` // Ajustar `type` conforme o dialeto do banco.
}

// TableName especifica o nome da tabela para GORM.
func (AuditLogEntry) TableName() string {
	return "audit_logs" // Nome da tabela padronizado para plural.
}

// ValidSeverities define os níveis de severidade válidos para logs de auditoria.
// Usado para validação no serviço antes de persistir.
var ValidSeverities = map[string]bool{
	"DEBUG":    true,
	"INFO":     true,
	"WARNING":  true,
	"ERROR":    true,
	"CRITICAL": true,
}

// StringToRolesPtr converte uma string de roles (separados por vírgula) para um ponteiro de string.
// Retorna nil se a string de entrada for vazia.
func StringToRolesPtr(rolesStr string) *string {
	trimmed := strings.TrimSpace(rolesStr)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// RolesPtrToString converte um ponteiro de string de roles para uma string.
// Retorna uma string vazia se o ponteiro for nil.
func RolesPtrToString(rolesPtr *string) string {
	if rolesPtr == nil {
		return ""
	}
	return *rolesPtr
}
