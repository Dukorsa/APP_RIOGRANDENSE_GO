package types

import (
    "time"
    "github.com/google/uuid"
)

// LoggableSession representa o que o AuditLogService precisa de uma sessão.
type LoggableSession interface {
    GetID() string // ID da sessão
    GetUserID() uuid.UUID
    GetUsername() string
    GetRoles() []string
    GetIPAddress() string
    GetUserAgent() string
    GetCreatedAt() time.Time
    GetLastActivity() time.Time
    GetExpiresAt() time.Time
    GetMetadata() map[string]interface{}
}