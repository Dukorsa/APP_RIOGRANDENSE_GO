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

	"github.com/xuri/excelize/v2" // Para XLSX

	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core"                  // Para Config (ExportDir)
	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/errors" // Para ErrExport
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
)

// DataInput é uma interface para abstrair a fonte dos dados de exportação.
// Isso permite que o exportador trabalhe com diferentes tipos de dados de entrada.
type DataInput interface {
	Headers() ([]string, error) // Retorna os cabeçalhos das colunas
	Rows() ([][]string, error)  // Retorna todas as linhas como slice de slices de string
	RowCount() (int, error)     // Retorna o número de linhas de dados (sem cabeçalho)
	GetSheetName() string       // Nome da planilha (para Excel com múltiplas abas)
}

// SliceDataInput é uma implementação de DataInput para um `[][]string`.
type SliceDataInput struct {
	data      [][]string
	sheetName string
}

// NewSliceDataInput cria um DataInput a partir de um slice de slices de string.
// A primeira linha é considerada o cabeçalho.
func NewSliceDataInput(data [][]string, sheetName string) (*SliceDataInput, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: nenhum dado fornecido para SliceDataInput", appErrors.ErrInvalidInput)
	}
	if sheetName == "" {
		sheetName = "Dados"
	}
	return &SliceDataInput{data: data, sheetName: sheetName}, nil
}

func (s *SliceDataInput) Headers() ([]string, error) {
	if len(s.data) == 0 {
		return []string{}, fmt.Errorf("%w: dados vazios, sem cabeçalhos", appErrors.ErrInvalidInput)
	}
	return s.data[0], nil
}

