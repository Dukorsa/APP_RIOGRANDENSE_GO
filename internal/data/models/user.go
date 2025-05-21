package models

import (
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	// "gorm.io/gorm" // Descomentado se GORM for usado diretamente aqui

	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core"
	// "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/utils" // Para validadores se usados no modelo
)

// DBUser representa a entidade User no banco de dados.
type DBUser struct {
	// ID é a chave primária, usando UUID v4 por padrão.
	ID uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v4()"`

	// Username é único e armazenado em minúsculas.
	Username string `gorm:"type:varchar(50);uniqueIndex;not null"`

	// Email é único e armazenado em minúsculas.
	Email string `gorm:"type:varchar(255);uniqueIndex;not null"`

	FullName     *string `gorm:"type:varchar(100)"`          // Nome completo do usuário (opcional).
	PasswordHash string  `gorm:"type:varchar(255);not null"` // Hash da senha do usuário.
	Active       bool    `gorm:"not null;default:true"`      // Status do usuário (ativo/inativo).
	IsSuperuser  bool    `gorm:"not null;default:false"`     // Indica se o usuário é um superusuário (raro).

	// Campos para controle de login e bloqueio de conta.
	FailedAttempts  int        `gorm:"not null;default:0"` // Número de tentativas de login falhas consecutivas.
	LastFailedLogin *time.Time `gorm:"type:timestamptz"`   // Timestamp da última tentativa de login falha.
	LastLogin       *time.Time `gorm:"type:timestamptz"`   // Timestamp do último login bem-sucedido.

	// Campos para o processo de redefinição de senha.
	PasswordResetToken   *string    `gorm:"type:varchar(255);index"` // Token (hash) para redefinição de senha.
	PasswordResetExpires *time.Time `gorm:"type:timestamptz"`        // Timestamp de expiração do token de reset.

	// Campos de auditoria padrão do GORM.
	CreatedAt time.Time `gorm:"not null;autoCreateTime"` // GORM preenche na criação.
	UpdatedAt time.Time `gorm:"not null;autoUpdateTime"` // GORM preenche na criação e atualização.

	// Relação Muitos-para-Muitos com Roles.
	// `gorm:"many2many:user_roles;"` instrui o GORM a usar a tabela de junção `user_roles`.
	// Os DBRoles associados são pré-carregados pelo repositório quando necessário.
	Roles []*DBRole `gorm:"many2many:user_roles;"`
}

// TableName especifica o nome da tabela para GORM.
func (DBUser) TableName() string {
	return "users"
}

// --- Structs para Transferência de Dados (DTOs) e Validação ---

// UserCreate é usado para criar um novo usuário.
// As tags `validate` são para uso com bibliotecas de validação como `go-playground/validator`.
type UserCreate struct {
	// `validate:"required,min=3,max=50,username_format"` sugere validação customizada.
	Username string `json:"username" validate:"required,min=3,max=50,username_format"`

	// `validate:"required,email"` usa a validação de e-mail padrão da biblioteca.
	Email string `json:"email" validate:"required,email"`

	FullName *string `json:"full_name,omitempty" validate:"omitempty,max=100"`

	// `validate:"required,min=12,password_strength"` sugere validação customizada para força da senha.
	// O comprimento mínimo (ex: 12) deve vir da configuração da aplicação.
	Password string `json:"password" validate:"required,password_strength"`

	// RoleNames é uma lista de NOMES de roles a serem atribuídos inicialmente.
	// O serviço validará se esses roles existem e os converterá para DBRoles.
	// `validate:"omitempty,dive,min=1"`: opcional, mas se presente, cada nome de role deve ter min 1 caractere.
	RoleNames []string `json:"role_names" validate:"omitempty,dive,min=1"`
}

var (
	// Regex para Username: letras (Unicode), números, underscore, hífen.
	usernameValidationRegex = regexp.MustCompile(`^[\p{L}\d_-]{3,50}$`)
	// Regex para Email (simplificada, a validação principal é com net/mail).
	// emailValidationRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
)

