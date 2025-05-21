package core

import (
	"errors"
	"fmt"
	"strings"
)

// Erros sentinela pré-definidos para tipos comuns de falha na aplicação.
// Estes podem ser verificados usando errors.Is(err, ErrNotFound).
var (
	// --- Erros Gerais ---
	ErrInternal        = errors.New("erro interno da aplicação")
	ErrConfiguration   = errors.New("erro de configuração da aplicação")
	ErrResourceLoading = errors.New("falha ao carregar recurso essencial")

	// --- Erros de Autenticação e Sessão ---
	ErrUnauthorized       = errors.New("não autenticado") // Geralmente para falta de autenticação (401)
	ErrAuthentication     = errors.New("falha na autenticação")
	ErrInvalidCredentials = errors.New("credenciais inválidas (usuário ou senha)")
	ErrAccountLocked      = errors.New("conta temporariamente bloqueada")
	ErrTokenExpired       = errors.New("token expirado ou inválido")
	ErrSessionExpired     = errors.New("sessão expirada")
	ErrInvalidSession     = errors.New("sessão inválida ou não encontrada")
	ErrSessionLimit       = errors.New("limite máximo de sessões ativas atingido") // Menos comum em desktop

	// --- Erros de Autorização e Permissões ---
	ErrPermissionDenied = errors.New("permissão negada") // Falta de autorização para uma ação (403)
	ErrRoleNotFound     = errors.New("perfil (role) não encontrado")
	ErrPermissionConfig = errors.New("erro na configuração interna de permissões")

	// --- Erros de Banco de Dados / Repositório ---
	ErrDatabase  = errors.New("erro na operação com o banco de dados")
	ErrNotFound  = errors.New("registro não encontrado")
	ErrConflict  = errors.New("conflito de dados (ex: registro duplicado, violação de unicidade)")
	ErrIntegrity = errors.New("violação de integridade de dados (ex: constraint de chave estrangeira)")

	// --- Erros de Validação e Entrada ---
	ErrValidation   = errors.New("erro de validação nos dados fornecidos")     // Erro genérico de validação de regras de negócio
	ErrInvalidInput = errors.New("entrada de dados inválida ou mal formatada") // Erro de formato/tipo de dado

	// --- Erros Específicos da Aplicação ---
	ErrExport     = errors.New("falha ao exportar dados")
	ErrDataImport = errors.New("falha ao importar dados")
	ErrEmail      = errors.New("falha no serviço de envio de e-mail")

	// --- Erros Críticos / Segurança ---
	ErrSecurityViolation = errors.New("tentativa de violação de segurança detectada")
	ErrCriticalSystem    = errors.New("erro crítico irrecuperável no sistema")
)

// ValidationError é um tipo de erro que contém detalhes sobre os campos que falharam na validação.
type ValidationError struct {
	// Message é uma mensagem geral sobre a falha de validação.
	Message string
	// Fields mapeia nomes de campos para suas respectivas mensagens de erro.
	Fields map[string]string
	// Underlying é o erro original que pode ter causado a falha de validação (opcional).
	Underlying error
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
	if ve.Message != "" {
		sb.WriteString(ve.Message)
	} else {
		sb.WriteString("Erro de validação") // Mensagem padrão se não houver uma específica
	}

	if len(ve.Fields) > 0 {
		sb.WriteString(" (Detalhes: ")
		fieldErrors := []string{}
		for field, desc := range ve.Fields {
			fieldErrors = append(fieldErrors, fmt.Sprintf("%s: %s", field, desc))
		}
		sb.WriteString(strings.Join(fieldErrors, ", "))
		sb.WriteString(")")
	}
	if ve.Underlying != nil {
		sb.WriteString(fmt.Sprintf(" | Erro original: %v", ve.Underlying))
	}
	return sb.String()
}

// Unwrap retorna o erro encapsulado, permitindo o uso de errors.Is e errors.As com o erro original.
func (ve *ValidationError) Unwrap() error {
	return ve.Underlying
}

// Is permite que `errors.Is(err, ErrValidation)` funcione corretamente,
// mesmo que `err` seja um `*ValidationError` que não tenha ErrValidation como `Underlying`.
func (ve *ValidationError) Is(target error) bool {
	return target == ErrValidation
}

// DatabaseErrorDetail é um tipo de erro para carregar mais informações sobre um erro de banco de dados.
type DatabaseErrorDetail struct {
	// Operation descreve a operação que estava sendo realizada (ex: "criando usuário").
	Operation string
	// Query é a query SQL (opcional e deve ser usado com cautela devido a dados sensíveis).
	Query string
	// Err é o erro original retornado pelo driver do banco de dados ou ORM.
	Err error
}

// NewDatabaseErrorDetail cria um novo DatabaseErrorDetail.
func NewDatabaseErrorDetail(operation string, query string, originalErr error) *DatabaseErrorDetail {
	if originalErr == nil {
		originalErr = ErrDatabase // Garante que haja um erro base se nenhum for fornecido
	}
	return &DatabaseErrorDetail{
		Operation: operation,
		Query:     query,
		Err:       originalErr,
	}
}

// Error implementa a interface error.
func (de *DatabaseErrorDetail) Error() string {
	msg := fmt.Sprintf("erro de banco de dados durante %s", de.Operation)
	if de.Query != "" {
		maxQueryLen := 100
		displayQuery := de.Query
		if len(displayQuery) > maxQueryLen {
			displayQuery = displayQuery[:maxQueryLen] + "..."
		}
		msg += fmt.Sprintf(" (Query: %s)", displayQuery)
	}
	// Adiciona a mensagem do erro original envolvido.
	msg += fmt.Sprintf(": %v", de.Err)
	return msg
}

// Unwrap retorna o erro original do banco de dados, permitindo que `errors.As` extraia o erro específico do driver.
func (de *DatabaseErrorDetail) Unwrap() error {
	return de.Err
}

// Is permite que `errors.Is(returnedError, ErrDatabase)` funcione corretamente,
// mesmo que `returnedError` seja um `*DatabaseErrorDetail` que envolveu um erro diferente.
func (de *DatabaseErrorDetail) Is(target error) bool {
	// Um DatabaseErrorDetail é sempre considerado um ErrDatabase.
	if target == ErrDatabase {
		return true
	}
	// Também permite verificar se o erro original (de.Err) corresponde ao target.
	return errors.Is(de.Err, target)
}

// --- Funções Helper ---

// WrapErrorf cria um novo erro que envolve um erro existente com uma mensagem formatada,
// preservando o erro original para verificação com `errors.Is` e `errors.As`.
func WrapErrorf(originalErr error, format string, args ...interface{}) error {
	if originalErr == nil {
		return fmt.Errorf(format, args...) // Se não há erro original, apenas formata a mensagem.
	}
	// O formato ": %w" no final é crucial para que errors.Unwrap funcione.
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), originalErr)
}

// NewAppError cria um erro simples com uma mensagem.
// Útil para erros de lógica de negócios que não se encaixam nos sentinels padrão.
func NewAppError(message string) error {
	return errors.New(message)
}
