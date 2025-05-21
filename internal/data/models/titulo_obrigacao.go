package models

import (
	"time"
	// "github.com/shopspring/decimal" // Descomente se usar para manipulação
	// "gorm.io/gorm" // Se usando GORM
)

// DBTituloObrigacao representa um registro na tabela 'titulos_obrigacoes'.
// A estrutura é muito similar à DBTituloDireito, com possíveis renomeações semânticas.
type DBTituloObrigacao struct {
	ID uint64 `gorm:"primaryKey;autoIncrement"` // Chave primária interna

	Pessoa        *string `gorm:"type:varchar(255)"`
	CNPJCPF       string  `gorm:"type:varchar(14);not null;index"`
	NumeroEmpresa int     `gorm:"not null;index"`

	// Campo renomeado para clareza semântica em relação a obrigações
	IdentificadorObrigacao string `gorm:"type:varchar(100);not null;index"` // Era 'Titulo' em Direitos

	CodigoEspecie  *string    `gorm:"type:varchar(50)"`
	DataVencimento *time.Time `gorm:"type:date"`
	DataQuitacao   *time.Time `gorm:"type:date"` // Data de quitação/pagamento da obrigação

	// Campo renomeado para clareza
	ValorNominalObrigacao string `gorm:"type:varchar(30);not null"` // Era 'ValorNominal' em Direitos

	ValorPago *string `gorm:"type:varchar(30)"`

	Operacao        *string    `gorm:"type:varchar(50)"`
	DataOperacao    *time.Time `gorm:"type:date"`
	DataContabiliza *time.Time `gorm:"type:date"`
	DataAlteracao   *time.Time `gorm:"autoUpdateTime"` // Pode ser gerenciado pelo GORM/banco

	Observacao       *string    `gorm:"type:text"`
	ValorOperacao    *string    `gorm:"type:varchar(30)"`
	UsuarioAlteracao *string    `gorm:"type:varchar(50)"`
	EspecieAbatcomp  *string    `gorm:"type:varchar(100)"`
	ObsTitulo        *string    `gorm:"type:text"` // Observações específicas da obrigação
	ContasQuitacao   *string    `gorm:"type:text"`
	DataProgramada   *time.Time `gorm:"type:date"`

	// Campos de auditoria do GORM (opcional)
	// CreatedAt time.Time
	// UpdatedAt time.Time
	// DeletedAt gorm.DeletedAt `gorm:"index"`
}

// TableName especifica o nome da tabela para GORM.
func (DBTituloObrigacao) TableName() string {
	return "titulos_obrigacoes"
}

// --- Struct para ler dados brutos da linha do arquivo ---

// TituloObrigacaoFromRow representa os dados como lidos diretamente de uma linha do CSV/TXT
// para "Movimento de Títulos - Obrigações".
// Os nomes dos campos aqui devem corresponder aos cabeçalhos do arquivo de obrigações.
// Se os cabeçalhos forem idênticos aos de "Direitos", esta struct será idêntica a TituloDireitoFromRow,
// mas é bom mantê-las separadas para o caso de divergências futuras.
type TituloObrigacaoFromRow struct {
	Pessoa        string // Corresponde a 'PESSOA' no CSV
	CNPJCPF       string // Corresponde a 'CNPJ/CPF'
	NumeroEmpresa string // Corresponde a 'NROEMPRESA'

	// Se o cabeçalho no arquivo de Obrigações ainda for 'TÍTULO':
	Titulo string // Corresponde a 'TÍTULO' (será mapeado para IdentificadorObrigacao)
	// Se o cabeçalho for diferente, ajuste o nome do campo aqui.

	CodigoEspecie  string // Corresponde a 'CODESPÉCIE'
	DataVencimento string // Corresponde a 'DTAVENCIMENTO'
	DataQuitacao   string // Corresponde a 'DTAQUITAÇÃO'

	// Se o cabeçalho no arquivo de Obrigações ainda for 'VLRNOMINAL':
	ValorNominal string // Corresponde a 'VLRNOMINAL' (será mapeado para ValorNominalObrigacao)
	// Se o cabeçalho for diferente, ajuste.

	ValorPago        string // Corresponde a 'VLRPAGO'
	Operacao         string // Corresponde a 'OPERAÇÃO'
	DataOperacao     string // Corresponde a 'DTAOPERAÇÃO'
	DataContabiliza  string // Corresponde a 'DTACONTABILIZA'
	DataAlteracaoCSV string // Corresponde a 'DTAALTERAÇÃO'
	Observacao       string // Corresponde a 'OBSERVAÇÃO'
	ValorOperacao    string // Corresponde a 'VLROPERAÇÃO'
	UsuarioAlteracao string // Corresponde a 'USUALTERAÇÃO'
	EspecieAbatcomp  string // Corresponde a 'ESPECIEABATCOMP'
	ObsTitulo        string // Corresponde a 'OBSTÍTULO'
	ContasQuitacao   string // Corresponde a 'CONTASQUITAÇÃO'
	DataProgramada   string // Corresponde a 'DTAPROGRAMADA'
}

// --- Struct Pública (Opcional, para API/UI) ---
// Se você precisar de uma struct específica para exibir Títulos de Obrigação,
// similar à TituloDireitoPublic, você pode defini-la aqui.
// Por enquanto, vou omiti-la para brevidade, assumindo que a DBTituloObrigacao
// (ou uma conversão dela com valores decimais e datas formatadas) pode ser usada.
/*
type TituloObrigacaoPublic struct {
	ID                     uint64           `json:"id"`
	Pessoa                 *string          `json:"pessoa,omitempty"`
	CNPJCPF                string           `json:"cnpj_cpf"`
	NumeroEmpresa          int              `json:"numero_empresa"`
	IdentificadorObrigacao string           `json:"identificador_obrigacao"`
	CodigoEspecie          *string          `json:"codigo_especie,omitempty"`
	DataVencimento         *string          `json:"data_vencimento,omitempty"` // Formatado
	// ... outros campos ...
	ValorNominalObrigacao  decimal.Decimal  `json:"valor_nominal_obrigacao"`
	// ...
}

func ToTituloObrigacaoPublic(dbto *DBTituloObrigacao) (*TituloObrigacaoPublic, error) {
    // Lógica de conversão similar a ToTituloDireitoPublic,
    // ajustando para os campos de DBTituloObrigacao (ex: IdentificadorObrigacao, ValorNominalObrigacao)
    // ...
    return &TituloObrigacaoPublic{...}, nil
}
*/
