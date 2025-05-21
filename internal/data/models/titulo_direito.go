package models

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal" // Usado para manipulação precisa de valores monetários
	// "gorm.io/gorm" // Descomentado se GORM for usado diretamente aqui
)

// DBTituloDireito representa um registro na tabela 'titulos_direitos'.
// Os nomes das colunas e tipos devem corresponder à definição da tabela no banco de dados
// e aos dados esperados do arquivo de importação.
type DBTituloDireito struct {
	ID uint64 `gorm:"primaryKey;autoIncrement"` // Chave primária interna

	// --- Colunas mapeadas do arquivo de importação ---
	// O asterisco indica que o campo pode ser nulo no banco de dados.

	Pessoa        *string `gorm:"type:varchar(255)"` // Mapeia para 'PESSOA'
	CNPJCPF       string  `gorm:"type:varchar(14);not null;index"`
	NumeroEmpresa int     `gorm:"not null;index"`                   // Mapeia para 'NROEMPRESA'
	Titulo        string  `gorm:"type:varchar(100);not null;index"` // Mapeia para 'TÍTULO'
	CodigoEspecie *string `gorm:"type:varchar(50)"`                 // Mapeia para 'CODESPÉCIE'

	// Datas são armazenadas como `time.Time` e mapeadas para o tipo DATE ou DATETIME do banco.
	DataVencimento *time.Time `gorm:"type:date"` // Mapeia para 'DTAVENCIMENTO'
	DataQuitacao   *time.Time `gorm:"type:date"` // Mapeia para 'DTAQUITAÇÃO'

	// Valores monetários são armazenados como strings para manter a precisão exata
	// lida do arquivo. A conversão para/de `decimal.Decimal` ocorre na lógica de serviço/repositório
	// antes da persistência ou ao preparar para exibição/cálculo.
	ValorNominal string  `gorm:"type:varchar(30);not null"` // Mapeia para 'VLRNOMINAL' (ex: "1234.56")
	ValorPago    *string `gorm:"type:varchar(30)"`          // Mapeia para 'VLRPAGO'

	Operacao        *string    `gorm:"type:varchar(50)"` // Mapeia para 'OPERAÇÃO'
	DataOperacao    *time.Time `gorm:"type:date"`        // Mapeia para 'DTAOPERAÇÃO'
	DataContabiliza *time.Time `gorm:"type:date"`        // Mapeia para 'DTACONTABILIZA'

	// DataAlteracaoCSV representa o valor da coluna 'DTAALTERAÇÃO' do arquivo CSV.
	// O campo UpdatedAt do GORM (`gorm:"autoUpdateTime"`) é geralmente usado para
	// rastrear quando o registro no banco foi modificado pela aplicação.
	// Se 'DTAALTERAÇÃO' do CSV for uma data de modificação externa, ela deve ser armazenada.
	DataAlteracaoCSV *time.Time `gorm:"type:date;column:data_alteracao_csv"` // Nome explícito da coluna

	Observacao       *string `gorm:"type:text"`        // Mapeia para 'OBSERVAÇÃO'
	ValorOperacao    *string `gorm:"type:varchar(30)"` // Mapeia para 'VLROPERAÇÃO'
	UsuarioAlteracao *string `gorm:"type:varchar(50)"` // Mapeia para 'USUALTERAÇÃO'

	// 'ESPECIEABATCOMP' e 'OBSTÍTULO' são campos de texto adicionais.
	EspecieAbatcomp *string `gorm:"type:varchar(100)"`
	ObsTitulo       *string `gorm:"type:text"`

	ContasQuitacao *string    `gorm:"type:text"` // Mapeia para 'CONTASQUITAÇÃO'
	DataProgramada *time.Time `gorm:"type:date"` // Mapeia para 'DTAPROGRAMADA'

	// Campos de auditoria padrão do GORM (opcional, se não gerenciados explicitamente)
	// CreatedAt time.Time      `gorm:"autoCreateTime"`
	// UpdatedAt time.Time      `gorm:"autoUpdateTime"`
	// DeletedAt gorm.DeletedAt `gorm:"index"`
}

// TableName especifica o nome da tabela para GORM.
func (DBTituloDireito) TableName() string {
	return "titulos_direitos"
}

// --- Structs para Transferência de Dados (DTO) ---

