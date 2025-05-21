package data

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger" // Logger do GORM
	"gorm.io/gorm/schema"            // Para NamingStrategy

	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/models"
	"github.com/sirupsen/logrus" // Para usar com o logger do GORM
)

// dbInstance é a instância global da conexão GORM.
// Evitar o uso direto de variáveis globais para o DB é geralmente uma boa prática em
// aplicações maiores (preferir injeção de dependência), mas pode ser aceitável
// em cenários mais simples ou quando gerenciado cuidadosamente.
var dbInstance *gorm.DB

// GormLoggerAdapter adapta o logger da aplicação (Logrus) para a interface do logger do GORM.
type GormLoggerAdapter struct {
	*logrus.Logger
	SlowThreshold time.Duration
	LogLevel      gormlogger.LogLevel
}

// LogMode define o nível de log para o logger do GORM.
func (l *GormLoggerAdapter) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	newLogger := *l
	newLogger.LogLevel = level
	return &newLogger
}

// Info imprime logs no nível info.
func (l *GormLoggerAdapter) Info(ctx gormlogger.Context, msg string, data ...interface{}) {
	if l.LogLevel >= gormlogger.Info {
		l.WithContext(ctx).Infof(msg, data...)
	}
}

// Warn imprime logs no nível warn.
func (l *GormLoggerAdapter) Warn(ctx gormlogger.Context, msg string, data ...interface{}) {
	if l.LogLevel >= gormlogger.Warn {
		l.WithContext(ctx).Warnf(msg, data...)
	}
}

// Error imprime logs no nível error.
func (l *GormLoggerAdapter) Error(ctx gormlogger.Context, msg string, data ...interface{}) {
	if l.LogLevel >= gormlogger.Error {
		l.WithContext(ctx).Errorf(msg, data...)
	}
}

// Trace imprime SQL, tempo decorrido e linhas afetadas.
func (l *GormLoggerAdapter) Trace(ctx gormlogger.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.LogLevel <= gormlogger.Silent {
		return
	}

	elapsed := time.Since(begin)
	sql, rows := fc()
	fields := logrus.Fields{
		"latency_ms": float64(elapsed.Nanoseconds()) / 1e6, // Latência em milissegundos
		"sql":        sql,
		"rows":       rows,
		// "source":     utils.FileWithLineNum(), // Adicionar fonte se tiver helper
	}

	// Logar erros de forma mais proeminente
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) { // Não logar RecordNotFound como erro GORM
		l.WithContext(ctx).WithFields(fields).Errorf("GORM TRACE ERROR: %v", err)
		return
	}

	// Logar queries lentas
	if l.SlowThreshold != 0 && elapsed > l.SlowThreshold {
		l.WithContext(ctx).WithFields(fields).Warnf("GORM SLOW QUERY (%.2fms > %.2fms)", float64(elapsed.Nanoseconds())/1e6, float64(l.SlowThreshold.Nanoseconds())/1e6)
		return
	}

	// Logar todas as queries se o nível for Info
	if l.LogLevel >= gormlogger.Info {
		l.WithContext(ctx).WithFields(fields).Debug("GORM TRACE")
	}
}

