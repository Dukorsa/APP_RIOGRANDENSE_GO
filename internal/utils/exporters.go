package utils

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2" // Para exportação para XLSX

	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core"                  // Para Config (ExportDir)
	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/errors" // Para ErrExport, ErrInvalidInput
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
)

// DataInput é uma interface para abstrair a fonte dos dados de exportação.
// Permite que os exportadores (CSV, XLSX) trabalhem com diferentes tipos de
// estruturas de dados de entrada de forma genérica.
type DataInput interface {
	// Headers retorna os nomes das colunas (cabeçalhos) para a exportação.
	Headers() ([]string, error)
	// Rows retorna todas as linhas de dados, onde cada linha é um slice de strings.
	Rows() ([][]string, error)
	// RowCount retorna o número total de linhas de dados (sem contar o cabeçalho).
	RowCount() (int, error)
	// GetSheetName retorna o nome sugerido para a planilha (usado em XLSX).
	GetSheetName() string
}

// --- Implementações de DataInput ---

// SliceDataInput é uma implementação de DataInput para um `[][]string` (slice de slices de string).
// A primeira linha do slice de dados é considerada o cabeçalho.
type SliceDataInput struct {
	data      [][]string // Formato: [ [header1, header2], [row1col1, row1col2], [row2col1, row2col2] ]
	sheetName string
}

// NewSliceDataInput cria um DataInput a partir de um `[][]string`.
// `data` deve conter pelo menos uma linha (o cabeçalho).
// `sheetName` é o nome da planilha para exportações XLSX.
func NewSliceDataInput(data [][]string, sheetName string) (*SliceDataInput, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: nenhum dado fornecido para SliceDataInput (necessário pelo menos cabeçalho)", appErrors.ErrInvalidInput)
	}
	if strings.TrimSpace(sheetName) == "" {
		sheetName = "DadosExportados" // Nome padrão se não fornecido.
	}
	return &SliceDataInput{data: data, sheetName: sheetName}, nil
}

func (s *SliceDataInput) Headers() ([]string, error) {
	// `data` já foi validado em `NewSliceDataInput` para não ser vazio.
	return s.data[0], nil
}

func (s *SliceDataInput) Rows() ([][]string, error) {
	if len(s.data) <= 1 { // Se só tiver cabeçalho ou estiver vazio (embora `New` cheque vazio).
		return [][]string{}, nil
	}
	return s.data[1:], nil
}

func (s *SliceDataInput) RowCount() (int, error) {
	if len(s.data) <= 1 {
		return 0, nil
	}
	return len(s.data) - 1, nil
}
func (s *SliceDataInput) GetSheetName() string { return s.sheetName }

// StructSliceDataInput é uma implementação de DataInput para um slice de structs (ex: `[]*models.MyStruct`).
// Usa reflexão para extrair cabeçalhos (baseados em tags JSON ou nomes de campo) e dados.
type StructSliceDataInput struct {
	dataSlice interface{} // Deve ser um slice de structs (ex: `[]*MyStruct` ou `[]MyStruct`).
	sheetName string
	headers   []string // Cache dos cabeçalhos extraídos.
}

