package errors // Nome do pacote 'errors' para evitar conflito com o pacote padrão 'errors'

import (
	"errors"
	"fmt"
	"strings"
)

// Sentinel errors (erros pré-definidos) para tipos comuns de falha.
// Estes podem ser verificados usando errors.Is(err, ErrNotFound).
var (
	// --- Geral ---
	ErrInternal        = errors.New("erro interno do servidor/aplicação") // Equivalente a um 500 genérico
	ErrConfiguration   = errors.New("erro de configuração da aplicação")
	ErrResourceLoading = errors.New("falha ao carregar recurso essencial")

	// --- Autenticação e Sessão ---
	ErrUnauthorized       = errors.New("não autorizado") // Geralmente para falta de autenticação
	ErrAuthentication     = errors.New("falha na autenticação")
	ErrInvalidCredentials = errors.New("credenciais inválidas (usuário ou senha)")
	ErrAccountLocked      = errors.New("conta temporariamente bloqueada")
	ErrTokenExpired       = errors.New("token expirado ou inválido")
	ErrSessionExpired     = errors.New("sessão expirada")
	ErrInvalidSession     = errors.New("sessão inválida ou não encontrada")
	ErrSessionLimit       = errors.New("limite máximo de sessões ativas atingido")

	// --- Permissões ---
	ErrPermissionDenied = errors.New("permissão negada")
	ErrRoleNotFound     = errors.New("perfil (role) não encontrado")
	ErrPermissionConfig = errors.New("erro na configuração interna de permissões")

	// --- Banco de Dados / Repositório ---
	ErrDatabase  = errors.New("erro na operação com o banco de dados") // Erro genérico de DB
	ErrNotFound  = errors.New("registro não encontrado")
	ErrConflict  = errors.New("conflito de dados (ex: registro duplicado)")
	ErrIntegrity = errors.New("violação de integridade de dados (ex: constraint de chave estrangeira)")

	// --- Validação ---
	// ErrValidation é um erro mais genérico. Para detalhes, use ValidationError struct.
	ErrValidation   = errors.New("erro de validação nos dados fornecidos")
	ErrInvalidInput = errors.New("entrada de dados inválida ou mal formatada")

	// --- Runtime Específico da Aplicação ---
	ErrExport     = errors.New("falha ao exportar dados")
	ErrDataImport = errors.New("falha ao importar dados") // Renomeado de ImportError
	ErrEmail      = errors.New("falha no serviço de envio de e-mail")

	// --- Crítico / Segurança ---
	ErrSecurityViolation = errors.New("tentativa de violação de segurança detectada")
	ErrCriticalSystem    = errors.New("erro crítico irrecuperável no sistema")
)

// ValidationError é um tipo de erro que pode conter detalhes sobre os campos que falharam.
type ValidationError struct {
	Message string
	Fields  map[string]string // Campo -> Mensagem de erro para aquele campo
	// Underlying error error      // Opcional: erro original que causou a falha de validação
}

// NewValidationError cria uma nova instância de ValidationError.
func NewValidationError(message string, fields map[string]string) *ValidationError {
	return &ValidationError{
		Message: message,
		Fields:  fields,
	}
}

// Error implementa a interface error.
func (ve *ValidationError) Error() string {
	var sb strings.Builder
	sb.WriteString(ve.Message)
	if len(ve.Fields) > 0 {
		sb.WriteString(" (Detalhes: ")
		fieldErrors := []string{}
		for field, desc := range ve.Fields {
			fieldErrors = append(fieldErrors, fmt.Sprintf("%s: %s", field, desc))
		}
		sb.WriteString(strings.Join(fieldErrors, ", "))
		sb.WriteString(")")
	}
	return sb.String()
}

// Unwrap pode ser usado com errors.Is e errors.As se você definir um Underlying error.
// func (ve *ValidationError) Unwrap() error { return ve.Underlying }

// DatabaseErrorDetail é um tipo de erro para carregar mais informações sobre um erro de banco de dados.
type DatabaseErrorDetail struct {
	Operation string // Ex: "creating user", "fetching network"
	Query     string // Opcional: a query SQL (cuidado com dados sensíveis)
	Err       error  // O erro original do driver do banco ou ORM
}

// NewDatabaseErrorDetail cria um novo DatabaseErrorDetail.
func NewDatabaseErrorDetail(operation string, query string, originalErr error) *DatabaseErrorDetail {
	return &DatabaseErrorDetail{
		Operation: operation,
		Query:     query,
		Err:       originalErr, // Envolve o erro original
	}
}

// Error implementa a interface error.
func (de *DatabaseErrorDetail) Error() string {
	msg := fmt.Sprintf("erro de banco de dados durante %s", de.Operation)
	if de.Query != "" {
		// Truncar query longa para o log
		maxQueryLen := 100
		displayQuery := de.Query
		if len(displayQuery) > maxQueryLen {
			displayQuery = displayQuery[:maxQueryLen] + "..."
		}
		msg += fmt.Sprintf(" (Query: %s)", displayQuery)
	}
	if de.Err != nil {
		msg += fmt.Sprintf(": %s", de.Err.Error()) // Inclui a mensagem do erro original
	}
	return msg
}

// Unwrap permite que errors.Is(err, ErrDatabase) funcione, e errors.As possa extrair o erro original.
func (de *DatabaseErrorDetail) Unwrap() error {
	// Se quisermos que ele seja "comparável" com o sentinel ErrDatabase:
	// return ErrDatabase // Mas isso perde o erro original para errors.As
	// Para manter a cadeia de erros original e também permitir a verificação com ErrDatabase,
	// o ideal é que as funções que retornam DatabaseErrorDetail envolvam ErrDatabase:
	// return fmt.Errorf("%w: %s", ErrDatabase, de.Err.Error()) // Isso é feito no local da criação.
	// Se o erro original (de.Err) já for um desses sentinels, não precisa envolver ErrDatabase novamente.
	return de.Err
}

// Is permite que `errors.Is(returnedError, ErrDatabase)` funcione corretamente
// mesmo que `returnedError` seja um `*DatabaseErrorDetail` que envolveu um erro diferente.
// Isso é útil se o erro original `de.Err` não for `ErrDatabase` diretamente.
func (de *DatabaseErrorDetail) Is(target error) bool {
	// Se o alvo for o sentinel ErrDatabase, consideramos que corresponde.
	if target == ErrDatabase {
		return true
	}
	// Caso contrário, delegamos para a verificação do erro aninhado.
	return errors.Is(de.Err, target)
}

// --- Helper Functions (Opcional, mas pode ser útil) ---

// WrapErrorf cria um novo erro que envolve um erro existente com uma mensagem formatada.
// Útil para adicionar contexto a erros retornados de outras bibliotecas.
// Exemplo: return WrapErrorf(err, "falha ao processar arquivo %s", filename)
func WrapErrorf(originalErr error, format string, args ...interface{}) error {
	if originalErr == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), originalErr)
}

// NewAppError cria um erro simples com uma mensagem.
// Pode ser usado para erros de lógica de negócios que não se encaixam nos sentinels.
func NewAppError(message string) error {
	return errors.New(message)
}
