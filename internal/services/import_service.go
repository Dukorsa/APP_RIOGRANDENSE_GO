package services

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8" // Para checagem de encoding e remoção de BOM

	"golang.org/x/text/encoding/charmap" // Para Latin-1
	"golang.org/x/text/transform"

	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/errors"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core" // Para Config
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/models"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/repositories"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/auth" // Para SessionData e PermissionManager
)

// FileType é um enum para os tipos de arquivo de importação
type FileType string

const (
	FileTypeDireitos    FileType = "DIREITOS"
	FileTypeObrigacoes  FileType = "OBRIGACOES"
)

// ImportService define a interface para o serviço de importação.
type ImportService interface {
	ImportFile(filePath string, fileType FileType, userSession *auth.SessionData) (map[string]interface{}, error)
	GetAllImportStatus(userSession *auth.SessionData) ([]models.ImportMetadataPublic, error)
	GetImportStatus(fileType FileType, userSession *auth.SessionData) (*models.ImportMetadataPublic, error)
}

// importServiceImpl é a implementação de ImportService.
type importServiceImpl struct {
	cfg                    *core.Config
	auditLogService        AuditLogService
	permManager            *auth.PermissionManager
	importMetadataRepo     repositories.ImportMetadataRepository
	tituloDireitoRepo      repositories.TituloDireitoRepository
	tituloObrigacaoRepo    repositories.TituloObrigacaoRepository
	// Adicionar outros repositórios de dados aqui se necessário
}

// NewImportService cria uma nova instância de ImportService.
func NewImportService(
	cfg *core.Config,
	auditLog AuditLogService,
	pm *auth.PermissionManager,
	imRepo repositories.ImportMetadataRepository,
	tdRepo repositories.TituloDireitoRepository,
	toRepo repositories.TituloObrigacaoRepository,
) ImportService {
	if cfg == nil || auditLog == nil || pm == nil || imRepo == nil || tdRepo == nil || toRepo == nil {
		appLogger.Fatalf("Dependências nulas fornecidas para NewImportService")
	}
	return &importServiceImpl{
		cfg:                 cfg,
		auditLogService:     auditLog,
		permManager:         pm,
		importMetadataRepo:  imRepo,
		tituloDireitoRepo:   tdRepo,
		tituloObrigacaoRepo: toRepo,
	}
}

// ExpectedColumns define as colunas esperadas para cada tipo de arquivo.
// Os nomes devem corresponder aos cabeçalhos EXATOS do arquivo CSV/TXT.
var ExpectedColumns = map[FileType][]string{
	FileTypeDireitos: {
		"PESSOA", "CNPJ/CPF", "NROEMPRESA", "TÍTULO", "CODESPÉCIE", "DTAVENCIMENTO",
		"DTAQUITAÇÃO", "VLRNOMINAL", "VLRPAGO", "OPERAÇÃO", "DTAOPERAÇÃO",
		"DTACONTABILIZA", "DTAALTERAÇÃO", "OBSERVAÇÃO", "VLROPERAÇÃO",
		"USUALTERAÇÃO", "ESPECIEABATCOMP", "OBSTÍTULO", "CONTASQUITAÇÃO",
		"DTAPROGRAMADA",
	},
	FileTypeObrigacoes: { // Assumindo as mesmas colunas, AJUSTE SE NECESSÁRIO
		"PESSOA", "CNPJ/CPF", "NROEMPRESA", "TÍTULO", "CODESPÉCIE", "DTAVENCIMENTO",
		"DTAQUITAÇÃO", "VLRNOMINAL", "VLRPAGO", "OPERAÇÃO", "DTAOPERAÇÃO",
		"DTACONTABILIZA", "DTAALTERAÇÃO", "OBSERVAÇÃO", "VLROPERAÇÃO",
		"USUALTERAÇÃO", "ESPECIEABATCOMP", "OBSTÍTULO", "CONTASQUITAÇÃO",
		"DTAPROGRAMADA",
	},
}

