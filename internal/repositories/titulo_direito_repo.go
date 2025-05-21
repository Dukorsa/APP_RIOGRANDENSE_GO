package repositories

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal" // TODO: go get github.com/shopspring/decimal
	"gorm.io/gorm"

	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/errors"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/models"
)

// TituloDireitoRepository define a interface para operações no repositório de títulos de direitos.
type TituloDireitoRepository interface {
	// ReplaceAll substitui TODOS os dados na tabela.
	// Recebe uma lista de structs TituloDireitoFromRow, que representam os dados brutos do CSV.
	// Retorna o número de registros efetivamente inseridos e um erro, se houver.
	ReplaceAll(rawData []models.TituloDireitoFromRow) (insertedCount int, skippedCount int, err error)
}

// gormTituloDireitoRepository é a implementação GORM de TituloDireitoRepository.
type gormTituloDireitoRepository struct {
	db *gorm.DB
}

// NewGormTituloDireitoRepository cria uma nova instância de gormTituloDireitoRepository.
func NewGormTituloDireitoRepository(db *gorm.DB) TituloDireitoRepository {
	if db == nil {
		appLogger.Fatalf("gorm.DB não pode ser nil para NewGormTituloDireitoRepository")
	}
	return &gormTituloDireitoRepository{db: db}
}

const (
	varchar50Limit  = 50
	varchar100Limit = 100
	// Definir placeholders como no Python
	defaultPlaceholderString = "PENDENTE"
	defaultPlaceholderCNPJ   = "00000000000000" // 14 zeros
	defaultPlaceholderInt    = 0
)

var defaultPlaceholderDecimal = decimal.Zero // 0.00

// Helper para truncar string
func truncateString(value string, limit int) string {
	if len(value) > limit {
		return value[:limit]
	}
	return value
}

// Helper para parsear string para *time.Time (formato "dd/mm/yyyy")
func parseDate(dateStr string, rowNum int, colName string) *time.Time {
	trimmed := strings.TrimSpace(dateStr)
	if trimmed == "" {
		return nil
	}
	// Tentativas comuns de formato de data em arquivos brasileiros
	formats := []string{"02/01/2006", "2/1/2006", "02-01-2006", "2-1-2006", "2006-01-02"}
	for _, format := range formats {
		t, err := time.Parse(format, trimmed)
		if err == nil {
			// Se for apenas data, a hora será 00:00:00 no local padrão de time.Parse
			// Para consistência, pode-se querer retornar t.UTC() ou similar.
			// GORM com `gorm:"type:date"` geralmente lida bem com isso.
			return &t
		}
	}
	appLogger.Warnf("[Linha %d TDir] Formato de data inválido na coluna '%s': '%s'. Usando NULL.", rowNum, colName, dateStr)
	return nil
}

// Helper para parsear string para string representando decimal (ex: "1234.56")
// Retorna o ponteiro para a string processada, ou nil se a entrada for vazia/inválida.
// Retorna um erro se o parsing falhar mas a string não era vazia.
func parseDecimalString(valStr string, rowNum int, colName string) (*string, error) {
	trimmed := strings.TrimSpace(valStr)
	if trimmed == "" {
		return nil, nil // Vazio é NULL
	}
	// Tenta converter para decimal.Decimal para validar o formato
	// Substitui vírgula por ponto para o padrão decimal.
	normalizedForDecimal := strings.ReplaceAll(trimmed, ",", ".")
	// Remove separadores de milhar se existirem (ex: "1.234.567,89")
	// Esta é uma simplificação; uma regex mais robusta pode ser necessária para formatos complexos.
	normalizedForDecimal = strings.ReplaceAll(normalizedForDecimal, ".", "") // Remove todos os pontos
	if strings.Contains(trimmed, ",") {                                      // Se tinha vírgula, o último ponto removido era o separador decimal
		if lastCommaIdx := strings.LastIndex(trimmed, ","); lastCommaIdx != -1 {
			normalizedForDecimal = trimmed[:lastCommaIdx] + "." + trimmed[lastCommaIdx+1:]
			normalizedForDecimal = strings.ReplaceAll(normalizedForDecimal, ".", "")                                                             // Remove milhares
			normalizedForDecimal = normalizedForDecimal[:len(normalizedForDecimal)-3] + "." + normalizedForDecimal[len(normalizedForDecimal)-2:] // Reinsere ponto decimal
		}
	}

	d, err := decimal.NewFromString(normalizedForDecimal)
	if err != nil {
		appLogger.Warnf("[Linha %d TDir] Valor decimal inválido na coluna '%s': '%s' (normalizado: '%s'). Erro: %v. Usando NULL.", rowNum, colName, valStr, normalizedForDecimal, err)
		// Retorna erro para que o chamador possa decidir se usa placeholder ou ignora
		return nil, fmt.Errorf("valor decimal '%s' inválido para coluna '%s'", valStr, colName)
	}
	// Retorna a string no formato "XXXX.YY" com duas casas decimais
	// Isso é importante para consistência ao salvar no banco como VARCHAR
	formattedDecimalStr := d.StringFixedBank(2) // Arredonda para 2 casas (padrão bancário)
	return &formattedDecimalStr, nil
}

