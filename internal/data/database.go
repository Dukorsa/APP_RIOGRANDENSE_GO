package data

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger" // Logger do GORM

	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/models"
	"github.com/sirupsen/logrus"
)

var dbInstance *gorm.DB // Instância global do GORM DB (ou *sql.DB se não usar GORM)

// InitializeDB configura e estabelece a conexão com o banco de dados
// e executa migrações automáticas.
func InitializeDB(cfg *core.Config) (*gorm.DB, error) {
	var dialector gorm.Dialector
	var err error

	appLogger.Infof("Inicializando conexão com banco de dados: %s", cfg.DBEngine)

	// Configuração do logger do GORM
	gormLogLevel := gormlogger.Silent
	if cfg.AppDebug {
		gormLogLevel = gormlogger.Info // Loga todas as queries SQL em modo debug
	}
	newGormLogger := gormlogger.New(
		appLogger.WithFields(logrus.Fields{"component": "gorm"}), // Passa o logger do Logrus para o GORM
		gormlogger.Config{
			SlowThreshold:             200 * time.Millisecond,
			LogLevel:                  gormLogLevel,
			IgnoreRecordNotFoundError: true,  // Não logar ErrRecordNotFound como erro
			Colorful:                  false, // Pode habilitar se o terminal suportar
		},
	)

	gormConfig := &gorm.Config{
		Logger: newGormLogger,
		NamingStrategy: gorm.NamingStrategy{
			// TablePrefix: "app_", // Exemplo de prefixo de tabela
			SingularTable: false, // Usa nomes de tabela no plural (ex: users)
		},
		// NowFunc: func() time.Time { // Se precisar de um NowFunc customizado
		// 	return time.Now().UTC()
		// },
	}

	switch cfg.DBEngine {
	case "postgresql":
		dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d sslmode=disable TimeZone=UTC",
			cfg.DBHost, cfg.DBUser, cfg.DBPassword, cfg.DBName, cfg.DBPort)
		// Nota: Para produção, sslmode=require ou verify-full é recomendado.
		// TimeZone=UTC é uma boa prática para consistência.
		dialector = postgres.Open(dsn)
		appLogger.Infof("Conectando ao PostgreSQL: host=%s dbname=%s user=%s port=%d", cfg.DBHost, cfg.DBName, cfg.DBUser, cfg.DBPort)
	case "sqlite":
		// GORM criará o arquivo se não existir. O diretório já deve ter sido criado por config.go
		dialector = sqlite.Open(cfg.DBName + "?_foreign_keys=on") // Habilita FKs para SQLite
		appLogger.Infof("Usando banco de dados SQLite: %s", cfg.DBName)
	// case "mysql":
	// dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
	// 	cfg.DBUser, cfg.DBPassword, cfg.DBHost, cfg.DBPort, cfg.DBName)
	// dialector = mysql.Open(dsn)
	default:
		return nil, fmt.Errorf("motor de banco de dados não suportado: %s", cfg.DBEngine)
	}

	dbInstance, err = gorm.Open(dialector, gormConfig)
	if err != nil {
		appLogger.Errorf("Falha ao conectar ao banco de dados %s: %v", cfg.DBEngine, err)
		return nil, fmt.Errorf("falha ao abrir conexão com %s: %w", cfg.DBEngine, err)
	}

	// Configurações do Pool de Conexão (opcional, mas recomendado para produção)
	sqlDB, err := dbInstance.DB()
	if err != nil {
		appLogger.Errorf("Falha ao obter instância *sql.DB do GORM: %v", err)
		return nil, fmt.Errorf("falha ao configurar pool de conexões: %w", err)
	}
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)
	sqlDB.SetConnMaxIdleTime(10 * time.Minute)

	appLogger.Info("Conexão com banco de dados estabelecida.")

	// Executar Migrações Automáticas
	// Isso criará/alterará tabelas para corresponder às suas structs de modelo.
	// Para produção, ferramentas de migração dedicadas (como GORM Migrate ou Alembic/Flyway equivalentes) são mais robustas.
	appLogger.Info("Executando migrações automáticas do GORM...")
	err = dbInstance.AutoMigrate(
		&models.DBUser{},
		&models.DBRole{},
		&models.DBNetwork{},
		&models.DBCNPJ{},
		&models.AuditLogEntry{}, // Era DBLogEntry
		&models.DBImportMetadata{},
		&models.DBTituloDireito{},
		&models.DBTituloObrigacao{},
		// Adicione &models.DBRolePermission{} e &models.DBUserRole{} aqui
		// se você NÃO estiver usando a mágica many2many do GORM e quiser que GORM crie essas tabelas de junção.
		// Se estiver usando many2many (ex: em DBUser.Roles e DBRole.Users), GORM
		// tentará criar a tabela de junção automaticamente (ex: user_roles).
	)
	if err != nil {
		appLogger.Errorf("Falha durante AutoMigrate: %v", err)
		return nil, fmt.Errorf("falha na migração do esquema do banco de dados: %w", err)
	}
	appLogger.Info("Migrações automáticas do GORM concluídas.")

	return dbInstance, nil
}

// GetDB retorna a instância global do GORM DB.
// Panics se InitializeDB não tiver sido chamado com sucesso.
func GetDB() *gorm.DB {
	if dbInstance == nil {
		// Isso é um erro de programação - InitializeDB não foi chamado ou falhou.
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
		return err
	}
	appLogger.Info("Fechando conexão com o banco de dados...")
	return sqlDB.Close()
}

// CreateDatabaseTables é uma função wrapper para a migração, se você quiser chamá-la separadamente.
// Normalmente, InitializeDB já faz isso.
func CreateDatabaseTables(db *gorm.DB) error {
	if db == nil {
		return errors.New("instância de banco de dados é nil, não é possível criar tabelas")
	}
	appLogger.Info("Forçando execução de migrações (CreateDatabaseTables)...")
	err := db.AutoMigrate(
		&models.DBUser{},
		&models.DBRole{},
		// ... todos os seus modelos ...
		&models.DBTituloObrigacao{},
	)
	if err != nil {
		appLogger.Errorf("Falha durante CreateDatabaseTables (AutoMigrate): %v", err)
		return fmt.Errorf("falha na criação/migração de tabelas: %w", err)
	}
	appLogger.Info("CreateDatabaseTables (AutoMigrate) concluído.")
	return nil
}

// Context Manager para Sessão/Transação (se não quiser que cada repo gerencie)
// Esta é uma abordagem. Outra é cada repositório pegar o *gorm.DB e iniciar suas próprias transações.
// Ou o serviço orquestra a transação.

type DBSessionFunc func(tx *gorm.DB) error

// WithTransaction executa uma função dentro de uma transação GORM.
// Faz commit se a função não retornar erro, rollback caso contrário.
func WithTransaction(db *gorm.DB, fn DBSessionFunc) error {
	tx := db.Begin()
	if tx.Error != nil {
		return fmt.Errorf("falha ao iniciar transação: %w", tx.Error)
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			// Re-panic para que o erro original não seja perdido, ou logue e retorne um erro
			panic(r) // ou appLogger.Fatalf("Panic durante transação: %v", r)
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("erro ao executar função (%v) E erro no rollback (%w)", err, rbErr)
		}
		return err // Retorna o erro original da função fn
	}

	if err := tx.Commit(); err.Error != nil {
		return fmt.Errorf("falha ao commitar transação: %w", err.Error)
	}
	return nil
}
