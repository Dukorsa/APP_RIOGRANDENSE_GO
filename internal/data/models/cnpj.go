package models

import (
	"regexp"
	"strings"
	"time"
	// "github.com/google/uuid" // Não usado diretamente aqui, mas network_id pode ser um UUID se Network.ID for
)

// DBCNPJ representa a entidade CNPJ no banco de dados.
type DBCNPJ struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement"`              // ID único do registro CNPJ
	CNPJ      string `gorm:"type:varchar(14);uniqueIndex;not null"` // CNPJ armazenado apenas com dígitos
	NetworkID uint64 `gorm:"not null;index"`                        // ID da Rede (DBNetwork.ID) à qual este CNPJ pertence
	// Network    DBNetwork `gorm:"foreignKey:NetworkID"` // Opcional: Relação com DBNetwork se usando GORM com Preload

	RegistrationDate time.Time `gorm:"not null;default:now()"` // Data de cadastro do CNPJ
	Active           bool      `gorm:"not null;default:true"`  // Indica se o CNPJ está ativo
}

// TableName especifica o nome da tabela para GORM.
func (DBCNPJ) TableName() string {
	return "cnpjs"
}

// --- Structs para Transferência de Dados e Validação ---

// CNPJBase contém campos comuns para criação e leitura.
type CNPJBase struct {
	CNPJ      string `json:"cnpj"`       // Pode vir formatado ou não do input
	NetworkID uint64 `json:"network_id"` // ID da Rede
}

// CNPJCreate é usado para criar um novo CNPJ.
// Inclui tags de validação.
type CNPJCreate struct {
	CNPJ      string `json:"cnpj" validate:"required,cnpj"` // "cnpj" seria uma tag customizada de validação
	NetworkID uint64 `json:"network_id" validate:"required,gt=0"`
	// 'Active' é true por padrão no banco, não precisa ser passado na criação geralmente
}

// CleanAndValidateCNPJ limpa e valida o número do CNPJ.
// Retorna o CNPJ limpo (apenas dígitos) ou um erro.
// Este método pode ser chamado pelo serviço antes de passar para o repositório.
// Ou a tag `validate:"cnpj"` pode ser implementada para fazer isso.
func (cc *CNPJCreate) CleanAndValidateCNPJ() (string, error) {
	if cc.CNPJ == "" {
		return "", NewValidationError("CNPJ é obrigatório", nil)
	}
	// Remove todos os caracteres não numéricos
	cleanedCNPJ := regexp.MustCompile(`[^0-9]`).ReplaceAllString(cc.CNPJ, "")

	if len(cleanedCNPJ) != 14 {
		// Retorna um erro específico se o CNPJ limpo não tiver 14 dígitos
		return "", NewValidationError("CNPJ deve conter 14 dígitos após a limpeza.", map[string]string{"cnpj": "CNPJ deve ter 14 dígitos."})
	}

	// TODO: Implementar ou chamar a lógica de validação de dígitos do CNPJ (do utils/validators.go)
	// if !validators.IsValidCNPJ(cleanedCNPJ) {
	// 	return "", NewValidationError("CNPJ inválido (dígitos verificadores não conferem).", map[string]string{"cnpj": "CNPJ inválido."})
	// }
	return cleanedCNPJ, nil
}

// CNPJUpdate é usado para atualizar um CNPJ existente.
// Todos os campos são ponteiros para indicar quais devem ser atualizados.
// O CNPJ em si geralmente não é atualizado; se for, seria uma nova entrada.
type CNPJUpdate struct {
	NetworkID *uint64 `json:"network_id,omitempty" validate:"omitempty,gt=0"`
	Active    *bool   `json:"active,omitempty"`
}

// CNPJPublic representa os dados de um CNPJ como retornados pela API ou para a UI.
// Espelha o CNPJInDB do Python.
type CNPJPublic struct {
	ID               uint64    `json:"id"`
	CNPJ             string    `json:"cnpj"` // Pode ser formatado para exibição aqui, ou na UI
	NetworkID        uint64    `json:"network_id"`
	RegistrationDate time.Time `json:"registration_date"`
	Active           bool      `json:"active"`
	// NetworkName string `json:"network_name,omitempty"` // Opcional: Nome da rede, se fizer join
}

// FormatCNPJ é um helper para formatar o CNPJ para exibição.
func (cp *CNPJPublic) FormatCNPJ() string {
	if len(cp.CNPJ) == 14 {
		return cp.CNPJ[0:2] + "." + cp.CNPJ[2:5] + "." + cp.CNPJ[5:8] + "/" + cp.CNPJ[8:12] + "-" + cp.CNPJ[12:14]
	}
	return cp.CNPJ // Retorna como está se não tiver 14 dígitos
}

// ToCNPJPublic converte um DBCNPJ para CNPJPublic.
// Opcionalmente, pode receber o nome da rede para incluir.
func ToCNPJPublic(dbCNPJ *DBCNPJ /*, networkName ...string */) *CNPJPublic {
	if dbCNPJ == nil {
		return nil
	}
	publicCNPJ := &CNPJPublic{
		ID:               dbCNPJ.ID,
		CNPJ:             dbCNPJ.CNPJ, // CNPJ já está limpo no DB
		NetworkID:        dbCNPJ.NetworkID,
		RegistrationDate: dbCNPJ.RegistrationDate,
		Active:           dbCNPJ.Active,
	}
	// if len(networkName) > 0 {
	// 	publicCNPJ.NetworkName = networkName[0]
	// }
	return publicCNPJ
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
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1 // Descarta o caractere
	}, cnpjStr)
}
