package repositories

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal" // Para validação e conversão de valores monetários
	"gorm.io/gorm"

	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/errors"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/models"
)

// TituloObrigacaoRepository define a interface para operações no repositório de títulos de obrigações.
type TituloObrigacaoRepository interface {
	// ReplaceAll substitui TODOS os dados na tabela `titulos_obrigacoes`.
	// Recebe uma lista de structs `models.TituloObrigacaoFromRow`.
	// Retorna o número de registros inseridos, pulados e um erro, se houver.
	ReplaceAll(rawData []models.TituloObrigacaoFromRow) (insertedCount int, skippedCount int, err error)
}

// gormTituloObrigacaoRepository é a implementação GORM de TituloObrigacaoRepository.
type gormTituloObrigacaoRepository struct {
	db *gorm.DB
}

// NewGormTituloObrigacaoRepository cria uma nova instância de gormTituloObrigacaoRepository.
func NewGormTituloObrigacaoRepository(db *gorm.DB) TituloObrigacaoRepository {
	if db == nil {
		appLogger.Fatalf("gorm.DB não pode ser nil para NewGormTituloObrigacaoRepository")
	}
	return &gormTituloObrigacaoRepository{db: db}
}

// Constantes e helpers de parsing podem ser compartilhados com `titulo_direito_repo.go`
// se forem idênticos. Para este exemplo, eles são replicados/adaptados.
// Em um projeto real, mova-os para um pacote `utils/parser` ou similar se forem comuns.

// parseDateObrig (adaptado para logs de TObrig)
func parseDateObrig(dateStr string, rowNumForLog int, colNameForLog string) *time.Time {
	trimmed := strings.TrimSpace(dateStr)
	if trimmed == "" {
		return nil
	}
	formats := []string{"02/01/2006", "2/1/2006", "02-01-2006", "2-1-2006", "2006-01-02", "20060102"}
	for _, format := range formats {
		t, err := time.Parse(format, trimmed)
		if err == nil {
			return &t
		}
	}
	appLogger.Warnf("[Linha %d TObrig] Formato de data inválido na coluna '%s': '%s'. Usando NULL.", rowNumForLog, colNameForLog, dateStr)
	return nil
}

// parseDateTimeObrig (adaptado para logs de TObrig)
func parseDateTimeObrig(dateTimeStr string, rowNumForLog int, colNameForLog string) *time.Time {
	trimmed := strings.TrimSpace(dateTimeStr)
	if trimmed == "" {
		return nil
	}
	formats := []string{
		"02/01/2006 15:04:05", "2/1/2006 15:04:05", "2006-01-02 15:04:05",
		"02/01/2006 15:04", "2/1/2006 15:04", "2006-01-02T15:04:05Z07:00", "2006-01-02T15:04:05",
		"02/01/2006", "2/1/2006", "2006-01-02",
	}
	for _, format := range formats {
		t, err := time.Parse(format, trimmed)
		if err == nil {
			return &t
		}
	}
	appLogger.Warnf("[Linha %d TObrig] Formato de data/hora inválido na coluna '%s': '%s'. Usando NULL.", rowNumForLog, colNameForLog, dateTimeStr)
	return nil
}

// parseDecimalStringObrig (adaptado para logs de TObrig)
func parseDecimalStringObrig(valStr string, rowNumForLog int, colNameForLog string) (*string, error) {
	trimmed := strings.TrimSpace(valStr)
	if trimmed == "" {
		return nil, nil
	}
	normalizedForDecimal := strings.ReplaceAll(trimmed, " ", "")
	if strings.Contains(normalizedForDecimal, ",") {
		normalizedForDecimal = strings.ReplaceAll(normalizedForDecimal, ".", "")
		normalizedForDecimal = strings.ReplaceAll(normalizedForDecimal, ",", ".")
	}

	d, err := decimal.NewFromString(normalizedForDecimal)
	if err != nil {
		cleanedFurther := regexp.MustCompile(`[^\d.,]`).ReplaceAllString(trimmed, "")
		if strings.Contains(cleanedFurther, ",") && strings.Count(cleanedFurther, ",") == 1 {
			cleanedFurther = strings.ReplaceAll(cleanedFurther, ".", "")
			cleanedFurther = strings.ReplaceAll(cleanedFurther, ",", ".")
		} else if strings.Count(cleanedFurther, ".") > 1 {
			lastDotIndex := strings.LastIndex(cleanedFurther, ".")
			if lastDotIndex != -1 {
				firstPart := strings.ReplaceAll(cleanedFurther[:lastDotIndex], ".", "")
				cleanedFurther = firstPart + cleanedFurther[lastDotIndex:]
			}
		}
		d, err = decimal.NewFromString(cleanedFurther)
		if err != nil {
			appLogger.Warnf("[Linha %d TObrig] Valor decimal inválido na coluna '%s': '%s'. Erro: %v. Será NULL ou placeholder.", rowNumForLog, colNameForLog, valStr, err)
			return nil, fmt.Errorf("valor decimal '%s' inválido para coluna '%s'", valStr, colNameForLog)
		}
	}
	formattedDecimalStr := d.StringFixedBank(2)
	return &formattedDecimalStr, nil
}

