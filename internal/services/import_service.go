package services

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8" // Para checagem de encoding e remoção de BOM

	"golang.org/x/text/encoding/charmap" // Para Latin-1 (ISO-8859-1)
	"golang.org/x/text/transform"

	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/auth"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/config"
	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/errors"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/models"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/repositories"
)

// FileType é um enum para os tipos de arquivo de importação.
type FileType string

const (
	FileTypeDireitos   FileType = "DIREITOS"
	FileTypeObrigacoes FileType = "OBRIGACOES"
	// Adicionar outros tipos de arquivo conforme necessário.
)

// ImportService define a interface para o serviço de importação.
type ImportService interface {
	// ImportFile processa a importação de um arquivo.
	// Retorna um mapa com resultados (ex: "records_processed") e um erro, se houver.
	ImportFile(filePath string, fileType FileType, userSession *auth.SessionData) (map[string]interface{}, error)

	GetAllImportStatus(userSession *auth.SessionData) ([]models.ImportMetadataPublic, error)
	GetImportStatus(fileType FileType, userSession *auth.SessionData) (*models.ImportMetadataPublic, error)
}

// importServiceImpl é a implementação de ImportService.
type importServiceImpl struct {
	cfg                 *core.Config
	auditLogService     AuditLogService
	permManager         *auth.PermissionManager
	importMetadataRepo  repositories.ImportMetadataRepository
	tituloDireitoRepo   repositories.TituloDireitoRepository
	tituloObrigacaoRepo repositories.TituloObrigacaoRepository
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
		appLogger.Fatalf("Dependências nulas fornecidas para NewImportService (cfg, auditLog, pm, imRepo, tdRepo, toRepo)")
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

// getExpectedHeaders retorna os cabeçalhos esperados para um tipo de arquivo.
func getExpectedHeaders(fileType FileType) ([]string, error) {
	switch fileType {
	case FileTypeDireitos:
		return models.ExpectedHeadersTituloDireito, nil
	case FileTypeObrigacoes:
		return models.ExpectedHeadersTituloObrigacao, nil
	default:
		return nil, fmt.Errorf("tipo de arquivo '%s' não tem cabeçalhos esperados definidos", fileType)
	}
}

// detectAndDecode tenta detectar o encoding (UTF-8 com/sem BOM, Latin-1) e decodifica para UTF-8.
func (s *importServiceImpl) detectAndDecode(rawBytes []byte) ([]byte, string, error) {
	// Remover BOM UTF-8 se presente.
	bomUtf8 := []byte{0xEF, 0xBB, 0xBF}
	if bytes.HasPrefix(rawBytes, bomUtf8) {
		appLogger.Debug("BOM UTF-8 detectado e removido.")
		return bytes.TrimPrefix(rawBytes, bomUtf8), "UTF-8 (com BOM)", nil
	}

	// Verificar se é UTF-8 válido.
	if utf8.Valid(rawBytes) {
		appLogger.Debug("Arquivo detectado como UTF-8 válido (sem BOM).")
		return rawBytes, "UTF-8", nil
	}

	// Tentar decodificar como Latin-1 (ISO-8859-1).
	appLogger.Debug("Arquivo não é UTF-8 válido. Tentando decodificar como Latin-1.")
	decoder := charmap.ISO8859_1.NewDecoder()
	utf8Bytes, _, errTransform := transform.Bytes(decoder, rawBytes)
	if errTransform != nil {
		appLogger.Errorf("Erro ao decodificar arquivo como Latin-1 após falha UTF-8: %v", errTransform)
		return nil, "Desconhecido", fmt.Errorf("arquivo não pôde ser decodificado como UTF-8 ou Latin-1: %w", errTransform)
	}
	appLogger.Info("Arquivo decodificado com sucesso de Latin-1 para UTF-8.")
	return utf8Bytes, "Latin-1 (convertido para UTF-8)", nil
}

// readAndValidateCSV lê o conteúdo do arquivo, valida o cabeçalho e retorna as linhas de dados.
// Retorna as linhas de dados (sem o cabeçalho), o encoding detectado, e um erro.
func (s *importServiceImpl) readAndValidateCSV(filePath string, fileType FileType) ([][]string, string, error) {
	rawBytes, err := os.ReadFile(filePath)
	if err != nil {
		appLogger.Errorf("Erro ao ler arquivo de importação '%s': %v", filePath, err)
		return nil, "", fmt.Errorf("%w: falha ao ler arquivo '%s'", appErrors.ErrResourceLoading, filepath.Base(filePath))
	}
	if len(rawBytes) == 0 {
		appLogger.Warnf("Arquivo de importação '%s' está vazio.", filePath)
		return [][]string{}, "Vazio", nil // Arquivo vazio não é um erro de formato, mas não tem dados.
	}

	utf8Bytes, detectedEncoding, err := s.detectAndDecode(rawBytes)
	if err != nil {
		return nil, detectedEncoding, err // Propaga erro de decodificação.
	}

	// Usar encoding/csv para parsear.
	// O delimitador é ponto e vírgula. LazyQuotes lida com algumas aspas malformadas.
	// TrimLeadingSpace remove espaços antes dos campos.
	csvReader := csv.NewReader(bytes.NewReader(utf8Bytes))
	csvReader.Comma = ';'
	csvReader.LazyQuotes = true
	csvReader.TrimLeadingSpace = true
	// csvReader.FieldsPerRecord = -1 // Permite número variável de campos por linha para pegar erros.

	allRecords, err := csvReader.ReadAll()
	if err != nil {
		// Tenta fornecer mais contexto sobre o erro do CSV.
		if e, ok := err.(*csv.ParseError); ok {
			// Log detalhado do erro de parsing, incluindo a linha (se disponível).
			// A linha original antes do join pode ser útil para depuração, mas não é trivial obter aqui.
			appLogger.Errorf("Erro de parse CSV no arquivo '%s' na linha %d, coluna %d (relativo aos dados processados): %v",
				filepath.Base(filePath), e.Line, e.Column, e.Err)
			return nil, detectedEncoding, fmt.Errorf("%w: arquivo '%s' mal formatado (erro na linha %d, coluna %d): %v",
				appErrors.ErrValidation, filepath.Base(filePath), e.Line, e.Column, e.Err)
		}
		appLogger.Errorf("Erro desconhecido ao parsear CSV do arquivo '%s' (encoding: %s): %v", filepath.Base(filePath), detectedEncoding, err)
		return nil, detectedEncoding, fmt.Errorf("%w: falha ao parsear conteúdo CSV do arquivo '%s'", appErrors.ErrValidation, filepath.Base(filePath))
	}

	if len(allRecords) == 0 {
		appLogger.Warnf("Nenhum registro encontrado no CSV de '%s' após parse (pode ser apenas uma linha de cabeçalho vazia ou comentários).", filepath.Base(filePath))
		return [][]string{}, detectedEncoding, nil
	}

	// Validar cabeçalho.
	headerRow := allRecords[0]
	expectedHeaders, err := getExpectedHeaders(fileType)
	if err != nil { // Deveria ser pego antes, mas checagem de segurança.
		return nil, detectedEncoding, fmt.Errorf("%w: %v", appErrors.ErrConfiguration, err)
	}

	if len(headerRow) != len(expectedHeaders) {
		appLogger.Errorf("Arquivo '%s': número de colunas no cabeçalho (%d) diferente do esperado (%d). Cabeçalho recebido: %v. Esperado: %v",
			filepath.Base(filePath), len(headerRow), len(expectedHeaders), headerRow, expectedHeaders)
		return nil, detectedEncoding, fmt.Errorf("%w: arquivo '%s' tem %d colunas no cabeçalho, esperado %d",
			appErrors.ErrValidation, filepath.Base(filePath), len(headerRow), len(expectedHeaders))
	}

	for i, expected := range expectedHeaders {
		// Comparação case-insensitive e trim de espaços para cabeçalhos.
		if !strings.EqualFold(strings.TrimSpace(headerRow[i]), strings.TrimSpace(expected)) {
			appLogger.Errorf("Arquivo '%s': cabeçalho da coluna %d é '%s', esperado '%s'.",
				filepath.Base(filePath), i+1, headerRow[i], expected)
			return nil, detectedEncoding, fmt.Errorf("%w: arquivo '%s' tem cabeçalho inválido (coluna %d: '%s' != '%s')",
				appErrors.ErrValidation, filepath.Base(filePath), i+1, headerRow[i], expected)
		}
	}

	appLogger.Infof("Arquivo '%s' (encoding: %s) lido. Cabeçalho validado. %d linhas de dados encontradas.",
		filepath.Base(filePath), detectedEncoding, len(allRecords)-1)
	return allRecords[1:], detectedEncoding, nil // Retorna apenas as linhas de dados.
}

// ImportFile processa a importação de um arquivo.
func (s *importServiceImpl) ImportFile(filePath string, fileType FileType, userSession *auth.SessionData) (map[string]interface{}, error) {
	// 1. Verificar Permissão
	if err := s.permManager.CheckPermission(userSession, auth.PermImportExecute, nil); err != nil {
		return nil, err
	}

	// 2. Validações Iniciais do Arquivo
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		appLogger.Errorf("Arquivo de importação não encontrado: %s", filePath)
		return nil, fmt.Errorf("%w: arquivo '%s' não encontrado", appErrors.ErrNotFound, filepath.Base(filePath))
	}

	expectedHeaders, err := getExpectedHeaders(fileType)
	if err != nil { // Se o fileType não for suportado/configurado.
		appLogger.Errorf("Tipo de arquivo de importação inválido ou não configurado: '%s'. Erro: %v", fileType, err)
		return nil, fmt.Errorf("%w: tipo de arquivo '%s' não é suportado ou não tem colunas definidas: %v", appErrors.ErrConfiguration, fileType, err)
	}

	fileName := filepath.Base(filePath)
	appLogger.Infof("Iniciando importação: Tipo='%s', Arquivo='%s', Usuário='%s'", fileType, fileName, userSession.Username)

	// 3. Ler e Validar Arquivo CSV/TXT
	allDataRows, detectedEncoding, err := s.readAndValidateCSV(filePath, fileType)
	if err != nil {
		// `readAndValidateCSV` já loga o erro específico e formata para `appErrors`.
		// Registrar falha na auditoria.
		s.auditLogService.LogAction(models.AuditLogEntry{
			Action:      fmt.Sprintf("IMPORT_%s_FAILED_READ", strings.ToUpper(string(fileType))),
			Description: fmt.Sprintf("Falha ao ler ou validar arquivo '%s' (Encoding: %s): %v", fileName, detectedEncoding, err),
			Severity:    "ERROR",
			Metadata:    map[string]interface{}{"file_type": fileType, "filename": fileName, "error": err.Error()},
		}, userSession)
		return nil, err
	}

	if len(allDataRows) == 0 {
		appLogger.Warnf("Arquivo '%s' (Tipo: %s, Encoding: %s) não contém dados para importar (apenas cabeçalho ou vazio).", fileName, fileType, detectedEncoding)
		// Atualizar metadados para registrar a tentativa de importação de arquivo vazio.
		zeroRecords := 0
		if _, metaErr := s.importMetadataRepo.Upsert(models.ImportMetadataUpsert{
			FileType: string(fileType), OriginalFilename: &fileName, RecordCount: &zeroRecords, ImportedBy: &userSession.Username,
		}); metaErr != nil {
			appLogger.Warnf("Falha ao atualizar metadados para importação de arquivo vazio '%s': %v", fileName, metaErr)
		}
		return map[string]interface{}{
			"status":                  "success_empty_file",
			"records_processed":       0,
			"records_skipped_parsing": 0,
			"records_skipped_repo":    0,
			"message":                 "Arquivo vazio ou contém apenas cabeçalho. Nenhuma linha de dados processada.",
		}, nil
	}

	// 4. Mapear dados para as structs FromRow e chamar o repositório.
	var insertedCount, skippedInRepoCount int
	var repoErr error
	linesSkippedDuringMapping := 0

	switch fileType {
	case FileTypeDireitos:
		direitosData := make([]models.TituloDireitoFromRow, 0, len(allDataRows))
		for i, record := range allDataRows {
			if len(record) != len(expectedHeaders) {
				appLogger.Warnf("[Linha CSV %d, Tipo: %s] Número incorreto de campos: %d, esperado %d. Linha ignorada: %v",
					i+2, fileType, len(record), len(expectedHeaders), record) // i+2 para contar a partir da primeira linha do arquivo
				linesSkippedDuringMapping++
				continue
			}
			// Mapeamento direto de colunas para campos da struct.
			direitosData = append(direitosData, models.TituloDireitoFromRow{
				Pessoa: record[0], CNPJCPF: record[1], NumeroEmpresa: record[2], Titulo: record[3],
				CodigoEspecie: record[4], DataVencimento: record[5], DataQuitacao: record[6], ValorNominal: record[7],
				ValorPago: record[8], Operacao: record[9], DataOperacao: record[10], DataContabiliza: record[11],
				DataAlteracaoCSV: record[12], Observacao: record[13], ValorOperacao: record[14], UsuarioAlteracao: record[15],
				EspecieAbatcomp: record[16], ObsTitulo: record[17], ContasQuitacao: record[18], DataProgramada: record[19],
			})
		}
		if len(direitosData) > 0 { // Só chama o repo se houver dados válidos após o mapeamento.
			insertedCount, skippedInRepoCount, repoErr = s.tituloDireitoRepo.ReplaceAll(direitosData)
		} else if linesSkippedDuringMapping == len(allDataRows) { // Todas as linhas foram puladas.
			repoErr = fmt.Errorf("%w: todas as %d linhas de dados no arquivo '%s' tinham um número incorreto de campos e foram ignoradas", appErrors.ErrValidation, len(allDataRows), fileName)
		}

	case FileTypeObrigacoes:
		obrigacoesData := make([]models.TituloObrigacaoFromRow, 0, len(allDataRows))
		for i, record := range allDataRows {
			if len(record) != len(expectedHeaders) {
				appLogger.Warnf("[Linha CSV %d, Tipo: %s] Número incorreto de campos: %d, esperado %d. Linha ignorada: %v",
					i+2, fileType, len(record), len(expectedHeaders), record)
				linesSkippedDuringMapping++
				continue
			}
			obrigacoesData = append(obrigacoesData, models.TituloObrigacaoFromRow{
				Pessoa: record[0], CNPJCPF: record[1], NumeroEmpresa: record[2], Titulo: record[3],
				CodigoEspecie: record[4], DataVencimento: record[5], DataQuitacao: record[6], ValorNominal: record[7],
				ValorPago: record[8], Operacao: record[9], DataOperacao: record[10], DataContabiliza: record[11],
				DataAlteracaoCSV: record[12], Observacao: record[13], ValorOperacao: record[14], UsuarioAlteracao: record[15],
				EspecieAbatcomp: record[16], ObsTitulo: record[17], ContasQuitacao: record[18], DataProgramada: record[19],
			})
		}
		if len(obrigacoesData) > 0 {
			insertedCount, skippedInRepoCount, repoErr = s.tituloObrigacaoRepo.ReplaceAll(obrigacoesData)
		} else if linesSkippedDuringMapping == len(allDataRows) {
			repoErr = fmt.Errorf("%w: todas as %d linhas de dados no arquivo '%s' tinham um número incorreto de campos e foram ignoradas", appErrors.ErrValidation, len(allDataRows), fileName)
		}

	default:
		// Este caso não deveria ser alcançado se `getExpectedHeaders` for chamado antes.
		repoErr = fmt.Errorf("%w: lógica de importação não implementada para tipo '%s'", appErrors.ErrInternal, fileType)
	}

	if repoErr != nil {
		// Erro já logado pelo repositório ou pelo bloco de mapeamento.
		s.auditLogService.LogAction(models.AuditLogEntry{
			Action:      fmt.Sprintf("IMPORT_%s_FAILED_REPO", strings.ToUpper(string(fileType))),
			Description: fmt.Sprintf("Falha na persistência de dados do arquivo '%s': %v", fileName, repoErr),
			Severity:    "ERROR",
			Metadata:    map[string]interface{}{"file_type": fileType, "filename": fileName, "error": repoErr.Error()},
		}, userSession)
		return nil, repoErr // Propaga o erro do repositório ou de mapeamento.
	}

	// 5. Atualizar Metadados e Logar Sucesso
	if _, metaErr := s.importMetadataRepo.Upsert(models.ImportMetadataUpsert{
		FileType: string(fileType), OriginalFilename: &fileName, RecordCount: &insertedCount, ImportedBy: &userSession.Username,
	}); metaErr != nil {
		appLogger.Warnf("Falha ao atualizar metadados para importação de '%s' (Tipo: %s): %v", fileName, fileType, metaErr)
		// Não falha a operação principal por isso, mas é um aviso importante.
	}

	description := fmt.Sprintf("Arquivo '%s' importado com sucesso. Registros inseridos: %d. Linhas puladas (parsing CSV): %d. Linhas puladas (processamento repositório): %d.",
		fileName, insertedCount, linesSkippedDuringMapping, skippedInRepoCount)
	s.auditLogService.LogAction(models.AuditLogEntry{
		Action:      fmt.Sprintf("IMPORT_%s_SUCCESS", strings.ToUpper(string(fileType))),
		Description: description,
		Severity:    "INFO",
		Metadata: map[string]interface{}{
			"file_type":                  fileType,
			"filename":                   fileName,
			"encoding_detected":          detectedEncoding,
			"total_data_rows_in_file":    len(allDataRows),
			"records_mapped_to_model":    len(allDataRows) - linesSkippedDuringMapping,
			"records_inserted_by_repo":   insertedCount,
			"records_skipped_by_parsing": linesSkippedDuringMapping,
			"records_skipped_by_repo":    skippedInRepoCount,
		},
	}, userSession)

	appLogger.Infof("Importação para Tipo='%s' (Arquivo: '%s') concluída. Inseridos: %d, Pulados (parsing): %d, Pulados (repo): %d.",
		fileType, fileName, insertedCount, linesSkippedDuringMapping, skippedInRepoCount)

	return map[string]interface{}{
		"status":                  "success",
		"records_processed":       insertedCount, // Registros efetivamente no banco.
		"records_skipped_parsing": linesSkippedDuringMapping,
		"records_skipped_repo":    skippedInRepoCount,
		"total_data_rows_in_file": len(allDataRows),
		"message":                 fmt.Sprintf("Importação concluída. %d registros inseridos.", insertedCount),
	}, nil
}

