package repositories

import (
	"errors"
	"fmt"
	"maps" // Requer Go 1.21+; para versões anteriores, use um helper ou itere.
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/errors"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/models"
)

// UserRepository define a interface para operações no repositório de usuários.
type UserRepository interface {
	// CreateUser cria um novo usuário. `userData.Username` e `userData.Email` devem estar normalizados (minúsculas).
	// `passwordHash` é o hash da senha. `initialRoles` são os DBRoles a serem associados.
	CreateUser(userData models.UserCreate, passwordHash string, initialRoles []*models.DBRole) (*models.DBUser, error)

	GetByID(userID uuid.UUID) (*models.DBUser, error)
	GetByEmail(email string) (*models.DBUser, error)                // `email` deve estar normalizado (minúsculas).
	GetByUsername(username string) (*models.DBUser, error)          // `username` deve estar normalizado (minúsculas).
	GetByUsernameOrEmail(identifier string) (*models.DBUser, error) // `identifier` deve estar normalizado.

	// UpdateUser atualiza dados do usuário e/ou seus roles.
	// `updateData` é um mapa de campos básicos a serem atualizados.
	// `newRoles` (se não nil) substitui completamente os roles existentes do usuário.
	UpdateUser(userID uuid.UUID, updateData map[string]interface{}, newRoles []*models.DBRole) (*models.DBUser, error)

	UpdateLoginAttempts(userID uuid.UUID, failedAttempts int, lastFailedLogin *time.Time, lastLogin *time.Time) error
	GetAllUsers(includeInactive bool) ([]*models.DBUser, error)
	DeactivateUser(userID uuid.UUID) error // Exclusão lógica.
	UpdatePasswordResetToken(userID uuid.UUID, tokenHash *string, expires *time.Time) error
	UpdatePasswordHash(userID uuid.UUID, newPasswordHash string) error
}

// gormUserRepository é a implementação GORM de UserRepository.
type gormUserRepository struct {
	db *gorm.DB
}

// NewGormUserRepository cria uma nova instância de gormUserRepository.
func NewGormUserRepository(db *gorm.DB) UserRepository {
	if db == nil {
		appLogger.Fatalf("gorm.DB não pode ser nil para NewGormUserRepository")
	}
	return &gormUserRepository{db: db}
}

