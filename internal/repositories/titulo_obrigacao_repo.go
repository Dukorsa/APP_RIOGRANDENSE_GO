package repositories

import (
	// Ainda necessário para sql.NullString, etc., se não usar ponteiros em todos os lugares

	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal" // Presumindo que será usado para consistência, mesmo que os valores sejam string no DB
	"gorm.io/gorm"

	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/errors"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/models"
)

// TituloObrigacaoRepository define a interface para operações no repositório de títulos de obrigações.
type TituloObrigacaoRepository interface {
	// ReplaceAll substitui TODOS os dados na tabela.
	// Recebe uma lista de structs TituloObrigacaoFromRow.
	// Retorna o número de registros efetivamente inseridos, pulados e um erro.
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

// Constantes e helpers podem ser os mesmos do titulo_direito_repo.go se os limites e placeholders forem idênticos.
// Se forem diferentes, defina-os aqui ou em um pacote utilitário compartilhado.
// Por agora, vamos assumir que são os mesmos e que os helpers (truncateString, parseDate, parseDecimalString)
// são acessíveis (ou podem ser copiados/refatorados para um pacote utilitário comum).
// Para este exemplo, vou repetir os helpers relevantes com pequenas modificações no log para clareza.

// Helper para parsear string para *time.Time (formato "dd/mm/yyyy")
func parseDateObrig(dateStr string, rowNum int, colName string) *time.Time {
	trimmed := strings.TrimSpace(dateStr)
	if trimmed == "" {
		return nil
	}
	formats := []string{"02/01/2006", "2/1/2006", "02-01-2006", "2-1-2006", "2006-01-02"}
	for _, format := range formats {
		t, err := time.Parse(format, trimmed)
		if err == nil {
			return &t
		}
	}
	appLogger.Warnf("[Linha %d TObrig] Formato de data inválido na coluna '%s': '%s'. Usando NULL.", rowNum, colName, dateStr)
	return nil
}

// Helper para parsear string para string representando decimal (ex: "1234.56")
func parseDecimalStringObrig(valStr string, rowNum int, colName string) (*string, error) {
	trimmed := strings.TrimSpace(valStr)
	if trimmed == "" {
		return nil, nil
	}
	// Lógica de normalização e parsing (idêntica à de TituloDireito)
	normalizedForDecimal := strings.ReplaceAll(trimmed, ",", ".")
	normalizedForDecimal = strings.ReplaceAll(normalizedForDecimal, ".", "") // Remove todos os pontos
	if strings.Contains(trimmed, ",") {
		if lastCommaIdx := strings.LastIndex(trimmed, ","); lastCommaIdx != -1 {
			// Reconstruir com base na última vírgula como separador decimal
			integerPart := strings.ReplaceAll(trimmed[:lastCommaIdx], ".", "")
			decimalPart := trimmed[lastCommaIdx+1:]
			normalizedForDecimal = integerPart + "." + decimalPart
		}
	} else if strings.Count(trimmed, ".") > 1 { // Múltiplos pontos sem vírgula, ex: 1.234.567
		normalizedForDecimal = strings.ReplaceAll(trimmed, ".", "") // Remove todos, assume que não há decimais
	}

	d, err := decimal.NewFromString(normalizedForDecimal)
	if err != nil {
		appLogger.Warnf("[Linha %d TObrig] Valor decimal inválido na coluna '%s': '%s' (normalizado: '%s'). Erro: %v. Usando NULL.", rowNum, colName, valStr, normalizedForDecimal, err)
		return nil, fmt.Errorf("valor decimal '%s' inválido para coluna '%s'", valStr, colName)
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
	rowsWithPlaceholders := 0
	// rowsTruncated := 0 // Se precisar contar truncamentos especificamente aqui

	for i, row := range rawData {
		rowNum := i + 1
		usedPlaceholder := false
		var currentErr error

		// Pessoa (obrigatório para processar a linha)
		pessoa := strings.TrimSpace(row.Pessoa)
		if pessoa == "" {
			appLogger.Warnf("[Linha %d TObrig] IGNORADA: Campo 'PESSOA' ausente/inválido. Dados: %+v", rowNum, row)
			skippedCount++
			continue
		}

		// CNPJ/CPF (obrigatório no DB)
		cleanedCNPJCPF := models.CleanCNPJ(row.CNPJCPF)
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
				appLogger.Warnf("[Linha %d TObrig] Valor inválido para NROEMPRESA: '%s'. Usando placeholder %d.", rowNum, row.NumeroEmpresa, defaultPlaceholderInt)
				numeroEmpresa = defaultPlaceholderInt
				usedPlaceholder = true
			}
		}

		// IdentificadorObrigacao (era 'Titulo' no FromRow, obrigatório no DB, VARCHAR(100))
		identificadorObrigacao := strings.TrimSpace(row.Titulo) // Mapeia de row.Titulo
		if identificadorObrigacao == "" {
			identificadorObrigacao = defaultPlaceholderString
			usedPlaceholder = true
		}
		identificadorObrigacao = truncateString(identificadorObrigacao, varchar100Limit)

		// ValorNominalObrigacao (era 'ValorNominal' no FromRow, obrigatório no DB)
		var valorNominalObrigacaoStr string
		parsedVNO, vnoErr := parseDecimalStringObrig(row.ValorNominal, rowNum, "VLRNOMINAL (Obrigação)") // Mapeia de row.ValorNominal
		if vnoErr != nil || parsedVNO == nil {
			valorNominalObrigacaoStr = defaultPlaceholderDecimal.StringFixedBank(2)
			usedPlaceholder = true
			if vnoErr != nil {
				appLogger.Warnf("[Linha %d TObrig] Erro ao parsear VLRNOMINAL (Obrigação) '%s', usando placeholder. Erro: %v", rowNum, row.ValorNominal, vnoErr)
			}
		} else {
			valorNominalObrigacaoStr = *parsedVNO
		}

		// Campos Opcionais (Nullable)
		codigoEspecie := strings.TrimSpace(row.CodigoEspecie)
		var pCodigoEspecie *string
		if codigoEspecie != "" {
			pCodigoEspecie = &[]string{truncateString(codigoEspecie, varchar50Limit)}[0]
		}

		dataVencimento := parseDateObrig(row.DataVencimento, rowNum, "DTAVENCIMENTO")
		dataQuitacao := parseDateObrig(row.DataQuitacao, rowNum, "DTAQUITAÇÃO")
		valorPagoStr, vpErr := parseDecimalStringObrig(row.ValorPago, rowNum, "VLRPAGO")
		if vpErr != nil {
			appLogger.Warnf("[Linha %d TObrig] Erro ao parsear VLRPAGO '%s', será NULL. Erro: %v", rowNum, row.ValorPago, vpErr)
		}

		operacao := strings.TrimSpace(row.Operacao)
		var pOperacao *string
		if operacao != "" {
			pOperacao = &[]string{truncateString(operacao, varchar50Limit)}[0]
		}

		dataOperacao := parseDateObrig(row.DataOperacao, rowNum, "DTAOPERAÇÃO")
		dataContabiliza := parseDateObrig(row.DataContabiliza, rowNum, "DTACONTABILIZA")

		observacao := strings.TrimSpace(row.Observacao)
		var pObservacao *string
		if observacao != "" {
			pObservacao = &observacao
		}
		valorOperacaoStr, voErr := parseDecimalStringObrig(row.ValorOperacao, rowNum, "VLROPERAÇÃO")
		if voErr != nil {
			appLogger.Warnf("[Linha %d TObrig] Erro ao parsear VLROPERAÇÃO '%s', será NULL. Erro: %v", rowNum, row.ValorOperacao, voErr)
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
		dataProgramada := parseDateObrig(row.DataProgramada, rowNum, "DTAPROGRAMADA")

		if usedPlaceholder {
			rowsWithPlaceholders++
		}

		dbEntry := models.DBTituloObrigacao{
			Pessoa:                 &pessoa,
			CNPJCPF:                cleanedCNPJCPF,
			NumeroEmpresa:          numeroEmpresa,
			IdentificadorObrigacao: identificadorObrigacao, // Mapeado
			CodigoEspecie:          pCodigoEspecie,
			DataVencimento:         dataVencimento,
			DataQuitacao:           dataQuitacao,
			ValorNominalObrigacao:  valorNominalObrigacaoStr, // Mapeado
			ValorPago:              valorPagoStr,
			Operacao:               pOperacao,
			DataOperacao:           dataOperacao,
			DataContabiliza:        dataContabiliza,
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

	appLogger.Infof("Títulos de Obrigações Processados: %d válidos, %d ignorados por 'PESSOA' inválida, %d receberam placeholders.",
		len(dbEntries), skippedCount, rowsWithPlaceholders)
	// if rowsTruncated > 0 { ... }

	err = r.db.Transaction(func(tx *gorm.DB) error {
		appLogger.Info("Deletando dados antigos de Títulos de Obrigações...")
		if txErr := tx.Exec("DELETE FROM " + models.DBTituloObrigacao{}.TableName()).Error; txErr != nil {
			return appErrors.WrapErrorf(txErr, "falha ao limpar dados antigos de títulos de obrigações (GORM)")
		}
		appLogger.Debug("Dados antigos de Títulos de Obrigações deletados.")

		if len(dbEntries) > 0 {
			appLogger.Infof("Inserindo %d novos registros de Títulos de Obrigações...", len(dbEntries))
			if txErr := tx.CreateInBatches(&dbEntries, 1000).Error; txErr != nil {
				return appErrors.WrapErrorf(txErr, "falha ao inserir novos títulos de obrigações em lote (GORM)")
			}
			appLogger.Debugf("%d Títulos de Obrigações inseridos com sucesso.", len(dbEntries))
		}
		return nil // Commit
	})

	if err != nil {
		appLogger.Errorf("Erro na transação de ReplaceAll para Títulos de Obrigações: %v", err)
		return 0, skippedCount, err
	}

	return len(dbEntries), skippedCount, nil
}