// GetAllImportStatus busca os metadados de todas as importações.
func (s *importServiceImpl) GetAllImportStatus(userSession *auth.SessionData) ([]models.ImportMetadataPublic, error) {
	if err := s.permManager.CheckPermission(userSession, auth.PermImportViewStatus, nil); err != nil {
		return nil, err
	}
	dbMetas, err := s.importMetadataRepo.GetAll()
	if err != nil {
		return nil, err // Erro já logado pelo repo.
	}
	return models.ToImportMetadataPublicList(dbMetas), nil
}

// GetImportStatus busca os metadados de um tipo de importação específico.
func (s *importServiceImpl) GetImportStatus(fileType FileType, userSession *auth.SessionData) (*models.ImportMetadataPublic, error) {
	if err := s.permManager.CheckPermission(userSession, auth.PermImportViewStatus, nil); err != nil {
		return nil, err
	}
	dbMeta, err := s.importMetadataRepo.GetByFileType(string(fileType))
	if err != nil {
		// Se ErrNotFound, o DTO será nil, o que é esperado.
		// Outros erros são propagados.
		if errors.Is(err, appErrors.ErrNotFound) {
			return nil, nil // Indica que não há metadados para este tipo, não um erro de serviço.
		}
		return nil, err
	}
	return models.ToImportMetadataPublic(dbMeta), nil
}