// NewStructSliceDataInput cria um DataInput a partir de um slice de structs.
// `slice` é a coleção de dados. `sheetName` para XLSX.
func NewStructSliceDataInput(slice interface{}, sheetName string) (*StructSliceDataInput, error) {
	sliceVal := reflect.ValueOf(slice)
	if sliceVal.Kind() != reflect.Slice {
		return nil, fmt.Errorf("%w: entrada para StructSliceDataInput deve ser um slice, recebido %T", appErrors.ErrInvalidInput, slice)
	}

	var elemType reflect.Type
	if sliceVal.Len() == 0 {
		// Se o slice estiver vazio, tenta obter o tipo do elemento do tipo do slice.
		elemType = sliceVal.Type().Elem()
	} else {
		// Se não estiver vazio, obtém o tipo do primeiro elemento.
		elemType = sliceVal.Index(0).Type()
	}

	// Se for um slice de ponteiros para structs (ex: []*MyStruct), pega o tipo da struct.
	if elemType.Kind() == reflect.Ptr {
		elemType = elemType.Elem()
	}
	if elemType.Kind() != reflect.Struct {
		return nil, fmt.Errorf("%w: elementos do slice devem ser structs ou ponteiros para structs, recebido %s", appErrors.ErrInvalidInput, elemType.Name())
	}

	// Extrai os cabeçalhos dos nomes dos campos da struct ou das tags JSON.
	numFields := elemType.NumField()
	headers := make([]string, 0, numFields)
	for i := 0; i < numFields; i++ {
		field := elemType.Field(i)
		// Pula campos não exportados (minúsculos) ou com tag json:"-"
		if !field.IsExported() {
			continue
		}

		jsonTag := field.Tag.Get("json")
		if jsonTag == "-" {
			continue
		} // Pula campos explicitamente ignorados pela tag json.

		headerName := field.Name // Usa o nome do campo como padrão.
		if jsonTag != "" {
			parts := strings.Split(jsonTag, ",") // Remove opções como ",omitempty".
			if parts[0] != "" {
				headerName = parts[0]
			}
		}
		headers = append(headers, headerName)
	}
	if len(headers) == 0 {
		return nil, fmt.Errorf("%w: nenhuma coluna exportável encontrada na struct %s (verifique campos exportados e tags json)", appErrors.ErrInvalidInput, elemType.Name())
	}

	if strings.TrimSpace(sheetName) == "" {
		sheetName = elemType.Name() // Usa o nome da struct como nome da planilha se não fornecido.
		if sheetName == "" {
			sheetName = "DadosExportados"
		}
	}
	return &StructSliceDataInput{dataSlice: slice, sheetName: sheetName, headers: headers}, nil
}

func (s *StructSliceDataInput) Headers() ([]string, error) {
	return s.headers, nil
}

func (s *StructSliceDataInput) Rows() ([][]string, error) {
	sliceVal := reflect.ValueOf(s.dataSlice)
	numRows := sliceVal.Len()
	if numRows == 0 {
		return [][]string{}, nil
	}
	numCols := len(s.headers)
	rowsData := make([][]string, numRows)

	// Para mapear o índice do header para o índice do campo na struct (caso pulemos campos não exportados)
	headerToFieldIndex := make(map[string]int)
	elemType := sliceVal.Type().Elem()
	if elemType.Kind() == reflect.Ptr {
		elemType = elemType.Elem()
	}

	currentHeaderIndex := 0
	for i := 0; i < elemType.NumField(); i++ {
		field := elemType.Field(i)
		if !field.IsExported() || field.Tag.Get("json") == "-" {
			continue
		}
		// Certifica que o header em s.headers[currentHeaderIndex] corresponde ao campo atual.
		// Isso é mais seguro se a lógica de extração de headers e dados divergir.
		// No entanto, a construção atual de `s.headers` já deve estar correta.
		headerToFieldIndex[s.headers[currentHeaderIndex]] = i
		currentHeaderIndex++
	}

	for i := 0; i < numRows; i++ {
		elem := sliceVal.Index(i)
		if elem.Kind() == reflect.Ptr {
			elem = elem.Elem() // Desreferencia ponteiro para acessar campos da struct.
		}

		rowData := make([]string, numCols)
		for j, headerName := range s.headers {
			fieldIndex, ok := headerToFieldIndex[headerName]
			if !ok { // Não deveria acontecer se os headers foram gerados corretamente.
				rowData[j] = "ERRO_CAMPO_NAO_ENCONTRADO"
				continue
			}
			fieldVal := elem.Field(fieldIndex)
			// Formata o valor do campo para string.
			// Lida com tipos comuns como time.Time, ponteiros, etc.
			rowData[j] = formatFieldValue(fieldVal)
		}
		rowsData[i] = rowData
	}
	return rowsData, nil
}

// formatFieldValue converte um reflect.Value para sua representação string apropriada.
func formatFieldValue(fieldVal reflect.Value) string {
	if !fieldVal.IsValid() {
		return ""
	}

	// Lida com ponteiros (ex: *string, *time.Time, *int)
	if fieldVal.Kind() == reflect.Ptr {
		if fieldVal.IsNil() {
			return ""
		} // Retorna string vazia para ponteiros nulos.
		fieldVal = fieldVal.Elem() // Desreferencia para obter o valor real.
	}

	// Formatação específica para tipos comuns.
	switch val := fieldVal.Interface().(type) {
	case time.Time:
		// Formato de data/hora comum para exportação (pode ser configurável).
		// "02/01/2006 15:04:05" (BR) ou "2006-01-02 15:04:05" (ISO)
		if val.IsZero() {
			return ""
		} // String vazia para time.Time zero.
		return val.Format("02/01/2006 15:04:05")
	case bool:
		if val {
			return "Sim"
		} // Ou "Verdadeiro" / "True"
		return "Não" // Ou "Falso" / "False"
	// Adicionar outros tipos se necessário (ex: decimal.Decimal).
	// case decimal.Decimal:
	// 	return val.StringFixedBank(2) // Formato com 2 casas decimais
	default:
		return fmt.Sprint(fieldVal.Interface()) // Conversão padrão para string.
	}
}