// CreateUser cria um novo usuário no banco de dados e associa roles iniciais.
// `userData.Username` e `userData.Email` já devem estar normalizados (minúsculas) pelo serviço.
// `initialRoles` são os DBRoles (com IDs) a serem associados.
func (r *gormUserRepository) CreateUser(userData models.UserCreate, passwordHash string, initialRoles []*models.DBRole) (*models.DBUser, error) {
	// O serviço já deve ter chamado CleanAndValidate em userData.
	// A verificação de duplicidade de username/email também é responsabilidade primária do serviço
	// para fornecer feedback de validação mais específico. O DB constraint é a garantia final.

	// Segurança: verificar novamente a existência no repositório para mitigar race conditions,
	// embora a constraint UNIQUE do DB seja a defesa final.
	var existingUser models.DBUser
	errCheck := r.db.Where("username = ? OR email = ?", userData.Username, userData.Email).First(&existingUser).Error
	if errCheck == nil { // Encontrou um usuário com mesmo username ou email.
		if existingUser.Username == userData.Username {
			return nil, fmt.Errorf("%w: nome de usuário '%s' já está em uso (conflito no DB)", appErrors.ErrConflict, userData.Username)
		}
		if existingUser.Email == userData.Email {
			return nil, fmt.Errorf("%w: endereço de e-mail '%s' já está em uso (conflito no DB)", appErrors.ErrConflict, userData.Email)
		}
	} else if !errors.Is(errCheck, gorm.ErrRecordNotFound) {
		appLogger.Errorf("Erro ao verificar duplicidade de usuário/email para '%s'/'%s' antes de criar: %v", userData.Username, userData.Email, errCheck)
		return nil, appErrors.WrapErrorf(errCheck, "falha ao verificar duplicidade de usuário/email (GORM)")
	}
	// Se gorm.ErrRecordNotFound, podemos prosseguir.

	dbUser := models.DBUser{
		// ID é gerado automaticamente pelo DB (uuid_generate_v4).
		Username:     userData.Username, // Já normalizado.
		Email:        userData.Email,    // Já normalizado.
		FullName:     userData.FullName,
		PasswordHash: passwordHash,
		Active:       true,         // Novos usuários são ativos por padrão.
		IsSuperuser:  false,        // Default.
		Roles:        initialRoles, // GORM associará na criação se `initialRoles` não for nil e contiver DBRoles válidos.
		// CreatedAt e UpdatedAt são gerenciados por `autoCreateTime` e `autoUpdateTime` do GORM.
	}

	// Usar transação para criar usuário e suas associações de role.
	txErr := r.db.Transaction(func(tx *gorm.DB) error {
		// `Create` com associações (dbUser.Roles) deve criar o usuário e as entradas na tabela de junção.
		if err := tx.Create(&dbUser).Error; err != nil {
			// GORM já deve tratar erros de UNIQUE constraint para username e email se configurado no modelo/DB.
			return appErrors.WrapErrorf(err, "falha ao criar usuário (GORM)")
		}
		// Se a associação de roles não funcionar automaticamente com `Create` (depende da config GORM e do modelo),
		// seria necessário associá-los explicitamente aqui dentro da transação:
		// if len(initialRoles) > 0 {
		// 	if err := tx.Model(&dbUser).Association("Roles").Replace(initialRoles); err != nil {
		// 		return appErrors.WrapErrorf(err, "falha ao associar roles iniciais ao usuário (GORM)")
		// 	}
		// }
		return nil // Commit.
	})

	if txErr != nil {
		appLogger.Errorf("Erro na transação de criação do usuário '%s': %v", userData.Username, txErr)
		return nil, txErr
	}

	// Recarregar o usuário com os roles para garantir que o objeto retornado esteja completo.
	// O GORM `Create` com associações pode já retornar o objeto completo, mas uma recarga explícita é mais segura.
	var createdUserWithRoles models.DBUser
	if err := r.db.Preload("Roles").First(&createdUserWithRoles, "id = ?", dbUser.ID).Error; err != nil {
		appLogger.Warnf("Usuário '%s' (ID: %s) criado, mas erro ao recarregar com roles: %v", dbUser.Username, dbUser.ID, err)
		// Retornar o dbUser original (sem roles recarregados) se o reload falhar, mas a criação foi ok.
		// Ou considerar isso um erro que impede o retorno bem-sucedido.
		return &dbUser, nil // Ou: return nil, appErrors.WrapErrorf(err, "falha ao recarregar usuário após criação")
	}

	appLogger.Infof("Novo usuário criado: '%s' (ID: %s) com %d roles iniciais.", createdUserWithRoles.Username, createdUserWithRoles.ID, len(createdUserWithRoles.Roles))
	return &createdUserWithRoles, nil
}

// getUserByCondition é um helper interno para buscar usuário com roles pré-carregados.
// `condition` é a string da query SQL (ex: "id = ?"), `value` é o argumento.
func (r *gormUserRepository) getUserByCondition(condition string, value interface{}) (*models.DBUser, error) {
	var dbUser models.DBUser
	// `Preload("Roles")` garante que os roles associados sejam carregados junto com o usuário.
	if err := r.db.Preload("Roles").Where(condition, value).First(&dbUser).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Para GetByUsernameOrEmail, não sabemos qual campo não foi encontrado.
			if strings.Contains(condition, "username = ? OR email = ?") { // Adaptar para a query real
				return nil, fmt.Errorf("%w: usuário com identificador '%v' não encontrado", appErrors.ErrNotFound, value)
			}
			// Extrai o nome do campo da condição para a mensagem de erro.
			fieldName := strings.Split(strings.Fields(condition)[0], ".")[0] // Pega "id", "username", "email"
			return nil, fmt.Errorf("%w: usuário com %s = '%v' não encontrado", appErrors.ErrNotFound, fieldName, value)
		}
		appLogger.Errorf("Erro ao buscar usuário por '%s = %v': %v", condition, value, err)
		fieldNameForLog := strings.Split(strings.Fields(condition)[0], ".")[0]
		return nil, appErrors.WrapErrorf(err, fmt.Sprintf("falha ao buscar usuário por %s (GORM)", fieldNameForLog))
	}
	return &dbUser, nil
}

// GetByID busca um usuário pelo ID, incluindo seus roles.
func (r *gormUserRepository) GetByID(userID uuid.UUID) (*models.DBUser, error) {
	return r.getUserByCondition("id = ?", userID)
}

// GetByEmail busca um usuário pelo email (normalizado para minúsculas), incluindo seus roles.
func (r *gormUserRepository) GetByEmail(email string) (*models.DBUser, error) {
	// Assume que `email` já foi normalizado (lowercase) pelo serviço.
	if email == "" {
		return nil, fmt.Errorf("%w: email não pode ser vazio para busca", appErrors.ErrInvalidInput)
	}
	return r.getUserByCondition("email = ?", email) // DB armazena email em minúsculas.
}

