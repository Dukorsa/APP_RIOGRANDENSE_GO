package core

import (
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/joho/godotenv" // TODO: go get github.com/joho/godotenv
)

// Config struct para armazenar todas as configurações da aplicação
type Config struct {
	AppName    string
	AppVersion string
	AppDebug   bool
	SecretKey  string

	// Database
	DBEngine   string
	DBName     string
	DBHost     string
	DBPort     int
	DBUser     string
	DBPassword string

	// Logging
	LogDir         string
	LogLevel       string
	LogMaxBytes    int
	LogBackupCount int
	LogToConsole   bool

	// Auth & Session
	MaxLoginAttempts       int
	AccountLockoutTime     time.Duration // Em segundos no .env, converter para time.Duration
	SessionTimeout         time.Duration // Em segundos no .env
	SessionCleanupInterval time.Duration // Em segundos no .env
	PasswordResetTimeout   time.Duration // Em segundos no .env
	SessionsJSONFile       string
	SessionCleanupEnabled  bool

	// Export
	ExportDir string

	// Email
	EmailSMTPServer string
	EmailPort       int
	EmailUser       string
	EmailPassword   string
	EmailUseTLS     bool
	SupportEmail    string
}

// LoadConfig carrega as configurações do arquivo .env
func LoadConfig(envPath string) (*Config, error) {
	// Tenta encontrar o .env subindo na árvore de diretórios se não for path absoluto
	if !filepath.IsAbs(envPath) {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		// Procura o .env a partir do diretório atual até a raiz do projeto
		// (isso é útil se o binário for executado de um subdiretório)
		for i := 0; i < 5; i++ { // Limita a busca para evitar loops infinitos
			tryPath := filepath.Join(cwd, envPath)
			if _, err := os.Stat(tryPath); err == nil {
				envPath = tryPath
				break
			}
			parent := filepath.Dir(cwd)
			if parent == cwd { // Chegou na raiz
				break
			}
			cwd = parent
		}
	}

	log.Printf("Tentando carregar .env de: %s", envPath)
	if err := godotenv.Load(envPath); err != nil {
		log.Printf("Aviso: Erro ao carregar arquivo .env de '%s': %v. Usando valores padrão ou variáveis de ambiente existentes.", envPath, err)
		// Não retorna erro fatal, pode ser que as vars de ambiente já existam
	}

	cfg := &Config{}

	cfg.AppName = getEnv("APP_NAME", "Riograndense App GO")
	cfg.AppVersion = getEnv("APP_VERSION", "1.0.0-go")
	cfg.AppDebug = getEnvAsBool("APP_DEBUG", false)
	cfg.SecretKey = getEnv("SECRET_KEY", "a_very_secure_random_default_key_for_go_app_32_chars") // Mude isso!

	cfg.DBEngine = getEnv("APP_DB_ENGINE", "sqlite") // sqlite como fallback
	cfg.DBName = getEnv("APP_DB_NAME", "riograndense_go.db")
	cfg.DBHost = getEnv("APP_DB_HOST", "localhost")
	cfg.DBPort = getEnvAsInt("APP_DB_PORT", 5432)
	cfg.DBUser = getEnv("APP_DB_USER", "user")
	cfg.DBPassword = getEnv("APP_DB_PASSWORD", "password")

	cfg.LogDir = getEnv("APP_LOG_DIR", "./logs_go")
	cfg.LogLevel = getEnv("APP_LOG_LEVEL", "INFO")
	cfg.LogMaxBytes = getEnvAsInt("APP_LOG_MAX_BYTES", 5*1024*1024)
	cfg.LogBackupCount = getEnvAsInt("APP_LOG_BACKUP_COUNT", 7)
	cfg.LogToConsole = getEnvAsBool("APP_LOG_TO_CONSOLE", true)

	cfg.MaxLoginAttempts = getEnvAsInt("APP_MAX_LOGIN_ATTEMPTS", 5)
	cfg.AccountLockoutTime = getEnvAsDuration("APP_ACCOUNT_LOCKOUT_TIME", 1800)        // 30 min
	cfg.SessionTimeout = getEnvAsDuration("APP_SESSION_TIMEOUT", 3600)                 // 1 hora
	cfg.SessionCleanupInterval = getEnvAsDuration("APP_SESSION_CLEANUP_INTERVAL", 600) // 10 min
	cfg.PasswordResetTimeout = getEnvAsDuration("APP_PASSWORD_RESET_TIMEOUT", 900)     // 15 min
	cfg.SessionsJSONFile = getEnv("APP_SESSIONS_JSON_FILE", "sessions_go.json")
	cfg.SessionCleanupEnabled = getEnvAsBool("APP_SESSION_CLEANUP_ENABLED", true)

	cfg.ExportDir = getEnv("APP_EXPORT_DIR", "./exports_go")

	cfg.EmailSMTPServer = getEnv("APP_EMAIL_SMTP_SERVER", "")
	cfg.EmailPort = getEnvAsInt("APP_EMAIL_PORT", 587)
	cfg.EmailUser = getEnv("APP_EMAIL_USER", "")
	cfg.EmailPassword = getEnv("APP_EMAIL_PASSWORD", "")
	cfg.EmailUseTLS = getEnvAsBool("APP_EMAIL_USE_TLS", true)
	cfg.SupportEmail = getEnv("APP_SUPPORT_EMAIL", "support@example.com")

	// TODO: Validação das configurações (ex: SecretKey não pode ser o padrão em produção)
	// if !cfg.AppDebug && cfg.SecretKey == "a_very_secure_random_default_key_for_go_app_32_chars" {
	// 	return nil, fmt.Errorf("SECRET_KEY não pode ser o valor padrão em ambiente de produção")
	// }

	// Garantir que diretórios essenciais existam
	dirsToCreate := []string{cfg.LogDir, cfg.ExportDir}
	sessionsDir := filepath.Dir(cfg.SessionsJSONFile)
	if sessionsDir != "." && sessionsDir != "/" { // Adiciona dir do sessions.json se não for o diretório atual ou raiz
		dirsToCreate = append(dirsToCreate, sessionsDir)
	}
	if cfg.DBEngine == "sqlite" {
		dbDir := filepath.Dir(cfg.DBName)
		if dbDir != "." && dbDir != "/" { // Adiciona dir do sqlite se não for o diretório atual ou raiz
			dirsToCreate = append(dirsToCreate, dbDir)
		}
	}

	for _, dirPath := range dirsToCreate {
		absPath, err := filepath.Abs(dirPath)
		if err != nil {
			log.Printf("Aviso: Não foi possível resolver path absoluto para '%s': %v", dirPath, err)
			continue // Pula este diretório, mas continua com os outros
		}
		if err := os.MkdirAll(absPath, os.ModePerm); err != nil {
			log.Printf("Aviso: Não foi possível criar diretório '%s': %v", absPath, err)
			// Pode ser um erro fatal dependendo da importância do diretório
		} else {
			log.Printf("Diretório garantido/criado: %s", absPath)
		}
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

func getEnvAsInt(key string, fallback int) int {
	valueStr := getEnv(key, "")
	if value, err := strconv.Atoi(valueStr); err == nil {
		return value
	}
	return fallback
}

func getEnvAsBool(key string, fallback bool) bool {
	valueStr := getEnv(key, "")
	if value, err := strconv.ParseBool(valueStr); err == nil {
		return value
	}
	return fallback
}

func getEnvAsDuration(key string, fallbackSeconds int) time.Duration {
	valueStr := getEnv(key, "")
	if value, err := strconv.Atoi(valueStr); err == nil {
		return time.Duration(value) * time.Second
	}
	return time.Duration(fallbackSeconds) * time.Second
}
