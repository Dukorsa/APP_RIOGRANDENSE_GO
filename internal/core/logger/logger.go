package logger // Nome do pacote 'logger' para evitar conflito com var 'logger'

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core" // Para Config
	"github.com/sirupsen/logrus"                           // TODO: go get github.com/sirupsen/logrus
	"gopkg.in/natefinch/lumberjack.v2"                     // TODO: go get gopkg.in/natefinch/lumberjack.v2
)

var (
	log *logrus.Logger // Variável global para o logger
)

// Init inicializa o logger global da aplicação.
// Deve ser chamado uma vez no início.
func SetupLogger(cfg *core.Config) error {
	log = logrus.New()

	// Nível de Log
	level, err := logrus.ParseLevel(cfg.LogLevel)
	if err != nil {
		level = logrus.InfoLevel
		fmt.Fprintf(os.Stderr, "Nível de log inválido '%s', usando INFO: %v\n", cfg.LogLevel, err)
	}
	log.SetLevel(level)

	// Formato JSON
	log.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: "2006-01-02T15:04:05.000Z07:00", // ISO8601 com milissegundos
		// TODO: Adicionar campos customizados como em JSONFormatter do Python se necessário
	})

	// Saída para arquivo com rotação
	logFilePath := filepath.Join(cfg.LogDir, strings.ToLower(strings.ReplaceAll(cfg.AppName, " ", "_"))+".log")

	// Garante que o diretório de log exista (config.go já deveria ter feito isso)
	logDirAbs, _ := filepath.Abs(cfg.LogDir)
	if err := os.MkdirAll(logDirAbs, os.ModePerm); err != nil {
		fmt.Fprintf(os.Stderr, "Falha ao criar diretório de log '%s': %v. Logs de arquivo podem não funcionar.\n", logDirAbs, err)
	}

	fileLogger := &lumberjack.Logger{
		Filename:   logFilePath,
		MaxSize:    cfg.LogMaxBytes / (1024 * 1024), // Em megabytes
		MaxBackups: cfg.LogBackupCount,
		MaxAge:     28, // dias
		Compress:   true,
	}

	var writers []io.Writer
	writers = append(writers, fileLogger)

	if cfg.LogToConsole {
		writers = append(writers, os.Stderr) // Log para stderr
	}

	multiWriter := io.MultiWriter(writers...)
	log.SetOutput(multiWriter)

	log.Infof("Logger configurado. Nível: %s. Arquivo: %s", level.String(), logFilePath)
	return nil
}

// Funções de logging exportadas (Debug, Info, Warn, Error, Fatal)
func Debug(args ...interface{}) {
	if log == nil {
		fmt.Println("Logger não inicializado:", args)
		return
	}
	log.Debug(args...)
}
func Debugf(format string, args ...interface{}) {
	if log == nil {
		fmt.Printf("Logger não inicializado: "+format+"\n", args...)
		return
	}
	log.Debugf(format, args...)
}
func Info(args ...interface{}) {
	if log == nil {
		fmt.Println("Logger não inicializado:", args)
		return
	}
	log.Info(args...)
}
func Infof(format string, args ...interface{}) {
	if log == nil {
		fmt.Printf("Logger não inicializado: "+format+"\n", args...)
		return
	}
	log.Infof(format, args...)
}
func Warn(args ...interface{}) {
	if log == nil {
		fmt.Println("Logger não inicializado:", args)
		return
	}
	log.Warn(args...)
}
func Warnf(format string, args ...interface{}) {
	if log == nil {
		fmt.Printf("Logger não inicializado: "+format+"\n", args...)
		return
	}
	log.Warnf(format, args...)
}
func Error(args ...interface{}) {
	if log == nil {
		fmt.Println("Logger não inicializado:", args)
		return
	}
	log.Error(args...)
}
func Errorf(format string, args ...interface{}) {
	if log == nil {
		fmt.Printf("Logger não inicializado: "+format+"\n", args...)
		return
	}
	log.Errorf(format, args...)
}
func Fatal(args ...interface{}) {
	if log == nil {
		fmt.Println("Logger não inicializado:", args)
		os.Exit(1)
		return
	}
	log.Fatal(args...)
}
func Fatalf(format string, args ...interface{}) {
	if log == nil {
		fmt.Printf("Logger não inicializado: "+format+"\n", args...)
		os.Exit(1)
		return
	}
	log.Fatalf(format, args...)
}

// Adicionar WithFields se necessário para log estruturado com contexto
func WithFields(fields logrus.Fields) *logrus.Entry {
	if log == nil {
		fmt.Println("Logger não inicializado, retornando entry dummy")
		dummyLogger := logrus.New()
		dummyLogger.SetOutput(io.Discard) // Não escreve em lugar nenhum
		return dummyLogger.WithFields(fields)
	}
	return log.WithFields(fields)
}
