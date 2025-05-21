package models

import (
	"strings"
	"time"

	"github.com/google/uuid"
	// "gorm.io/gorm" // Se usando GORM
)

// DBUser representa a entidade User no banco de dados.
type DBUser struct {
	ID uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v4()"` // Usando UUID como no Python

	Username     string  `gorm:"type:varchar(50);uniqueIndex;not null"`
	Email        string  `gorm:"type:varchar(255);uniqueIndex;not null"`
	FullName     *string `gorm:"type:varchar(100)"` // Ponteiro para string opcional
	PasswordHash string  `gorm:"type:varchar(255);not null"`
	Active       bool    `gorm:"not null;default:true"`
	IsSuperuser  bool    `gorm:"not null;default:false"` // No seu Python era is_superuser

	FailedAttempts  int        `gorm:"not null;default:0"`
	LastFailedLogin *time.Time `gorm:"type:timestamptz"` // timestamptz para PostgreSQL, datetime para SQLite
	LastLogin       *time.Time `gorm:"type:timestamptz"`

	PasswordResetToken   *string    `gorm:"type:varchar(255);index"`
	PasswordResetExpires *time.Time `gorm:"type:timestamptz"`

	CreatedAt time.Time `gorm:"not null;default:now()"`
	UpdatedAt time.Time `gorm:"not null;default:now()"` // GORM atualiza com autoUpdateTime

	// Relação Muitos-para-Muitos com Roles.
	// Se não usar GORM, este campo será preenchido programaticamente pelo repositório.
	Roles []*DBRole `gorm:"many2many:user_roles;"` // Tag GORM para relação
	// Se não usar GORM, pode ser `gorm:"-"`
	// e Roles seria `[]string` (nomes) ou `[]*DBRole`
	// preenchido pelo repositório.
}

// TableName especifica o nome da tabela para GORM.
func (DBUser) TableName() string {
	return "users"
}

// --- Structs para Transferência de Dados e Validação ---

// UserBase contém campos comuns para criação e leitura.
type UserBase struct {
	Username string  `json:"username"`
	Email    string  `json:"email"`
	FullName *string `json:"full_name,omitempty"`
}

// UserCreate é usado para criar um novo usuário.
type UserCreate struct {
	Username string  `json:"username" validate:"required,min=3,max=50,username_custom_regex"` // username_custom_regex para ^[a-zA-Z0-9_-]+$
	Email    string  `json:"email" validate:"required,email"`
	FullName *string `json:"full_name,omitempty" validate:"omitempty,max=100"`
	Password string  `json:"password" validate:"required,min=12,password_strength"` // password_strength validador custom
	// RoleNames: Lista de NOMES de roles a serem atribuídos inicialmente.
	// O valor padrão ["user"] é uma boa prática.
	RoleNames []string `json:"role_names" validate:"omitempty,dive,min=1"`
}

// CleanAndValidate normaliza e valida os campos de UserCreate.
// O serviço chamaria este método. Username e Email são convertidos para minúsculas.
func (uc *UserCreate) CleanAndValidate() error {
	// Username
	cleanedUsername := strings.TrimSpace(uc.Username)
	if cleanedUsername == "" {
		return NewValidationError("Nome de usuário é obrigatório.", map[string]string{"username": "Nome de usuário obrigatório"})
	}
	// TODO: Chamar validador para formato do username (do utils/validators.go)
	// if !validators.IsValidUsernameFormat(cleanedUsername) {
	// 	return NewValidationError("Formato do nome de usuário inválido (letras, números, _, -).", map[string]string{"username": "Formato inválido"})
	// }
	uc.Username = strings.ToLower(cleanedUsername)

	// Email
	cleanedEmail := strings.TrimSpace(uc.Email)
	if cleanedEmail == "" {
		return NewValidationError("E-mail é obrigatório.", map[string]string{"email": "E-mail obrigatório"})
	}
	// TODO: Chamar validador de email (do utils/validators.go)
	// if !validators.IsValidEmail(cleanedEmail) {
	// 	return NewValidationError("Formato de e-mail inválido.", map[string]string{"email": "E-mail inválido"})
	// }
	uc.Email = strings.ToLower(cleanedEmail)

	// FullName (opcional)
	if uc.FullName != nil {
		*uc.FullName = strings.TrimSpace(*uc.FullName)
		if len(*uc.FullName) > 100 {
			return NewValidationError("Nome completo excede 100 caracteres.", map[string]string{"full_name": "Nome completo muito longo"})
		}
		if *uc.FullName == "" {
			uc.FullName = nil
		}
	}

	// Password (validação de força e comprimento é feita pelo serviço/validador de senha)
	if uc.Password == "" {
		return NewValidationError("Senha é obrigatória.", map[string]string{"password": "Senha obrigatória"})
	}
	// A validação de força real (min_length, charsets) seria feita usando um validador
	// em utils/validators.go, possivelmente chamado pelo UserService.

	// RoleNames (validação de se os roles existem é feita pelo UserService)
	if len(uc.RoleNames) == 0 { // Se vazio, define "user" como padrão
		uc.RoleNames = []string{"user"}
	}
	validRoleNames := []string{}
	for _, rName := range uc.RoleNames {
		rTrimmed := strings.TrimSpace(rName)
		if rTrimmed != "" {
			validRoleNames = append(validRoleNames, strings.ToLower(rTrimmed))
		}
	}
	uc.RoleNames = validRoleNames
	if len(uc.RoleNames) == 0 { // Se após limpeza ficou vazio, erro ou default novamente
		return NewValidationError("Pelo menos um role deve ser atribuído.", map[string]string{"role_names": "Atribuição de role obrigatória"})
	}

	return nil
}