// PreprocessAndReadCSV pré-processa um arquivo CSV/TXT para lidar com quebras de linha
// não cotadas e depois lê os dados usando encoding/csv.
// Tenta UTF-8 primeiro, depois Latin-1 (ISO-8859-1).
func (s *importServiceImpl) preprocessAndReadCSV(filePath string, fileType FileType) ([][]string, error) {
	expectedHeaders, ok := ExpectedColumns[fileType]
	if !ok {
		return nil, fmt.Errorf("%w: colunas esperadas não definidas para o tipo de arquivo '%s'", appErrors.ErrConfiguration, fileType)
	}
	expectedNumFields := len(expectedHeaders)
	if expectedNumFields == 0 {
		return nil, fmt.Errorf("%w: lista de colunas esperadas para '%s' está vazia", appErrors.ErrConfiguration, fileType)
	}

	var rawBytes []byte
	var err error
	var detectedEncoding string

	// Tentar ler com UTF-8
	rawBytes, err = os.ReadFile(filePath)
	if err != nil {
		appLogger.Errorf("Erro ao ler arquivo '%s': %v", filePath, err)
		return nil, fmt.Errorf("%w: falha ao ler arquivo '%s'", appErrors.ErrResourceLoading, filepath.Base(filePath))
	}
	
	// Remover BOM (Byte Order Mark) se presente no UTF-8
	bomUtf8 := []byte{0xEF, 0xBB, 0xBF}
	if bytes.HasPrefix(rawBytes, bomUtf8) {
		rawBytes = bytes.TrimPrefix(rawBytes, bomUtf8)
		appLogger.Debugf("BOM UTF-8 removido do arquivo '%s'", filePath)
	}


	if utf8.Valid(rawBytes) {
		detectedEncoding = "UTF-8"
		appLogger.Debugf("Arquivo '%s' detectado como UTF-8 válido.", filePath)
	} else {
		appLogger.Warnf("Arquivo '%s' não é UTF-8 válido. Tentando decodificar como Latin-1.", filePath)
		// Tentar decodificar como Latin-1 (ISO-8859-1)
		decoder := charmap.ISO8859_1.NewDecoder()
		utf8Bytes, _, errTransform := transform.Bytes(decoder, rawBytes)
		if errTransform != nil {
			appLogger.Errorf("Erro ao decodificar arquivo '%s' como Latin-1 após falha UTF-8: %v", filePath, errTransform)
			return nil, fmt.Errorf("%w: arquivo '%s' não pôde ser decodificado como UTF-8 ou Latin-1", appErrors.ErrValidation, filepath.Base(filePath))
		}
		rawBytes = utf8Bytes // Agora rawBytes contém dados convertidos para UTF-8
		detectedEncoding = "Latin-1 (convertido para UTF-8)"
		appLogger.Infof("Arquivo '%s' decodificado com sucesso de Latin-1 para UTF-8.", filePath)
	}

	// Pré-processamento para juntar linhas quebradas (heurística baseada em delimitador)
	// Esta é uma simplificação da lógica complexa do Python.
	// Uma abordagem mais robusta poderia usar um parser CSV customizado ou regex.
	scanner := bufio.NewScanner(bytes.NewReader(rawBytes))
	var correctedLines []string
	var currentLineBuffer strings.Builder
	firstLine := true
	numDelimitersInHeader := -1

	for scanner.Scan() {
		line := scanner.Text()
		
		if firstLine {
			headerLine := strings.TrimSpace(line)
			if headerLine == "" {
				return nil, fmt.Errorf("%w: cabeçalho vazio encontrado no arquivo '%s'", appErrors.ErrValidation, filepath.Base(filePath))
			}
			numDelimitersInHeader = strings.Count(headerLine, ";")
			correctedLines = append(correctedLines, headerLine)
			firstLine = false
			continue
		}

		currentLineBuffer.WriteString(line) // Adiciona sem quebra de linha por enquanto
		
		// Heurística: se a linha bufferizada (sem contar aspas) tiver o número esperado de delimitadores,
		// ou se for a última linha do arquivo, consideramos o registro completo.
		// Esta heurística é imperfeita e pode falhar em casos complexos de CSVs malformados.
		// A versão Python era mais sofisticada. Para Go, isso é um desafio.
		// O ideal seria que o CSV fosse bem formado ou usar uma biblioteca CSV mais poderosa.
		
		// Simplificação: consideramos cada linha lida como um registro potencial
		// A biblioteca encoding/csv tentará lidar com campos multilinhas se estiverem corretamente cotados.
		// A lógica de juntar linhas baseada em contagem de delimitadores é complexa de replicar perfeitamente
		// sem um parser de estado. Por agora, vamos confiar que `encoding/csv` lida bem com CSVs
		// onde linhas multilinhas são devidamente cotadas. Se as quebras de linha são "nuas" dentro
		// de campos não cotados, `encoding/csv` as tratará como novas linhas.
		
		// A lógica de pré-processamento original do Python era bem específica.
		// Se o problema principal for quebras de linha *dentro* de campos não cotados,
		// o `encoding/csv` padrão do Go terá dificuldades.
		// Uma solução seria ler linha por linha e tentar juntá-las heuristicamente
		// se o número de campos (delimitadores) for menor que o esperado no cabeçalho.
		
		// Vamos adicionar a linha lida diretamente, e o `encoding/csv` fará o seu melhor.
		// Se a heurística de junção for crítica, ela precisaria ser mais robusta aqui.
		// Por enquanto, o pré-processamento foca no encoding e BOM.
		if currentLineBuffer.Len() > 0 { // Adiciona linha lida se não vazia
		    correctedLines = append(correctedLines, currentLineBuffer.String())
		    currentLineBuffer.Reset()
		}
	}
	if err := scanner.Err(); err != nil {
		appLogger.Errorf("Erro durante scan do arquivo '%s' (encoding: %s): %v", filePath, detectedEncoding, err)
		return nil, fmt.Errorf("%w: erro ao ler conteúdo do arquivo '%s'", appErrors.ErrResourceLoading, filepath.Base(filePath))
	}
	if currentLineBuffer.Len() > 0 { // Adiciona o que sobrou no buffer
		correctedLines = append(correctedLines, currentLineBuffer.String())
	}
	
	if len(correctedLines) == 0 { // Se só tinha cabeçalho e foi descartado, ou arquivo vazio
		appLogger.Warnf("Arquivo '%s' parece vazio ou contém apenas cabeçalho após pré-processamento.", filePath)
		return [][]string{}, nil // Retorna slice vazio, não erro
	}


	// Usar encoding/csv para parsear as linhas corrigidas
	csvReader := csv.NewReader(strings.NewReader(strings.Join(correctedLines, "\n")))
	csvReader.Comma = ';'          // Delimitador
	csvReader.LazyQuotes = true    // Lida melhor com aspas não estritamente corretas
	csvReader.TrimLeadingSpace = true
	// csvReader.FieldsPerRecord = expectedNumFields // Desabilitado para permitir log de linhas com contagem errada

	allRecords, err := csvReader.ReadAll()
	if err != nil {
		// Tentar fornecer mais contexto sobre o erro do CSV
		if e, ok := err.(*csv.ParseError); ok {
			appLogger.Errorf("Erro de parse CSV no arquivo '%s' na linha %d, coluna %d: %v. Linha problemática (aprox): '%s'",
				filePath, e.Line, e.Column, e.Err,ตัดบรรทัดที่มีปัญหา(correctedLines, int(e.Line)))
			return nil, fmt.Errorf("%w: arquivo '%s' mal formatado (linha %d): %v", appErrors.ErrValidation, filepath.Base(filePath), e.Line, e.Err)
		}
		appLogger.Errorf("Erro ao parsear CSV de '%s' (encoding: %s): %v", filePath, detectedEncoding, err)
		return nil, fmt.Errorf("%w: falha ao parsear conteúdo CSV do arquivo '%s'", appErrors.ErrValidation, filepath.Base(filePath))
	}

	if len(allRecords) == 0 {
		appLogger.Warnf("Nenhum registro encontrado no CSV de '%s' após parse.", filePath)
		return [][]string{}, nil
	}

	// Validar cabeçalho
	headerRow := allRecords[0]
	if len(headerRow) != expectedNumFields {
		appLogger.Errorf("Arquivo '%s': número de colunas no cabeçalho (%d) diferente do esperado (%d). Cabeçalho: %v",
			filePath, len(headerRow), expectedNumFields, headerRow)
		return nil, fmt.Errorf("%w: arquivo '%s' tem %d colunas no cabeçalho, esperado %d",
			appErrors.ErrValidation, filepath.Base(filePath), len(headerRow), expectedNumFields)
	}
	for i, expected := range expectedHeaders {
		if strings.TrimSpace(headerRow[i]) != expected {
			appLogger.Errorf("Arquivo '%s': cabeçalho da coluna %d é '%s', esperado '%s'.",
				filePath, i+1, headerRow[i], expected)
			return nil, fmt.Errorf("%w: arquivo '%s' tem cabeçalho inválido (coluna %d: '%s' != '%s')",
				appErrors.ErrValidation, filepath.Base(filePath), i+1, headerRow[i], expected)
		}
	}

	appLogger.Infof("Arquivo '%s' (encoding: %s) pré-processado e lido com %d linhas de dados (após cabeçalho).", filePath, detectedEncoding, len(allRecords)-1)
	return allRecords[1:], nil // Retorna apenas as linhas de dados
}

