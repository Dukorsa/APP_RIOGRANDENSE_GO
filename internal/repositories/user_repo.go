package repositories

import (
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	// Para Preload e outras cláusulas
	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/errors"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/models"
)

// UserRepository define a interface para operações no repositório de usuários.
type UserRepository interface {
	CreateUser(userData models.UserCreate, passwordHash string, initialRoles []*models.DBRole) (*models.DBUser, error)
	GetByID(userID uuid.UUID) (*models.DBUser, error)
	GetByEmail(email string) (*models.DBUser, error)
	GetByUsername(username string) (*models.DBUser, error)
	GetByUsernameOrEmail(identifier string) (*models.DBUser, error) // Combina GetByEmail e GetByUsername
	UpdateUser(userID uuid.UUID, updateData map[string]interface{}, newRoles []*models.DBRole) (*models.DBUser, error)
	UpdateLoginAttempts(userID uuid.UUID, failedAttempts int, lastFailedLogin *time.Time, lastLogin *time.Time) error
	GetAllUsers(includeInactive bool) ([]*models.DBUser, error)
	DeactivateUser(userID uuid.UUID) error
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
// Espera que userData já tenha sido limpa e validada pelo serviço.
func (r *gormUserRepository) CreateUser(userData models.UserCreate, passwordHash string, initialRoles []*models.DBRole) (*models.DBUser, error) {
	// O serviço já deve ter chamado CleanAndValidate em userData.
	// Username e Email já devem estar em minúsculas.

	// Verificar duplicação de username ou email antes de tentar criar
	var existingUser models.DBUser
	err := r.db.Where("LOWER(username) = LOWER(?) OR LOWER(email) = LOWER(?)", userData.Username, userData.Email).First(&existingUser).Error
	if err == nil { // Encontrou um usuário
		if strings.ToLower(existingUser.Username) == strings.ToLower(userData.Username) {
			return nil, fmt.Errorf("%w: nome de usuário '%s' já está em uso", appErrors.ErrConflict, userData.Username)
		}
		if strings.ToLower(existingUser.Email) == strings.ToLower(userData.Email) {
			return nil, fmt.Errorf("%w: endereço de e-mail '%s' já está em uso", appErrors.ErrConflict, userData.Email)
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) { // Erro diferente de "não encontrado"
		appLogger.Errorf("Erro ao verificar duplicidade de usuário/email para '%s'/'%s': %v", userData.Username, userData.Email, err)
		return nil, appErrors.WrapErrorf(err, "falha ao verificar duplicidade de usuário/email (GORM)")
	}
	// Se gorm.ErrRecordNotFound, podemos prosseguir.

	now := time.Now().UTC()
	dbUser := models.DBUser{
		// ID é gerado automaticamente (uuid_generate_v4)
		Username:     userData.Username,
		Email:        userData.Email,
		FullName:     userData.FullName, // Pode ser nil
		PasswordHash: passwordHash,
		Active:       true,  // Novos usuários são ativos por padrão
		IsSuperuser:  false, // Novos usuários não são superusuários por padrão
		CreatedAt:    now,
		UpdatedAt:    now,
		Roles:        initialRoles, // GORM associará via many2many se initialRoles não for nil
	}

	// Usar transação para criar usuário e suas associações de role
	txErr := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&dbUser).Error; err != nil {
			// GORM já deve tratar erros de UNIQUE constraint para username e email
			return appErrors.WrapErrorf(err, "falha ao criar usuário (GORM)")
		}
		// Se `dbUser.Roles` foi fornecido e GORM está configurado para many2many,
		// a associação deve ter sido tratada pelo `Create`.
		// Se precisasse de associação manual:
		// if len(initialRoles) > 0 {
		// 	if err := tx.Model(&dbUser).Association("Roles").Replace(initialRoles); err != nil {
		// 		return appErrors.WrapErrorf(err, "falha ao associar roles iniciais ao usuário (GORM)")
		// 	}
		// }
		return nil
	})

	if txErr != nil {
		appLogger.Errorf("Erro na transação de criação do usuário '%s': %v", userData.Username, txErr)
		return nil, txErr
	}

	// Recarregar o usuário com os roles para garantir que o objeto retornado esteja completo
	// (GORM Create com associação já deve retornar o objeto completo com ID e associações se configurado corretamente)
	// Mas uma recarga explícita não faz mal para garantir.
	err = r.db.Preload("Roles").First(&dbUser, "id = ?", dbUser.ID).Error
	if err != nil {
		appLogger.Warnf("Usuário '%s' criado, mas erro ao recarregar com roles: %v", dbUser.Username, err)
		// Retornar o dbUser sem roles preenchidos se o reload falhar, mas a criação principal foi ok.
	}

	appLogger.Infof("Novo usuário criado: '%s' (ID: %s) com %d roles iniciais.", dbUser.Username, dbUser.ID, len(dbUser.Roles))
	return &dbUser, nil
}