// GetByUsername busca um usuário pelo username (normalizado para minúsculas), incluindo roles.
func (r *gormUserRepository) GetByUsername(username string) (*models.DBUser, error) {
	// Assume que `username` já foi normalizado (lowercase) pelo serviço.
	if username == "" {
		return nil, fmt.Errorf("%w: username não pode ser vazio para busca", appErrors.ErrInvalidInput)
	}
	return r.getUserByCondition("username = ?", username) // DB armazena username em minúsculas.
}

// GetByUsernameOrEmail busca um usuário pelo username OU email (normalizados), incluindo roles.
func (r *gormUserRepository) GetByUsernameOrEmail(identifier string) (*models.DBUser, error) {
	// Assume que `identifier` já foi normalizado (lowercase) pelo serviço.
	if identifier == "" {
		return nil, fmt.Errorf("%w: identificador (username/email) não pode ser vazio para busca", appErrors.ErrInvalidInput)
	}
	// DB armazena username e email em minúsculas.
	return r.getUserByCondition("username = ? OR email = ?", identifier, identifier)
}

// UpdateUser atualiza dados do usuário e/ou seus roles.
// `updateData` é um mapa de campos básicos a serem atualizados.
// `newRoles` (se não nil) substitui completamente os roles existentes do usuário.
// Campos como `username` e `email` em `updateData` devem estar normalizados.
func (r *gormUserRepository) UpdateUser(userID uuid.UUID, updateData map[string]interface{}, newRoles []*models.DBRole) (*models.DBUser, error) {
	// Buscar usuário existente primeiro para ter o objeto GORM para atualizações de associação.
	userToUpdate, err := r.GetByID(userID) // GetByID já pré-carrega os roles atuais.
	if err != nil {
		return nil, err // Trata ErrNotFound ou DB error.
	}

	// O serviço deve ter validado a unicidade de username/email se estiverem sendo alterados.
	// A constraint do DB é a garantia final.

	// Adicionar `updated_at` manualmente se não usar `autoUpdateTime` no GORM.
	// Se `autoUpdateTime` estiver ativo, o GORM cuida disso.
	// if len(updateData) > 0 || newRoles != nil { // Apenas se houver alguma mudança
	// 	updateData["updated_at"] = time.Now().UTC()
	// }

	txErr := r.db.Transaction(func(tx *gorm.DB) error {
		// Atualizar campos básicos do usuário se `updateData` não estiver vazio.
		if len(updateData) > 0 {
			if err := tx.Model(&models.DBUser{}).Where("id = ?", userID).Updates(updateData).Error; err != nil {
				// GORM deve tratar erros de UNIQUE constraint (username, email).
				return appErrors.WrapErrorf(err, "falha ao atualizar campos do usuário (GORM)")
			}
		}

		// Se `newRoles` for fornecido (não nil), substitui os roles existentes.
		// Um slice vazio `[]*models.DBRole{}` removerá todos os roles.
		if newRoles != nil {
			// `Replace` remove associações antigas e adiciona as novas para many2many.
			// Requer que `userToUpdate` seja o objeto GORM do usuário.
			if err := tx.Model(&userToUpdate).Association("Roles").Replace(newRoles); err != nil {
				return appErrors.WrapErrorf(err, "falha ao atualizar associação de roles do usuário (GORM)")
			}
		}
		return nil // Commit.
	})

	if txErr != nil {
		appLogger.Errorf("Erro na transação de atualização do usuário ID %s: %v", userID, txErr)
		return nil, txErr
	}

	// Recarregar o usuário com os roles atualizados para retornar o estado mais recente.
	updatedUser, errLoad := r.GetByID(userID)
	if errLoad != nil {
		appLogger.Errorf("Falha ao recarregar usuário ID %s após update: %v", userID, errLoad)
		return nil, fmt.Errorf("falha ao recarregar usuário após atualização: %w", errLoad)
	}

	appLogger.Infof("Usuário ID %s ('%s') atualizado. Campos básicos alterados: %v. Total de roles: %d",
		userID, updatedUser.Username, maps.Keys(updateData), len(updatedUser.Roles))
	return updatedUser, nil
}