func (s *StructSliceDataInput) RowCount() (int, error) {
	return reflect.ValueOf(s.dataSlice).Len(), nil
}
func (s *StructSliceDataInput) GetSheetName() string { return s.sheetName }

// --- Sanitização de Dados ---
var (
	// Regex para identificar e mascarar CPFs.
	cpfRegex = regexp.MustCompile(`\b(\d{3})[.-]?(\d{3})[.-]?(\d{3})-?(\d{2})\b`)
	// Regex para identificar e mascarar CNPJs.
	cnpjRegex = regexp.MustCompile(`\b(\d{2})[.-]?(\d{3})[.-]?(\d{3})/?(\d{4})-?(\d{2})\b`)
	// Regex para identificar e mascarar e-mails.
	emailRegex = regexp.MustCompile(`\b([A-Za-z0-9._%+-]+)@([A-Za-z0-9.-]+\.[A-Z|a-z]{2,})\b`)
)

// sanitizeString aplica regras de mascaramento a uma string.
func sanitizeString(s string) string {
	s = cpfRegex.ReplaceAllString(s, "$1.***.***-$4")     // Mantém primeiro e último grupo do CPF.
	s = cnpjRegex.ReplaceAllString(s, "$1.***.***/$4-$5") // Mantém partes do CNPJ.
	s = emailRegex.ReplaceAllString(s, "****@$2")         // Mascara parte local do e-mail.
	// Adicionar mais regras de sanitização conforme necessário (ex: telefones, endereços).
	return s
}

// sanitizeData aplica sanitização a colunas específicas das linhas de dados.
func sanitizeData(headers []string, rows [][]string, sanitizeColumns []string) ([][]string, error) {
	if len(sanitizeColumns) == 0 || len(rows) == 0 {
		return rows, nil // Nenhuma sanitização a ser feita.
	}

	sanitizedRows := make([][]string, len(rows))
	colIndicesToSanitize := make(map[int]bool)

	// Mapeia nomes de colunas para seus índices.
	for _, colName := range sanitizeColumns {
		found := false
		for i, h := range headers {
			if strings.EqualFold(h, colName) { // Comparação case-insensitive.
				colIndicesToSanitize[i] = true
				found = true
				break
			}
		}
		if !found {
			appLogger.Warnf("Coluna de sanitização '%s' não encontrada nos cabeçalhos. Sanitização para esta coluna será ignorada.", colName)
		}
	}

	if len(colIndicesToSanitize) == 0 {
		return rows, nil // Nenhuma coluna válida encontrada para sanitizar.
	}

	// Aplica a sanitização nas colunas identificadas.
	for i, row := range rows {
		newRow := make([]string, len(row))
		copy(newRow, row) // Cria uma cópia da linha para modificar.
		for colIdx, cellValue := range row {
			if colIndicesToSanitize[colIdx] {
				newRow[colIdx] = sanitizeString(cellValue)
			}
		}
		sanitizedRows[i] = newRow
	}
	return sanitizedRows, nil
}

// ExportOptions contém opções para a exportação de arquivos.
type ExportOptions struct {
	CreateBackup    bool     // Se true, cria backup do arquivo existente antes de sobrescrever.
	Sanitize        bool     // Se true, aplica sanitização aos dados.
	SanitizeColumns []string // Nomes das colunas a serem sanitizadas (se `Sanitize` for true).

	// Opções específicas para XLSX (Excel):
	HeaderStyle  *excelize.Style    // Estilo para a linha de cabeçalho.
	DataStyle    *excelize.Style    // Estilo padrão para as células de dados.
	ColumnWidths map[string]float64 // map[ColunaLetraOuNome]largura (ex: "A" -> 20.0).
	// AutoFilterRange string      // Ex: "A1:C10" para aplicar autofiltro.
}