// TituloDireitoPublic representa dados de um título de direito formatados para exibição ou API.
// Converte valores string do DB para tipos mais apropriados para a UI (ex: decimal.Decimal).
type TituloDireitoPublic struct {
	ID               uint64           `json:"id"`
	Pessoa           *string          `json:"pessoa,omitempty"`
	CNPJCPF          string           `json:"cnpj_cpf"` // CNPJ formatado XXX.XXX.XXX/XXXX-XX
	NumeroEmpresa    int              `json:"numero_empresa"`
	Titulo           string           `json:"titulo"`
	CodigoEspecie    *string          `json:"codigo_especie,omitempty"`
	DataVencimento   *string          `json:"data_vencimento,omitempty"` // Formatado como "DD/MM/YYYY"
	DataQuitacao     *string          `json:"data_quitacao,omitempty"`   // Formatado
	ValorNominal     *decimal.Decimal `json:"valor_nominal"`             // Convertido para decimal
	ValorPago        *decimal.Decimal `json:"valor_pago,omitempty"`      // Convertido para decimal
	Operacao         *string          `json:"operacao,omitempty"`
	DataOperacao     *string          `json:"data_operacao,omitempty"`      // Formatado
	DataContabiliza  *string          `json:"data_contabiliza,omitempty"`   // Formatado
	DataAlteracaoCSV *string          `json:"data_alteracao_csv,omitempty"` // Formatado
	Observacao       *string          `json:"observacao,omitempty"`
	ValorOperacao    *decimal.Decimal `json:"valor_operacao,omitempty"` // Convertido
	UsuarioAlteracao *string          `json:"usuario_alteracao,omitempty"`
	EspecieAbatcomp  *string          `json:"especie_abatcomp,omitempty"`
	ObsTitulo        *string          `json:"obs_titulo,omitempty"`
	ContasQuitacao   *string          `json:"contas_quitacao,omitempty"`
	DataProgramada   *string          `json:"data_programada,omitempty"` // Formatado
}

// Helper para converter string de valor para *decimal.Decimal.
// Retorna nil se a string for vazia, ou erro se o parsing falhar.
func parseDecimalFromString(valStr *string) (*decimal.Decimal, error) {
	if valStr == nil || *valStr == "" {
		return nil, nil
	}
	// A string no DB já deve estar no formato "XXXX.YY".
	d, err := decimal.NewFromString(*valStr)
	if err != nil {
		return nil, fmt.Errorf("falha ao converter valor '%s' para decimal: %w", *valStr, err)
	}
	return &d, nil
}

// Helper para formatar *time.Time para string "DD/MM/YYYY".
// Retorna nil se o tempo for nil.
func formatDatePtr(t *time.Time) *string {
	if t == nil || t.IsZero() { // IsZero também verifica se é o valor zero de time.Time
		return nil
	}
	s := t.Format("02/01/2006")
	return &s
}

// ToTituloDireitoPublic converte DBTituloDireito para TituloDireitoPublic.
// Realiza conversões de tipo e formatações necessárias.
func ToTituloDireitoPublic(dbtd *DBTituloDireito) (*TituloDireitoPublic, error) {
	if dbtd == nil {
		return nil, nil
	}

	var errVlrNominal, errVlrPago, errVlrOperacao error
	var vlrNominal, vlrPago, vlrOperacao *decimal.Decimal

	vlrNominal, errVlrNominal = parseDecimalFromString(&dbtd.ValorNominal) // ValorNominal é not null
	if errVlrNominal != nil {
		return nil, fmt.Errorf("erro ao converter ValorNominal de DBTituloDireito ID %d: %w", dbtd.ID, errVlrNominal)
	}
	if vlrNominal == nil { // Segurança, não deveria acontecer se not null
		zeroDecimal := decimal.Zero
		vlrNominal = &zeroDecimal
	}

	vlrPago, errVlrPago = parseDecimalFromString(dbtd.ValorPago)
	if errVlrPago != nil {
		return nil, fmt.Errorf("erro ao converter ValorPago de DBTituloDireito ID %d: %w", dbtd.ID, errVlrPago)
	}

	vlrOperacao, errVlrOperacao = parseDecimalFromString(dbtd.ValorOperacao)
	if errVlrOperacao != nil {
		return nil, fmt.Errorf("erro ao converter ValorOperacao de DBTituloDireito ID %d: %w", dbtd.ID, errVlrOperacao)
	}

	// Formatar CNPJ
	formattedCNPJ := dbtd.CNPJCPF
	if len(dbtd.CNPJCPF) == 14 { // Simples formatação se for CNPJ
		formattedCNPJ = fmt.Sprintf("%s.%s.%s/%s-%s", dbtd.CNPJCPF[0:2], dbtd.CNPJCPF[2:5], dbtd.CNPJCPF[5:8], dbtd.CNPJCPF[8:12], dbtd.CNPJCPF[12:14])
	} else if len(dbtd.CNPJCPF) == 11 { // Formatação se for CPF
		formattedCNPJ = fmt.Sprintf("%s.%s.%s-%s", dbtd.CNPJCPF[0:3], dbtd.CNPJCPF[3:6], dbtd.CNPJCPF[6:9], dbtd.CNPJCPF[9:11])
	}

	return &TituloDireitoPublic{
		ID:               dbtd.ID,
		Pessoa:           dbtd.Pessoa,
		CNPJCPF:          formattedCNPJ,
		NumeroEmpresa:    dbtd.NumeroEmpresa,
		Titulo:           dbtd.Titulo,
		CodigoEspecie:    dbtd.CodigoEspecie,
		DataVencimento:   formatDatePtr(dbtd.DataVencimento),
		DataQuitacao:     formatDatePtr(dbtd.DataQuitacao),
		ValorNominal:     vlrNominal,
		ValorPago:        vlrPago,
		Operacao:         dbtd.Operacao,
		DataOperacao:     formatDatePtr(dbtd.DataOperacao),
		DataContabiliza:  formatDatePtr(dbtd.DataContabiliza),
		DataAlteracaoCSV: formatDatePtr(dbtd.DataAlteracaoCSV),
		Observacao:       dbtd.Observacao,
		ValorOperacao:    vlrOperacao,
		UsuarioAlteracao: dbtd.UsuarioAlteracao,
		EspecieAbatcomp:  dbtd.EspecieAbatcomp,
		ObsTitulo:        dbtd.ObsTitulo,
		ContasQuitacao:   dbtd.ContasQuitacao,
		DataProgramada:   formatDatePtr(dbtd.DataProgramada),
	}, nil
}

