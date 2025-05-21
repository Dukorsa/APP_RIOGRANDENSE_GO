package models

import (
	"fmt"
	// "strings" // Pode ser necessário para formatação de CNPJ/CPF se não usar um helper
	"time"

	"github.com/shopspring/decimal" // Usado para manipulação precisa de valores monetários
	// "gorm.io/gorm" // Descomentado se GORM for usado diretamente aqui
)

// DBTituloObrigacao representa um registro na tabela 'titulos_obrigacoes'.
// Similar a DBTituloDireito, mas para obrigações.
type DBTituloObrigacao struct {
	ID uint64 `gorm:"primaryKey;autoIncrement"` // Chave primária interna

	// --- Colunas mapeadas do arquivo de importação ---
	Pessoa        *string `gorm:"type:varchar(255)"`
	CNPJCPF       string  `gorm:"type:varchar(14);not null;index"`
	NumeroEmpresa int     `gorm:"not null;index"`

	// Identificador da Obrigação (anteriormente 'Titulo' em Direitos).
	IdentificadorObrigacao string `gorm:"type:varchar(100);not null;index"`

	CodigoEspecie  *string    `gorm:"type:varchar(50)"`
	DataVencimento *time.Time `gorm:"type:date"`
	DataQuitacao   *time.Time `gorm:"type:date"` // Data de quitação/pagamento da obrigação

	// Valor Nominal da Obrigação (anteriormente 'ValorNominal' em Direitos).
	ValorNominalObrigacao string `gorm:"type:varchar(30);not null"`

	ValorPago *string `gorm:"type:varchar(30)"`

	Operacao        *string    `gorm:"type:varchar(50)"`
	DataOperacao    *time.Time `gorm:"type:date"`
	DataContabiliza *time.Time `gorm:"type:date"`

	// Data de alteração conforme o arquivo CSV.
	DataAlteracaoCSV *time.Time `gorm:"type:date;column:data_alteracao_csv"`

	Observacao       *string    `gorm:"type:text"`
	ValorOperacao    *string    `gorm:"type:varchar(30)"`
	UsuarioAlteracao *string    `gorm:"type:varchar(50)"`
	EspecieAbatcomp  *string    `gorm:"type:varchar(100)"`
	ObsTitulo        *string    `gorm:"type:text"` // Observações específicas da obrigação
	ContasQuitacao   *string    `gorm:"type:text"`
	DataProgramada   *time.Time `gorm:"type:date"`

	// Campos de auditoria padrão do GORM (opcional)
	// CreatedAt time.Time      `gorm:"autoCreateTime"`
	// UpdatedAt time.Time      `gorm:"autoUpdateTime"`
}

// TableName especifica o nome da tabela para GORM.
func (DBTituloObrigacao) TableName() string {
	return "titulos_obrigacoes"
}

// --- Struct para ler dados brutos da linha do arquivo ---

