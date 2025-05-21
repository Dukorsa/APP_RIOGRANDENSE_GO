package models

import (
	"regexp"
	"strings"

	// "time" // Descomente se adicionar CreatedAt/UpdatedAt em DBRole

	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/errors"
	// "gorm.io/gorm" // Descomentado se GORM for usado diretamente aqui
)

// DBRole representa a entidade Role (Perfil) no banco de dados.
type DBRole struct {
	ID uint64 `gorm:"primaryKey;autoIncrement"`

	// Nome único do role (ex: "admin", "editor_chefe"), armazenado em minúsculas.
	Name string `gorm:"type:varchar(50);uniqueIndex;not null"`

	// Descrição opcional do role.
	Description *string `gorm:"type:varchar(255)"`

	// IsSystemRole indica se é um role do sistema (true) ou customizado (false).
	// Roles do sistema geralmente não podem ser excluídos ou ter o nome alterado.
	IsSystemRole bool `gorm:"not null;default:false"`

	// Permissions é uma lista de NOMES de permissões associadas a este role.
	// Este campo é preenchido programaticamente pelo repositório/serviço
	// a partir da tabela de junção `role_permissions`.
	// A tag `gorm:"-"` impede que o GORM tente mapear este campo diretamente para uma coluna na tabela `roles`.
	// Se estivesse usando a funcionalidade `many2many` do GORM com uma struct `DBPermission`,
	// a tag seria: `gorm:"many2many:role_permissions;"`.
	Permissions []string `gorm:"-"`

	// Opcional: Campos de auditoria para o próprio role.
	// CreatedAt time.Time `gorm:"not null;autoCreateTime"`
	// UpdatedAt time.Time `gorm:"not null;autoUpdateTime"`
}

// TableName especifica o nome da tabela para GORM.
func (DBRole) TableName() string {
	return "roles"
}

// DBRolePermission representa a tabela de junção `role_permissions`.
// Esta struct é usada pelo GORM para gerenciar a relação muitos-para-muitos
// entre roles e permissões, se você não estiver usando a associação implícita do GORM.
// Se o GORM gerencia a tabela de junção implicitamente através de `many2many`,
// esta struct pode não ser explicitamente necessária no código do modelo,
// mas o GORM a criará no banco.
type DBRolePermission struct {
	RoleID uint64 `gorm:"primaryKey"` // Chave estrangeira para DBRole.ID

	// PermissionName é o nome da permissão (ex: "network:create").
	// Parte da chave primária composta com RoleID.
	PermissionName string `gorm:"type:varchar(100);primaryKey"`
}

// TableName para a tabela de junção.
func (DBRolePermission) TableName() string {
	return "role_permissions"
}

// DBUserRole representa a tabela de junção `user_roles`.
// Usada para a relação muitos-para-muitos entre usuários e roles.
type DBUserRole struct {
	UserID string `gorm:"type:uuid;primaryKey"` // Chave estrangeira para DBUser.ID
	RoleID uint64 `gorm:"primaryKey"`           // Chave estrangeira para DBRole.ID
}

// TableName para a tabela de junção.
func (DBUserRole) TableName() string {
	return "user_roles"
}

// --- Structs para Transferência de Dados e Validação ---

// RoleCreate é usado para criar um novo role.
type RoleCreate struct {
	// `validate:"required,min=3,max=50,rolename_format"` sugere validação customizada.
	Name string `json:"name" validate:"required,min=3,max=50,rolename_format"`

	// `validate:"omitempty,max=255"` significa opcional, mas se presente, máximo de 255 caracteres.
	Description *string `json:"description,omitempty" validate:"omitempty,max=255"`

	// PermissionNames é uma lista de NOMES de permissões a serem associadas ao novo role.
	// `validate:"omitempty,dive,min=1"` significa que a lista pode ser vazia, mas se
	// elementos estiverem presentes (dive), cada um deve ter pelo menos 1 caractere.
	// A validação de que os nomes de permissão existem de fato no sistema é feita pelo serviço.
	PermissionNames []string `json:"permission_names" validate:"omitempty,dive,min=1"`
}

var roleNameValidationRegex = regexp.MustCompile(`^[a-zA-Z0-9_]{3,50}$`)