// Helper para buscar usuário com roles pré-carregados
func (r *gormUserRepository) getUserByCondition(condition string, value interface{}) (*models.DBUser, error) {
	var dbUser models.DBUser
	// Preload("Roles") garante que os roles associados sejam carregados junto com o usuário.
	if err := r.db.Preload("Roles").Where(condition, value).First(&dbUser).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Para GetByUsernameOrEmail, não sabemos qual campo não foi encontrado, então mensagem genérica
			if strings.Contains(condition, "username") && strings.Contains(condition, "email") {
				return nil, fmt.Errorf("%w: usuário com identificador '%v' não encontrado", appErrors.ErrNotFound, value)
			}
			return nil, fmt.Errorf("%w: usuário com %s = '%v' não encontrado", appErrors.ErrNotFound, strings.Split(condition, " ")[0], value)
		}
		appLogger.Errorf("Erro ao buscar usuário por '%s = %v': %v", condition, value, err)
		return nil, appErrors.WrapErrorf(err, fmt.Sprintf("falha ao buscar usuário por %s (GORM)", strings.Split(condition, " ")[0]))
	}
	return &dbUser, nil
}

// GetByID busca um usuário pelo ID, incluindo seus roles.
func (r *gormUserRepository) GetByID(userID uuid.UUID) (*models.DBUser, error) {
	return r.getUserByCondition("id = ?", userID)
}

// GetByEmail busca um usuário pelo email (case-insensitive), incluindo seus roles.
func (r *gormUserRepository) GetByEmail(email string) (*models.DBUser, error) {
	return r.getUserByCondition("LOWER(email) = LOWER(?)", email)
}

// GetByUsername busca um usuário pelo username (case-insensitive), incluindo seus roles.
func (r *gormUserRepository) GetByUsername(username string) (*models.DBUser, error) {
	return r.getUserByCondition("LOWER(username) = LOWER(?)", username)
}

// GetByUsernameOrEmail busca um usuário pelo username OU email (case-insensitive), incluindo roles.
func (r *gormUserRepository) GetByUsernameOrEmail(identifier string) (*models.DBUser, error) {
	return r.getUserByCondition("LOWER(username) = LOWER(?) OR LOWER(email) = LOWER(?)", identifier, identifier)
}

// UpdateUser atualiza dados do usuário e/ou seus roles.
// `updateData` é um mapa de campos a serem atualizados.
// `newRoles` (se não nil) substitui completamente os roles existentes do usuário.
func (r *gormUserRepository) UpdateUser(userID uuid.UUID, updateData map[string]interface{}, newRoles []*models.DBRole) (*models.DBUser, error) {
	// Buscar usuário existente primeiro
	dbUser, err := r.GetByID(userID) // GetByID já pré-carrega os roles
	if err != nil {
		return nil, err
	}

	// Lidar com username/email case-insensitivity e checagem de conflito se forem alterados
	if newUsernameVal, ok := updateData["username"]; ok {
		newUsername, _ := newUsernameVal.(string)
		if strings.ToLower(newUsername) != strings.ToLower(dbUser.Username) {
			existingUser, errGet := r.GetByUsername(newUsername)
			if errGet == nil && existingUser.ID != userID {
				return nil, fmt.Errorf("%w: nome de usuário '%s' já está em uso por outro usuário", appErrors.ErrConflict, newUsername)
			}
			if errGet != nil && !errors.Is(errGet, appErrors.ErrNotFound) {
				return nil, appErrors.WrapErrorf(errGet, "falha ao verificar novo nome de usuário (GORM)")
			}
			updateData["username"] = strings.ToLower(newUsername) // Garante que seja salvo em minúsculas
		}
	}
	if newEmailVal, ok := updateData["email"]; ok {
		newEmail, _ := newEmailVal.(string)
		if strings.ToLower(newEmail) != strings.ToLower(dbUser.Email) {
			existingUser, errGet := r.GetByEmail(newEmail)
			if errGet == nil && existingUser.ID != userID {
				return nil, fmt.Errorf("%w: e-mail '%s' já está em uso por outro usuário", appErrors.ErrConflict, newEmail)
			}
			if errGet != nil && !errors.Is(errGet, appErrors.ErrNotFound) {
				return nil, appErrors.WrapErrorf(errGet, "falha ao verificar novo e-mail (GORM)")
			}
			updateData["email"] = strings.ToLower(newEmail) // Garante que seja salvo em minúsculas
		}
	}

	// Adicionar UpdatedAt manualmente se não usar autoUpdateTime no GORM
	// updateData["updated_at"] = time.Now().UTC()

	txErr := r.db.Transaction(func(tx *gorm.DB) error {
		// Atualizar campos básicos do usuário
		if len(updateData) > 0 {
			if err := tx.Model(&models.DBUser{}).Where("id = ?", userID).Updates(updateData).Error; err != nil {
				// GORM já deve tratar erros de UNIQUE constraint
				return appErrors.WrapErrorf(err, "falha ao atualizar campos do usuário (GORM)")
			}
		}

		// Se newRoles for fornecido, substitui os roles existentes
		if newRoles != nil {
			// `Replace` remove associações antigas e adiciona as novas para many2many
			if err := tx.Model(&dbUser).Association("Roles").Replace(newRoles); err != nil {
				return appErrors.WrapErrorf(err, "falha ao atualizar associação de roles do usuário (GORM)")
			}
		}
		return nil // Commit
	})

	if txErr != nil {
		appLogger.Errorf("Erro na transação de atualização do usuário ID %s: %v", userID, txErr)
		return nil, txErr
	}

	// Recarregar o usuário com os roles atualizados
	updatedUser, err := r.GetByID(userID)
	if err != nil {
		appLogger.Errorf("Falha ao recarregar usuário ID %s após update: %v", userID, err)
		return nil, err
	}

	appLogger.Infof("Usuário ID %s ('%s') atualizado. Campos: %v. Roles: %d",
		userID, updatedUser.Username, maps.Keys(updateData), len(updatedUser.Roles)) // Go 1.21+ maps.Keys
	return updatedUser, nil
}