// ReplaceAll substitui todos os dados na tabela titulos_direitos.
func (r *gormTituloDireitoRepository) ReplaceAll(rawData []models.TituloDireitoFromRow) (insertedCount int, skippedCount int, err error) {
	if len(rawData) == 0 {
		appLogger.Info("ReplaceAll Títulos de Direitos: Nenhum dado bruto fornecido.")
		// Deletar dados antigos mesmo se não houver novos para inserir
		if txErr := r.db.Exec("DELETE FROM " + models.DBTituloDireito{}.TableName()).Error; txErr != nil {
			appLogger.Errorf("Erro SQLAlchemy ao deletar dados antigos de Títulos de Direitos (com input vazio): %v", txErr)
			return 0, 0, appErrors.WrapErrorf(txErr, "falha ao limpar dados antigos de títulos de direitos (GORM)")
		}
		return 0, 0, nil
	}

	dbEntries := make([]models.DBTituloDireito, 0, len(rawData))
	rowsWithPlaceholders := 0
	rowsTruncated := 0 // Não usado diretamente aqui, mas os helpers podem logar

	for i, row := range rawData {
		rowNum := i + 1 // Para logs baseados em 1
		usedPlaceholder := false
		var currentErr error

		// Pessoa (obrigatório para processar a linha, como no Python)
		pessoa := strings.TrimSpace(row.Pessoa)
		if pessoa == "" {
			appLogger.Warnf("[Linha %d TDir] IGNORADA: Campo 'PESSOA' ausente/inválido. Dados: %+v", rowNum, row)
			skippedCount++
			continue
		}

		// CNPJ/CPF (obrigatório no DB)
		cleanedCNPJCPF := models.CleanCNPJ(row.CNPJCPF) // Usa helper do models
		if len(cleanedCNPJCPF) > 14 {
			cleanedCNPJCPF = cleanedCNPJCPF[:14]
		}
		if cleanedCNPJCPF == "" {
			cleanedCNPJCPF = defaultPlaceholderCNPJ
			usedPlaceholder = true
		}

		// NumeroEmpresa (obrigatório no DB)
		var numeroEmpresa int
		trimmedNumEmp := strings.TrimSpace(row.NumeroEmpresa)
		if trimmedNumEmp == "" {
			numeroEmpresa = defaultPlaceholderInt
			usedPlaceholder = true
		} else {
			numeroEmpresa, currentErr = strconv.Atoi(trimmedNumEmp)
			if currentErr != nil {
				appLogger.Warnf("[Linha %d TDir] Valor inválido para NROEMPRESA: '%s'. Usando placeholder %d.", rowNum, row.NumeroEmpresa, defaultPlaceholderInt)
				numeroEmpresa = defaultPlaceholderInt
				usedPlaceholder = true
			}
		}

		// Titulo (obrigatório no DB, VARCHAR(100))
		titulo := strings.TrimSpace(row.Titulo)
		if titulo == "" {
			titulo = defaultPlaceholderString
			usedPlaceholder = true
		}
		titulo = truncateString(titulo, varchar100Limit)

		// ValorNominal (obrigatório no DB)
		var valorNominalStr string
		parsedVN, vnErr := parseDecimalString(row.ValorNominal, rowNum, "VLRNOMINAL")
		if vnErr != nil || parsedVN == nil { // Erro no parse ou string vazia retornou nil
			valorNominalStr = defaultPlaceholderDecimal.StringFixedBank(2)
			usedPlaceholder = true
			if vnErr != nil { // Loga o erro de parsing se ocorreu
				appLogger.Warnf("[Linha %d TDir] Erro ao parsear VLRNOMINAL '%s', usando placeholder. Erro: %v", rowNum, row.ValorNominal, vnErr)
			}
		} else {
			valorNominalStr = *parsedVN
		}

		// Campos Opcionais (Nullable)
		codigoEspecie := strings.TrimSpace(row.CodigoEspecie)
		var pCodigoEspecie *string
		if codigoEspecie != "" {
			pCodigoEspecie = &[]string{truncateString(codigoEspecie, varchar50Limit)}[0]
		}

		dataVencimento := parseDate(row.DataVencimento, rowNum, "DTAVENCIMENTO")
		dataQuitacao := parseDate(row.DataQuitacao, rowNum, "DTAQUITAÇÃO")

		valorPagoStr, vpErr := parseDecimalString(row.ValorPago, rowNum, "VLRPAGO")
		if vpErr != nil { // Loga erro, mas continua com nil (campo é opcional)
			appLogger.Warnf("[Linha %d TDir] Erro ao parsear VLRPAGO '%s', será NULL. Erro: %v", rowNum, row.ValorPago, vpErr)
		}

		operacao := strings.TrimSpace(row.Operacao)
		var pOperacao *string
		if operacao != "" {
			pOperacao = &[]string{truncateString(operacao, varchar50Limit)}[0]
		}

		dataOperacao := parseDate(row.DataOperacao, rowNum, "DTAOPERAÇÃO")
		dataContabiliza := parseDate(row.DataContabiliza, rowNum, "DTACONTABILIZA")
		// DataAlteracaoCSV não é usada para DataAlteracao do DB que é autoUpdateTime
		// Se precisar usar o valor do CSV, adicione o parsing aqui.

		observacao := strings.TrimSpace(row.Observacao)
		var pObservacao *string
		if observacao != "" {
			pObservacao = &observacao
		}

		valorOperacaoStr, voErr := parseDecimalString(row.ValorOperacao, rowNum, "VLROPERAÇÃO")
		if voErr != nil {
			appLogger.Warnf("[Linha %d TDir] Erro ao parsear VLROPERAÇÃO '%s', será NULL. Erro: %v", rowNum, row.ValorOperacao, voErr)
		}

		usuarioAlteracao := strings.TrimSpace(row.UsuarioAlteracao)
		var pUsuarioAlteracao *string
		if usuarioAlteracao != "" {
			pUsuarioAlteracao = &[]string{truncateString(usuarioAlteracao, varchar50Limit)}[0]
		}

		especieAbatcomp := strings.TrimSpace(row.EspecieAbatcomp)
		var pEspecieAbatcomp *string
		if especieAbatcomp != "" {
			pEspecieAbatcomp = &[]string{truncateString(especieAbatcomp, varchar100Limit)}[0]
		}

		obsTitulo := strings.TrimSpace(row.ObsTitulo)
		var pObsTitulo *string
		if obsTitulo != "" {
			pObsTitulo = &obsTitulo
		}

		contasQuitacao := strings.TrimSpace(row.ContasQuitacao)
		var pContasQuitacao *string
		if contasQuitacao != "" {
			pContasQuitacao = &contasQuitacao
		}
		dataProgramada := parseDate(row.DataProgramada, rowNum, "DTAPROGRAMADA")

		if usedPlaceholder {
			rowsWithPlaceholders++
		}

		dbEntry := models.DBTituloDireito{
			Pessoa:          &pessoa,
			CNPJCPF:         cleanedCNPJCPF,
			NumeroEmpresa:   numeroEmpresa,
			Titulo:          titulo,
			CodigoEspecie:   pCodigoEspecie,
			DataVencimento:  dataVencimento,
			DataQuitacao:    dataQuitacao,
			ValorNominal:    valorNominalStr,
			ValorPago:       valorPagoStr,
			Operacao:        pOperacao,
			DataOperacao:    dataOperacao,
			DataContabiliza: dataContabiliza,
			// DataAlteracao é autoUpdateTime
			Observacao:       pObservacao,
			ValorOperacao:    valorOperacaoStr,
			UsuarioAlteracao: pUsuarioAlteracao,
			EspecieAbatcomp:  pEspecieAbatcomp,
			ObsTitulo:        pObsTitulo,
			ContasQuitacao:   pContasQuitacao,
			DataProgramada:   dataProgramada,
		}
		dbEntries = append(dbEntries, dbEntry)
	}

	appLogger.Infof("Títulos de Direitos Processados: %d válidos, %d ignorados por 'PESSOA' inválida, %d receberam placeholders.",
		len(dbEntries), skippedCount, rowsWithPlaceholders)
	if rowsTruncated > 0 { // Este contador não está sendo incrementado neste exemplo, mas poderia ser em truncateString
		appLogger.Infof("ATENÇÃO TDir: %d valores de texto foram truncados.", rowsTruncated)
	}

	// Usar transação para deletar e inserir
	err = r.db.Transaction(func(tx *gorm.DB) error {
		// 1. Deletar dados antigos
		appLogger.Info("Deletando dados antigos de Títulos de Direitos...")
		if txErr := tx.Exec("DELETE FROM " + models.DBTituloDireito{}.TableName()).Error; txErr != nil {
			return appErrors.WrapErrorf(txErr, "falha ao limpar dados antigos de títulos de direitos (GORM)")
		}
		appLogger.Debug("Dados antigos de Títulos de Direitos deletados.")

		// 2. Inserir novos dados em lote
		if len(dbEntries) > 0 {
			appLogger.Infof("Inserindo %d novos registros de Títulos de Direitos...", len(dbEntries))
			// CreateInBatches é bom para performance com muitos registros
			if txErr := tx.CreateInBatches(&dbEntries, 1000).Error; txErr != nil { // Batch de 1000
				return appErrors.WrapErrorf(txErr, "falha ao inserir novos títulos de direitos em lote (GORM)")
			}
			appLogger.Debugf("%d Títulos de Direitos inseridos com sucesso.", len(dbEntries))
		}
		return nil // Commit
	})

	if err != nil {
		appLogger.Errorf("Erro na transação de ReplaceAll para Títulos de Direitos: %v", err)
		return 0, skippedCount, err // Retorna o erro da transação
	}

	return len(dbEntries), skippedCount, nil
}