// ExportToCSV exporta dados para um arquivo CSV.
// `input` fornece os dados. `outputPath` é o nome do arquivo (pode ser relativo).
// `cfg` para obter `ExportDir`. `opts` para opções de exportação.
func ExportToCSV(input DataInput, outputPath string, cfg *core.Config, opts *ExportOptions) (string, error) {
	finalPath := resolveOutputPath(outputPath, cfg.ExportDir, ".csv")

	if opts == nil {
		opts = &ExportOptions{}
	} // Garante que `opts` não seja nil.

	if opts.CreateBackup && fileExists(finalPath) {
		if err := createBackup(finalPath); err != nil {
			// Não fatal, mas logar o aviso. Opcionalmente, poderia retornar erro.
			appLogger.Warnf("Falha ao criar backup para CSV '%s': %v. Prosseguindo com a sobrescrita.", finalPath, err)
		}
	}

	file, err := os.Create(finalPath)
	if err != nil {
		return "", appErrors.WrapErrorf(err, "falha ao criar arquivo CSV '%s'", finalPath)
	}
	defer file.Close()

	// Usar delimitador ponto e vírgula (;) para melhor compatibilidade com Excel em PT-BR.
	writer := csv.NewWriter(file)
	writer.Comma = ';'

	headers, err := input.Headers()
	if err != nil {
		return "", appErrors.WrapErrorf(err, "falha ao obter cabeçalhos para CSV")
	}
	if err := writer.Write(headers); err != nil {
		return "", appErrors.WrapErrorf(err, "falha ao escrever cabeçalhos no CSV")
	}

	rows, err := input.Rows()
	if err != nil {
		return "", appErrors.WrapErrorf(err, "falha ao obter linhas de dados para CSV")
	}

	if opts.Sanitize {
		rows, err = sanitizeData(headers, rows, opts.SanitizeColumns)
		if err != nil {
			return "", appErrors.WrapErrorf(err, "falha ao sanitizar dados para CSV")
		}
	}

	// Escreve todas as linhas de dados.
	if err := writer.WriteAll(rows); err != nil {
		// WriteAll já faz Flush.
		appLogger.Errorf("Erro ao escrever todas as linhas no CSV: %v.", err)
		return "", appErrors.WrapErrorf(err, "falha ao escrever dados no CSV")
	}

	// Flush final (embora WriteAll já faça).
	// writer.Flush()
	// if err := writer.Error(); err != nil {
	// 	return "", appErrors.WrapErrorf(err, "falha ao dar flush no writer CSV")
	// }

	appLogger.Infof("Dados exportados com sucesso para CSV: %s", finalPath)
	return finalPath, nil
}