// InitializeDB configura e estabelece a conexão com o banco de dados
// e executa migrações automáticas.
func InitializeDB(cfg *core.Config) (*gorm.DB, error) {
	var dialector gorm.Dialector
	var err error

	appLogger.Infof("Inicializando conexão com banco de dados: %s", cfg.DBEngine)

	// Configuração do logger do GORM usando o adaptador
	gormLogLevel := gormlogger.Silent
	if cfg.AppDebug {
		gormLogLevel = gormlogger.Info // Loga todas as queries SQL em modo debug
	}
	// O logger base do appLogger (logrus.Logger) precisa ser acessível ou recriado para o adaptador.
	// Assumindo que appLogger.WithFields retorna um *logrus.Entry, e podemos obter o Logger dele.
	// Se appLogger for um wrapper simples, pode ser necessário acesso direto ao *logrus.Logger.
	// Por simplicidade, vamos assumir que podemos criar um novo logrus.Logger para o GORM,
	// ou que o global appLogger (se for um *logrus.Logger) pode ser usado.
	// O ideal é que appLogger.GetLogger() retorne o *logrus.Logger.

	// Para este exemplo, vamos usar o WithFields para obter um Entry e seu Logger
	// Se appLogger.log for exportado (não é o caso), seria `appLogger.log`
	gormLogrusInstance := appLogger.WithFields(logrus.Fields{"component": "gorm"}).Logger
	if gormLogrusInstance == nil { // Fallback se o logger do app não estiver pronto/acessível
		gormLogrusInstance = logrus.New()
		gormLogrusInstance.SetOutput(os.Stdout) // Ou io.Discard
		gormLogrusInstance.SetLevel(logrus.InfoLevel)
	}

	adaptedGormLogger := &GormLoggerAdapter{
		Logger:        gormLogrusInstance,
		SlowThreshold: 200 * time.Millisecond, // Exemplo de threshold para queries lentas
		LogLevel:      gormLogLevel,
	}

	gormConfig := &gorm.Config{
		Logger: adaptedGormLogger,
		NamingStrategy: schema.NamingStrategy{
			TablePrefix:   "",    // Sem prefixo de tabela global
			SingularTable: false, // Usa nomes de tabela no plural (ex: users)
			// NameReplacer: strings.NewReplacer("CNPJ", "cnpj"), // Exemplo para mapear nomes de campo para coluna
		},
		NowFunc: func() time.Time { // Garante que todas as timestamps do GORM sejam UTC
			return time.Now().UTC()
		},
		// DisableForeignKeyConstraintWhenMigrating: true, // Pode ser útil em cenários de migração complexos
	}

	switch strings.ToLower(cfg.DBEngine) {
	case "postgresql", "postgres":
		dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d sslmode=disable TimeZone=UTC",
			cfg.DBHost, cfg.DBUser, cfg.DBPassword, cfg.DBName, cfg.DBPort)
		dialector = postgres.Open(dsn)
		appLogger.Infof("Conectando ao PostgreSQL: host=%s dbname=%s user=%s port=%d", cfg.DBHost, cfg.DBName, cfg.DBUser, cfg.DBPort)
	case "sqlite":
		// GORM criará o arquivo se não existir. O diretório já deve ter sido criado por config.go
		// Habilitar FKs para SQLite e definir busy_timeout para melhor concorrência (embora GIO seja single-threaded para UI)
		sqliteDSN := fmt.Sprintf("%s?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000", cfg.DBName)
		dialector = sqlite.Open(sqliteDSN)
		appLogger.Infof("Usando banco de dados SQLite: %s", sqliteDSN)
	default:
		return nil, fmt.Errorf("motor de banco de dados não suportado: %s", cfg.DBEngine)
	}

	db, err := gorm.Open(dialector, gormConfig)
	if err != nil {
		appLogger.Errorf("Falha ao conectar ao banco de dados %s: %v", cfg.DBEngine, err)
		return nil, fmt.Errorf("falha ao abrir conexão com %s: %w", cfg.DBEngine, err)
	}

	// Configurações do Pool de Conexão
	sqlDB, err := db.DB()
	if err != nil {
		appLogger.Errorf("Falha ao obter instância *sql.DB do GORM: %v", err)
		_ = CloseDB(db) // Tenta fechar a conexão GORM se a obtenção do sql.DB falhar
		return nil, fmt.Errorf("falha ao configurar pool de conexões: %w", err)
	}
	sqlDB.SetMaxIdleConns(10)                  // Número de conexões inativas no pool.
	sqlDB.SetMaxOpenConns(50)                  // Número máximo de conexões abertas (para desktop apps, pode ser menor).
	sqlDB.SetConnMaxLifetime(time.Hour)        // Tempo máximo que uma conexão pode ser reutilizada.
	sqlDB.SetConnMaxIdleTime(10 * time.Minute) // Tempo máximo que uma conexão pode ficar inativa.

	appLogger.Info("Conexão com banco de dados estabelecida e pool configurado.")

	// Executar Migrações Automáticas
	appLogger.Info("Executando migrações automáticas do GORM...")
	err = db.AutoMigrate(
		&models.DBUser{},
		&models.DBRole{},
		&models.DBUserRole{},       // Tabela de junção User-Role
		&models.DBRolePermission{}, // Tabela de junção Role-Permission
		&models.DBNetwork{},
		&models.DBCNPJ{},
		&models.AuditLogEntry{},
		&models.DBImportMetadata{},
		&models.DBTituloDireito{},
		&models.DBTituloObrigacao{},
	)
	if err != nil {
		appLogger.Errorf("Falha durante AutoMigrate: %v", err)
		_ = CloseDB(db) // Tenta fechar em caso de falha na migração
		return nil, fmt.Errorf("falha na migração do esquema do banco de dados: %w", err)
	}
	appLogger.Info("Migrações automáticas do GORM concluídas.")

	dbInstance = db // Define a instância global se tudo correu bem
	return dbInstance, nil
}

