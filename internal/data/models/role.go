package models

import (
	"strings"
	// "github.com/google/uuid" // Para UserID se DBRole tivesse referência direta
	// "gorm.io/gorm" // Se usando GORM
)

// DBRole representa a entidade Role (Perfil) no banco de dados.
type DBRole struct {
	ID           uint64  `gorm:"primaryKey;autoIncrement"`
	Name         string  `gorm:"type:varchar(50);uniqueIndex;not null"` // Nome único do role (ex: "admin", "editor")
	Description  *string `gorm:"type:varchar(255)"`                     // Descrição opcional do role
	IsSystemRole bool    `gorm:"not null;default:false"`                // Indica se é um role do sistema (não pode ser excluído/renomeado facilmente)

	// Relação Muitos-para-Muitos com Permissões.
	// Em GORM, a tabela de junção 'role_permissions' seria gerenciada.
	// Se não usar GORM, esta será uma lista de strings de nomes de permissão
	// que o repositório/serviço preencherá ao buscar um DBRole.
	Permissions []string `gorm:"-"` // Ignorado pelo GORM para mapeamento direto de coluna,
	// preenchido programaticamente. Ou, com GORM:
	// Permissions []*DBPermission `gorm:"many2many:role_permissions;"` // Se DBPermission fosse uma tabela

	// Relação Muitos-para-Muitos com Usuários (GORM)
	// Users       []*DBUser `gorm:"many2many:user_roles;"`

	// CreatedAt e UpdatedAt poderiam ser adicionados se necessário para auditoria dos roles em si.
	// CreatedAt time.Time `gorm:"not null;default:now()"`
	// UpdatedAt time.Time `gorm:"not null;default:now()"`
}

// TableName especifica o nome da tabela para GORM.
func (DBRole) TableName() string {
	return "roles"
}

// DBRolePermission representa a tabela de junção 'role_permissions'.
// Necessário se não estiver usando a mágica many2many do GORM
// e precisar manipular a tabela de junção diretamente.
type DBRolePermission struct {
	RoleID         uint64 `gorm:"primaryKey"`
	PermissionName string `gorm:"type:varchar(100);primaryKey"` // Nome da permissão (ex: "network:create")
	// Role           DBRole `gorm:"foreignKey:RoleID"` // Opcional para GORM
}

// TableName para a tabela de junção.
func (DBRolePermission) TableName() string {
	return "role_permissions"
}

// DBUserRole representa a tabela de junção 'user_roles'.
// Necessário se não estiver usando a mágica many2many do GORM.
type DBUserRole struct {
	UserID string `gorm:"type:uuid;primaryKey"` // Ou uuid.UUID se User.ID for uuid.UUID
	RoleID uint64 `gorm:"primaryKey"`
	// User   DBUser `gorm:"foreignKey:UserID"` // Opcional para GORM
	// Role   DBRole `gorm:"foreignKey:RoleID"` // Opcional para GORM
}

// TableName para a tabela de junção.
func (DBUserRole) TableName() string {
	return "user_roles"
}

// --- Structs para Transferência de Dados e Validação ---

// RoleBase contém campos comuns.
type RoleBase struct {
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
}

// RoleCreate é usado para criar um novo role.
type RoleCreate struct {
	Name            string   `json:"name" validate:"required,min=3,max=50,rolename_custom"` // rolename_custom para regex ^[a-zA-Z0-9_]+$
	Description     *string  `json:"description,omitempty" validate:"omitempty,max=255"`
	PermissionNames []string `json:"permission_names"` // Lista de NOMES de permissões a serem associadas
}

