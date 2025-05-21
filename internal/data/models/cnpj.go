package models

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/errors"
	// "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/utils" // Se a validação de dígitos do CNPJ estivesse aqui
)

// DBCNPJ representa a entidade CNPJ no banco de dados.
type DBCNPJ struct {
	ID uint64 `gorm:"primaryKey;autoIncrement"` // ID único do registro CNPJ

	// CNPJ armazenado apenas com dígitos (14 caracteres).
	// `uniqueIndex` garante que não haja CNPJs duplicados.
	CNPJ string `gorm:"type:varchar(14);uniqueIndex;not null"`

	// ID da Rede (DBNetwork.ID) à qual este CNPJ pertence.
	// `index` melhora a performance de buscas por NetworkID.
	NetworkID uint64 `gorm:"not null;index"`

	// Opcional: Relação com DBNetwork se estiver usando GORM com Preload/Joins frequentes.
	// Network    DBNetwork `gorm:"foreignKey:NetworkID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`

	// Data de cadastro do CNPJ, com valor padrão para o momento da criação.
	RegistrationDate time.Time `gorm:"not null;default:now()"`

	// Indica se o CNPJ está ativo (true) ou inativo (false).
	Active bool `gorm:"not null;default:true"`
}

// TableName especifica o nome da tabela para GORM.
func (DBCNPJ) TableName() string {
	return "cnpjs"
}

// --- Structs para Transferência de Dados e Validação ---

// CNPJCreate é usado para criar um novo CNPJ.
// Inclui tags de validação que seriam usadas por uma biblioteca como `go-playground/validator`.
type CNPJCreate struct {
	// CNPJ pode vir formatado ou não do input. A limpeza e validação ocorrem antes da persistência.
	// A tag `validate:"required,cnpj"` sugere uma validação customizada.
	CNPJ string `json:"cnpj" validate:"required,cnpj_format"`

	// NetworkID é o ID da Rede à qual o CNPJ será associado.
	// `validate:"required,gt=0"` significa que é obrigatório e deve ser maior que zero.
	NetworkID uint64 `json:"network_id" validate:"required,gt=0"`
}

var cnpjNonDigitRegex = regexp.MustCompile(`[^0-9]`)

// CleanAndValidateCNPJ limpa a string do CNPJ (removendo não dígitos)
// e valida se o resultado tem 14 dígitos.
// A validação dos dígitos verificadores deve ser feita separadamente (ex: em `utils.IsValidCNPJ`).
func (cc *CNPJCreate) CleanAndValidateCNPJ() (string, error) {
	if strings.TrimSpace(cc.CNPJ) == "" {
		return "", appErrors.NewValidationError("CNPJ é obrigatório.", map[string]string{"cnpj": "obrigatório"})
	}

	cleanedCNPJ := cnpjNonDigitRegex.ReplaceAllString(cc.CNPJ, "")

	if len(cleanedCNPJ) != 14 {
		return "", appErrors.NewValidationError(
			"CNPJ deve conter 14 dígitos após a remoção de caracteres não numéricos.",
			map[string]string{"cnpj": "CNPJ deve ter 14 dígitos."},
		)
	}
	return cleanedCNPJ, nil
}

// CNPJUpdate é usado para atualizar um CNPJ existente.
// Todos os campos são ponteiros para indicar quais devem ser atualizados (omitempty).
// O número do CNPJ em si geralmente não é alterado; se for, é considerado uma nova entrada.
type CNPJUpdate struct {
	// `validate:"omitempty,gt=0"` significa que se fornecido, deve ser maior que zero.
	NetworkID *uint64 `json:"network_id,omitempty" validate:"omitempty,gt=0"`
	Active    *bool   `json:"active,omitempty"`
}

// CNPJPublic representa os dados de um CNPJ como são retornados pela API ou para a UI.
type CNPJPublic struct {
	ID        uint64 `json:"id"`
	CNPJ      string `json:"cnpj"` // CNPJ limpo (apenas dígitos)
	NetworkID uint64 `json:"network_id"`
	// NetworkName string    `json:"network_name,omitempty"` // Opcional: Nome da rede, se fizer join
	RegistrationDate time.Time `json:"registration_date"`
	Active           bool      `json:"active"`
}

// FormatCNPJ é um helper para formatar o CNPJ (apenas dígitos) para exibição no formato padrão.
// Ex: "XX.XXX.XXX/XXXX-XX".
func (cp *CNPJPublic) FormatCNPJ() string {
	if len(cp.CNPJ) == 14 {
		return fmt.Sprintf("%s.%s.%s/%s-%s",
			cp.CNPJ[0:2], cp.CNPJ[2:5], cp.CNPJ[5:8], cp.CNPJ[8:12], cp.CNPJ[12:14])
	}
	return cp.CNPJ // Retorna como está se não tiver 14 dígitos
}

// ToCNPJPublic converte um DBCNPJ (modelo do banco) para CNPJPublic (modelo de exibição/API).
func ToCNPJPublic(dbCNPJ *DBCNPJ) *CNPJPublic {
	if dbCNPJ == nil {
		return nil
	}
	return &CNPJPublic{
		ID:               dbCNPJ.ID,
		CNPJ:             dbCNPJ.CNPJ, // Já está limpo no DB
		NetworkID:        dbCNPJ.NetworkID,
		RegistrationDate: dbCNPJ.RegistrationDate,
		Active:           dbCNPJ.Active,
	}
}

// ToCNPJPublicList converte uma lista de DBCNPJ para uma lista de CNPJPublic.
func ToCNPJPublicList(dbCNPJs []*DBCNPJ) []*CNPJPublic {
	publicList := make([]*CNPJPublic, len(dbCNPJs))
	for i, dbCNPJ := range dbCNPJs {
		publicList[i] = ToCNPJPublic(dbCNPJ)
	}
	return publicList
}

// --- Utilitários / Helpers ---

// CleanCNPJ remove caracteres não numéricos de uma string CNPJ.
// Usado antes de salvar no banco ou validar.
func CleanCNPJ(cnpjStr string) string {
	return cnpjNonDigitRegex.ReplaceAllString(cnpjStr, "")
}