// ExportToXLSX exporta dados para um arquivo XLSX (Excel).
// `inputs` é um slice de `DataInput` para permitir múltiplas planilhas no mesmo arquivo.
// `outputPath`, `cfg`, `globalOpts` são similares a `ExportToCSV`.
func ExportToXLSX(inputs []DataInput, outputPath string, cfg *core.Config, globalOpts *ExportOptions) (string, error) {
	if len(inputs) == 0 {
		return "", fmt.Errorf("%w: nenhum dado de entrada fornecido para exportação XLSX", appErrors.ErrInvalidInput)
	}
	finalPath := resolveOutputPath(outputPath, cfg.ExportDir, ".xlsx")

	if globalOpts == nil {
		globalOpts = &ExportOptions{}
	}

	if globalOpts.CreateBackup && fileExists(finalPath) {
		if err := createBackup(finalPath); err != nil {
			appLogger.Warnf("Falha ao criar backup para XLSX '%s': %v. Prosseguindo.", finalPath, err)
		}
	}

	xlsx := excelize.NewFile() // Cria um novo arquivo Excel.
	defer func() {
		if err := xlsx.Close(); err != nil { // Importante para liberar recursos.
			appLogger.Errorf("Erro ao fechar arquivo XLSX durante o defer: %v", err)
		}
	}()

	// Estilo padrão para cabeçalhos (pode ser sobrescrito por `globalOpts.HeaderStyle`).
	headerStyle := globalOpts.HeaderStyle
	if headerStyle == nil { // Define um estilo padrão se não fornecido.
		style, _ := xlsx.NewStyle(&excelize.Style{
			Fill:      excelize.Fill{Type: "pattern", Color: []string{"#1A659E"}, Pattern: 1}, // Azul escuro
			Font:      &excelize.Font{Color: "FFFFFF", Bold: true, Size: 11, Family: "Calibri"},
			Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: false},
			Border: []excelize.Border{
				{Type: "bottom", Color: "FFFFFF", Style: 2}, // Linha dupla branca abaixo
			},
		})
		headerStyle = &style // Usa o ponteiro para o estilo criado.
	}
	// Estilo padrão para dados (pode ser sobrescrito por `globalOpts.DataStyle`).
	// dataStyle := globalOpts.DataStyle
	// if dataStyle == nil { ... }

	firstSheet := true
	for _, input := range inputs {
		sheetName := input.GetSheetName()
		// Limpa e trunca o nome da planilha para atender aos limites do Excel (31 caracteres, sem caracteres inválidos).
		sheetName = sanitizeSheetName(sheetName)

		var currentSheetIndex int
		var err error
		if firstSheet {
			// Excelize cria "Sheet1" por padrão. Renomeia-a para a primeira planilha.
			xlsx.SetSheetName("Sheet1", sheetName)
			currentSheetIndex, err = xlsx.GetSheetIndex("Sheet1") //Pega o novo nome da sheet
			if err != nil {                                       // Segurança, não deveria falhar.
				currentSheetIndex = xlsx.GetActiveSheetIndex()
			}

			firstSheet = false
		} else {
			currentSheetIndex, err = xlsx.NewSheet(sheetName)
			if err != nil {
				return "", appErrors.WrapErrorf(err, "falha ao criar nova planilha '%s'", sheetName)
			}
		}
		xlsx.SetActiveSheet(currentSheetIndex) // Define a planilha atual como ativa.

		headers, err := input.Headers()
		if err != nil {
			return "", appErrors.WrapErrorf(err, "falha ao obter cabeçalhos para planilha '%s'", sheetName)
		}

		// Escreve Cabeçalhos e aplica estilo.
		for colIdx, headerVal := range headers {
			cell, _ := excelize.CoordinatesToCellName(colIdx+1, 1) // Linha 1 para cabeçalhos.
			xlsx.SetCellValue(sheetName, cell, headerVal)
			if headerStyle != nil {
				xlsx.SetCellStyle(sheetName, cell, cell, *headerStyle)
			}
		}

		rows, err := input.Rows()
		if err != nil {
			return "", appErrors.WrapErrorf(err, "falha ao obter linhas para planilha '%s'", sheetName)
		}

		if globalOpts.Sanitize {
			rows, err = sanitizeData(headers, rows, globalOpts.SanitizeColumns)
			if err != nil {
				return "", appErrors.WrapErrorf(err, "falha ao sanitizar dados para planilha '%s'", sheetName)
			}
		}

		// Escreve Dados.
		for rowIdx, rowData := range rows {
			for colIdx, cellData := range rowData {
				cell, _ := excelize.CoordinatesToCellName(colIdx+1, rowIdx+2) // +2 porque cabeçalho está na linha 1.
				// Tenta converter para número ou booleano se possível, para melhor formatação no Excel.
				if num, errConv := strconv.ParseFloat(strings.Replace(cellData, ",", ".", -1), 64); errConv == nil {
					xlsx.SetCellValue(sheetName, cell, num)
				} else if b, errConvBool := strconv.ParseBool(cellData); errConvBool == nil {
					xlsx.SetCellValue(sheetName, cell, b)
				} else {
					xlsx.SetCellValue(sheetName, cell, cellData) // Salva como string.
				}
				// TODO: Aplicar `dataStyle` se definido.
			}
		}

		// Ajustar largura das colunas (opcional, pode ser feito com `globalOpts.ColumnWidths`).
		// Ou um auto-ajuste básico (excelize pode ter melhorias para isso).
		// for colIdx := range headers {
		// 	colLetter, _ := excelize.ColumnNumberToName(colIdx + 1)
		// 	xlsx.SetColWidth(sheetName, colLetter, colLetter, 20) // Largura fixa de exemplo.
		// }
		if globalOpts.ColumnWidths != nil {
			for colRef, width := range globalOpts.ColumnWidths {
				// ColRef pode ser "A", "B", ou um nome de header que precisa ser mapeado para letra.
				// Por simplicidade, assume que são letras de coluna.
				xlsx.SetColWidth(sheetName, colRef, colRef, width)
			}
		}
		// Aplicar autofiltro se definido.
		// if globalOpts.AutoFilterRange != "" {
		// 	xlsx.AutoFilter(sheetName, globalOpts.AutoFilterRange, []excelize.AutoFilterOptions{})
		// }
	}

	// Se "Sheet1" original não foi usada e ainda existe, e não é a única planilha, remove-a.
	// Esta lógica pode precisar de ajuste se a primeira planilha de `inputs` for nomeada "Sheet1".
	// A lógica atual renomeia "Sheet1" para o nome da primeira planilha de `inputs`.
	// Se `inputs` estiver vazio, cria um arquivo com uma "Sheet1" vazia ou com mensagem.
	if len(inputs) == 0 {
		xlsx.SetCellValue("Sheet1", "A1", "Nenhum dado para exportar.")
	}

	if err := xlsx.SaveAs(finalPath); err != nil {
		return "", appErrors.WrapErrorf(err, "falha ao salvar arquivo XLSX '%s'", finalPath)
	}
	appLogger.Infof("Dados exportados com sucesso para XLSX: %s", finalPath)
	return finalPath, nil
}