// ReplaceAll substitui todos os dados na tabela titulos_obrigacoes.
func (r *gormTituloObrigacaoRepository) ReplaceAll(rawData []models.TituloObrigacaoFromRow) (insertedCount int, skippedCount int, err error) {
	if len(rawData) == 0 {
		appLogger.Info("ReplaceAll Títulos de Obrigações: Nenhum dado bruto fornecido.")
		if txErr := r.db.Exec("DELETE FROM " + models.DBTituloObrigacao{}.TableName()).Error; txErr != nil {
			appLogger.Errorf("Erro ao deletar dados antigos de Títulos de Obrigações (input vazio): %v", txErr)
			return 0, 0, appErrors.WrapErrorf(txErr, "falha ao limpar dados antigos de títulos de obrigações (GORM)")
		}
		return 0, 0, nil
	}

	dbEntries := make([]models.DBTituloObrigacao, 0, len(rawData))
	rowsWithPlaceholdersUsed := 0

	for i, row := range rawData {
		rowNumForLog := i + 1
		usedPlaceholderInThisRow := false

		pessoaStr := strings.TrimSpace(row.Pessoa)
		var pPessoa *string
		if pessoaStr != "" {
			val := truncateString(pessoaStr, varchar255Limit)
			pPessoa = &val
		}

		cleanedCNPJCPF := models.CleanCNPJ(row.CNPJCPF)
		if len(cleanedCNPJCPF) > 14 {
			cleanedCNPJCPF = cleanedCNPJCPF[:14]
		}
		if cleanedCNPJCPF == "" {
			cleanedCNPJCPF = defaultPlaceholderCNPJ
			usedPlaceholderInThisRow = true
			appLogger.Debugf("[Linha %d TObrig] CNPJ/CPF vazio, usando placeholder.", rowNumForLog)
		}

		var numeroEmpresa int
		trimmedNumEmp := strings.TrimSpace(row.NumeroEmpresa)
		if trimmedNumEmp == "" {
			numeroEmpresa = defaultPlaceholderInt
			usedPlaceholderInThisRow = true
			appLogger.Debugf("[Linha %d TObrig] NROEMPRESA vazio, usando placeholder.", rowNumForLog)
		} else {
			parsedNum, parseErr := strconv.Atoi(trimmedNumEmp)
			if parseErr != nil {
				appLogger.Warnf("[Linha %d TObrig] Valor inválido para NROEMPRESA: '%s'. Usando placeholder. Erro: %v", rowNumForLog, row.NumeroEmpresa, parseErr)
				numeroEmpresa = defaultPlaceholderInt
				usedPlaceholderInThisRow = true
			} else {
				numeroEmpresa = parsedNum
			}
		}

		// Mapeia 'Titulo' do CSV para 'IdentificadorObrigacao'
		identObrigacaoStr := strings.TrimSpace(row.Titulo)
		if identObrigacaoStr == "" {
			identObrigacaoStr = defaultPlaceholderString
			usedPlaceholderInThisRow = true
			appLogger.Debugf("[Linha %d TObrig] TÍTULO (IdentificadorObrigacao) vazio, usando placeholder.", rowNumForLog)
		}
		identObrigacaoStr = truncateString(identObrigacaoStr, varchar100Limit)

		// Mapeia 'ValorNominal' do CSV para 'ValorNominalObrigacao'
		var valorNominalObrigacaoStr string
		parsedVNO, vnoErr := parseDecimalStringObrig(row.ValorNominal, rowNumForLog, "VLRNOMINAL (Obrigação)")
		if vnoErr != nil || parsedVNO == nil {
			valorNominalObrigacaoStr = defaultPlaceholderDecimalStr
			usedPlaceholderInThisRow = true
			appLogger.Warnf("[Linha %d TObrig] VLRNOMINAL (Obrigação) inválido ou vazio ('%s'), usando placeholder. Erro: %v", rowNumForLog, row.ValorNominal, vnoErr)
		} else {
			valorNominalObrigacaoStr = truncateString(*parsedVNO, varchar30Limit)
		}

		// Campos Opcionais
		codigoEspecie := strings.TrimSpace(row.CodigoEspecie)
		var pCodigoEspecie *string
		if codigoEspecie != "" {
			val := truncateString(codigoEspecie, varchar50Limit)
			pCodigoEspecie = &val
		}

		dataVencimento := parseDateObrig(row.DataVencimento, rowNumForLog, "DTAVENCIMENTO")
		dataQuitacao := parseDateObrig(row.DataQuitacao, rowNumForLog, "DTAQUITAÇÃO")
		valorPagoStr, vpErr := parseDecimalStringObrig(row.ValorPago, rowNumForLog, "VLRPAGO")
		if vpErr != nil {
			appLogger.Warnf("[Linha %d TObrig] Erro ao parsear VLRPAGO '%s', será NULL. Erro: %v", rowNumForLog, row.ValorPago, vpErr)
			valorPagoStr = nil
		} else if valorPagoStr != nil {
			val := truncateString(*valorPagoStr, varchar30Limit)
			valorPagoStr = &val
		}

		operacao := strings.TrimSpace(row.Operacao)
		var pOperacao *string
		if operacao != "" {
			val := truncateString(operacao, varchar50Limit)
			pOperacao = &val
		}

		dataOperacao := parseDateObrig(row.DataOperacao, rowNumForLog, "DTAOPERAÇÃO")
		dataContabiliza := parseDateObrig(row.DataContabiliza, rowNumForLog, "DTACONTABILIZA")
		dataAlteracaoDoCSV := parseDateTimeObrig(row.DataAlteracaoCSV, rowNumForLog, "DTAALTERAÇÃO_CSV")

		observacao := strings.TrimSpace(row.Observacao)
		var pObservacao *string
		if observacao != "" {
			pObservacao = &observacao
		}

		valorOperacaoStr, voErr := parseDecimalStringObrig(row.ValorOperacao, rowNumForLog, "VLROPERAÇÃO")
		if voErr != nil {
			appLogger.Warnf("[Linha %d TObrig] Erro ao parsear VLROPERAÇÃO '%s', será NULL. Erro: %v", rowNumForLog, row.ValorOperacao, voErr)
			valorOperacaoStr = nil
		} else if valorOperacaoStr != nil {
			val := truncateString(*valorOperacaoStr, varchar30Limit)
			valorOperacaoStr = &val
		}

		usuarioAlteracao := strings.TrimSpace(row.UsuarioAlteracao)
		var pUsuarioAlteracao *string
		if usuarioAlteracao != "" {
			val := truncateString(usuarioAlteracao, varchar50Limit)
			pUsuarioAlteracao = &val
		}

		especieAbatcomp := strings.TrimSpace(row.EspecieAbatcomp)
		var pEspecieAbatcomp *string
		if especieAbatcomp != "" {
			val := truncateString(especieAbatcomp, varchar100Limit)
			pEspecieAbatcomp = &val
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
		dataProgramada := parseDateObrig(row.DataProgramada, rowNumForLog, "DTAPROGRAMADA")

		if usedPlaceholderInThisRow {
			rowsWithPlaceholdersUsed++
		}

		dbEntry := models.DBTituloObrigacao{
			Pessoa:                 pPessoa,
			CNPJCPF:                cleanedCNPJCPF,
			NumeroEmpresa:          numeroEmpresa,
			IdentificadorObrigacao: identObrigacaoStr, // Mapeado de row.Titulo
			CodigoEspecie:          pCodigoEspecie,
			DataVencimento:         dataVencimento,
			DataQuitacao:           dataQuitacao,
			ValorNominalObrigacao:  valorNominalObrigacaoStr, // Mapeado de row.ValorNominal
			ValorPago:              valorPagoStr,
			Operacao:               pOperacao,
			DataOperacao:           dataOperacao,
			DataContabiliza:        dataContabiliza,
			DataAlteracaoCSV:       dataAlteracaoDoCSV,
			Observacao:             pObservacao,
			ValorOperacao:          valorOperacaoStr,
			UsuarioAlteracao:       pUsuarioAlteracao,
			EspecieAbatcomp:        pEspecieAbatcomp,
			ObsTitulo:              pObsTitulo,
			ContasQuitacao:         pContasQuitacao,
			DataProgramada:         dataProgramada,
			// CreatedAt/UpdatedAt gerenciados pelo GORM ou DB
		}
		dbEntries = append(dbEntries, dbEntry)
	}

	appLogger.Infof("Títulos de Obrigações Processados para inserção: %d. Linhas com placeholders: %d.",
		len(dbEntries), rowsWithPlaceholdersUsed)
	skippedCount = 0 // Lógica de pular linhas baseada em `PESSOA` deve estar no serviço.

	err = r.db.Transaction(func(tx *gorm.DB) error {
		appLogger.Info("Deletando dados antigos de Títulos de Obrigações...")
		if txErr := tx.Exec("DELETE FROM " + models.DBTituloObrigacao{}.TableName()).Error; txErr != nil {
			return appErrors.WrapErrorf(txErr, "falha ao limpar dados antigos de títulos de obrigações (GORM)")
		}
		appLogger.Debugf("Dados antigos de Títulos de Obrigações (%s) deletados.", models.DBTituloObrigacao{}.TableName())

		if len(dbEntries) > 0 {
			appLogger.Infof("Inserindo %d novos registros de Títulos de Obrigações...", len(dbEntries))
			if txErr := tx.CreateInBatches(&dbEntries, 1000).Error; txErr != nil {
				return appErrors.WrapErrorf(txErr, "falha ao inserir novos títulos de obrigações em lote (GORM)")
			}
			appLogger.Debugf("%d Títulos de Obrigações inseridos com sucesso.", len(dbEntries))
		}
		return nil
	})

	if err != nil {
		appLogger.Errorf("Erro na transação de ReplaceAll para Títulos de Obrigações: %v", err)
		return 0, skippedCount, err
	}

	return len(dbEntries), skippedCount, nil
}
