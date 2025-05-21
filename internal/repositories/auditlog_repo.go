package repositories

import (
	"time"

	"gorm.io/gorm"

	appErrors "github.com/seu_usuario/riograndense_gio/internal/core/errors" // Alias para evitar conflito
	appLogger "github.com/seu_usuario/riograndense_gio/internal/core/logger"
	"github.com/seu_usuario/riograndense_gio/internal/data/models"
)

// AuditLogRepository define a interface para operações no repositório de logs de auditoria.
type AuditLogRepository interface {
	Create(entry models.AuditLogEntry) (*models.AuditLogEntry, error)
	GetFiltered(
		startDate, endDate *time.Time,
		severity, user, action *string,
		limit, offset int,
	) ([]models.AuditLogEntry, int64, error) // Retorna entradas e contagem total para paginação
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
	// O timestamp geralmente é definido por default:now() no modelo/DB,
	// mas podemos garantir que seja UTC se definido aqui.
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}

	result := r.db.Create(&entry)
	if result.Error != nil {
		appLogger.Errorf("Erro ao criar entrada de log de auditoria (Ação: %s, Usuário: %s): %v", entry.Action, entry.Username, result.Error)
		return nil, appErrors.WrapErrorf(result.Error, "falha ao criar entrada de log no banco (GORM)")
	}
	// appLogger.Debugf("Entrada de log de auditoria criada com ID: %d", entry.ID)
	return &entry, nil
}

// GetFiltered busca logs de auditoria com base nos filtros fornecidos, com paginação.
// Retorna as entradas de log, a contagem total de registros que correspondem aos filtros (para paginação), e um erro.
func (r *gormAuditLogRepository) GetFiltered(
	startDate, endDate *time.Time,
	severity, user, action *string,
	limit, offset int,
) ([]models.AuditLogEntry, int64, error) {
	var entries []models.AuditLogEntry
	var totalCount int64

	// Inicia a query base
	query := r.db.Model(&models.AuditLogEntry{})

	// Aplica filtros
	if startDate != nil {
		query = query.Where("timestamp >= ?", *startDate)
	}
	if endDate != nil {
		// Para incluir o dia final inteiro, podemos ajustar o endDate para o final do dia
		// endOfDay := time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 23, 59, 59, 999999999, endDate.Location())
		// query = query.Where("timestamp <= ?", endOfDay)
		query = query.Where("timestamp <= ?", *endDate)
	}
	if severity != nil && *severity != "" {
		query = query.Where("UPPER(severity) = UPPER(?)", *severity)
	}
	if user != nil && *user != "" {
		// Usar ILIKE para busca case-insensitive se o DB suportar (PostgreSQL)
		// Para SQLite, pode ser necessário LOWER() em ambos os lados.
		// GORM lida com isso razoavelmente bem com Where e placeholders.
		// query = query.Where("LOWER(username) LIKE LOWER(?)", "%"+*user+"%") // Busca parcial
		query = query.Where("LOWER(username) = LOWER(?)", *user) // Busca exata (case-insensitive)
	}
	if action != nil && *action != "" {
		query = query.Where("LOWER(action) = LOWER(?)", *action)
	}

	// 1. Obter a contagem total de registros que correspondem aos filtros (ANTES de limit/offset)
	if err := query.Count(&totalCount).Error; err != nil {
		appLogger.Errorf("Erro ao contar logs de auditoria filtrados: %v", err)
		return nil, 0, appErrors.WrapErrorf(err, "falha ao contar logs de auditoria (GORM)")
	}

	// Se não houver registros, não há necessidade de buscar
	if totalCount == 0 {
		return []models.AuditLogEntry{}, 0, nil
	}

	// 2. Aplicar ordenação, limite e offset para buscar os dados paginados
	query = query.Order("timestamp DESC") // Mais recentes primeiro

	if limit <= 0 {
		limit = 100 // Default limit
	}
	query = query.Limit(limit)

	if offset < 0 {
		offset = 0 // Offset não pode ser negativo
	}
	query = query.Offset(offset)

	// Executar a query para obter as entradas
	if err := query.Find(&entries).Error; err != nil {
		appLogger.Errorf("Erro ao buscar logs de auditoria filtrados: %v", err)
		return nil, 0, appErrors.WrapErrorf(err, "falha ao buscar logs de auditoria (GORM)")
	}

	// appLogger.Debugf("Retornando %d logs de auditoria (total correspondente: %d)", len(entries), totalCount)
	return entries, totalCount, nil
}