// --- Funções Utilitárias Internas ---

// resolveOutputPath constrói o caminho absoluto para o arquivo de saída,
// garantindo que o diretório exista e aplicando uma extensão padrão se necessário.
func resolveOutputPath(path string, defaultDir string, defaultExt string) string {
	p := filepath.Clean(path)
	if !filepath.IsAbs(p) {
		// Garante que defaultDir seja absoluto antes de juntar.
		absDefaultDir, err := filepath.Abs(defaultDir)
		if err != nil { // Fallback se não conseguir resolver defaultDir.
			appLogger.Warnf("Não foi possível resolver o caminho absoluto para o diretório padrão de exportação '%s': %v. Usando diretório atual.", defaultDir, err)
			absDefaultDir = "." // Usa diretório atual como fallback.
		}
		p = filepath.Join(absDefaultDir, p)
	}

	// Garante que o diretório pai exista.
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		appLogger.Warnf("Não foi possível criar diretório de exportação '%s': %v. Arquivo será salvo no diretório atual.", dir, err)
		// Fallback para salvar no diretório atual se a criação do diretório de destino falhar.
		p = filepath.Base(p) // Apenas o nome do arquivo.
	}

	ext := filepath.Ext(p)
	if ext == "" { // Se nenhuma extensão for fornecida, adiciona a padrão.
		p += defaultExt
	} else if !strings.EqualFold(ext, defaultExt) {
		// Se a extensão fornecida for diferente da padrão para o formato,
		// pode ser intenção do usuário ou um erro. Loga um aviso.
		// Opcionalmente, poderia forçar a `defaultExt`.
		appLogger.Debugf("Extensão de arquivo de saída '%s' é diferente da padrão '%s' para o formato de exportação.", ext, defaultExt)
	}
	return p
}

// fileExists verifica se um arquivo existe no caminho especificado.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	// `os.IsNotExist` retorna true se o erro for que o arquivo não existe.
	// Portanto, `!os.IsNotExist(err)` é true se o arquivo existe ou se ocorreu outro erro.
	// Uma checagem mais precisa seria `err == nil`.
	return err == nil
}

// createBackup renomeia um arquivo existente para criar um backup com timestamp.
func createBackup(path string) error {
	timestamp := time.Now().Format("20060102_150405") // Formato YYYYMMDD_HHMMSS.
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	backupPath := fmt.Sprintf("%s_backup_%s%s", base, timestamp, ext)

	err := os.Rename(path, backupPath)
	if err == nil {
		appLogger.Infof("Backup do arquivo existente criado: %s", backupPath)
	} else {
		appLogger.Errorf("Falha ao criar backup de '%s' para '%s': %v", path, backupPath, err)
	}
	return err
}

// sanitizeSheetName limpa o nome da planilha para conformidade com Excel.
// Limita a 31 caracteres e remove caracteres inválidos.
func sanitizeSheetName(name string) string {
	if name == "" {
		return "PlanilhaPadrao"
	}

	// Caracteres inválidos no Excel: \ / ? * [ ] :
	// Substitui por underscore ou remove.
	invalidChars := regexp.MustCompile(`[\\/?*[\]:]`)
	sanitized := invalidChars.ReplaceAllString(name, "_")

	// Limita o comprimento a 31 caracteres.
	if len(sanitized) > 31 {
		sanitized = sanitized[:31]
	}
	// Garante que não comece ou termine com apóstrofo (embora não seja estritamente inválido, pode causar problemas).
	sanitized = strings.Trim(sanitized, "'")
	if sanitized == "" {
		return "PlanilhaRenomeada"
	} // Se ficou vazio após sanitização.

	return sanitized
}