// UpdateLoginAttempts atualiza dados de tentativas de login e último acesso.
func (r *gormUserRepository) UpdateLoginAttempts(userID uuid.UUID, failedAttempts int, lastFailedLogin *time.Time, lastLogin *time.Time) error {
	updates := map[string]interface{}{
		"failed_attempts":   failedAttempts,
		"last_failed_login": lastFailedLogin, // GORM lida com *time.Time (NULL se nil)
		// "updated_at": time.Now().UTC(), // Se não usar autoUpdateTime
	}
	if lastLogin != nil { // Se for um login bem-sucedido, lastLogin é preenchido
		updates["last_login"] = lastLogin
	}

	result := r.db.Model(&models.DBUser{}).Where("id = ?", userID).Updates(updates)
	if result.Error != nil {
		appLogger.Errorf("Erro ao atualizar dados de login para usuário ID %s: %v", userID, result.Error)
		return appErrors.WrapErrorf(result.Error, "falha ao atualizar dados de login (GORM)")
	}
	if result.RowsAffected == 0 {
		appLogger.Warnf("Tentativa de atualizar login para usuário ID %s, mas nenhuma linha foi afetada (usuário existe?).", userID)
		// Não retorna erro, pois o usuário pode não existir, o que é tratado no fluxo de login.
	}
	return nil
}

// GetAllUsers lista todos os usuários, opcionalmente incluindo inativos, e pré-carrega roles.
func (r *gormUserRepository) GetAllUsers(includeInactive bool) ([]*models.DBUser, error) {
	var users []*models.DBUser
	query := r.db.Preload("Roles").Order("username ASC")

	if !includeInactive {
		query = query.Where("active = ?", true)
	}

	if err := query.Find(&users).Error; err != nil {
		appLogger.Errorf("Erro ao buscar todos os usuários: %v", err)
		return nil, appErrors.WrapErrorf(err, "falha ao buscar lista de usuários (GORM)")
	}
	return users, nil
}

// DeactivateUser realiza a exclusão LÓGICA do usuário (marca como inativo).
func (r *gormUserRepository) DeactivateUser(userID uuid.UUID) error {
	// updated_at será atualizado pelo GORM ou manualmente
	result := r.db.Model(&models.DBUser{}).Where("id = ?", userID).Updates(map[string]interface{}{
		"active": false,
		// "updated_at": time.Now().UTC(),
	})
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
func (r *gormUserRepository) UpdatePasswordResetToken(userID uuid.UUID, tokenHash *string, expires *time.Time) error {
	updates := map[string]interface{}{
		"password_reset_token":   tokenHash, // Pode ser nil para limpar
		"password_reset_expires": expires,   // Pode ser nil para limpar
		// "updated_at": time.Now().UTC(),
	}
	result := r.db.Model(&models.DBUser{}).Where("id = ?", userID).Updates(updates)
	if result.Error != nil {
		appLogger.Errorf("Erro de DB ao atualizar token de reset para %s: %v", userID, result.Error)
		return appErrors.WrapErrorf(result.Error, "falha ao atualizar token de recuperação de senha (GORM)")
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("%w: usuário com ID %s não encontrado para token de reset", appErrors.ErrNotFound, userID)
	}
	return nil
}

// UpdatePasswordHash atualiza apenas o hash da senha e reseta campos de login.
func (r *gormUserRepository) UpdatePasswordHash(userID uuid.UUID, newPasswordHash string) error {
	updates := map[string]interface{}{
		"password_hash":          newPasswordHash,
		"failed_attempts":        0,
		"last_failed_login":      nil, // Limpa o último login falho
		"password_reset_token":   nil, // Limpa tokens de reset após uso
		"password_reset_expires": nil,
		// "updated_at": time.Now().UTC(),
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
