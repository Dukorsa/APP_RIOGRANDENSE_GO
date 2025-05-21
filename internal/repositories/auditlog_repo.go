package repositories

import (
	"strings"
	"time"

	"gorm.io/gorm"

	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/errors"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/models"
)

// AuditLogRepository define a interface para operações no repositório de logs de auditoria.
type AuditLogRepository interface {
	// Create insere uma nova entrada de log de auditoria.
	Create(entry models.AuditLogEntry) (*models.AuditLogEntry, error)

	// GetFiltered busca logs de auditoria com base nos filtros fornecidos, com paginação.
	// Retorna as entradas de log, a contagem total de registros que correspondem aos filtros, e um erro.
	GetFiltered(
		startDate, endDate *time.Time,
		severity, user, action *string,
		limit, offset int,
	) (logs []models.AuditLogEntry, totalCount int64, err error)
}

// gormAuditLogRepository é a implementação GORM de AuditLogRepository.
type gormAuditLogRepository struct {
	db *gorm.DB
}

// NewGormAuditLogRepository cria uma nova instância de gormAuditLogRepository.
func NewGormAuditLogRepository(db *gorm.DB) AuditLogRepository {
	if db == nil {
		appLogger.Fatalf("gorm.DB não pode ser nil para NewGormAuditLogRepository")
	}
	return &gormAuditLogRepository{db: db}
}

// Create insere uma nova entrada de log de auditoria no banco de dados.
func (r *gormAuditLogRepository) Create(entry models.AuditLogEntry) (*models.AuditLogEntry, error) {
	// Garante que o timestamp seja UTC se não estiver definido.
	// O GORM `default:now()` no modelo já deve cuidar disso se o NowFunc do GORM estiver configurado para UTC.
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}
	// Garante que a severidade seja armazenada em maiúsculas.
	entry.Severity = strings.ToUpper(entry.Severity)

	// Usar `Session(&gorm.Session{SkipHooks: true})` se houver hooks que não devem ser disparados aqui.
	result := r.db.Create(&entry)
	if result.Error != nil {
		// Evitar logar dados sensíveis que possam estar em `entry.Metadata` no erro de alto nível.
		appLogger.Errorf("Erro ao criar entrada de log de auditoria (Ação: %s, Usuário: %s, Severidade: %s): %v",
			entry.Action, entry.Username, entry.Severity, result.Error)
		return nil, appErrors.WrapErrorf(result.Error, "falha ao criar entrada de log no banco (GORM)")
	}

	// O ID da entrada é preenchido automaticamente pelo GORM após a criação.
	// appLogger.Debugf("Entrada de log de auditoria criada com ID: %d", entry.ID)
	return &entry, nil
}

// GetFiltered busca logs de auditoria com base nos filtros fornecidos, com paginação.
func (r *gormAuditLogRepository) GetFiltered(
	startDate, endDate *time.Time,
	severityFilter, userFilter, actionFilter *string, // Renomeado para evitar conflito com variáveis locais
	limit, offset int,
) ([]models.AuditLogEntry, int64, error) {
	var entries []models.AuditLogEntry
	var totalCount int64

	// Inicia a query base no modelo AuditLogEntry.
	query := r.db.Model(&models.AuditLogEntry{})

	// Aplica filtros de data.
	if startDate != nil {
		// Garante que a data de início inclua desde o começo do dia.
		startOfDay := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, startDate.Location())
		query = query.Where("timestamp >= ?", startOfDay)
	}
	if endDate != nil {
		// Garante que a data de fim inclua até o final do dia.
		endOfDay := time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 23, 59, 59, 999999999, endDate.Location())
		query = query.Where("timestamp <= ?", endOfDay)
	}

	// Aplica filtro de severidade (case-insensitive).
	if severityFilter != nil && *severityFilter != "" {
		query = query.Where("UPPER(severity) = UPPER(?)", *severityFilter)
	}

	// Aplica filtro de nome de usuário (case-insensitive, busca exata).
	// Para busca parcial (LIKE), seria: query = query.Where("LOWER(username) LIKE LOWER(?)", "%"+*userFilter+"%")
	if userFilter != nil && *userFilter != "" {
		query = query.Where("LOWER(username) = LOWER(?)", *userFilter)
	}

	// Aplica filtro de ação (case-insensitive, busca exata).
	if actionFilter != nil && *actionFilter != "" {
		query = query.Where("LOWER(action) = LOWER(?)", *actionFilter)
	}

	// 1. Obter a contagem total de registros que correspondem aos filtros (ANTES de limit/offset).
	//    É importante clonar a query aqui para que os filtros de paginação não afetem a contagem.
	countQuery := query // Clonar não é necessário com GORM se Count for chamado antes de Limit/Offset
	if err := countQuery.Count(&totalCount).Error; err != nil {
		appLogger.Errorf("Erro ao contar logs de auditoria filtrados: %v", err)
		return nil, 0, appErrors.WrapErrorf(err, "falha ao contar logs de auditoria (GORM)")
	}

	// Se não houver registros, não há necessidade de buscar os dados.
	if totalCount == 0 {
		return []models.AuditLogEntry{}, 0, nil
	}

	// 2. Aplicar ordenação, limite e offset para buscar os dados paginados.
	// Ordenar pelos mais recentes primeiro.
	query = query.Order("timestamp DESC")

	// Aplicar limite.
	if limit <= 0 {
		limit = 100 // Limite padrão se inválido ou não fornecido.
	} else if limit > 1000 {
		limit = 1000 // Limite máximo para evitar sobrecarga.
	}
	query = query.Limit(limit)

	// Aplicar offset.
	if offset < 0 {
		offset = 0 // Offset não pode ser negativo.
	}
	query = query.Offset(offset)

	// Executar a query para obter as entradas de log.
	if err := query.Find(&entries).Error; err != nil {
		appLogger.Errorf("Erro ao buscar logs de auditoria filtrados: %v", err)
		return nil, 0, appErrors.WrapErrorf(err, "falha ao buscar logs de auditoria (GORM)")
	}

	// appLogger.Debugf("Retornando %d logs de auditoria (total correspondente: %d) para offset %d, limit %d",
	// 	len(entries), totalCount, offset, limit)
	return entries, totalCount, nil
}