// CleanAndValidate normaliza e valida os campos de RoleCreate.
// O serviço chamaria este método. Nome do role é convertido para minúsculas.
func (rc *RoleCreate) CleanAndValidate() error {
	cleanedName := strings.TrimSpace(rc.Name)
	if cleanedName == "" {
		return NewValidationError("Nome do role é obrigatório.", map[string]string{"name": "Nome é obrigatório"})
	}
	// TODO: Chamar validador customizado para nome do role (ex: regex ^[a-zA-Z0-9_]+$)
	// if !validators.IsValidRoleName(cleanedName) {
	//  return NewValidationError("Nome do role inválido (use letras, números, _).", map[string]string{"name": "Formato inválido"})
	// }
	rc.Name = strings.ToLower(cleanedName)

	if rc.Description != nil {
		*rc.Description = strings.TrimSpace(*rc.Description)
		if len(*rc.Description) > 255 {
			return NewValidationError("Descrição excede 255 caracteres.", map[string]string{"description": "Descrição muito longa"})
		}
		if *rc.Description == "" { // Se ficou vazia após trim, torna nil
			rc.Description = nil
		}
	}

	// Validação dos nomes das permissões (se eles existem no sistema)
	// geralmente é feita pelo RoleService, que tem acesso ao PermissionManager.
	// Aqui, podemos apenas garantir que não são strings vazias.
	validPermNames := []string{}
	for _, pName := range rc.PermissionNames {
		pTrimmed := strings.TrimSpace(pName)
		if pTrimmed != "" {
			validPermNames = append(validPermNames, pTrimmed)
		}
	}
	rc.PermissionNames = validPermNames

	return nil
}

// RoleUpdate é usado para atualizar um role existente.
type RoleUpdate struct {
	Name        *string `json:"name,omitempty" validate:"omitempty,min=3,max=50,rolename_custom"`
	Description *string `json:"description,omitempty" validate:"omitempty,max=255"`
	// PermissionNames: Se fornecido, substitui TODAS as permissões existentes do role.
	// Se nil, as permissões não são alteradas por este payload (mas podem ser por outro método).
	PermissionNames *[]string `json:"permission_names,omitempty"`
}

// CleanAndValidate para RoleUpdate.
func (ru *RoleUpdate) CleanAndValidate() error {
	if ru.Name != nil {
		cleanedName := strings.TrimSpace(*ru.Name)
		if cleanedName == "" {
			return NewValidationError("Nome do role não pode ser vazio se fornecido para atualização.", map[string]string{"name": "Nome não pode ser vazio"})
		}
		// TODO: Chamar validador customizado
		// if !validators.IsValidRoleName(cleanedName) {
		//  return NewValidationError("Nome do role inválido.", map[string]string{"name": "Formato inválido"})
		// }
		*ru.Name = strings.ToLower(cleanedName)
	}
	if ru.Description != nil {
		*ru.Description = strings.TrimSpace(*ru.Description)
		if len(*ru.Description) > 255 {
			return NewValidationError("Descrição excede 255 caracteres.", map[string]string{"description": "Descrição muito longa"})
		}
		if *ru.Description == "" {
			ru.Description = nil
		}
	}
	if ru.PermissionNames != nil {
		validPermNames := []string{}
		for _, pName := range *ru.PermissionNames {
			pTrimmed := strings.TrimSpace(pName)
			if pTrimmed != "" {
				validPermNames = append(validPermNames, pTrimmed)
			}
		}
		*ru.PermissionNames = validPermNames
	}
	return nil
}

// RolePublic representa os dados de um role para a UI ou API, incluindo suas permissões.
// Espelha o RoleInDB do Python.
type RolePublic struct {
	ID           uint64   `json:"id"`
	Name         string   `json:"name"`
	Description  *string  `json:"description,omitempty"`
	IsSystemRole bool     `json:"is_system_role"`
	Permissions  []string `json:"permissions"` // Lista de nomes de permissões
}

// ToRolePublic converte um DBRole para RolePublic.
// As permissões devem ser preenchidas externamente pelo serviço/repositório.
func ToRolePublic(dbRole *DBRole) *RolePublic {
	if dbRole == nil {
		return nil
	}
	// dbRole.Permissions já deve ser um slice de strings preenchido
	// pelo repositório ao buscar o DBRole.
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
