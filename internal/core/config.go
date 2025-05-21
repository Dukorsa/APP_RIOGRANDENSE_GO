package core

import (
	"errors"
	"fmt"
	"log" // Usado para logs iniciais antes que o logger da aplicação esteja configurado
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
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
	AccountLockoutTime     time.Duration
	SessionTimeout         time.Duration
	SessionCleanupInterval time.Duration
	PasswordResetTimeout   time.Duration
	PasswordMinLength      int // Adicionado para centralizar a configuração de comprimento mínimo da senha
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

// LoadConfig carrega as configurações do arquivo .env especificado ou encontrado na árvore de diretórios.
func LoadConfig(envPath string) (*Config, error) {
	foundEnvPath, err := findEnvFile(envPath)
	if err != nil {
		log.Printf("Aviso: Arquivo .env em '%s' não encontrado ou inacessível: %v. Tentando carregar variáveis de ambiente globais.", envPath, err)
		// Tenta carregar de variáveis de ambiente globais se o arquivo não for encontrado.
		// godotenv.Load() sem path tenta carregar .env do diretório atual.
		if loadErr := godotenv.Load(); loadErr != nil {
			log.Printf("Aviso: Nenhum arquivo .env carregado e erro ao tentar carregar .env padrão: %v. Usando apenas variáveis de ambiente existentes ou defaults.", loadErr)
		}
	} else {
		log.Printf("Carregando configurações de: %s", foundEnvPath)
		if err := godotenv.Load(foundEnvPath); err != nil {
			// Mesmo que tenhamos encontrado o arquivo, pode haver um erro ao carregá-lo.
			log.Printf("Aviso: Erro ao carregar arquivo .env de '%s': %v. Usando valores padrão ou variáveis de ambiente existentes.", foundEnvPath, err)
		}
	}

	cfg := &Config{}

	cfg.AppName = getEnv("APP_NAME", "Riograndense App GO")
	cfg.AppVersion = getEnv("APP_VERSION", "1.0.0-go")
	cfg.AppDebug = getEnvAsBool("APP_DEBUG", false)
	cfg.SecretKey = getEnv("SECRET_KEY", "default_secret_key_please_change_this_in_production_12345") // Chave padrão fraca

	cfg.DBEngine = getEnv("APP_DB_ENGINE", "sqlite")
	cfg.DBName = getEnv("APP_DB_NAME", "riograndense_go.db")
	cfg.DBHost = getEnv("APP_DB_HOST", "localhost")
	cfg.DBPort = getEnvAsInt("APP_DB_PORT", 5432)
	cfg.DBUser = getEnv("APP_DB_USER", "user")
	cfg.DBPassword = getEnv("APP_DB_PASSWORD", "password")

	cfg.LogDir = getEnv("APP_LOG_DIR", "./app_logs") // Alterado para evitar conflito com `logs` da raiz
	cfg.LogLevel = strings.ToUpper(getEnv("APP_LOG_LEVEL", "INFO"))
	cfg.LogMaxBytes = getEnvAsInt("APP_LOG_MAX_BYTES", 5*1024*1024) // 5MB
	cfg.LogBackupCount = getEnvAsInt("APP_LOG_BACKUP_COUNT", 7)
	cfg.LogToConsole = getEnvAsBool("APP_LOG_TO_CONSOLE", true)

	cfg.MaxLoginAttempts = getEnvAsInt("APP_MAX_LOGIN_ATTEMPTS", 5)
	cfg.AccountLockoutTime = getEnvAsDuration("APP_ACCOUNT_LOCKOUT_TIME", 1800)        // 30 minutos
	cfg.SessionTimeout = getEnvAsDuration("APP_SESSION_TIMEOUT", 3600)                 // 1 hora
	cfg.SessionCleanupInterval = getEnvAsDuration("APP_SESSION_CLEANUP_INTERVAL", 600) // 10 minutos
	cfg.PasswordResetTimeout = getEnvAsDuration("APP_PASSWORD_RESET_TIMEOUT", 900)     // 15 minutos
	cfg.PasswordMinLength = getEnvAsInt("APP_PASSWORD_MIN_LENGTH", 12)                 // Comprimento mínimo da senha
	cfg.SessionsJSONFile = getEnv("APP_SESSIONS_JSON_FILE", "sessions_go.json")
	cfg.SessionCleanupEnabled = getEnvAsBool("APP_SESSION_CLEANUP_ENABLED", true)

	cfg.ExportDir = getEnv("APP_EXPORT_DIR", "./app_exports")

	cfg.EmailSMTPServer = getEnv("APP_EMAIL_SMTP_SERVER", "")
	cfg.EmailPort = getEnvAsInt("APP_EMAIL_PORT", 587) // Porta padrão para STARTTLS
	cfg.EmailUser = getEnv("APP_EMAIL_USER", "")
	cfg.EmailPassword = getEnv("APP_EMAIL_PASSWORD", "")
	cfg.EmailUseTLS = getEnvAsBool("APP_EMAIL_USE_TLS", true) // TLS geralmente é STARTTLS na porta 587. Porta 465 é SSL/TLS direto.
	cfg.SupportEmail = getEnv("APP_SUPPORT_EMAIL", "support@example.com")

	// Validações de Configurações Críticas
	if !cfg.AppDebug && cfg.SecretKey == "default_secret_key_please_change_this_in_production_12345" {
		return nil, errors.New("FATAL: SECRET_KEY não pode ser o valor padrão em ambiente de não depuração (AppDebug=false)")
	}
	if len(cfg.SecretKey) < 32 && !cfg.AppDebug { // Chave deve ter pelo menos 32 bytes para segurança razoável
		log.Printf("AVISO: SECRET_KEY tem menos de 32 caracteres (%d). Recomenda-se uma chave mais longa para produção.", len(cfg.SecretKey))
	}

	// Garantir que diretórios essenciais existam
	// LogDir é crítico
	if err := ensureDir(cfg.LogDir, true); err != nil {
		return nil, fmt.Errorf("falha ao criar diretório de log essencial '%s': %w", cfg.LogDir, err)
	}
	// Diretório do banco de dados SQLite (se usado)
	if cfg.DBEngine == "sqlite" {
		sqliteDir := filepath.Dir(cfg.DBName)
		// Só cria se não for o diretório atual "."
		if sqliteDir != "." && sqliteDir != string(filepath.Separator) {
			if err := ensureDir(sqliteDir, true); err != nil {
				return nil, fmt.Errorf("falha ao criar diretório para banco de dados SQLite '%s': %w", sqliteDir, err)
			}
		}
	}
	// Outros diretórios (avisar em caso de falha, mas não ser fatal para inicialização)
	_ = ensureDir(cfg.ExportDir, false)
	sessionsDir := filepath.Dir(cfg.SessionsJSONFile)
	if sessionsDir != "." && sessionsDir != string(filepath.Separator) {
		_ = ensureDir(sessionsDir, false)
	}

	log.Println("Configurações carregadas e validadas.")
	return cfg, nil
}

// findEnvFile tenta localizar o arquivo .env.
// Primeiro no path fornecido, depois subindo na árvore de diretórios a partir do CWD.
func findEnvFile(envPath string) (string, error) {
	// Se um caminho absoluto ou relativo direto é fornecido e existe.
	if _, err := os.Stat(envPath); err == nil {
		absPath, _ := filepath.Abs(envPath)
		return absPath, nil
	}

	// Tentar encontrar subindo na árvore de diretórios (máximo 5 níveis)
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("não foi possível obter o diretório de trabalho atual: %w", err)
	}

	for i := 0; i < 5; i++ {
		tryPath := filepath.Join(cwd, ".env") // Assume que envPath é apenas ".env" se não encontrado diretamente
		if _, err := os.Stat(tryPath); err == nil {
			return tryPath, nil
		}
		parent := filepath.Dir(cwd)
		if parent == cwd { // Chegou à raiz
			break
		}
		cwd = parent
	}
	return "", fmt.Errorf("arquivo .env não encontrado no caminho '%s' ou nos diretórios pais", envPath)
}

