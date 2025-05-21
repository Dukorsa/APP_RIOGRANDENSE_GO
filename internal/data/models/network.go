package models

import (
	"regexp"
	"strings"
	"time"
	// "github.com/google/uuid" // Se o ID fosse UUID
)

// DBNetwork representa a entidade Network (Rede) no banco de dados.
type DBNetwork struct {
	ID     uint64 `gorm:"primaryKey;autoIncrement"`              // ID único da rede
	Name   string `gorm:"type:varchar(50);uniqueIndex;not null"` // Nome único da rede (case-insensitive no DB se possível)
	Buyer  string `gorm:"type:varchar(100);not null"`            // Nome do comprador responsável
	Status bool   `gorm:"not null;default:true"`                 // Status da rede (ativa/inativa)

	// Campos de Auditoria
	CreatedAt time.Time `gorm:"not null;default:now()"`
	UpdatedAt time.Time `gorm:"not null;default:now()"` // GORM atualiza automaticamente com autoUpdateTime
	CreatedBy *string   `gorm:"type:varchar(50)"`       // Username de quem criou (opcional)
	UpdatedBy *string   `gorm:"type:varchar(50)"`       // Username de quem atualizou por último (opcional)

	// Relação com CNPJs (Opcional, se usando GORM com Preload/Joins)
	// CNPJs     []DBCNPJ `gorm:"foreignKey:NetworkID"`
}

// TableName especifica o nome da tabela para GORM.
func (DBNetwork) TableName() string {
	return "networks"
}

// --- Structs para Transferência de Dados e Validação ---

// NetworkBase contém campos comuns.
type NetworkBase struct {
	Name  string `json:"name"`  // Nome da rede
	Buyer string `json:"buyer"` // Nome do comprador
}

// NetworkCreate é usado para criar uma nova rede.
// Inclui tags de validação.
type NetworkCreate struct {
	Name  string `json:"name" validate:"required,min=3,max=50,network_name_custom"` // network_name_custom seria uma tag custom
	Buyer string `json:"buyer" validate:"required,min=5,max=100,buyer_name_custom"` // buyer_name_custom seria uma tag custom
	// Status é true por padrão no banco
	// CreatedBy/UpdatedBy são definidos pelo serviço com base no usuário logado
}

// CleanAndValidate normaliza e valida os campos de NetworkCreate.
// Retorna um erro se a validação falhar. O serviço chamaria este método.
// O nome da rede é convertido para minúsculas, e o comprador para Title Case.
func (nc *NetworkCreate) CleanAndValidate() error {
	// Limpar e normalizar nome
	cleanedName := strings.TrimSpace(nc.Name)
	if cleanedName == "" {
		return NewValidationError("Nome da rede é obrigatório.", map[string]string{"name": "Nome é obrigatório"})
	}
	// TODO: Chamar validador customizado para nome (do utils/validators.go)
	// if !validators.IsValidNetworkName(cleanedName) { // Supondo que exista IsValidNetworkName
	//  return NewValidationError("Nome da rede inválido.", map[string]string{"name": "Formato do nome inválido"})
	// }
	nc.Name = strings.ToLower(cleanedName) // Padroniza para minúsculas

	// Limpar e normalizar comprador
	cleanedBuyer := strings.TrimSpace(nc.Buyer)
	if cleanedBuyer == "" {
		return NewValidationError("Nome do comprador é obrigatório.", map[string]string{"buyer": "Comprador é obrigatório"})
	}
	// TODO: Chamar validador customizado para comprador
	// if !validators.IsValidBuyerName(cleanedBuyer) {
	//  return NewValidationError("Nome do comprador inválido.", map[string]string{"buyer": "Formato do comprador inválido"})
	// }
	// Converte para Title Case (primeira letra de cada palavra maiúscula)
	// Atenção: strings.Title está depreciado. Usar golang.org/x/text/cases e golang.org/x/text/language
	// Exemplo simplificado (pode não ser perfeito para todos os casos):
	words := strings.Fields(cleanedBuyer)
	for i, word := range words {
		if len(word) > 0 {
			// Isso é uma simplificação, para Title Case correto, use x/text/cases.
			// Gostaríamos de usar cases.Title(language.BrazilianPortuguese).String(cleanedBuyer)
			// mas para evitar dependência externa extra aqui, fazemos uma aproximação.
			runes := []rune(strings.ToLower(word))
			runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
			words[i] = string(runes)
		}
	}
	nc.Buyer = strings.Join(words, " ")

	return nil
}