// CleanAndValidate normaliza e valida os campos de UserCreate.
// O serviço deve chamar este método. Username e Email são convertidos para minúsculas.
func (uc *UserCreate) CleanAndValidate() error {
	// Normalizar e validar Username
	cleanedUsername := strings.TrimSpace(uc.Username)
	if cleanedUsername == "" {
		return appErrors.NewValidationError("Nome de usuário é obrigatório.", map[string]string{"username": "obrigatório"})
	}
	if !usernameValidationRegex.MatchString(cleanedUsername) {
		return appErrors.NewValidationError(
			"Nome de usuário deve ter entre 3 e 50 caracteres e conter apenas letras, números, '_' ou '-'.",
			map[string]string{"username": "formato inválido"},
		)
	}
	uc.Username = strings.ToLower(cleanedUsername)

	// Normalizar e validar Email
	cleanedEmail := strings.TrimSpace(uc.Email)
	if cleanedEmail == "" {
		return appErrors.NewValidationError("E-mail é obrigatório.", map[string]string{"email": "obrigatório"})
	}
	// A validação de formato de email mais robusta (ex: com net/mail) é feita no serviço
	// ou por uma tag de validação mais específica.
	// if !emailValidationRegex.MatchString(cleanedEmail) { // Exemplo de regex simples
	// 	return appErrors.NewValidationError("Formato de e-mail inválido.", map[string]string{"email": "formato inválido"})
	// }
	uc.Email = strings.ToLower(cleanedEmail)

	// Normalizar FullName (opcional)
	if uc.FullName != nil {
		*uc.FullName = strings.TrimSpace(*uc.FullName)
		if len(*uc.FullName) > 100 {
			return appErrors.NewValidationError("Nome completo excede 100 caracteres.", map[string]string{"full_name": "muito longo"})
		}
		if *uc.FullName == "" { // Se ficou vazia após trim, torna nil.
			uc.FullName = nil
		}
	}

	// Validar Password (a validação de força é feita no serviço usando utils.ValidatePasswordStrength)
	if strings.TrimSpace(uc.Password) == "" {
		return appErrors.NewValidationError("Senha é obrigatória.", map[string]string{"password": "obrigatória"})
	}
	// A validação de comprimento mínimo e complexidade é feita no serviço.

	// Normalizar RoleNames
	validRoleNames := []string{}
	if uc.RoleNames != nil {
		for _, rName := range uc.RoleNames {
			rTrimmedLower := strings.ToLower(strings.TrimSpace(rName))
			if rTrimmedLower != "" {
				validRoleNames = append(validRoleNames, rTrimmedLower)
			}
		}
	}
	// Se nenhum role for fornecido, o serviço pode atribuir um role padrão (ex: "user").
	uc.RoleNames = validRoleNames

	return nil
}

// UserUpdate é usado para atualizar um usuário existente.
// Ponteiros indicam campos opcionais para atualização.
type UserUpdate struct {
	Email    *string `json:"email,omitempty" validate:"omitempty,email"`
	FullName *string `json:"full_name,omitempty" validate:"omitempty,max=100"`
	Active   *bool   `json:"active,omitempty"`

	// RoleNames: Se fornecido (não nil), substitui TODOS os roles do usuário.
	// Um slice vazio `[]string{}` removeria todos os roles (se permitido pela lógica de negócio).
	RoleNames *[]string `json:"role_names,omitempty" validate:"omitempty,dive,min=1"`

	// Outros campos que podem ser atualizados por um administrador:
	// IsSuperuser     *bool      `json:"is_superuser,omitempty"`
	// FailedAttempts  *int       `json:"failed_attempts,omitempty"` // Para resetar bloqueio
}

