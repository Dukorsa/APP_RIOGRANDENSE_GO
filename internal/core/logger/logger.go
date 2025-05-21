package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time" // Adicionado para uso no CustomTextFormatter

	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/config" // Para Config
	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	log *logrus.Logger // Variável global para o logger
)

// CustomTextFormatter formata logs para o console de forma mais legível.
type CustomTextFormatter struct {
	TimestampFormat string
	ForceColors     bool // Adicionado para forçar cores se desejado
}

// Format implementa a interface logrus.Formatter.
func (f *CustomTextFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	var b strings.Builder

	levelText := strings.ToUpper(entry.Level.String())
	if f.ForceColors { // Exemplo de como adicionar cores (requer terminal que suporte ANSI)
		var levelColor int
		switch entry.Level {
		case logrus.DebugLevel, logrus.TraceLevel:
			levelColor = 37 // Cinza claro
		case logrus.InfoLevel:
			levelColor = 32 // Verde
		case logrus.WarnLevel:
			levelColor = 33 // Amarelo
		case logrus.ErrorLevel, logrus.FatalLevel, logrus.PanicLevel:
			levelColor = 31 // Vermelho
		default:
			levelColor = 37
		}
		levelText = fmt.Sprintf("\x1b[%dm%s\x1b[0m", levelColor, levelText) // \x1b[<COR>mTEXTO\x1b[0m
	}

	timestamp := entry.Time.Format(f.TimestampFormat)
	b.WriteString(fmt.Sprintf("[%s] [%s] %s", timestamp, levelText, entry.Message))

	// Adicionar campos (fields) se existirem
	if len(entry.Data) > 0 {
		b.WriteString(" (")
		firstField := true
		for k, v := range entry.Data {
			if !firstField {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%s=%v", k, v)
			firstField = false
		}
		b.WriteString(")")
	}
	b.WriteString("\n")
	return []byte(b.String()), nil
}

// SetupLogger inicializa o logger global da aplicação.
func SetupLogger(cfg *core.Config) error {
	log = logrus.New()

	level, err := logrus.ParseLevel(cfg.LogLevel)
	if err != nil {
		level = logrus.InfoLevel
		// Usar logrus para logar o erro de parse do nível, mesmo que não esteja totalmente configurado
		log.WithError(err).Warnf("Nível de log inválido '%s', usando INFO.", cfg.LogLevel)
	}
	log.SetLevel(level)

	// Garante que o diretório de log exista
	logDirAbs, err := filepath.Abs(cfg.LogDir)
	if err != nil {
		log.WithError(err).Errorf("Falha ao obter caminho absoluto para diretório de log '%s'.", cfg.LogDir)
		return fmt.Errorf("falha ao resolver diretório de log: %w", err)
	}
	if err := os.MkdirAll(logDirAbs, os.ModePerm); err != nil {
		log.WithError(err).Errorf("Falha ao criar diretório de log '%s'. Logs de arquivo podem não funcionar.", logDirAbs)
		// Continuar sem logs de arquivo se a criação do diretório falhar, mas logar para console
	}

	logFileName := strings.ToLower(strings.ReplaceAll(cfg.AppName, " ", "_")) + ".log"
	logFilePath := filepath.Join(logDirAbs, logFileName)

	// Configuração para rotação de arquivos de log
	fileLogger := &lumberjack.Logger{
		Filename:   logFilePath,
		MaxSize:    cfg.LogMaxBytes / (1024 * 1024), // Em megabytes
		MaxBackups: cfg.LogBackupCount,
		MaxAge:     28,    // Dias para manter logs antigos
		Compress:   false, // Desabilitar compressão se causar problemas ou não for desejado
		LocalTime:  true,  // Usar hora local para nomes de arquivos rotacionados
	}

	var writers []io.Writer
	writers = append(writers, fileLogger) // Sempre logar para arquivo

	if cfg.LogToConsole {
		writers = append(writers, os.Stderr)   // Log para stderr no console
		log.SetFormatter(&CustomTextFormatter{ // Formato customizado para console
			TimestampFormat: "2006-01-02 15:04:05.000",
			ForceColors:     true, // Ativar cores para o console
		})
	} else {
		// Se não logar para console, usar JSON para o arquivo para ser mais estruturado
		log.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339Nano, // Formato ISO8601 com nanossegundos
			// FieldMap: logrus.FieldMap{
			// 	logrus.FieldKeyTime:  "@timestamp",
			// 	logrus.FieldKeyLevel: "@level",
			// 	logrus.FieldKeyMsg:   "@message",
			// },
		})
	}

	// Se logar para ambos, o SetFormatter acima (CustomTextFormatter) será usado para console,
	// mas para o arquivo, o lumberjack apenas recebe os bytes brutos.
	// Para ter formatos diferentes para console e arquivo com MultiWriter,
	// é preciso abordagens mais complexas (ex: hooks do logrus ou wrappers).
	// A implementação atual usará o último SetFormatter para ambos se LogToConsole for true.
	// Para simplicidade, se LogToConsole, o formato de texto será usado para ambos.
	// Se for apenas para arquivo, JSON será usado.

	// Se queremos JSON para arquivo e texto para console:
	if cfg.LogToConsole {
		// Logrus não suporta múltiplos formatters diretamente para um MultiWriter.
		// O mais simples é usar o CustomTextFormatter para o console se estiver ativo.
		// Se apenas arquivo, usa-se JSON. Se ambos, o arquivo também receberá o formato de texto.
		// Se JSON no arquivo é CRÍTICO mesmo com console ativo, precisaria de um hook.
		log.SetFormatter(&CustomTextFormatter{
			TimestampFormat: "2006-01-02 15:04:05.000",
			ForceColors:     true,
		})
	} else { // Apenas arquivo
		log.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339Nano,
		})
	}

	multiWriter := io.MultiWriter(writers...)
	log.SetOutput(multiWriter)

	log.Infof("Logger configurado. Nível: %s. Arquivo: %s. Log no console: %t", level.String(), logFilePath, cfg.LogToConsole)
	return nil
}

