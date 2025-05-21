package models

import (
	"regexp"
	"strings"
	"time"
	"unicode"

	// "golang.org/x/text/cases" // Para Title Case correto
	// "golang.org/x/text/language" // Para Title Case correto

	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core"
)

// DBNetwork representa a entidade Network (Rede) no banco de dados.
type DBNetwork struct {
	ID uint64 `gorm:"primaryKey;autoIncrement"` // ID único da rede

	// Nome único da rede, armazenado em minúsculas para consistência e buscas case-insensitive.
	// A constraint `uniqueIndex` deve ser configurada no DB para ser case-insensitive se o DB suportar,
	// ou a lógica da aplicação deve garantir isso.
	Name string `gorm:"type:varchar(50);uniqueIndex;not null"`

	// Nome do comprador responsável, armazenado em Title Case.
	Buyer string `gorm:"type:varchar(100);not null"`

	// Status da rede (true para ativa, false para inativa).
	Status bool `gorm:"not null;default:true"`

	// Campos de Auditoria (gerenciados pelo GORM ou pela aplicação).
	CreatedAt time.Time `gorm:"not null;autoCreateTime"` // GORM preenche na criação
	UpdatedAt time.Time `gorm:"not null;autoUpdateTime"` // GORM preenche na criação e atualização
	CreatedBy *string   `gorm:"type:varchar(50)"`        // Username de quem criou (opcional).
	UpdatedBy *string   `gorm:"type:varchar(50)"`        // Username de quem atualizou por último (opcional).

	// Relação com CNPJs (Opcional, para GORM com Preload/Joins).
	// Se definida, é importante configurar o comportamento de onDelete e onUpdate.
	// CNPJs     []DBCNPJ `gorm:"foreignKey:NetworkID;constraint:OnDelete:RESTRICT"` // Ex: Restringir deleção se houver CNPJs.
}

// TableName especifica o nome da tabela para GORM.
func (DBNetwork) TableName() string {
	return "networks"
}

// --- Structs para Transferência de Dados e Validação ---

// NetworkCreate é usado para criar uma nova rede.
// Inclui tags de validação que seriam usadas por uma biblioteca como `go-playground/validator`.
type NetworkCreate struct {
	// `validate:"required,min=3,max=50,network_name_custom"` sugere validação customizada para o nome.
	Name string `json:"name" validate:"required,min=3,max=50,network_name_format"`

	// `validate:"required,min=5,max=100,buyer_name_custom"` sugere validação customizada para o comprador.
	Buyer string `json:"buyer" validate:"required,min=2,max=100,buyer_name_format"` // Min 2 para nomes como "Li Li"
}

var (
	// Regex para validação de nome de rede: letras, números, espaços, hífen, underscore.
	// \p{L} para letras Unicode, \d para dígitos.
	networkNameValidationRegex = regexp.MustCompile(`^[\p{L}\d\s_-]{3,50}$`)

	// Regex para nome do comprador: letras, espaços, ponto, hífen.
	buyerNameValidationRegex = regexp.MustCompile(`^[\p{L}\s.-]{2,100}$`) // Min 2
)

// CleanAndValidate normaliza e valida os campos de NetworkCreate.
// Retorna um erro se a validação falhar. Este método deve ser chamado pelo serviço.
// O nome da rede é convertido para minúsculas, e o comprador para Title Case.
func (nc *NetworkCreate) CleanAndValidate() error {
	// Limpar e normalizar nome
	cleanedName := strings.TrimSpace(nc.Name)
	if cleanedName == "" {
		return appErrors.NewValidationError("Nome da rede é obrigatório.", map[string]string{"name": "obrigatório"})
	}
	if !networkNameValidationRegex.MatchString(cleanedName) {
		return appErrors.NewValidationError(
			"Nome da rede deve ter entre 3 e 50 caracteres e conter apenas letras, números, espaços, '_' ou '-'.",
			map[string]string{"name": "formato inválido"},
		)
	}
	nc.Name = strings.ToLower(cleanedName) // Padroniza para minúsculas

	// Limpar e normalizar comprador
	cleanedBuyer := strings.TrimSpace(nc.Buyer)
	if cleanedBuyer == "" {
		return appErrors.NewValidationError("Nome do comprador é obrigatório.", map[string]string{"buyer": "obrigatório"})
	}
	if !buyerNameValidationRegex.MatchString(cleanedBuyer) {
		return appErrors.NewValidationError(
			"Nome do comprador deve ter entre 2 e 100 caracteres e conter apenas letras, espaços, '.' ou '-'.",
			map[string]string{"buyer": "formato inválido"},
		)
	}
	// Converte para Title Case (primeira letra de cada palavra maiúscula)
	// Para uma conversão correta de Title Case considerando a localidade:
	// nc.Buyer = cases.Title(language.BrazilianPortuguese, cases.NoLower).String(cleanedBuyer)
	// Se não usar x/text/cases, uma aproximação simples (pode não ser ideal para todos os nomes):
	words := strings.Fields(cleanedBuyer)
	for i, word := range words {
		if len(word) > 0 {
			runes := []rune(strings.ToLower(word))
			runes[0] = unicode.ToUpper(runes[0])
			words[i] = string(runes)
		}
	}
	nc.Buyer = strings.Join(words, " ")

	return nil
}

