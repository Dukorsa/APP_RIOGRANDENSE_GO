package repositories

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal" // Para validação e conversão de valores monetários
	"gorm.io/gorm"

	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/models"
)

// TituloDireitoRepository define a interface para operações no repositório de títulos de direitos.
type TituloDireitoRepository interface {
	// ReplaceAll substitui TODOS os dados na tabela `titulos_direitos`.
	// Recebe uma lista de structs `models.TituloDireitoFromRow`, que representam os dados brutos
	// como lidos do arquivo de importação.
	// Retorna o número de registros efetivamente inseridos, o número de linhas do arquivo
	// que foram puladas devido a erros de parsing/validação primária, e um erro, se houver.
	ReplaceAll(rawData []models.TituloDireitoFromRow) (insertedCount int, skippedCount int, err error)

	// GetAll (Exemplo, não solicitado, mas comum em repositórios)
	// GetAll() ([]models.DBTituloDireito, error)
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

// Constantes para valores padrão em caso de dados ausentes ou inválidos do arquivo.
// Estes são usados para garantir que colunas NOT NULL tenham um valor.
const (
	// VARCHAR_50_LIMIT e VARCHAR_100_LIMIT são usados para truncar strings se excederem os limites do DB.
	varchar50Limit  = 50
	varchar100Limit = 100
	varchar255Limit = 255 // Limite comum para campos de nome/pessoa.
	varchar30Limit  = 30  // Para valores monetários armazenados como string.

	defaultPlaceholderString = "N/A"            // Para campos string NOT NULL que estão vazios.
	defaultPlaceholderCNPJ   = "00000000000000" // CNPJ/CPF inválido, mas que satisfaz NOT NULL.
	defaultPlaceholderInt    = 0                // Para campos int NOT NULL que estão vazios/inválidos.
)

// defaultPlaceholderDecimalStr é o valor decimal padrão (0.00) como string.
var defaultPlaceholderDecimalStr = decimal.Zero.StringFixedBank(2)

// --- Funções Helper para Parsing e Limpeza de Dados da Linha do Arquivo ---

// truncateString garante que uma string não exceda um limite de caracteres.
func truncateString(value string, limit int) string {
	if len(value) > limit {
		// Considerar contagem de runas se houver caracteres multibyte.
		// Por simplicidade, len() é usado aqui (conta bytes).
		// Se precisar de precisão de runas: if utf8.RuneCountInString(value) > limit ...
		return value[:limit]
	}
	return value
}

// parseDate converte uma string de data (esperado "DD/MM/YYYY" ou "YYYY-MM-DD") para *time.Time.
// Retorna nil se a string for vazia ou o parsing falhar, logando um aviso.
func parseDate(dateStr string, rowNumForLog int, colNameForLog string) *time.Time {
	trimmed := strings.TrimSpace(dateStr)
	if trimmed == "" {
		return nil
	}
	// Formatos comuns de data a serem tentados.
	formats := []string{"02/01/2006", "2/1/2006", "02-01-2006", "2-1-2006", "2006-01-02", "20060102"}
	for _, format := range formats {
		t, err := time.Parse(format, trimmed)
		if err == nil {
			// GORM com `gorm:"type:date"` geralmente lida bem com a parte da hora sendo zero.
			// Se o fuso horário for importante, certifique-se de que `t` esteja em UTC ou no fuso correto.
			// t = t.UTC() // Exemplo para normalizar para UTC.
			return &t
		}
	}
	appLogger.Warnf("[Linha %d TDir] Formato de data inválido na coluna '%s': '%s'. Usando NULL.", rowNumForLog, colNameForLog, dateStr)
	return nil
}

// parseDateTime converte uma string de data e hora para *time.Time.
// Tenta formatos comuns. Retorna nil se vazio ou falhar.
func parseDateTime(dateTimeStr string, rowNumForLog int, colNameForLog string) *time.Time {
	trimmed := strings.TrimSpace(dateTimeStr)
	if trimmed == "" {
		return nil
	}
	// Formatos comuns de data e hora (ajuste conforme os dados reais do arquivo).
	formats := []string{
		"02/01/2006 15:04:05", "2/1/2006 15:04:05",
		"2006-01-02 15:04:05",
		"02/01/2006 15:04", "2/1/2006 15:04", // Sem segundos
		"2006-01-02T15:04:05Z07:00", // RFC3339
		"2006-01-02T15:04:05",       // ISO sem fuso
	}
	// Adiciona também os formatos de parseDate, caso a hora esteja ausente.
	formats = append(formats, "02/01/2006", "2/1/2006", "2006-01-02")

	for _, format := range formats {
		t, err := time.Parse(format, trimmed)
		if err == nil {
			// t = t.UTC() // Normalizar para UTC
			return &t
		}
	}
	appLogger.Warnf("[Linha %d TDir] Formato de data/hora inválido na coluna '%s': '%s'. Usando NULL.", rowNumForLog, colNameForLog, dateTimeStr)
	return nil
}

// parseDecimalString converte uma string de valor monetário para uma string formatada "XXXX.YY".
// Lida com "," como separador decimal e remove separadores de milhar ".".
// Retorna um ponteiro para a string processada, ou nil se a entrada for vazia.
// Retorna um erro se o parsing falhar mas a string não era vazia.
func parseDecimalString(valStr string, rowNumForLog int, colNameForLog string) (*string, error) {
	trimmed := strings.TrimSpace(valStr)
	if trimmed == "" {
		return nil, nil // Vazio é tratado como NULL no DB para campos opcionais.
	}

	// Normaliza a string: remove espaços, substitui vírgula por ponto decimal, remove pontos de milhar.
	normalizedForDecimal := strings.ReplaceAll(trimmed, " ", "")
	// Se houver vírgula, ela é o separador decimal. Outros pontos são milhares.
	if strings.Contains(normalizedForDecimal, ",") {
		normalizedForDecimal = strings.ReplaceAll(normalizedForDecimal, ".", "")  // Remove pontos de milhar
		normalizedForDecimal = strings.ReplaceAll(normalizedForDecimal, ",", ".") // Converte vírgula para ponto decimal
	} else {
		// Se não há vírgula, mas há múltiplos pontos, pode ser formato europeu ou erro.
		// Assume que se houver mais de um ponto, todos exceto o último (se houver) são de milhar.
		// Esta heurística pode precisar de ajuste dependendo da variabilidade dos dados.
		// Para maior robustez, seria ideal um formato de entrada mais padronizado.
		// A biblioteca shopspring/decimal é flexível, mas a normalização ajuda.
	}

	d, err := decimal.NewFromString(normalizedForDecimal)
	if err != nil {
		// Tenta remover todos os caracteres não numéricos exceto o último ponto/vírgula se for um erro de formato
		// Isso é uma tentativa de recuperação mais agressiva.
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
			appLogger.Warnf("[Linha %d TDir] Valor decimal inválido na coluna '%s': '%s' (tentativas: '%s', '%s'). Erro: %v. Será tratado como NULL ou placeholder.", rowNumForLog, colNameForLog, valStr, normalizedForDecimal, cleanedFurther, err)
			return nil, fmt.Errorf("valor decimal '%s' inválido para coluna '%s'", valStr, colNameForLog)
		}
	}

	// Retorna a string no formato "XXXX.YY" com duas casas decimais.
	formattedDecimalStr := d.StringFixedBank(2)
	return &formattedDecimalStr, nil
}