func (s *SliceDataInput) Rows() ([][]string, error) {
	if len(s.data) <= 1 { // Sem linhas de dados se só tiver cabeçalho ou estiver vazio
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

// StructSliceDataInput é uma implementação de DataInput para um slice de structs.
// Usa reflexão para extrair cabeçalhos e dados.
type StructSliceDataInput struct {
	dataSlice interface{} // Deve ser um slice de structs (ex: []*models.MyStruct)
	sheetName string
	headers   []string // Cache dos cabeçalhos
}

func NewStructSliceDataInput(slice interface{}, sheetName string) (*StructSliceDataInput, error) {
	sliceVal := reflect.ValueOf(slice)
	if sliceVal.Kind() != reflect.Slice {
		return nil, fmt.Errorf("%w: entrada para StructSliceDataInput deve ser um slice, recebido %T", appErrors.ErrInvalidInput, slice)
	}
	if sliceVal.Len() == 0 {
		// Tentar obter cabeçalhos do tipo do slice, mesmo se vazio
		elemType := sliceVal.Type().Elem()
		if elemType.Kind() == reflect.Ptr {
			elemType = elemType.Elem()
		} // Se for slice de ponteiros
		if elemType.Kind() != reflect.Struct {
			return nil, fmt.Errorf("%w: elementos do slice devem ser structs ou ponteiros para structs, recebido %T", appErrors.ErrInvalidInput, sliceVal.Type().Elem())
		}
		headers := make([]string, elemType.NumField())
		for i := 0; i < elemType.NumField(); i++ {
			// Usar tag json como nome da coluna se existir, senão nome do campo
			field := elemType.Field(i)
			jsonTag := field.Tag.Get("json")
			if jsonTag != "" && jsonTag != "-" {
				parts := strings.Split(jsonTag, ",")
				headers[i] = parts[0]
			} else {
				headers[i] = field.Name
			}
		}
		return &StructSliceDataInput{dataSlice: slice, sheetName: sheetName, headers: headers}, nil
	}

	// Se não estiver vazio, pegar cabeçalhos do primeiro elemento
	firstElem := sliceVal.Index(0)
	if firstElem.Kind() == reflect.Ptr {
		firstElem = firstElem.Elem()
	}
	if firstElem.Kind() != reflect.Struct {
		return nil, fmt.Errorf("%w: elementos do slice devem ser structs ou ponteiros para structs, recebido %T", appErrors.ErrInvalidInput, firstElem.Type())
	}
	elemType := firstElem.Type()
	headers := make([]string, elemType.NumField())
	for i := 0; i < elemType.NumField(); i++ {
		field := elemType.Field(i)
		jsonTag := field.Tag.Get("json")
		if jsonTag != "" && jsonTag != "-" {
			parts := strings.Split(jsonTag, ",")
			headers[i] = parts[0]
		} else {
			headers[i] = field.Name
		}
	}
	if sheetName == "" {
		sheetName = "Dados"
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

	for i := 0; i < numRows; i++ {
		elem := sliceVal.Index(i)
		if elem.Kind() == reflect.Ptr {
			elem = elem.Elem()
		} // Desreferencia ponteiro

		rowData := make([]string, numCols)
		for j := 0; j < numCols; j++ {
			fieldVal := elem.Field(j)
			rowData[j] = fmt.Sprint(fieldVal.Interface()) // Converte para string
		}
		rowsData[i] = rowData
	}
	return rowsData, nil
}
func (s *StructSliceDataInput) RowCount() (int, error) {
	return reflect.ValueOf(s.dataSlice).Len(), nil
}
func (s *StructSliceDataInput) GetSheetName() string { return s.sheetName }

// --- Exportador Base e Sanitização ---
var (
	// Sanitização básica, pode ser expandida
	cpfRegex   = regexp.MustCompile(`\b(\d{3}[.-]?\d{3}[.-]?\d{3}-?\d{2})\b`)
	cnpjRegex  = regexp.MustCompile(`\b(\d{2}[.-]?\d{3}[.-]?\d{3}/?\d{4}-?\d{2})\b`)
	emailRegex = regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`)
)

func sanitizeString(s string) string {
	s = cpfRegex.ReplaceAllString(s, "***.***.***-**")
	s = cnpjRegex.ReplaceAllString(s, "**.***.***/****-**")
	s = emailRegex.ReplaceAllString(s, "****@****.***")
	// TODO: Adicionar mais regras de sanitização conforme necessário
	return s
}

func sanitizeData(headers []string, rows [][]string, sanitizeColumns []string) ([][]string, error) {
	if len(sanitizeColumns) == 0 || len(rows) == 0 {
		return rows, nil
	}

	sanitizedRows := make([][]string, len(rows))
	colIndicesToSanitize := make(map[int]bool)

	for _, colName := range sanitizeColumns {
		found := false
		for i, h := range headers {
			if strings.EqualFold(h, colName) { // Case-insensitive match
				colIndicesToSanitize[i] = true
				found = true
				break
			}
		}
		if !found {
			appLogger.Warnf("Coluna de sanitização '%s' não encontrada nos cabeçalhos. Ignorando.", colName)
		}
	}

	if len(colIndicesToSanitize) == 0 {
		return rows, nil // Nenhuma coluna válida para sanitizar
	}

	for i, row := range rows {
		newRow := make([]string, len(row))
		copy(newRow, row) // Copia a linha
		for colIdx := range row {
			if colIndicesToSanitize[colIdx] {
				newRow[colIdx] = sanitizeString(row[colIdx])
			}
		}
		sanitizedRows[i] = newRow
	}
	return sanitizedRows, nil
}

// ExportOptions contém opções para a exportação.
type ExportOptions struct {
	SheetName       string
	CreateBackup    bool
	Sanitize        bool
	SanitizeColumns []string // Nomes das colunas a serem sanitizadas
	// Para Excel:
	HeaderStyle  *excelize.Style
	DataStyle    *excelize.Style
	ColumnWidths map[string]float64 // map[ColunaLetraOuNome]largura
}

// ExportToCSV exporta dados para um arquivo CSV.
func ExportToCSV(input DataInput, outputPath string, cfg *core.Config, opts *ExportOptions) (string, error) {
	finalPath := resolveOutputPath(outputPath, cfg.ExportDir, ".csv")

	if opts == nil {
		opts = &ExportOptions{}
	}
	if opts.CreateBackup && fileExists(finalPath) {
		if err := createBackup(finalPath); err != nil {
			return "", appErrors.WrapErrorf(err, "falha ao criar backup para CSV")
		}
	}

	file, err := os.Create(finalPath)
	if err != nil {
		return "", appErrors.WrapErrorf(err, "falha ao criar arquivo CSV '%s'", finalPath)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	writer.Comma = ';' // Delimitador ponto e vírgula

	headers, err := input.Headers()
	if err != nil {
		return "", err
	}
	if err := writer.Write(headers); err != nil {
		return "", appErrors.WrapErrorf(err, "falha ao escrever cabeçalhos CSV")
	}

	rows, err := input.Rows()
	if err != nil {
		return "", err
	}

	if opts.Sanitize {
		rows, err = sanitizeData(headers, rows, opts.SanitizeColumns)
		if err != nil {
			return "", err
		} // Erro já formatado
	}

	for _, row := range rows {
		if err := writer.Write(row); err != nil {
			// Logar e talvez continuar, ou retornar erro na primeira falha?
			appLogger.Errorf("Erro ao escrever linha no CSV: %v. Linha: %v", err, row)
			// return "", appErrors.WrapErrorf(err, "falha ao escrever linha CSV") // Para falha rápida
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return "", appErrors.WrapErrorf(err, "falha ao dar flush no writer CSV")
	}
	appLogger.Infof("Dados exportados para CSV: %s", finalPath)
	return finalPath, nil
}

// ExportToXLSX exporta dados para um arquivo XLSX (Excel).
func ExportToXLSX(inputs []DataInput, outputPath string, cfg *core.Config, globalOpts *ExportOptions) (string, error) {
	finalPath := resolveOutputPath(outputPath, cfg.ExportDir, ".xlsx")

	if globalOpts == nil {
		globalOpts = &ExportOptions{}
	}
	if globalOpts.CreateBackup && fileExists(finalPath) {
		if err := createBackup(finalPath); err != nil {
			return "", appErrors.WrapErrorf(err, "falha ao criar backup para XLSX")
		}
	}

	xlsx := excelize.NewFile()
	defer func() {
		if err := xlsx.Close(); err != nil {
			appLogger.Errorf("Erro ao fechar arquivo XLSX: %v", err)
		}
	}()

	defaultHeaderStyle, _ := xlsx.NewStyle(&excelize.Style{
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"#1A659E"}, Pattern: 1},
		Font:      &excelize.Font{Color: "FFFFFF", Bold: true, Size: 11, Family: "Segoe UI"},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
		Border: []excelize.Border{
			{Type: "bottom", Color: "FFFFFF", Style: 1},
		},
	})
	// defaultDataStyle, _ := xlsx.NewStyle(&excelize.Style{...}) // Definir estilo de dados se necessário

	for i, input := range inputs {
		sheetName := input.GetSheetName()
		if sheetName == "" {
			sheetName = fmt.Sprintf("Planilha%d", i+1)
		}
		// Excelize cria "Sheet1" por padrão. Se for a primeira, renomeia, senão, cria nova.
		if i == 0 && xlsx.GetSheetName(0) == "Sheet1" { // GetSheetName(0) é o índice da primeira aba
			xlsx.SetSheetName("Sheet1", sheetName)
		} else {
			_, err := xlsx.NewSheet(sheetName)
			if err != nil {
				return "", appErrors.WrapErrorf(err, "falha ao criar nova planilha '%s'", sheetName)
			}
		}
		xlsx.SetActiveSheet(xlsx.GetSheetIndex(sheetName))

		headers, err := input.Headers()
		if err != nil {
			return "", err
		}

		// Escrever Cabeçalhos e aplicar estilo
		for colIdx, headerVal := range headers {
			cell, _ := excelize.CoordinatesToCellName(colIdx+1, 1)
			xlsx.SetCellValue(sheetName, cell, headerVal)
			xlsx.SetCellStyle(sheetName, cell, cell, defaultHeaderStyle)
		}

		rows, err := input.Rows()
		if err != nil {
			return "", err
		}

		if globalOpts.Sanitize {
			rows, err = sanitizeData(headers, rows, globalOpts.SanitizeColumns)
			if err != nil {
				return "", err
			}
		}

		// Escrever Dados
		for rowIdx, rowData := range rows {
			for colIdx, cellData := range rowData {
				cell, _ := excelize.CoordinatesToCellName(colIdx+1, rowIdx+2) // +2 porque cabeçalho está na linha 1

				// Tentar converter para número se possível, para melhor formatação no Excel
				if num, errConv := strconv.ParseFloat(strings.Replace(cellData, ",", ".", -1), 64); errConv == nil {
					xlsx.SetCellValue(sheetName, cell, num)
				} else if b, errConvBool := strconv.ParseBool(cellData); errConvBool == nil {
					xlsx.SetCellValue(sheetName, cell, b)
				} else {
					xlsx.SetCellValue(sheetName, cell, cellData)
				}
				// TODO: Aplicar defaultDataStyle se definido
			}
		}

		// Ajustar largura das colunas (exemplo básico)
		for colIdx := range headers {
			colLetter, _ := excelize.ColumnNumberToName(colIdx + 1)
			// xlsx.SetColWidth(sheetName, colLetter, colLetter, 20) // Largura fixa de exemplo
			// Para auto-ajuste real, seria preciso calcular a largura do conteúdo.
			// A biblioteca excelize pode ter helpers ou você precisaria iterar e medir.
		}
		// TODO: Aplicar globalOpts.ColumnWidths se fornecido
	}

	// Remover a planilha "Sheet1" se ela não foi usada e não é a única
	if len(inputs) > 0 && xlsx.GetSheetName(0) == "Sheet1" && inputs[0].GetSheetName() != "Sheet1" {
		// Verificar se Sheet1 está vazia antes de deletar
		rows, _ := xlsx.GetRows("Sheet1")
		if len(rows) == 0 {
			xlsx.DeleteSheet("Sheet1")
		}
	} else if len(inputs) == 0 { // Se nenhum input foi dado, mas o arquivo precisa ser criado
		xlsx.SetCellValue("Sheet1", "A1", "Nenhum dado para exportar.")
	}

	if err := xlsx.SaveAs(finalPath); err != nil {
		return "", appErrors.WrapErrorf(err, "falha ao salvar arquivo XLSX '%s'", finalPath)
	}
	appLogger.Infof("Dados exportados para XLSX: %s", finalPath)
	return finalPath, nil
}

// --- Funções Utilitárias Internas ---
func resolveOutputPath(path string, defaultDir string, defaultExt string) string {
	p := filepath.Clean(path)
	if !filepath.IsAbs(p) {
		absDefaultDir, _ := filepath.Abs(defaultDir)
		p = filepath.Join(absDefaultDir, p)
	}

	// Garante que o diretório pai exista
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		appLogger.Warnf("Não foi possível criar diretório de exportação '%s': %v. Usando diretório atual.", dir, err)
		// Fallback para diretório atual se a criação falhar
		p = filepath.Base(p)
	}

	ext := filepath.Ext(p)
	if ext == "" {
		p += defaultExt
	} else if !strings.EqualFold(ext, defaultExt) {
		// Se a extensão for diferente, mas existir, pode ser intenção do usuário.
		// Ou pode-se forçar defaultExt: p = strings.TrimSuffix(p, ext) + defaultExt
		appLogger.Debugf("Extensão de arquivo '%s' é diferente da padrão '%s' para o formato.", ext, defaultExt)
	}
	return p
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

func createBackup(path string) error {
	timestamp := time.Now().Format("20060102_150405")
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	backupPath := fmt.Sprintf("%s_backup_%s%s", base, timestamp, ext)

	err := os.Rename(path, backupPath)
	if err == nil {
		appLogger.Infof("Backup criado: %s", backupPath)
	}
	return err
}