// NetworkUpdate é usado para atualizar uma rede existente.
// Campos são ponteiros para indicar atualizações parciais.
type NetworkUpdate struct {
	Name   *string `json:"name,omitempty" validate:"omitempty,min=3,max=50,network_name_custom"`
	Buyer  *string `json:"buyer,omitempty" validate:"omitempty,min=5,max=100,buyer_name_custom"`
	Status *bool   `json:"status,omitempty"`
	// UpdatedBy será definido pelo serviço
}

// CleanAndValidate normaliza e valida os campos de NetworkUpdate.
// Chamado pelo serviço.
func (nu *NetworkUpdate) CleanAndValidate() error {
	if nu.Name != nil {
		cleanedName := strings.TrimSpace(*nu.Name)
		if cleanedName == "" {
			return NewValidationError("Nome da rede não pode ser vazio se fornecido para atualização.", map[string]string{"name": "Nome não pode ser vazio"})
		}
		// TODO: Chamar validador customizado
		// if !validators.IsValidNetworkName(cleanedName) {
		//  return NewValidationError("Nome da rede inválido.", map[string]string{"name": "Formato do nome inválido"})
		// }
		*nu.Name = strings.ToLower(cleanedName)
	}

	if nu.Buyer != nil {
		cleanedBuyer := strings.TrimSpace(*nu.Buyer)
		if cleanedBuyer == "" {
			return NewValidationError("Nome do comprador não pode ser vazio se fornecido para atualização.", map[string]string{"buyer": "Comprador não pode ser vazio"})
		}
		// TODO: Chamar validador customizado
		// if !validators.IsValidBuyerName(cleanedBuyer) {
		//  return NewValidationError("Nome do comprador inválido.", map[string]string{"buyer": "Formato do comprador inválido"})
		// }
		words := strings.Fields(cleanedBuyer)
		for i, word := range words {
			if len(word) > 0 {
				runes := []rune(strings.ToLower(word))
				runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
				words[i] = string(runes)
			}
		}
		*nu.Buyer = strings.Join(words, " ")
	}
	return nil
}

// NetworkPublic representa os dados de uma rede para a UI ou API.
// Espelha o NetworkInDB do Python.
type NetworkPublic struct {
	ID        uint64    `json:"id"`
	Name      string    `json:"name"`
	Buyer     string    `json:"buyer"`
	Status    bool      `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	CreatedBy *string   `json:"created_by,omitempty"`
	UpdatedBy *string   `json:"updated_by,omitempty"`
	// CNPJCount int    `json:"cnpj_count,omitempty"` // Opcional: Contagem de CNPJs associados
}

// ToNetworkPublic converte um DBNetwork para NetworkPublic.
func ToNetworkPublic(dbNet *DBNetwork) *NetworkPublic {
	if dbNet == nil {
		return nil
	}
	return &NetworkPublic{
		ID:        dbNet.ID,
		Name:      dbNet.Name,
		Buyer:     dbNet.Buyer,
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

// Regex para validação de nome de rede e comprador (Exemplos, ajuste conforme necessário)
// Estes seriam usados em `utils/validators.go` e chamados por CleanAndValidate.
var (
	// Permite letras (incluindo acentuadas com \p{L}), números, espaços, hífen, underscore. Min 3, Max 50.
	networkNameRegex = regexp.MustCompile(`^[\p{L}\w\s\-]{3,50}$`)
	// Permite letras (incluindo acentuadas), espaços, ponto, hífen. Min 5, Max 100.
	buyerNameRegex = regexp.MustCompile(`^[\p{L}\s.\-]{5,100}$`)
)