// CleanAndValidate normaliza e valida os campos de UserUpdate que foram fornecidos.
func (uu *UserUpdate) CleanAndValidate() error {
	if uu.Email != nil {
		cleanedEmail := strings.TrimSpace(*uu.Email)
		if cleanedEmail == "" {
			return appErrors.NewValidationError("E-mail não pode ser vazio se fornecido para atualização.", map[string]string{"email": "não pode ser vazio"})
		}
		// Validação de formato de email no serviço ou tag.
		*uu.Email = strings.ToLower(cleanedEmail)
	}

	if uu.FullName != nil {
		*uu.FullName = strings.TrimSpace(*uu.FullName)
		if len(*uu.FullName) > 100 {
			return appErrors.NewValidationError("Nome completo excede 100 caracteres.", map[string]string{"full_name": "muito longo"})
		}
		if *uu.FullName == "" {
			uu.FullName = nil
		}
	}

	if uu.RoleNames != nil { // Se o ponteiro para o slice não for nil
		validRoleNames := []string{}
		for _, rName := range *uu.RoleNames { // Desreferencia o slice
			rTrimmedLower := strings.ToLower(strings.TrimSpace(rName))
			if rTrimmedLower != "" {
				validRoleNames = append(validRoleNames, rTrimmedLower)
			}
		}
		if len(*uu.RoleNames) > 0 && len(validRoleNames) == 0 {
			// Se foi fornecido um slice não vazio, mas todos os nomes eram inválidos/vazios.
			return appErrors.NewValidationError("Nenhum nome de role válido fornecido para atualização.", map[string]string{"role_names": "nomes inválidos"})
		}
		*uu.RoleNames = validRoleNames
	}
	return nil
}

// UserPublic representa os dados públicos de um usuário para a UI ou API (DTO).
type UserPublic struct {
	ID        uuid.UUID  `json:"id"`
	Username  string     `json:"username"`
	Email     string     `json:"email"`
	FullName  *string    `json:"full_name,omitempty"`
	Active    bool       `json:"active"`
	Roles     []string   `json:"roles"` // Lista de NOMES de roles.
	CreatedAt time.Time  `json:"created_at"`
	LastLogin *time.Time `json:"last_login,omitempty"` // Opcional, para exibir na UI.
}

// ToUserPublic converte um DBUser (modelo do banco) para UserPublic (DTO).
// O campo `DBUser.Roles` já deve estar populado com os objetos DBRole
// se os nomes dos roles precisarem ser extraídos.
func ToUserPublic(dbUser *DBUser) *UserPublic {
	if dbUser == nil {
		return nil
	}

	roleNames := make([]string, 0, len(dbUser.Roles))
	for _, role := range dbUser.Roles {
		if role != nil { // Checagem de segurança
			roleNames = append(roleNames, role.Name) // Role.Name já deve estar em minúsculas.
		}
	}

	return &UserPublic{
		ID:        dbUser.ID,
		Username:  dbUser.Username, // Já em minúsculas no DB.
		Email:     dbUser.Email,    // Já em minúsculas no DB.
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
// Usado internamente pelos serviços e repositórios para operações que necessitam
// de todos os campos, como PasswordHash.
// É essencialmente o mesmo que DBUser, mas pode ser usado para diferenciar
// semanticamente quando todos os campos são esperados/necessários.
// Em Go, é comum usar diretamente DBUser para este propósito se a estrutura for a mesma.
type UserInDB struct {
	DBUser // Embutir DBUser para ter todos os seus campos.
}

// ToUserInDB converte um DBUser para UserInDB.
// Usado principalmente para clareza semântica se necessário.
// O repositório já deve retornar DBUser com todos os campos necessários carregados.
func ToUserInDB(dbUser *DBUser) *UserInDB {
	if dbUser == nil {
		return nil
	}
	// Garante que dbUser.Roles (e outras associações, se houver) estejam carregados
	// se UserInDB tiver expectativas sobre esses campos.
	// O repositório que busca o DBUser é responsável por pré-carregar associações.
	return &UserInDB{
		DBUser: *dbUser,
	}
}
