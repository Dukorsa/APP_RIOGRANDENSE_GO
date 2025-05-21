// Em internal/services/auditlog_service.go
package models

import (
	"errors" // Para errors.Is
	"strings"
	"time"

	"github.com/google/uuid"

	// auth "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/auth" // Removido se LoggableSession for de 'types'
	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/errors"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/types" // Import para a interface LoggableSession
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/models"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/repositories"

	// Para evitar a dependência direta de 'auth' por SessionManager,
	// SessionManager também poderia ser uma interface aqui, se AuditLogService precisasse
	// de mais do que apenas obter a sessão atual. Por ora, *auth.SessionManager é mantido.
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/auth"
)

// AuditLogService define a interface para o serviço de log de auditoria.
type AuditLogService interface {
	// LogAction registra uma ação de auditoria.
	// `entry` são os dados básicos do log.
	// `userSession` (opcional) é a sessão do usuário que realizou a ação. Se nil,
	// o serviço tentará obter do contexto global ou usará defaults.
	// Agora usa types.LoggableSession para evitar ciclo de importação.
	LogAction(entry models.AuditLogEntry, userSession types.LoggableSession) error

	// GetAuditLogs busca logs de auditoria com base nos filtros fornecidos e com paginação.
	GetAuditLogs(
		startDate, endDate *time.Time,
		severity, user, action *string,
		limit, offset int,
	) (logs []models.AuditLogEntry, totalCount int64, err error)
}

// auditLogServiceImpl é a implementação de AuditLogService.
type auditLogServiceImpl struct {
	repo           repositories.AuditLogRepository
	sessionManager *auth.SessionManager // Mantido como *auth.SessionManager para buscar a sessão.
	// O resultado (*auth.SessionData) implementa types.LoggableSession.
}

// NewAuditLogService cria uma nova instância de AuditLogService.
func NewAuditLogService(repo repositories.AuditLogRepository, sm *auth.SessionManager) AuditLogService {
	if repo == nil {
		appLogger.Fatalf("AuditLogRepository não pode ser nil para NewAuditLogService")
	}
	if sm == nil {
		appLogger.Warn("SessionManager é nil para NewAuditLogService. Obtenção de sessão global para logs não funcionará se userSession não for passado explicitamente.")
	}
	return &auditLogServiceImpl{repo: repo, sessionManager: sm}
}

// LogAction registra uma ação de auditoria no banco de dados.
func (s *auditLogServiceImpl) LogAction(entry models.AuditLogEntry, userSession types.LoggableSession) error {
	// 1. Validar e Normalizar entrada básica
	if strings.TrimSpace(entry.Action) == "" {
		return appErrors.WrapErrorf(appErrors.ErrInvalidInput, "ação do log de auditoria não pode ser vazia")
	}
	if strings.TrimSpace(entry.Description) == "" {
		return appErrors.WrapErrorf(appErrors.ErrInvalidInput, "descrição do log de auditoria não pode ser vazia")
	}

	normalizedSeverity := strings.ToUpper(strings.TrimSpace(entry.Severity))
	if _, ok := models.ValidSeverities[normalizedSeverity]; !ok {
		appLogger.Warnf("Nível de severidade inválido '%s' fornecido para log. Usando 'INFO'. Ação: %s", entry.Severity, entry.Action)
		entry.Severity = "INFO"
	} else {
		entry.Severity = normalizedSeverity
	}

	// 2. Preencher detalhes do usuário e da sessão.
	effectiveSession := userSession // effectiveSession é do tipo types.LoggableSession
	if effectiveSession == nil && s.sessionManager != nil {
		// s.sessionManager.GetCurrentSession() retorna (*auth.SessionData, error).
		// *auth.SessionData implementa types.LoggableSession.
		sessFromMgr, errSess := s.sessionManager.GetCurrentSession()
		if errSess != nil && !errors.Is(errSess, appErrors.ErrSessionExpired) && !errors.Is(errSess, appErrors.ErrNotFound) {
			appLogger.Warnf("Erro ao tentar obter sessão global para log de auditoria (Ação: %s): %v", entry.Action, errSess)
		}
		if sessFromMgr != nil { // Se GetCurrentSession retornou uma sessão válida (*auth.SessionData)
			effectiveSession = sessFromMgr // Agora effectiveSession é *auth.SessionData, que satisfaz types.LoggableSession.
		}
	}

	if effectiveSession != nil {
		if entry.Username == "" {
			entry.Username = effectiveSession.GetUsername() // Usa método da interface
		}
		userID := effectiveSession.GetUserID() // Usa método da interface
		if entry.UserID == nil || *entry.UserID == uuid.Nil {
			entry.UserID = &userID
		}
		roles := effectiveSession.GetRoles() // Usa método da interface
		if entry.Roles == nil && len(roles) > 0 {
			rolesStr := strings.Join(roles, ", ")
			entry.Roles = &rolesStr
		}
		ipAddr := effectiveSession.GetIPAddress() // Usa método da interface
		if entry.IPAddress == nil || *entry.IPAddress == "" {
			if ipAddr != "" {
				entry.IPAddress = &ipAddr
			}
		}
	} else {
		if entry.Username == "" {
			entry.Username = "system"
		}
		if entry.IPAddress == nil || *entry.IPAddress == "" {
			val := "N/A"
			entry.IPAddress = &val
		}
	}

	// 3. Sanitização e Limites
	if len(entry.Description) > 4000 {
		entry.Description = entry.Description[:3997] + "..."
		appLogger.Warnf("Descrição do log de auditoria truncada para 4000 caracteres. Ação: %s", entry.Action)
	}

	// 4. Timestamp
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}

	// 5. Persistir o log
	_, err := s.repo.Create(entry)
	if err != nil {
		return appErrors.WrapErrorf(err, "falha ao persistir log de auditoria (Ação: %s)", entry.Action)
	}
	return nil
}

// GetAuditLogs busca logs de auditoria com base nos filtros fornecidos.
func (s *auditLogServiceImpl) GetAuditLogs(
	startDate, endDate *time.Time,
	severity, user, action *string,
	limit, offset int,
) (logs []models.AuditLogEntry, totalCount int64, err error) {

	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
		appLogger.Warnf("Solicitação de GetAuditLogs com limite > 1000. Reduzido para 1000.")
	}
	if offset < 0 {
		offset = 0
	}

	var startUTC, endUTC *time.Time
	if startDate != nil {
		val := startDate.In(time.UTC)
		startUTC = &val
	}
	if endDate != nil {
		val := endDate.In(time.UTC)
		endUTC = &val
	}

	logs, totalCount, err = s.repo.GetFiltered(startUTC, endUTC, severity, user, action, limit, offset)
	if err != nil {
		return nil, 0, appErrors.WrapErrorf(err, "falha ao buscar logs de auditoria do repositório")
	}
	return logs, totalCount, nil
}
