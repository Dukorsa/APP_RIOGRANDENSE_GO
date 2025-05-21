package services

import (
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/auth" // Para SessionData
	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/errors"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/models"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/repositories"
	// "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/utils" // Para SecurityValidator, se usado aqui
)

// AuditLogService define a interface para o serviço de log de auditoria.
type AuditLogService interface {
	// LogAction registra uma ação de auditoria.
	// Se userSession for nil, tentará obter do contexto global (se implementado) ou usará defaults.
	LogAction(entry models.AuditLogEntry, userSession *auth.SessionData) error

	GetAuditLogs(
		startDate, endDate *time.Time,
		severity, user, action *string,
		limit, offset int,
	) (logs []models.AuditLogEntry, totalCount int64, err error)
}

// auditLogServiceImpl é a implementação de AuditLogService.
type auditLogServiceImpl struct {
	repo           repositories.AuditLogRepository
	sessionManager *auth.SessionManager // Para obter a sessão atual se não fornecida
}

// NewAuditLogService cria uma nova instância de AuditLogService.
func NewAuditLogService(repo repositories.AuditLogRepository, sm *auth.SessionManager) AuditLogService {
	if repo == nil {
		appLogger.Fatalf("AuditLogRepository não pode ser nil para NewAuditLogService")
	}
	if sm == nil {
		appLogger.Warn("SessionManager é nil para NewAuditLogService. Obtenção de sessão global não funcionará.")
	}
	return &auditLogServiceImpl{repo: repo, sessionManager: sm}
}

// LogAction registra uma ação de auditoria no banco de dados.
func (s *auditLogServiceImpl) LogAction(entry models.AuditLogEntry, userSession *auth.SessionData) error {
	// 1. Validar entrada básica
	if strings.TrimSpace(entry.Action) == "" {
		return appErrors.WrapErrorf(appErrors.ErrInvalidInput, "ação do log não pode ser vazia")
	}
	if strings.TrimSpace(entry.Description) == "" {
		return appErrors.WrapErrorf(appErrors.ErrInvalidInput, "descrição do log não pode ser vazia")
	}
	if _, ok := models.ValidSeverities[strings.ToUpper(entry.Severity)]; !ok {
		appLogger.Warnf("Nível de severidade inválido '%s' fornecido para log. Usando 'INFO'.", entry.Severity)
		entry.Severity = "INFO"
	} else {
		entry.Severity = strings.ToUpper(entry.Severity) // Garante maiúsculas
	}

	// 2. Preencher detalhes do usuário se a sessão for fornecida ou puder ser obtida
	effectiveSession := userSession
	if effectiveSession == nil && s.sessionManager != nil {
		// Tenta obter a sessão atual do gerenciador
		// Nota: GetCurrentSession pode retornar nil, nil se não houver sessão no contexto
		sess, _ := s.sessionManager.GetCurrentSession() // Ignora erro aqui, trata sess nil abaixo
		effectiveSession = sess
	}

	if effectiveSession != nil {
		if entry.Username == "" { // Prioriza username da sessão se não definido na entry
			entry.Username = effectiveSession.Username
		}
		if entry.UserID == nil || *entry.UserID == uuid.Nil { // Prioriza UserID da sessão
			entry.UserID = &effectiveSession.UserID
		}
		if entry.Roles == nil && len(effectiveSession.Roles) > 0 {
			rolesStr := strings.Join(effectiveSession.Roles, ", ")
			entry.Roles = &rolesStr
		}
		if entry.IPAddress == nil || *entry.IPAddress == "" {
			entry.IPAddress = &effectiveSession.IPAddress
		}
	} else {
		// Se nenhuma sessão disponível, e Username/UserID não foram preenchidos na entry,
		// usar defaults para indicar ação do sistema ou anônima.
		if entry.Username == "" {
			entry.Username = "system" // Ou "anonymous"
		}
		if entry.UserID == nil || *entry.UserID == uuid.Nil {
			// Deixar UserID nulo para sistema/anônimo
		}
		if entry.IPAddress == nil || *entry.IPAddress == "" {
			val := "N/A"
			entry.IPAddress = &val
		}
	}

	// Sanitização básica (o logger já pode fazer sanitização mais robusta)
	// Aqui podemos focar em limites de tamanho, se necessário, ou validações específicas.
	// entry.Description = utils.SecurityValidator.SanitizeInput(entry.Description) // Exemplo

	if len(entry.Description) > 4000 { // Limite do modelo Python
		entry.Description = entry.Description[:3997] + "..."
		appLogger.Warnf("Descrição do log truncada para 4000 caracteres. Ação: %s", entry.Action)
	}
	if entry.Metadata != nil {
		// Serializar para JSON para verificar tamanho, como no Python
		// jsonBytes, _ := json.Marshal(entry.Metadata)
		// if len(jsonBytes) > 10000 { ... }
		// Por simplicidade, vamos assumir que metadados pequenos são ok.
	}

	// Timestamp será definido pelo repositório/DB se não preenchido aqui
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}

	_, err := s.repo.Create(entry)
	if err != nil {
		// Não logar o erro aqui de novo, o repositório já logou.
		// Apenas envolva e retorne se necessário.
		return appErrors.WrapErrorf(err, "falha ao persistir log de auditoria")
	}
	return nil
}

// GetAuditLogs busca logs de auditoria com base nos filtros fornecidos.
func (s *auditLogServiceImpl) GetAuditLogs(
	startDate, endDate *time.Time,
	severity, user, action *string,
	limit, offset int,
) (logs []models.AuditLogEntry, totalCount int64, err error) {

	// Validação básica dos parâmetros de entrada
	if limit <= 0 {
		limit = 100 // Default
	}
	if limit > 1000 { // Max limit
		limit = 1000
	}
	if offset < 0 {
		offset = 0
	}

	// Normalizar datas para UTC se tiverem fuso horário diferente ou nenhum
	var startUTC, endUTC *time.Time
	if startDate != nil {
		val := startDate.In(time.UTC)
		startUTC = &val
	}
	if endDate != nil {
		// Para incluir o dia final inteiro, ajustar para o final do dia
		val := time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 23, 59, 59, 999999999, time.UTC)
		endUTC = &val
	}

	logs, totalCount, err = s.repo.GetFiltered(startUTC, endUTC, severity, user, action, limit, offset)
	if err != nil {
		// Erro já logado pelo repositório
		return nil, 0, appErrors.WrapErrorf(err, "falha ao buscar logs de auditoria do repositório")
	}
	return logs, totalCount, nil
}