// CleanAndValidate normaliza e valida os campos de RoleCreate.
// O serviço deve chamar este método. O nome do role é convertido para minúsculas.
func (rc *RoleCreate) CleanAndValidate() error {
	// Normalizar e validar Nome
	cleanedName := strings.TrimSpace(rc.Name)
	if cleanedName == "" {
		return appErrors.NewValidationError("Nome do role é obrigatório.", map[string]string{"name": "obrigatório"})
	}
	if !roleNameValidationRegex.MatchString(cleanedName) {
		return appErrors.NewValidationError(
			"Nome do role deve ter entre 3 e 50 caracteres e conter apenas letras (a-z, A-Z), números (0-9) ou underscore (_).",
			map[string]string{"name": "formato inválido"},
		)
	}
	rc.Name = strings.ToLower(cleanedName) // Padroniza para minúsculas

	// Normalizar Descrição
	if rc.Description != nil {
		*rc.Description = strings.TrimSpace(*rc.Description)
		if len(*rc.Description) > 255 {
			return appErrors.NewValidationError("Descrição do role excede 255 caracteres.", map[string]string{"description": "muito longa"})
		}
		if *rc.Description == "" { // Se ficou vazia após trim, torna nil para consistência.
			rc.Description = nil
		}
	}

	// Normalizar Nomes de Permissões (remover espaços e vazios)
	validPermNames := []string{}
	if rc.PermissionNames != nil {
		for _, pName := range rc.PermissionNames {
			pTrimmed := strings.TrimSpace(pName)
			if pTrimmed != "" {
				validPermNames = append(validPermNames, pTrimmed) // A validação de existência é no serviço.
			}
		}
	}
	rc.PermissionNames = validPermNames // Pode ser uma lista vazia se todas forem inválidas/vazias.

	return nil
}

// RoleUpdate é usado para atualizar um role existente.
// Ponteiros indicam campos opcionais para atualização.
type RoleUpdate struct {
	Name        *string `json:"name,omitempty" validate:"omitempty,min=3,max=50,rolename_format"`
	Description *string `json:"description,omitempty" validate:"omitempty,max=255"`

	// PermissionNames: Se fornecido (não nil), substitui TODAS as permissões existentes do role.
	// Se for um slice vazio `[]string{}`, remove todas as permissões.
	// Se for `nil`, as permissões não são alteradas por este payload.
	PermissionNames *[]string `json:"permission_names,omitempty" validate:"omitempty,dive,min=1"`
}

// CleanAndValidate normaliza e valida os campos de RoleUpdate que foram fornecidos.
func (ru *RoleUpdate) CleanAndValidate() error {
	if ru.Name != nil {
		cleanedName := strings.TrimSpace(*ru.Name)
		if cleanedName == "" {
			return appErrors.NewValidationError("Nome do role não pode ser vazio se fornecido para atualização.", map[string]string{"name": "não pode ser vazio"})
		}
		if !roleNameValidationRegex.MatchString(cleanedName) {
			return appErrors.NewValidationError(
				"Nome do role deve ter entre 3 e 50 caracteres e conter apenas letras, números ou underscore.",
				map[string]string{"name": "formato inválido"},
			)
		}
		*ru.Name = strings.ToLower(cleanedName)
	}

	if ru.Description != nil {
		*ru.Description = strings.TrimSpace(*ru.Description)
		if len(*ru.Description) > 255 {
			return appErrors.NewValidationError("Descrição do role excede 255 caracteres.", map[string]string{"description": "muito longa"})
		}
		if *ru.Description == "" {
			ru.Description = nil
		}
	}

	if ru.PermissionNames != nil { // Se o ponteiro para o slice não for nil
		validPermNames := []string{}
		for _, pName := range *ru.PermissionNames { // Desreferencia o slice
			pTrimmed := strings.TrimSpace(pName)
			if pTrimmed != "" {
				validPermNames = append(validPermNames, pTrimmed)
			}
		}
		*ru.PermissionNames = validPermNames // Atualiza o slice desreferenciado
	}
	return nil
}

// RolePublic representa os dados de um role para a UI ou API (DTO), incluindo suas permissões.
type RolePublic struct {
	ID           uint64   `json:"id"`
	Name         string   `json:"name"`
	Description  *string  `json:"description,omitempty"`
	IsSystemRole bool     `json:"is_system_role"`
	Permissions  []string `json:"permissions"` // Lista de nomes de permissões.
}

// ToRolePublic converte um DBRole (modelo do banco) para RolePublic (DTO).
// O campo `DBRole.Permissions` já deve estar preenchido pelo repositório.
func ToRolePublic(dbRole *DBRole) *RolePublic {
	if dbRole == nil {
		return nil
	}

	// Garante que `Permissions` seja um slice vazio, não nil, se não houver permissões.
	permissions := []string{}
	if dbRole.Permissions != nil {
		permissions = dbRole.Permissions
	}

	return &RolePublic{
		ID:           dbRole.ID,
		Name:         dbRole.Name,
		Description:  dbRole.Description,
		IsSystemRole: dbRole.IsSystemRole,
		Permissions:  permissions,
	}
}

// ToRolePublicList converte uma lista de DBRole para uma lista de RolePublic.
func ToRolePublicList(dbRoles []*DBRole) []*RolePublic {
	publicList := make([]*RolePublic, len(dbRoles))
	for i, dbRole := range dbRoles {
		publicList[i] = ToRolePublic(dbRole)
	}
	return publicList
}