// UserUpdate é usado para atualizar um usuário existente.
type UserUpdate struct {
	Email    *string `json:"email,omitempty" validate:"omitempty,email"`
	FullName *string `json:"full_name,omitempty" validate:"omitempty,max=100"`
	Active   *bool   `json:"active,omitempty"`
	// RoleNames: Se fornecido, substitui TODOS os roles do usuário.
	RoleNames *[]string `json:"role_names,omitempty" validate:"omitempty,dive,min=1"`
	// Outros campos que podem ser atualizados por um admin:
	// IsSuperuser     *bool
	// FailedAttempts  *int // Para resetar via admin
	// PasswordHash *string // Admin resetando senha diretamente (requer hash)
	// PasswordResetToken *string // Limpar token
	// PasswordResetExpires *time.Time // Limpar token
}

// CleanAndValidate para UserUpdate.
func (uu *UserUpdate) CleanAndValidate() error {
	if uu.Email != nil {
		cleanedEmail := strings.TrimSpace(*uu.Email)
		if cleanedEmail == "" {
			return NewValidationError("E-mail não pode ser vazio se fornecido para atualização.", map[string]string{"email": "E-mail não pode ser vazio"})
		}
		// TODO: Chamar validador de email
		// if !validators.IsValidEmail(cleanedEmail) {
		// 	return NewValidationError("Formato de e-mail inválido.", map[string]string{"email": "E-mail inválido"})
		// }
		*uu.Email = strings.ToLower(cleanedEmail)
	}
	if uu.FullName != nil {
		*uu.FullName = strings.TrimSpace(*uu.FullName)
		if len(*uu.FullName) > 100 {
			return NewValidationError("Nome completo excede 100 caracteres.", map[string]string{"full_name": "Nome muito longo"})
		}
		if *uu.FullName == "" {
			uu.FullName = nil
		}
	}
	if uu.RoleNames != nil {
		if len(*uu.RoleNames) == 0 {
			return NewValidationError("A lista de roles não pode ser vazia se fornecida para atualização (para remover todos os roles, passe um array vazio se a lógica permitir, ou use um método específico).", map[string]string{"role_names": "Lista de roles não pode ser vazia"})
		}
		validRoleNames := []string{}
		for _, rName := range *uu.RoleNames {
			rTrimmed := strings.TrimSpace(rName)
			if rTrimmed != "" {
				validRoleNames = append(validRoleNames, strings.ToLower(rTrimmed))
			}
		}
		*uu.RoleNames = validRoleNames
		if len(*uu.RoleNames) == 0 {
			return NewValidationError("Após limpeza, a lista de roles ficou vazia.", map[string]string{"role_names": "Nenhum role válido fornecido"})
		}
	}
	return nil
}

// UserPublic representa os dados públicos de um usuário para a UI ou API.
type UserPublic struct {
	ID        uuid.UUID  `json:"id"`
	Username  string     `json:"username"`
	Email     string     `json:"email"`
	FullName  *string    `json:"full_name,omitempty"`
	Active    bool       `json:"active"`
	Roles     []string   `json:"roles"` // Lista de NOMES de roles
	CreatedAt time.Time  `json:"created_at"`
	LastLogin *time.Time `json:"last_login,omitempty"`
}

// ToUserPublic converte um DBUser para UserPublic.
// O campo `DBUser.Roles` já deve estar populado com os objetos DBRole
// se você quiser extrair os nomes dos roles aqui.
func ToUserPublic(dbUser *DBUser) *UserPublic {
	if dbUser == nil {
		return nil
	}
	roleNames := make([]string, 0, len(dbUser.Roles))
	for _, role := range dbUser.Roles {
		if role != nil { // Checagem de segurança
			roleNames = append(roleNames, role.Name)
		}
	}

	return &UserPublic{
		ID:        dbUser.ID,
		Username:  dbUser.Username,
		Email:     dbUser.Email,
		FullName:  dbUser.FullName,
		Active:    dbUser.Active,
		Roles:     roleNames,
		CreatedAt: dbUser.CreatedAt,
		LastLogin: dbUser.LastLogin,
	}
}

// ToUserPublicList converte uma lista de DBUser para uma lista de UserPublic.
func ToUserPublicList(dbUsers []*DBUser) []*UserPublic {
	publicList := make([]*UserPublic, len(dbUsers))
	for i, dbUser := range dbUsers {
		publicList[i] = ToUserPublic(dbUser)
	}
	return publicList
}

// UserInDB representa o usuário como armazenado no banco, incluindo campos sensíveis.
// Usado internamente pelos serviços e repositórios.
// No Python, era `UserInDB`.
type UserInDB struct {
	DBUser // Embutir DBUser para ter todos os seus campos
	// PasswordHash já está em DBUser
	// PasswordResetToken já está em DBUser
	// Roles já está em DBUser (como []*DBRole)
}

// ToUserInDB é mais uma questão de garantir que os Roles estejam carregados.
// Normalmente, o repositório retornaria um *DBUser já com os roles.
// Esta função é mais para clareza se você precisar converter.
func ToUserInDB(dbUser *DBUser) *UserInDB {
	if dbUser == nil {
		return nil
	}
	// Assume que dbUser.Roles já está carregado pelo repositório
	return &UserInDB{
		DBUser: *dbUser,
	}
}
