package services

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/auth" // Para SessionData
	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/errors"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/models"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/repositories"
	// "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/utils" // Para sanitização, se fosse mais complexa
)

// AuditLogService define a interface para o serviço de log de auditoria.
type AuditLogService interface {
	// LogAction registra uma ação de auditoria.
	// `entry` são os dados básicos do log.
	// `userSession` (opcional) é a sessão do usuário que realizou a ação. Se nil,
	// o serviço tentará obter do contexto global (via `sessionManager`) ou usará defaults.
	LogAction(entry models.AuditLogEntry, userSession *auth.SessionData) error

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
	sessionManager *auth.SessionManager // Para obter a sessão atual se não fornecida em LogAction
}

// NewAuditLogService cria uma nova instância de AuditLogService.
// `sm` (SessionManager) pode ser nil se o serviço não precisar obter a sessão globalmente,
// mas é útil para enriquecer logs quando a sessão não é passada explicitamente.
func NewAuditLogService(repo repositories.AuditLogRepository, sm *auth.SessionManager) AuditLogService {
	if repo == nil {
		appLogger.Fatalf("AuditLogRepository não pode ser nil para NewAuditLogService")
	}
	if sm == nil {
		appLogger.Warn("SessionManager é nil para NewAuditLogService. Obtenção de sessão global para logs não funcionará se userSession não for passado.")
	}
	return &auditLogServiceImpl{repo: repo, sessionManager: sm}
}

// LogAction registra uma ação de auditoria no banco de dados.
func (s *auditLogServiceImpl) LogAction(entry models.AuditLogEntry, userSession *auth.SessionData) error {
	// 1. Validar e Normalizar entrada básica
	if strings.TrimSpace(entry.Action) == "" {
		return appErrors.WrapErrorf(appErrors.ErrInvalidInput, "ação do log de auditoria não pode ser vazia")
	}
	if strings.TrimSpace(entry.Description) == "" {
		return appErrors.WrapErrorf(appErrors.ErrInvalidInput, "descrição do log de auditoria não pode ser vazia")
	}

	// Normaliza a severidade para maiúsculas e valida.
	normalizedSeverity := strings.ToUpper(strings.TrimSpace(entry.Severity))
	if _, ok := models.ValidSeverities[normalizedSeverity]; !ok {
		appLogger.Warnf("Nível de severidade inválido '%s' fornecido para log. Usando 'INFO'. Ação: %s", entry.Severity, entry.Action)
		entry.Severity = "INFO"
	} else {
		entry.Severity = normalizedSeverity
	}

	// 2. Preencher detalhes do usuário e da sessão.
	// Tenta usar a sessão passada; se não houver, tenta obter a sessão global.
	effectiveSession := userSession
	if effectiveSession == nil && s.sessionManager != nil {
		// Tenta obter a sessão atual do gerenciador.
		// `GetCurrentSession` pode retornar `nil, nil` se não houver sessão no contexto.
		sess, errSess := s.sessionManager.GetCurrentSession()
		if errSess != nil && !errors.Is(errSess, appErrors.ErrSessionExpired) && !errors.Is(errSess, appErrors.ErrNotFound) {
			// Logar erro inesperado ao obter sessão, mas continuar (pode ser uma ação do sistema).
			appLogger.Warnf("Erro ao tentar obter sessão global para log de auditoria (Ação: %s): %v", entry.Action, errSess)
		}
		effectiveSession = sess // Pode ser nil se não houver sessão global.
	}

	// Preenche campos da `entry` com base na `effectiveSession`.
	if effectiveSession != nil {
		if entry.Username == "" { // Prioriza username da sessão se não definido na `entry`.
			entry.Username = effectiveSession.Username
		}
		if entry.UserID == nil || *entry.UserID == uuid.Nil { // Prioriza UserID da sessão.
			entry.UserID = &effectiveSession.UserID
		}
		if entry.Roles == nil && len(effectiveSession.Roles) > 0 {
			rolesStr := strings.Join(effectiveSession.Roles, ", ")
			entry.Roles = &rolesStr // Armazena como string CSV.
		}
		if entry.IPAddress == nil || *entry.IPAddress == "" { // Prioriza IP da sessão.
			if effectiveSession.IPAddress != "" {
				entry.IPAddress = &effectiveSession.IPAddress
			}
		}
	} else {
		// Se nenhuma sessão disponível, e Username/UserID não foram preenchidos na `entry`,
		// usar defaults para indicar ação do sistema ou anônima.
		if entry.Username == "" {
			entry.Username = "system" // Ou "anonymous", dependendo do contexto.
		}
		// UserID pode permanecer nulo para "system".
		if entry.IPAddress == nil || *entry.IPAddress == "" {
			val := "N/A"
			entry.IPAddress = &val
		}
	}

	// 3. Sanitização e Limites (exemplos)
	// A sanitização mais robusta contra XSS/SQLi deve ser feita na camada de entrada/saída.
	// Aqui, focamos em limites de tamanho para o DB.
	// entry.Description = utils.SanitizeInput(entry.Description) // Exemplo de sanitização genérica.

	if len(entry.Description) > 4000 { // Limite arbitrário (ex: do modelo Python).
		entry.Description = entry.Description[:3997] + "..." // Trunca com reticências.
		appLogger.Warnf("Descrição do log de auditoria truncada para 4000 caracteres. Ação: %s", entry.Action)
	}

	// Metadados: O tipo JSONMetadata já lida com a serialização para JSON.
	// Se houver limites de tamanho para o campo JSON no DB, a validação pode ser feita aqui.
	// if entry.Metadata != nil {
	// jsonBytes, _ := json.Marshal(entry.Metadata)
	// if len(jsonBytes) > 10240 { /* Truncar ou retornar erro */ }
	// }

	// 4. Timestamp
	// Se não preenchido, o GORM `default:now()` ou `autoCreateTime` deve cuidar disso.
	// Mas definir explicitamente em UTC é uma boa prática se o DB não estiver configurado para UTC.
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}

	// 5. Persistir o log
	_, err := s.repo.Create(entry)
	if err != nil {
		// O repositório já deve ter logado o erro de DB.
		// Envolver o erro para o chamador.
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

	// Validação e normalização dos parâmetros de paginação.
	if limit <= 0 {
		limit = 100 // Limite padrão.
	}
	if limit > 1000 { // Limite máximo para evitar sobrecarga.
		limit = 1000
		appLogger.Warnf("Solicitação de GetAuditLogs com limite > 1000. Reduzido para 1000.")
	}
	if offset < 0 {
		offset = 0 // Offset não pode ser negativo.
	}

	// Normalizar datas para UTC para consistência na query, se elas tiverem fuso horário.
	// O repositório já lida com startDate como início do dia e endDate como fim do dia.
	var startUTC, endUTC *time.Time
	if startDate != nil {
		val := startDate.In(time.UTC)
		startUTC = &val
	}
	if endDate != nil {
		val := endDate.In(time.UTC)
		endUTC = &val
	}

	// Chama o repositório para buscar os logs.
	logs, totalCount, err = s.repo.GetFiltered(startUTC, endUTC, severity, user, action, limit, offset)
	if err != nil {
		// Erro já logado pelo repositório. Envolver para o chamador.
		return nil, 0, appErrors.WrapErrorf(err, "falha ao buscar logs de auditoria do repositório")
	}
	return logs, totalCount, nil
}