// UpdateLoginAttempts atualiza dados de tentativas de login e último acesso.
func (r *gormUserRepository) UpdateLoginAttempts(userID uuid.UUID, failedAttempts int, lastFailedLogin *time.Time, lastLogin *time.Time) error {
	updates := map[string]interface{}{
		"failed_attempts":   failedAttempts,
		"last_failed_login": lastFailedLogin, // GORM lida com *time.Time (NULL se o ponteiro for nil).
		// `updated_at` é gerenciado pelo GORM `autoUpdateTime`.
	}
	if lastLogin != nil { // Se for um login bem-sucedido, `lastLogin` é preenchido.
		updates["last_login"] = lastLogin
	}

	result := r.db.Model(&models.DBUser{}).Where("id = ?", userID).Updates(updates)
	if result.Error != nil {
		appLogger.Errorf("Erro ao atualizar dados de login para usuário ID %s: %v", userID, result.Error)
		return appErrors.WrapErrorf(result.Error, "falha ao atualizar dados de login (GORM)")
	}
	if result.RowsAffected == 0 {
		// Isso pode acontecer se o userID não existir. O fluxo de login deve tratar isso.
		appLogger.Warnf("Tentativa de atualizar login para usuário ID %s, mas nenhuma linha foi afetada (usuário existe?).", userID)
	}
	return nil
}

// GetAllUsers lista todos os usuários, opcionalmente incluindo inativos, e pré-carrega roles.
// Ordena por username.
func (r *gormUserRepository) GetAllUsers(includeInactive bool) ([]*models.DBUser, error) {
	var users []*models.DBUser
	query := r.db.Preload("Roles").Order("username ASC") // Username já está em minúsculas no DB.

	if !includeInactive {
		query = query.Where("active = ?", true)
	}

	if err := query.Find(&users).Error; err != nil {
		appLogger.Errorf("Erro ao buscar todos os usuários (includeInactive: %t): %v", includeInactive, err)
		return nil, appErrors.WrapErrorf(err, "falha ao buscar lista de usuários (GORM)")
	}
	return users, nil
}

// DeactivateUser realiza a exclusão LÓGICA do usuário (marca como `active = false`).
func (r *gormUserRepository) DeactivateUser(userID uuid.UUID) error {
	// `updated_at` será atualizado pelo GORM `autoUpdateTime`.
	updates := map[string]interface{}{
		"active": false,
		// Opcional: Limpar tokens de sessão ou reset de senha ao desativar.
		// "password_reset_token":   nil,
		// "password_reset_expires": nil,
	}
	result := r.db.Model(&models.DBUser{}).Where("id = ?", userID).Updates(updates)
	if result.Error != nil {
		appLogger.Errorf("Erro de DB ao desativar usuário ID %s: %v", userID, result.Error)
		return appErrors.WrapErrorf(result.Error, "falha ao desativar usuário (GORM)")
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("%w: usuário com ID %s não encontrado para desativação", appErrors.ErrNotFound, userID)
	}
	appLogger.Infof("Usuário ID %s desativado logicamente.", userID)
	return nil
}

// UpdatePasswordResetToken atualiza o token de reset e a expiração.
// `tokenHash` e `expires` podem ser nil para limpar os campos.
func (r *gormUserRepository) UpdatePasswordResetToken(userID uuid.UUID, tokenHash *string, expires *time.Time) error {
	updates := map[string]interface{}{
		"password_reset_token":   tokenHash,
		"password_reset_expires": expires,
		// `updated_at` gerenciado pelo GORM.
	}
	result := r.db.Model(&models.DBUser{}).Where("id = ?", userID).Updates(updates)
	if result.Error != nil {
		appLogger.Errorf("Erro de DB ao atualizar token de reset para %s: %v", userID, result.Error)
		return appErrors.WrapErrorf(result.Error, "falha ao atualizar token de recuperação de senha (GORM)")
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("%w: usuário com ID %s não encontrado para atualização de token de reset", appErrors.ErrNotFound, userID)
	}
	return nil
}

// UpdatePasswordHash atualiza apenas o hash da senha e reseta campos de login/reset.
func (r *gormUserRepository) UpdatePasswordHash(userID uuid.UUID, newPasswordHash string) error {
	updates := map[string]interface{}{
		"password_hash":          newPasswordHash,
		"failed_attempts":        0,   // Reseta contador de falhas.
		"last_failed_login":      nil, // Limpa o último login falho.
		"password_reset_token":   nil, // Limpa tokens de reset após uso ou mudança de senha.
		"password_reset_expires": nil,
		// `updated_at` gerenciado pelo GORM.
	}
	result := r.db.Model(&models.DBUser{}).Where("id = ?", userID).Updates(updates)
	if result.Error != nil {
		appLogger.Errorf("Erro de DB ao atualizar hash de senha para %s: %v", userID, result.Error)
		return appErrors.WrapErrorf(result.Error, "falha ao atualizar hash de senha (GORM)")
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("%w: usuário com ID %s não encontrado para atualização de senha", appErrors.ErrNotFound, userID)
	}
	return nil
}