// ensureDir garante que um diretório exista, criando-o se necessário.
// Se 'critical' for true, retorna erro em caso de falha. Caso contrário, apenas loga um aviso.
func ensureDir(dirPath string, critical bool) error {
	absPath, err := filepath.Abs(dirPath)
	if err != nil {
		msg := fmt.Sprintf("Não foi possível resolver o caminho absoluto para '%s': %v", dirPath, err)
		if critical {
			log.Println("ERRO CRÍTICO:", msg)
			return errors.New(msg)
		}
		log.Println("AVISO:", msg)
		return nil // Não crítico, continua
	}

	if err := os.MkdirAll(absPath, os.ModePerm); err != nil {
		msg := fmt.Sprintf("Não foi possível criar o diretório '%s': %v", absPath, err)
		if critical {
			log.Println("ERRO CRÍTICO:", msg)
			return errors.New(msg)
		}
		log.Println("AVISO:", msg)
	} else {
		log.Printf("Diretório garantido/criado: %s", absPath)
	}
	return nil
}

// getEnv recupera o valor de uma variável de ambiente ou retorna um fallback.
func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

// getEnvAsInt recupera uma variável de ambiente como int ou retorna um fallback.
func getEnvAsInt(key string, fallback int) int {
	valueStr := getEnv(key, "")
	if value, err := strconv.Atoi(valueStr); err == nil {
		return value
	}
	return fallback
}

// getEnvAsBool recupera uma variável de ambiente como bool ou retorna um fallback.
func getEnvAsBool(key string, fallback bool) bool {
	valueStr := getEnv(key, "")
	if value, err := strconv.ParseBool(valueStr); err == nil {
		return value
	}
	return fallback
}

// getEnvAsDuration recupera uma variável de ambiente como time.Duration em segundos, ou retorna um fallback.
func getEnvAsDuration(key string, fallbackSeconds int) time.Duration {
	valueStr := getEnv(key, "")
	if value, err := strconv.Atoi(valueStr); err == nil {
		return time.Duration(value) * time.Second
	}
	return time.Duration(fallbackSeconds) * time.Second
}