// ToTituloDireitoPublicList converte uma lista de DBTituloDireito para TituloDireitoPublic.
func ToTituloDireitoPublicList(dbtds []*DBTituloDireito) ([]*TituloDireitoPublic, error) {
	if dbtds == nil {
		return nil, nil
	}
	publicList := make([]*TituloDireitoPublic, len(dbtds))
	for i, dbtd := range dbtds {
		publicItem, err := ToTituloDireitoPublic(dbtd)
		if err != nil {
			return nil, fmt.Errorf("erro ao converter item %d da lista de DBTituloDireito: %w", i, err)
		}
		publicList[i] = publicItem
	}
	return publicList, nil
}

// TituloDireitoFromRow representa os dados como lidos diretamente de uma linha do arquivo de importação.
// Os campos são todos strings para lidar com a leitura bruta do arquivo e possíveis valores vazios/mal formatados.
// A conversão para os tipos corretos (int, time.Time, decimal.Decimal representada como string)
// é feita durante o processamento da importação no repositório.
type TituloDireitoFromRow struct {
	Pessoa           string // Corresponde a 'PESSOA'
	CNPJCPF          string // Corresponde a 'CNPJ/CPF'
	NumeroEmpresa    string // Corresponde a 'NROEMPRESA'
	Titulo           string // Corresponde a 'TÍTULO'
	CodigoEspecie    string // Corresponde a 'CODESPÉCIE'
	DataVencimento   string // Corresponde a 'DTAVENCIMENTO' (formato DD/MM/YYYY)
	DataQuitacao     string // Corresponde a 'DTAQUITAÇÃO' (formato DD/MM/YYYY)
	ValorNominal     string // Corresponde a 'VLRNOMINAL' (formato numérico, ex: 1234,56)
	ValorPago        string // Corresponde a 'VLRPAGO'
	Operacao         string // Corresponde a 'OPERAÇÃO'
	DataOperacao     string // Corresponde a 'DTAOPERAÇÃO' (formato DD/MM/YYYY)
	DataContabiliza  string // Corresponde a 'DTACONTABILIZA' (formato DD/MM/YYYY)
	DataAlteracaoCSV string // Corresponde a 'DTAALTERAÇÃO' (formato DD/MM/YYYY HH:MM:SS ou similar)
	Observacao       string // Corresponde a 'OBSERVAÇÃO'
	ValorOperacao    string // Corresponde a 'VLROPERAÇÃO'
	UsuarioAlteracao string // Corresponde a 'USUALTERAÇÃO'
	EspecieAbatcomp  string // Corresponde a 'ESPECIEABATCOMP'
	ObsTitulo        string // Corresponde a 'OBSTÍTULO'
	ContasQuitacao   string // Corresponde a 'CONTASQUITAÇÃO'
	DataProgramada   string // Corresponde a 'DTAPROGRAMADA' (formato DD/MM/YYYY)
}

// ExpectedHeadersTituloDireito define os cabeçalhos esperados para o arquivo de Títulos de Direitos.
// Usado para validar o formato do arquivo de importação.
var ExpectedHeadersTituloDireito = []string{
	"PESSOA", "CNPJ/CPF", "NROEMPRESA", "TÍTULO", "CODESPÉCIE", "DTAVENCIMENTO",
	"DTAQUITAÇÃO", "VLRNOMINAL", "VLRPAGO", "OPERAÇÃO", "DTAOPERAÇÃO",
	"DTACONTABILIZA", "DTAALTERAÇÃO", "OBSERVAÇÃO", "VLROPERAÇÃO",
	"USUALTERAÇÃO", "ESPECIEABATCOMP", "OBSTÍTULO", "CONTASQUITAÇÃO",
	"DTAPROGRAMADA",
}