// ReplaceAll substitui todos os dados na tabela titulos_direitos.
func (r *gormTituloDireitoRepository) ReplaceAll(rawData []models.TituloDireitoFromRow) (insertedCount int, skippedCount int, err error) {
	if len(rawData) == 0 {
		appLogger.Info("ReplaceAll Títulos de Direitos: Nenhum dado bruto fornecido.")
		// Limpa a tabela mesmo se não houver novos dados para inserir.
		if txErr := r.db.Exec("DELETE FROM " + models.DBTituloDireito{}.TableName()).Error; txErr != nil {
			appLogger.Errorf("Erro ao deletar dados antigos de Títulos de Direitos (com input vazio): %v", txErr)
			return 0, 0, appErrors.WrapErrorf(txErr, "falha ao limpar dados antigos de títulos de direitos (GORM)")
		}
		return 0, 0, nil
	}

	dbEntries := make([]models.DBTituloDireito, 0, len(rawData))
	rowsWithPlaceholdersUsed := 0 // Conta linhas onde pelo menos um placeholder foi usado para um campo NOT NULL.

	for i, row := range rawData {
		rowNumForLog := i + 1 // Para logs de linha baseados em 1.
		usedPlaceholderInThisRow := false

		// Pessoa (opcional no CSV, mas pode ser string vazia)
		pessoaStr := strings.TrimSpace(row.Pessoa)
		var pPessoa *string
		if pessoaStr != "" {
			val := truncateString(pessoaStr, varchar255Limit)
			pPessoa = &val
		}

		// CNPJ/CPF (obrigatório no DB)
		cleanedCNPJCPF := models.CleanCNPJ(row.CNPJCPF) // Remove não dígitos.
		if len(cleanedCNPJCPF) > 14 {                   // Trunca se maior que 14 (embora CleanCNPJ deva lidar com isso).
			cleanedCNPJCPF = cleanedCNPJCPF[:14]
		}
		if cleanedCNPJCPF == "" {
			cleanedCNPJCPF = defaultPlaceholderCNPJ
			usedPlaceholderInThisRow = true
			appLogger.Debugf("[Linha %d TDir] CNPJ/CPF vazio, usando placeholder.", rowNumForLog)
		}

		// NumeroEmpresa (obrigatório no DB)
		var numeroEmpresa int
		trimmedNumEmp := strings.TrimSpace(row.NumeroEmpresa)
		if trimmedNumEmp == "" {
			numeroEmpresa = defaultPlaceholderInt
			usedPlaceholderInThisRow = true
			appLogger.Debugf("[Linha %d TDir] NROEMPRESA vazio, usando placeholder.", rowNumForLog)
		} else {
			parsedNum, parseErr := strconv.Atoi(trimmedNumEmp)
			if parseErr != nil {
				appLogger.Warnf("[Linha %d TDir] Valor inválido para NROEMPRESA: '%s'. Usando placeholder %d. Erro: %v", rowNumForLog, row.NumeroEmpresa, defaultPlaceholderInt, parseErr)
				numeroEmpresa = defaultPlaceholderInt
				usedPlaceholderInThisRow = true
			} else {
				numeroEmpresa = parsedNum
			}
		}

		// Titulo (obrigatório no DB, VARCHAR(100))
		tituloStr := strings.TrimSpace(row.Titulo)
		if tituloStr == "" {
			tituloStr = defaultPlaceholderString
			usedPlaceholderInThisRow = true
			appLogger.Debugf("[Linha %d TDir] TÍTULO vazio, usando placeholder.", rowNumForLog)
		}
		tituloStr = truncateString(tituloStr, varchar100Limit)

		// ValorNominal (obrigatório no DB)
		var valorNominalStr string
		parsedVN, vnErr := parseDecimalString(row.ValorNominal, rowNumForLog, "VLRNOMINAL")
		if vnErr != nil || parsedVN == nil { // Erro no parse ou string vazia resultou em nil
			valorNominalStr = defaultPlaceholderDecimalStr
			usedPlaceholderInThisRow = true
			appLogger.Warnf("[Linha %d TDir] VLRNOMINAL inválido ou vazio ('%s'), usando placeholder. Erro: %v", rowNumForLog, row.ValorNominal, vnErr)
		} else {
			valorNominalStr = truncateString(*parsedVN, varchar30Limit)
		}

		// Campos Opcionais (Nullable no DB)
		codigoEspecie := strings.TrimSpace(row.CodigoEspecie)
		var pCodigoEspecie *string
		if codigoEspecie != "" {
			val := truncateString(codigoEspecie, varchar50Limit)
			pCodigoEspecie = &val
		}

		dataVencimento := parseDate(row.DataVencimento, rowNumForLog, "DTAVENCIMENTO")
		dataQuitacao := parseDate(row.DataQuitacao, rowNumForLog, "DTAQUITAÇÃO")
		valorPagoStr, vpErr := parseDecimalString(row.ValorPago, rowNumForLog, "VLRPAGO")
		if vpErr != nil { // Loga erro, mas continua com nil, pois o campo é opcional.
			appLogger.Warnf("[Linha %d TDir] Erro ao parsear VLRPAGO '%s', será NULL no DB. Erro: %v", rowNumForLog, row.ValorPago, vpErr)
			valorPagoStr = nil // Garante que seja nil se o parse falhou
		} else if valorPagoStr != nil { // Trunca se o parse foi ok
			val := truncateString(*valorPagoStr, varchar30Limit)
			valorPagoStr = &val
		}

		operacao := strings.TrimSpace(row.Operacao)
		var pOperacao *string
		if operacao != "" {
			val := truncateString(operacao, varchar50Limit)
			pOperacao = &val
		}

		dataOperacao := parseDate(row.DataOperacao, rowNumForLog, "DTAOPERAÇÃO")
		dataContabiliza := parseDate(row.DataContabiliza, rowNumForLog, "DTACONTABILIZA")
		// DataAlteracaoCSV é a data do arquivo. GORM UpdatedAt é para quando o registro no DB foi alterado.
		dataAlteracaoDoCSV := parseDateTime(row.DataAlteracaoCSV, rowNumForLog, "DTAALTERAÇÃO_CSV")

		observacao := strings.TrimSpace(row.Observacao)
		var pObservacao *string
		if observacao != "" {
			pObservacao = &observacao // TEXT não precisa de truncamento aqui, mas o DB pode ter limite.
		}

		valorOperacaoStr, voErr := parseDecimalString(row.ValorOperacao, rowNumForLog, "VLROPERAÇÃO")
		if voErr != nil {
			appLogger.Warnf("[Linha %d TDir] Erro ao parsear VLROPERAÇÃO '%s', será NULL no DB. Erro: %v", rowNumForLog, row.ValorOperacao, voErr)
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
		dataProgramada := parseDate(row.DataProgramada, rowNumForLog, "DTAPROGRAMADA")

		// Pessoa é usado como critério para pular a linha se estiver vazio.
		// O CSV original parece ter PESSOA como um campo que pode ser nulo,
		// mas a lógica Python pulava se fosse inválido.
		// Aqui, se `pPessoa` for nil (após trim), a linha ainda pode ser incluída
		// se outros campos obrigatórios estiverem ok e o DB permitir Pessoa nula.
		// A lógica Python no `titulo_direito_use_case.py` tinha `if not valid_data.get("PESSOA"): ... skip`.
		// Adaptando: se Pessoa for estritamente necessário:
		if pPessoa == nil || *pPessoa == "" {
			// No entanto, a struct DBTituloDireito tem Pessoa como *string, permitindo nulo.
			// Se a regra de negócio for que Pessoa NÃO PODE ser nulo,
			// então o campo no DBTituloDireito deveria ser `Pessoa string gorm:"not null"`
			// e um placeholder seria usado aqui.
			// Assumindo que Pessoa pode ser nulo no DB, mas se estiver vazio no CSV
			// e for uma condição de "pular linha", o serviço ou use_case faria isso.
			// O repositório tenta processar o que recebe.
			// Para este exemplo, se Pessoa for fundamental para a validade da linha,
			// e o DB não permite nulo, um placeholder deveria ser definido.
			// Se DB permite nulo e CSV está vazio, pPessoa será nil.
		}

		if usedPlaceholderInThisRow {
			rowsWithPlaceholdersUsed++
		}

		dbEntry := models.DBTituloDireito{
			Pessoa:           pPessoa,
			CNPJCPF:          cleanedCNPJCPF,
			NumeroEmpresa:    numeroEmpresa,
			Titulo:           tituloStr,
			CodigoEspecie:    pCodigoEspecie,
			DataVencimento:   dataVencimento,
			DataQuitacao:     dataQuitacao,
			ValorNominal:     valorNominalStr,
			ValorPago:        valorPagoStr,
			Operacao:         pOperacao,
			DataOperacao:     dataOperacao,
			DataContabiliza:  dataContabiliza,
			DataAlteracaoCSV: dataAlteracaoDoCSV, // Armazena a data do arquivo
			Observacao:       pObservacao,
			ValorOperacao:    valorOperacaoStr,
			UsuarioAlteracao: pUsuarioAlteracao,
			EspecieAbatcomp:  pEspecieAbatcomp,
			ObsTitulo:        pObsTitulo,
			ContasQuitacao:   pContasQuitacao,
			DataProgramada:   dataProgramada,
			// CreatedAt/UpdatedAt gerenciados pelo GORM
		}
		dbEntries = append(dbEntries, dbEntry)
	}

	appLogger.Infof("Títulos de Direitos Processados para inserção: %d. Linhas que usaram placeholders para campos NOT NULL: %d.",
		len(dbEntries), rowsWithPlaceholdersUsed)
	// O `skippedCount` original do Python era sobre linhas com `PESSOA` inválida.
	// Essa lógica de pular linhas baseada em um campo específico é melhor no Serviço/Use Case.
	// O repositório tenta persistir o que recebe, aplicando defaults para NOT NULL.
	// Para este exemplo, o `skippedCount` é zero pois o repositório processa todas as linhas fornecidas.
	skippedCount = 0 // Ou ajustar se a lógica de pular linhas for movida para cá.

	// Usar transação para deletar todos os antigos e inserir os novos.
	err = r.db.Transaction(func(tx *gorm.DB) error {
		appLogger.Info("Deletando dados antigos de Títulos de Direitos...")
		if txErr := tx.Exec("DELETE FROM " + models.DBTituloDireito{}.TableName()).Error; txErr != nil {
			return appErrors.WrapErrorf(txErr, "falha ao limpar dados antigos de títulos de direitos (GORM)")
		}
		appLogger.Debugf("Dados antigos de Títulos de Direitos (%s) deletados.", models.DBTituloDireito{}.TableName())

		// Inserir novos dados em lote (batch) para performance.
		if len(dbEntries) > 0 {
			appLogger.Infof("Inserindo %d novos registros de Títulos de Direitos...", len(dbEntries))
			// GORM `CreateInBatches` divide as inserções em lotes.
			// O tamanho do lote (ex: 1000) pode ser ajustado conforme a necessidade.
			if txErr := tx.CreateInBatches(&dbEntries, 1000).Error; txErr != nil {
				return appErrors.WrapErrorf(txErr, "falha ao inserir novos títulos de direitos em lote (GORM)")
			}
			appLogger.Debugf("%d Títulos de Direitos inseridos com sucesso.", len(dbEntries))
		}
		return nil // Commit da transação.
	})

	if err != nil {
		appLogger.Errorf("Erro na transação de ReplaceAll para Títulos de Direitos: %v", err)
		return 0, skippedCount, err // Retorna o erro da transação.
	}

	return len(dbEntries), skippedCount, nil
}