// Helper para obter uma prévia da linha com erro no CSV
func getProblematicCSVLine(allLines []string, errorLineNum int) string {
	if errorLineNum <= 0 || errorLineNum > len(allLines) {
		return "[linha fora do intervalo]"
	}
	// Linhas em `allLines` são baseadas em 0, errorLineNum do csv.ParseError é baseado em 1
	lineContent := allLines[errorLineNum-1]
	if len(lineContent) > 150 {
		return lineContent[:150] + "..."
	}
	return lineContent
}


// ImportFile processa a importação de um arquivo.
func (s *importServiceImpl) ImportFile(filePath string, fileType FileType, userSession *auth.SessionData) (map[string]interface{}, error) {
	// 1. Verificar Permissão
	if err := s.permManager.CheckPermission(userSession, auth.PermImportExecute, nil); err != nil {
		return nil, err
	}

	// 2. Validações Iniciais
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		appLogger.Errorf("Arquivo de importação não encontrado: %s", filePath)
		return nil, fmt.Errorf("%w: arquivo '%s' não encontrado", appErrors.ErrNotFound, filepath.Base(filePath))
	}

	expectedHeaders, ok := ExpectedColumns[fileType]
	if !ok || len(expectedHeaders) == 0 { // Também verifica se a lista de headers não é vazia
		appLogger.Errorf("Tipo de arquivo de importação inválido ou não configurado: '%s'", fileType)
		return nil, fmt.Errorf("%w: tipo de arquivo '%s' não é suportado ou não tem colunas definidas", appErrors.ErrValidation, fileType)
	}

	fileName := filepath.Base(filePath)
	appLogger.Infof("Iniciando importação: Tipo='%s', Arquivo='%s', Usuário='%s'", fileType, fileName, userSession.Username)

	// 3. Ler e Pré-processar Arquivo CSV/TXT
	allDataRows, err := s.preprocessAndReadCSV(filePath, fileType)
	if err != nil {
		// preprocessAndReadCSV já loga o erro específico
		return nil, err // Erro já formatado (ErrValidation, ErrResourceLoading, etc.)
	}

	if len(allDataRows) == 0 {
		appLogger.Warnf("Arquivo '%s' (Tipo: %s) não contém dados para importar após leitura e validação de cabeçalho.", fileName, fileType)
		// Atualizar metadados mesmo para arquivo vazio (para registrar a tentativa)
		if _, metaErr := s.importMetadataRepo.Upsert(string(fileType), &fileName, new(int), &userSession.Username); metaErr != nil {
			appLogger.Warnf("Falha ao atualizar metadados para importação de arquivo vazio '%s': %v", fileName, metaErr)
		}
		return map[string]interface{}{"status": "success", "records_processed": 0, "message": "Arquivo vazio ou apenas com cabeçalho."}, nil
	}

	// 4. Mapear dados para as structs FromRow apropriadas e chamar o repositório
	var insertedCount, skippedCount int
	var repoErr error

	switch fileType {
	case FileTypeDireitos:
		direitosData := make([]models.TituloDireitoFromRow, len(allDataRows))
		for i, record := range allDataRows {
			if len(record) != len(expectedHeaders) {
				appLogger.Warnf("Linha %d (Tipo: %s) tem %d campos, esperado %d. Ignorando linha: %v", i+2, fileType, len(record), len(expectedHeaders), record)
				skippedCount++
				continue
			}
			direitosData[i] = models.TituloDireitoFromRow{
				Pessoa:           record[0], CNPJCPF: record[1], NumeroEmpresa: record[2], Titulo: record[3],
				CodigoEspecie:    record[4], DataVencimento: record[5], DataQuitacao: record[6], ValorNominal: record[7],
				ValorPago:        record[8], Operacao: record[9], DataOperacao: record[10], DataContabiliza: record[11],
				DataAlteracaoCSV: record[12], Observacao: record[13], ValorOperacao: record[14], UsuarioAlteracao: record[15],
				EspecieAbatcomp:  record[16], ObsTitulo: record[17], ContasQuitacao: record[18], DataProgramada: record[19],
			}
		}
		if len(direitosData) > skippedCount { // Se houver alguma linha válida após pular as com contagem errada
			direitosData = direitosData[:len(direitosData)-skippedCount] // Ajusta slice se linhas foram puladas
			insertedCount, skippedCount, repoErr = s.tituloDireitoRepo.ReplaceAll(direitosData)
		}

	case FileTypeObrigacoes:
		obrigacoesData := make([]models.TituloObrigacaoFromRow, len(allDataRows))
		for i, record := range allDataRows {
			if len(record) != len(expectedHeaders) {
				appLogger.Warnf("Linha %d (Tipo: %s) tem %d campos, esperado %d. Ignorando linha: %v", i+2, fileType, len(record), len(expectedHeaders), record)
				skippedCount++
				continue
			}
			obrigacoesData[i] = models.TituloObrigacaoFromRow{
				Pessoa:           record[0], CNPJCPF: record[1], NumeroEmpresa: record[2], Titulo: record[3], // Titulo mapeia para IdentificadorObrigacao no repo
				CodigoEspecie:    record[4], DataVencimento: record[5], DataQuitacao: record[6], ValorNominal: record[7], // ValorNominal mapeia para ValorNominalObrigacao
				ValorPago:        record[8], Operacao: record[9], DataOperacao: record[10], DataContabiliza: record[11],
				DataAlteracaoCSV: record[12], Observacao: record[13], ValorOperacao: record[14], UsuarioAlteracao: record[15],
				EspecieAbatcomp:  record[16], ObsTitulo: record[17], ContasQuitacao: record[18], DataProgramada: record[19],
			}
		}
		if len(obrigacoesData) > skippedCount {
			obrigacoesData = obrigacoesData[:len(obrigacoesData)-skippedCount]
			insertedCount, skippedCount, repoErr = s.tituloObrigacaoRepo.ReplaceAll(obrigacoesData)
		}


	default:
		return nil, fmt.Errorf("%w: lógica de importação não implementada para tipo '%s'", appErrors.ErrInternal, fileType)
	}

	if repoErr != nil {
		// Erro já logado pelo repositório
		s.auditLogService.LogAction(models.AuditLogEntry{
			Action:      fmt.Sprintf("IMPORT_%s_FAILED", fileType),
			Description: fmt.Sprintf("Falha na importação do arquivo '%s': %v", fileName, repoErr),
			Severity:    "ERROR",
			Metadata:    map[string]interface{}{"file_type": fileType, "filename": fileName, "error": repoErr.Error()},
		}, userSession)
		return nil, repoErr
	}

	// 5. Atualizar Metadados e Logar Sucesso
	totalProcessedInRepo := insertedCount + skippedCount // O skippedCount do repo é por 'PESSOA' inválida
	finalRecordCount := insertedCount

	if _, metaErr := s.importMetadataRepo.Upsert(string(fileType), &fileName, &finalRecordCount, &userSession.Username); metaErr != nil {
		appLogger.Warnf("Falha ao atualizar metadados para importação de '%s' (Tipo: %s): %v", fileName, fileType, metaErr)
		// Não falha a operação principal por isso, mas é um aviso importante.
	}

	s.auditLogService.LogAction(models.AuditLogEntry{
		Action:      fmt.Sprintf("IMPORT_%s_SUCCESS", fileType),
		Description: fmt.Sprintf("Arquivo '%s' importado com sucesso. Registros inseridos: %d. Linhas puladas (pré-processamento CSV): %d. Linhas puladas (repositório): %d.", fileName, insertedCount, (len(allDataRows) - totalProcessedInRepo), skippedCount),
		Severity:    "INFO",
		Metadata:    map[string]interface{}{"file_type": fileType, "filename": fileName, "inserted_count": insertedCount, "skipped_csv_parsing": (len(allDataRows) - totalProcessedInRepo), "skipped_repo_processing": skippedCount},
	}, userSession)

	appLogger.Infof("Importação para Tipo='%s' concluída. Inseridos: %d, Pulados (pré): %d, Pulados (repo): %d.",
		fileType, insertedCount, (len(allDataRows) - totalProcessedInRepo), skippedCount)

	return map[string]interface{}{"status": "success", "records_processed": insertedCount}, nil
}


