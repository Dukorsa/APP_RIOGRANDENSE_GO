package models

import (
	"time"
	// "github.com/shopspring/decimal" // Descomente se usar para manipulação
	// "gorm.io/gorm" // Se usando GORM
)

// DBTituloDireito representa um registro na tabela 'titulos_direitos'.
type DBTituloDireito struct {
	ID uint64 `gorm:"primaryKey;autoIncrement"` // Chave primária interna

	// Colunas mapeadas do arquivo de importação
	Pessoa         *string    `gorm:"type:varchar(255)"`                // PESSOA
	CNPJCPF        string     `gorm:"type:varchar(14);not null;index"`  // CNPJ/CPF (apenas dígitos)
	NumeroEmpresa  int        `gorm:"not null;index"`                   // NROEMPRESA (int, pois no Python era int)
	Titulo         string     `gorm:"type:varchar(100);not null;index"` // TÍTULO (VARCHAR(100) no Python, não VARCHAR(50))
	CodigoEspecie  *string    `gorm:"type:varchar(50)"`                 // CODESPÉCIE
	DataVencimento *time.Time `gorm:"type:date"`                        // DTAVENCIMENTO (armazenado como data)
	DataQuitacao   *time.Time `gorm:"type:date"`                        // DTAQUITAÇÃO (armazenado como data)

	// Para valores monetários, armazenaremos como string para precisão.
	// A conversão para/de shopspring/decimal.Decimal ocorrerá no repositório/serviço.
	ValorNominal string  `gorm:"type:varchar(30);not null"` // VLRNOMINAL (ex: "1234.56")
	ValorPago    *string `gorm:"type:varchar(30)"`          // VLRPAGO

	Operacao        *string    `gorm:"type:varchar(50)"` // OPERAÇÃO
	DataOperacao    *time.Time `gorm:"type:date"`        // DTAOPERAÇÃO
	DataContabiliza *time.Time `gorm:"type:date"`        // DTACONTABILIZA

	// DataAlteracao no Python era DateTime(timezone=True) com onupdate.
	// GORM pode lidar com autoUpdateTime, ou o repositório define na atualização.
	DataAlteracao *time.Time `gorm:"autoUpdateTime"` // DTAALTERAÇÃO (do CSV, mas aqui pode ser auto)

	Observacao       *string    `gorm:"type:text"`         // OBSERVAÇÃO
	ValorOperacao    *string    `gorm:"type:varchar(30)"`  // VLROPERAÇÃO
	UsuarioAlteracao *string    `gorm:"type:varchar(50)"`  // USUALTERAÇÃO
	EspecieAbatcomp  *string    `gorm:"type:varchar(100)"` // ESPECIEABATCOMP
	ObsTitulo        *string    `gorm:"type:text"`         // OBSTÍTULO
	ContasQuitacao   *string    `gorm:"type:text"`         // CONTASQUITAÇÃO
	DataProgramada   *time.Time `gorm:"type:date"`         // DTAPROGRAMADA

	// Campos de auditoria do GORM (opcional, se quiser que GORM gerencie)
	// CreatedAt time.Time
	// UpdatedAt time.Time
	// DeletedAt gorm.DeletedAt `gorm:"index"`
}

// TableName especifica o nome da tabela para GORM.
func (DBTituloDireito) TableName() string {
	return "titulos_direitos"
}

// --- Structs para Transferência de Dados (Opcional, para API/UI se necessário) ---