// GetDB retorna a instância global do GORM DB.
// Panics se InitializeDB não tiver sido chamado com sucesso.
func GetDB() *gorm.DB {
	if dbInstance == nil {
		appLogger.Fatalf("FATAL: Instância do banco de dados não inicializada. Chame InitializeDB primeiro.")
	}
	return dbInstance
}

// CloseDB fecha a conexão com o banco de dados.
func CloseDB(db *gorm.DB) error {
	if db == nil {
		appLogger.Warn("Tentativa de fechar conexão DB nula.")
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		appLogger.Errorf("Erro ao obter *sql.DB para fechar: %v", err)
		return fmt.Errorf("erro ao obter sql.DB para fechar: %w", err)
	}
	appLogger.Info("Fechando conexão com o banco de dados...")
	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("erro ao fechar conexão sql.DB: %w", err)
	}
	return nil
}

// WithTransaction executa uma função dentro de uma transação GORM.
// Faz commit se a função não retornar erro, rollback caso contrário.
// `dbParam` pode ser o db global ou uma instância específica.
func WithTransaction(dbParam *gorm.DB, fn func(tx *gorm.DB) error) (err error) {
	if dbParam == nil {
		return errors.New("instância de banco de dados para transação é nil")
	}
	tx := dbParam.Begin()
	if tx.Error != nil {
		return fmt.Errorf("falha ao iniciar transação: %w", tx.Error)
	}

	// Deferir um recover para lidar com panics dentro da transação
	defer func() {
		if r := recover(); r != nil {
			// Rollback em caso de panic
			if rbErr := tx.Rollback(); rbErr != nil {
				err = fmt.Errorf("panic durante transação: %v, E ERRO NO ROLLBACK: %w", r, rbErr)
			} else {
				err = fmt.Errorf("panic durante transação: %v (ROLLBACK EXECUTADO)", r)
			}
			// Re-panic para propagar o panic original se necessário, ou apenas logar e retornar o erro.
			// Se não re-panicarmos, o erro será o que definimos em `err`.
			// panic(r) // Descomente se quiser que o panic original continue
		}
	}()

	if err = fn(tx); err != nil { // Executa a função com a transação
		// Se a função retornar um erro, faz rollback
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("erro ao executar função na transação (%v) E ERRO NO ROLLBACK (%w)", err, rbErr)
		}
		return err // Retorna o erro original da função `fn`
	}

	// Se a função não retornar erro, faz commit
	if err = tx.Commit().Error; err != nil {
		// Se o commit falhar, tenta rollback (embora improvável que funcione se o commit falhou)
		if rbErr := tx.Rollback(); rbErr != nil { // Rollback em caso de falha no commit
			return fmt.Errorf("falha ao commitar transação (%v) E ERRO NO ROLLBACK APÓS FALHA NO COMMIT (%w)", err, rbErr)
		}
		return fmt.Errorf("falha ao commitar transação: %w", err)
	}
	return nil // Sucesso
}