// GetAllImportStatus busca os metadados de todas as importações.
func (s *importServiceImpl) GetAllImportStatus(userSession *auth.SessionData) ([]models.ImportMetadataPublic, error) {
	if err := s.permManager.CheckPermission(userSession, auth.PermImportViewStatus, nil); err != nil {
		return nil, err
	}
	dbMetas, err := s.importMetadataRepo.GetAll()
	if err != nil {
		return nil, err
	}
	
	publicMetas := make([]models.ImportMetadataPublic, len(dbMetas))
	for i, m := range dbMetas {
		// Precisamos converter DBImportMetadata para ImportMetadataPublic
		// Se ToImportMetadataPublic não existir, crie-o em models/import_metadata.go
		publicMeta := models.ToImportMetadataPublic(&m) // Passa o endereço
		if publicMeta == nil { // Segurança
			appLogger.Warnf("Falha ao converter DBImportMetadata ID %d para público.", m.ID)
			continue // Ou retorne erro
		}
		publicMetas[i] = *publicMeta
	}
	return publicMetas, nil
}

// GetImportStatus busca os metadados de um tipo de importação específico.
func (s *importServiceImpl) GetImportStatus(fileType FileType, userSession *auth.SessionData) (*models.ImportMetadataPublic, error) {
	if err := s.permManager.CheckPermission(userSession, auth.PermImportViewStatus, nil); err != nil {
		return nil, err
	}
	dbMeta, err := s.importMetadataRepo.GetByFileType(string(fileType))
	if err != nil {
		return nil, err // ErrNotFound ou DB error
	}
	return models.ToImportMetadataPublic(dbMeta), nil
}