// TituloDireitoPublic representa dados de um título de direito para exibição.
// Pode ser usado se você quiser formatar ou omitir certos campos da struct DB.
// Por enquanto, pode ser muito similar à DBTituloDireito, mas com tipos `decimal.Decimal` e datas formatadas.
/*
type TituloDireitoPublic struct {
	ID uint64 `json:"id"`
	Pessoa         *string           `json:"pessoa,omitempty"`
	CNPJCPF        string            `json:"cnpj_cpf"`
	NumeroEmpresa  int               `json:"numero_empresa"`
	Titulo         string            `json:"titulo"`
	CodigoEspecie  *string           `json:"codigo_especie,omitempty"`
	DataVencimento *string           `json:"data_vencimento,omitempty"` // Formatado como string "YYYY-MM-DD"
	DataQuitacao   *string           `json:"data_quitacao,omitempty"`   // Formatado
	ValorNominal   decimal.Decimal   `json:"valor_nominal"`
	ValorPago      *decimal.Decimal  `json:"valor_pago,omitempty"`
	// ... outros campos conforme necessário para exibição ...
	DataAlteracao  *string `json:"data_alteracao,omitempty"` // Formatado
}

// ToTituloDireitoPublic converte DBTituloDireito para TituloDireitoPublic.
// Esta função faria a conversão de string para decimal.Decimal e formatação de datas.
func ToTituloDireitoPublic(dbtd *DBTituloDireito) (*TituloDireitoPublic, error) {
	if dbtd == nil {
		return nil, nil
	}

	// Helper para converter string de valor para decimal.Decimal
	parseDecimal := func(valStr *string) (*decimal.Decimal, error) {
		if valStr == nil || *valStr == "" {
			return nil, nil
		}
		d, err := decimal.NewFromString(*valStr)
		if err != nil {
			return nil, fmt.Errorf("falha ao converter valor '%s' para decimal: %w", *valStr, err)
		}
		return &d, nil
	}

	// Helper para formatar time.Time para string "YYYY-MM-DD"
	formatDate := func(t *time.Time) *string {
		if t == nil {
			return nil
		}
		s := t.Format("2006-01-02")
		return &s
	}

	vn, err := parseDecimal(&dbtd.ValorNominal)
	if err != nil { return nil, err }
	if vn == nil { // ValorNominal é not null, então isso não deveria acontecer se o DB estiver correto
		return nil, errors.New("valor nominal não pode ser nulo ou vazio no banco de dados")
	}

	vp, err := parseDecimal(dbtd.ValorPago)
	if err != nil { return nil, err }

	// ... conversões similares para outros campos decimais ...

	return &TituloDireitoPublic{
		ID: dbtd.ID,
		Pessoa: dbtd.Pessoa,
		CNPJCPF: dbtd.CNPJCPF,
		NumeroEmpresa: dbtd.NumeroEmpresa,
		Titulo: dbtd.Titulo,
		CodigoEspecie: dbtd.CodigoEspecie,
		DataVencimento: formatDate(dbtd.DataVencimento),
		DataQuitacao: formatDate(dbtd.DataQuitacao),
		ValorNominal: *vn, // Desreferencia pois ValorNominal em Public é decimal.Decimal, não *decimal.Decimal
		ValorPago: vp,
		// ... outros campos ...
		DataAlteracao: formatDate(dbtd.DataAlteracao),
	}, nil
}
*/

// TituloDireitoFromCSV representa os dados como lidos diretamente de uma linha do CSV/TXT.
// Usado internamente pelo ImportService antes da conversão para DBTituloDireito.
// Os campos são todos strings ou ponteiros para strings para lidar com valores vazios.
type TituloDireitoFromRow struct {
	Pessoa           string // Tratado como string, mesmo que possa ser nulo no CSV
	CNPJCPF          string
	NumeroEmpresa    string
	Titulo           string
	CodigoEspecie    string
	DataVencimento   string
	DataQuitacao     string
	ValorNominal     string
	ValorPago        string
	Operacao         string
	DataOperacao     string
	DataContabiliza  string
	DataAlteracaoCSV string // Nome diferente para não confundir com DataAlteracao do DB
	Observacao       string
	ValorOperacao    string
	UsuarioAlteracao string
	EspecieAbatcomp  string
	ObsTitulo        string
	ContasQuitacao   string
	DataProgramada   string
	// LinhaOriginal string // Opcional: para logar a linha inteira em caso de erro de parsing
	// NumeroLinha int    // Opcional: para logs
}