// NetworkUpdate é usado para atualizar uma rede existente.
// Campos são ponteiros para indicar atualizações parciais (omitempty).
type NetworkUpdate struct {
	Name   *string `json:"name,omitempty" validate:"omitempty,min=3,max=50,network_name_format"`
	Buyer  *string `json:"buyer,omitempty" validate:"omitempty,min=2,max=100,buyer_name_format"`
	Status *bool   `json:"status,omitempty"`
	// UpdatedBy será definido pelo serviço com base no usuário logado.
}

// CleanAndValidate normaliza e valida os campos de NetworkUpdate que foram fornecidos.
// Chamado pelo serviço.
func (nu *NetworkUpdate) CleanAndValidate() error {
	if nu.Name != nil {
		cleanedName := strings.TrimSpace(*nu.Name)
		if cleanedName == "" {
			return appErrors.NewValidationError("Nome da rede não pode ser vazio se fornecido para atualização.", map[string]string{"name": "não pode ser vazio"})
		}
		if !networkNameValidationRegex.MatchString(cleanedName) {
			return appErrors.NewValidationError(
				"Nome da rede deve ter entre 3 e 50 caracteres e conter apenas letras, números, espaços, '_' ou '-'.",
				map[string]string{"name": "formato inválido"},
			)
		}
		*nu.Name = strings.ToLower(cleanedName)
	}

	if nu.Buyer != nil {
		cleanedBuyer := strings.TrimSpace(*nu.Buyer)
		if cleanedBuyer == "" {
			return appErrors.NewValidationError("Nome do comprador não pode ser vazio se fornecido para atualização.", map[string]string{"buyer": "não pode ser vazio"})
		}
		if !buyerNameValidationRegex.MatchString(cleanedBuyer) {
			return appErrors.NewValidationError(
				"Nome do comprador deve ter entre 2 e 100 caracteres e conter apenas letras, espaços, '.' ou '-'.",
				map[string]string{"buyer": "formato inválido"},
			)
		}
		// nu.Buyer = cases.Title(language.BrazilianPortuguese, cases.NoLower).String(cleanedBuyer)
		words := strings.Fields(cleanedBuyer)
		for i, word := range words {
			if len(word) > 0 {
				runes := []rune(strings.ToLower(word))
				runes[0] = unicode.ToUpper(runes[0])
				words[i] = string(runes)
			}
		}
		*nu.Buyer = strings.Join(words, " ")
	}
	return nil
}

// NetworkPublic representa os dados de uma rede para a UI ou API (DTO).
type NetworkPublic struct {
	ID        uint64    `json:"id"`
	Name      string    `json:"name"`  // Nome da rede (em minúsculas)
	Buyer     string    `json:"buyer"` // Nome do comprador (em Title Case)
	Status    bool      `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	CreatedBy *string   `json:"created_by,omitempty"`
	UpdatedBy *string   `json:"updated_by,omitempty"`
	// CNPJCount int       `json:"cnpj_count,omitempty"` // Opcional: Contagem de CNPJs associados, se necessário na UI.
}

// ToNetworkPublic converte um DBNetwork (modelo do banco) para NetworkPublic (DTO).
func ToNetworkPublic(dbNet *DBNetwork) *NetworkPublic {
	if dbNet == nil {
		return nil
	}
	return &NetworkPublic{
		ID:        dbNet.ID,
		Name:      dbNet.Name,  // Já está em minúsculas no DB
		Buyer:     dbNet.Buyer, // Já está em Title Case no DB
		Status:    dbNet.Status,
		CreatedAt: dbNet.CreatedAt,
		UpdatedAt: dbNet.UpdatedAt,
		CreatedBy: dbNet.CreatedBy,
		UpdatedBy: dbNet.UpdatedBy,
	}
}

// ToNetworkPublicList converte uma lista de DBNetwork para uma lista de NetworkPublic.
func ToNetworkPublicList(dbNets []*DBNetwork) []*NetworkPublic {
	publicList := make([]*NetworkPublic, len(dbNets))
	for i, dbNet := range dbNets {
		publicList[i] = ToNetworkPublic(dbNet)
	}
	return publicList
}