// TituloObrigacaoFromRow representa os dados como lidos diretamente de uma linha do CSV/TXT
// para "Movimento de Títulos - Obrigações".
// Os nomes dos campos aqui devem corresponder aos cabeçalhos do arquivo de obrigações.
// Se os cabeçalhos forem idênticos aos de "Direitos", esta struct será similar a TituloDireitoFromRow,
// mas é mantida separada para clareza e possíveis divergências futuras.
type TituloObrigacaoFromRow struct {
	Pessoa        string // Corresponde a 'PESSOA' no CSV
	CNPJCPF       string // Corresponde a 'CNPJ/CPF'
	NumeroEmpresa string // Corresponde a 'NROEMPRESA'

	// O campo 'TÍTULO' do CSV de obrigações será mapeado para IdentificadorObrigacao.
	Titulo string // Corresponde a 'TÍTULO'

	CodigoEspecie  string // Corresponde a 'CODESPÉCIE'
	DataVencimento string // Corresponde a 'DTAVENCIMENTO' (formato DD/MM/YYYY)
	DataQuitacao   string // Corresponde a 'DTAQUITAÇÃO' (formato DD/MM/YYYY)

	// O campo 'VLRNOMINAL' do CSV de obrigações será mapeado para ValorNominalObrigacao.
	ValorNominal string // Corresponde a 'VLRNOMINAL'

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

// ExpectedHeadersTituloObrigacao define os cabeçalhos esperados para o arquivo de Títulos de Obrigações.
// Usado para validar o formato do arquivo de importação.
// Assumindo que são os mesmos de Títulos de Direitos. Ajuste se necessário.
var ExpectedHeadersTituloObrigacao = []string{
	"PESSOA", "CNPJ/CPF", "NROEMPRESA", "TÍTULO", "CODESPÉCIE", "DTAVENCIMENTO",
	"DTAQUITAÇÃO", "VLRNOMINAL", "VLRPAGO", "OPERAÇÃO", "DTAOPERAÇÃO",
	"DTACONTABILIZA", "DTAALTERAÇÃO", "OBSERVAÇÃO", "VLROPERAÇÃO",
	"USUALTERAÇÃO", "ESPECIEABATCOMP", "OBSTÍTULO", "CONTASQUITAÇÃO",
	"DTAPROGRAMADA",
}

// --- Struct Pública (DTO) para Títulos de Obrigação ---

// TituloObrigacaoPublic representa dados de um título de obrigação formatados para exibição ou API.
type TituloObrigacaoPublic struct {
	ID                     uint64           `json:"id"`
	Pessoa                 *string          `json:"pessoa,omitempty"`
	CNPJCPF                string           `json:"cnpj_cpf"` // Formatado
	NumeroEmpresa          int              `json:"numero_empresa"`
	IdentificadorObrigacao string           `json:"identificador_obrigacao"`
	CodigoEspecie          *string          `json:"codigo_especie,omitempty"`
	DataVencimento         *string          `json:"data_vencimento,omitempty"` // Formatado "DD/MM/YYYY"
	DataQuitacao           *string          `json:"data_quitacao,omitempty"`   // Formatado
	ValorNominalObrigacao  *decimal.Decimal `json:"valor_nominal_obrigacao"`   // Convertido
	ValorPago              *decimal.Decimal `json:"valor_pago,omitempty"`      // Convertido
	Operacao               *string          `json:"operacao,omitempty"`
	DataOperacao           *string          `json:"data_operacao,omitempty"`      // Formatado
	DataContabiliza        *string          `json:"data_contabiliza,omitempty"`   // Formatado
	DataAlteracaoCSV       *string          `json:"data_alteracao_csv,omitempty"` // Formatado
	Observacao             *string          `json:"observacao,omitempty"`
	ValorOperacao          *decimal.Decimal `json:"valor_operacao,omitempty"` // Convertido
	UsuarioAlteracao       *string          `json:"usuario_alteracao,omitempty"`
	EspecieAbatcomp        *string          `json:"especie_abatcomp,omitempty"`
	ObsTitulo              *string          `json:"obs_titulo,omitempty"`
	ContasQuitacao         *string          `json:"contas_quitacao,omitempty"`
	DataProgramada         *string          `json:"data_programada,omitempty"` // Formatado
}

// ToTituloObrigacaoPublic converte DBTituloObrigacao para TituloObrigacaoPublic.
// Realiza conversões de tipo e formatações.
func ToTituloObrigacaoPublic(dbto *DBTituloObrigacao) (*TituloObrigacaoPublic, error) {
	if dbto == nil {
		return nil, nil
	}

	var errVlrNomObrig, errVlrPago, errVlrOperacao error
	var vlrNominalObrig, vlrPago, vlrOperacao *decimal.Decimal

	vlrNominalObrig, errVlrNomObrig = parseDecimalFromString(&dbto.ValorNominalObrigacao) // É not null
	if errVlrNomObrig != nil {
		return nil, fmt.Errorf("erro ao converter ValorNominalObrigacao de DBTituloObrigacao ID %d: %w", dbto.ID, errVlrNomObrig)
	}
	if vlrNominalObrig == nil { // Segurança, não deveria acontecer se not null
		zeroDecimal := decimal.Zero
		vlrNominalObrig = &zeroDecimal
	}

	vlrPago, errVlrPago = parseDecimalFromString(dbto.ValorPago)
	if errVlrPago != nil {
		return nil, fmt.Errorf("erro ao converter ValorPago de DBTituloObrigacao ID %d: %w", dbto.ID, errVlrPago)
	}

	vlrOperacao, errVlrOperacao = parseDecimalFromString(dbto.ValorOperacao)
	if errVlrOperacao != nil {
		return nil, fmt.Errorf("erro ao converter ValorOperacao de DBTituloObrigacao ID %d: %w", dbto.ID, errVlrOperacao)
	}

	// Formatar CNPJ/CPF (reutiliza a lógica de TituloDireitoPublic se for idêntica)
	formattedCNPJ := dbto.CNPJCPF
	if len(dbto.CNPJCPF) == 14 {
		formattedCNPJ = fmt.Sprintf("%s.%s.%s/%s-%s", dbto.CNPJCPF[0:2], dbto.CNPJCPF[2:5], dbto.CNPJCPF[5:8], dbto.CNPJCPF[8:12], dbto.CNPJCPF[12:14])
	} else if len(dbto.CNPJCPF) == 11 {
		formattedCNPJ = fmt.Sprintf("%s.%s.%s-%s", dbto.CNPJCPF[0:3], dbto.CNPJCPF[3:6], dbto.CNPJCPF[6:9], dbto.CNPJCPF[9:11])
	}

	return &TituloObrigacaoPublic{
		ID:                     dbto.ID,
		Pessoa:                 dbto.Pessoa,
		CNPJCPF:                formattedCNPJ,
		NumeroEmpresa:          dbto.NumeroEmpresa,
		IdentificadorObrigacao: dbto.IdentificadorObrigacao,
		CodigoEspecie:          dbto.CodigoEspecie,
		DataVencimento:         formatDatePtr(dbto.DataVencimento),
		DataQuitacao:           formatDatePtr(dbto.DataQuitacao),
		ValorNominalObrigacao:  vlrNominalObrig,
		ValorPago:              vlrPago,
		Operacao:               dbto.Operacao,
		DataOperacao:           formatDatePtr(dbto.DataOperacao),
		DataContabiliza:        formatDatePtr(dbto.DataContabiliza),
		DataAlteracaoCSV:       formatDatePtr(dbto.DataAlteracaoCSV),
		Observacao:             dbto.Observacao,
		ValorOperacao:          vlrOperacao,
		UsuarioAlteracao:       dbto.UsuarioAlteracao,
		EspecieAbatcomp:        dbto.EspecieAbatcomp,
		ObsTitulo:              dbto.ObsTitulo,
		ContasQuitacao:         dbto.ContasQuitacao,
		DataProgramada:         formatDatePtr(dbto.DataProgramada),
	}, nil
}

// ToTituloObrigacaoPublicList converte uma lista de DBTituloObrigacao para TituloObrigacaoPublic.
func ToTituloObrigacaoPublicList(dbtos []*DBTituloObrigacao) ([]*TituloObrigacaoPublic, error) {
	if dbtos == nil {
		return nil, nil
	}
	publicList := make([]*TituloObrigacaoPublic, len(dbtos))
	for i, dbto := range dbtos {
		publicItem, err := ToTituloObrigacaoPublic(dbto)
		if err != nil {
			return nil, fmt.Errorf("erro ao converter item %d da lista de DBTituloObrigacao: %w", i, err)
		}
		publicList[i] = publicItem
	}
	return publicList, nil
}