// Debug logs a message at level Debug on the standard logger.
func Debug(args ...interface{}) {
	if log == nil {
		fmt.Println("Logger não inicializado:", args)
		return
	}
	log.Debug(args...)
}

// Debugf logs a message at level Debug on the standard logger.
func Debugf(format string, args ...interface{}) {
	if log == nil {
		fmt.Printf("Logger não inicializado: "+format+"\n", args...)
		return
	}
	log.Debugf(format, args...)
}

// Info logs a message at level Info on the standard logger.
func Info(args ...interface{}) {
	if log == nil {
		fmt.Println("Logger não inicializado:", args)
		return
	}
	log.Info(args...)
}

// Infof logs a message at level Info on the standard logger.
func Infof(format string, args ...interface{}) {
	if log == nil {
		fmt.Printf("Logger não inicializado: "+format+"\n", args...)
		return
	}
	log.Infof(format, args...)
}

// Warn logs a message at level Warn on the standard logger.
func Warn(args ...interface{}) {
	if log == nil {
		fmt.Println("Logger não inicializado:", args)
		return
	}
	log.Warn(args...)
}

// Warnf logs a message at level Warn on the standard logger.
func Warnf(format string, args ...interface{}) {
	if log == nil {
		fmt.Printf("Logger não inicializado: "+format+"\n", args...)
		return
	}
	log.Warnf(format, args...)
}

// Error logs a message at level Error on the standard logger.
func Error(args ...interface{}) {
	if log == nil {
		fmt.Println("Logger não inicializado:", args)
		return
	}
	log.Error(args...)
}

// Errorf logs a message at level Error on the standard logger.
func Errorf(format string, args ...interface{}) {
	if log == nil {
		fmt.Printf("Logger não inicializado: "+format+"\n", args...)
		return
	}
	log.Errorf(format, args...)
}

// Fatal logs a message at level Fatal on the standard logger then the process will exit with status set to 1.
func Fatal(args ...interface{}) {
	if log == nil {
		fmt.Println("FATAL: Logger não inicializado:", args)
		os.Exit(1)
		return
	}
	log.Fatal(args...)
}

// Fatalf logs a message at level Fatal on the standard logger then the process will exit with status set to 1.
func Fatalf(format string, args ...interface{}) {
	if log == nil {
		fmt.Printf("FATAL: Logger não inicializado: "+format+"\n", args...)
		os.Exit(1)
		return
	}
	log.Fatalf(format, args...)
}

// Panic logs a message at level Panic on the standard logger then panics.
func Panic(args ...interface{}) {
	if log == nil {
		fmt.Println("PANIC: Logger não inicializado:", args)
		panic(fmt.Sprint(args...))
	}
	log.Panic(args...)
}

// Panicf logs a message at level Panic on the standard logger then panics.
func Panicf(format string, args ...interface{}) {
	if log == nil {
		fmt.Printf("PANIC: Logger não inicializado: "+format+"\n", args...)
		panic(fmt.Sprintf(format, args...))
	}
	log.Panicf(format, args...)
}

// WithFields creates an entry from the standard logger and adds multiple
// fields to it. This is useful for structured logging.
func WithFields(fields logrus.Fields) *logrus.Entry {
	if log == nil {
		// Cria um logger dummy para evitar nil pointer exception se chamado antes da inicialização.
		// Isto não é ideal, pois as mensagens não serão logadas.
		// Em uma aplicação robusta, garanta que SetupLogger seja chamado antes de qualquer log.
		fmt.Println("AVISO: Logger global não inicializado ao chamar WithFields. Usando logger dummy.", fields)
		dummyLogger := logrus.New()
		dummyLogger.SetOutput(io.Discard) // Não escreve em lugar nenhum
		return dummyLogger.WithFields(fields)
	}
	return log.WithFields(fields)
}

// WithError creates an entry from the standard logger and adds an error to it, using the value defined in ErrorKey as key.
func WithError(err error) *logrus.Entry {
	if log == nil {
		fmt.Println("AVISO: Logger global não inicializado ao chamar WithError. Usando logger dummy.", err)
		dummyLogger := logrus.New()
		dummyLogger.SetOutput(io.Discard)
		return dummyLogger.WithError(err)
	}
	return log.WithError(err)
}
