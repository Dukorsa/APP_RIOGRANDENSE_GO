package auth

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/errors"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/repositories"
	"github.comcom/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/models"
)

// Permission representa uma permissão no sistema.
type Permission string

// Definição de todas as permissões possíveis no sistema.
const (
	// Network Permissions
	PermNetworkView    Permission = "network:view"
	PermNetworkCreate  Permission = "network:create"
	PermNetworkUpdate  Permission = "network:update"
	PermNetworkDelete  Permission = "network:delete"
	PermNetworkStatus  Permission = "network:status"
	PermNetworkViewOwn Permission = "network:view_own"

	// CNPJ Permissions
	PermCNPJView   Permission = "cnpj:view"
	PermCNPJCreate Permission = "cnpj:create"
	PermCNPJUpdate Permission = "cnpj:update"
	PermCNPJDelete Permission = "cnpj:delete"

	// User Management Permissions
	PermUserCreate        Permission = "user:create"
	PermUserRead          Permission = "user:read"
	PermUserUpdate        Permission = "user:update"
	PermUserDelete        Permission = "user:delete"
	PermUserResetPassword Permission = "user:reset_password"
	PermUserUnlock        Permission = "user:unlock"
	PermUserManageRoles   Permission = "user:manage_roles"

	// Role Management Permissions
	PermRoleManage Permission = "role:manage"

	// Data Export Permissions
	PermExportData Permission = "export:data"

	// Audit Log Permissions
	PermLogView Permission = "log:view"

	// Import Permissions
	PermImportExecute    Permission = "import:execute"
	PermImportViewStatus Permission = "import:view_status"
)

// allDefinedPermissions mantém um mapa de todas as permissões definidas e suas descrições.
var allDefinedPermissions = map[Permission]string{
	PermNetworkView:    "Visualizar dados de redes",
	PermNetworkCreate:  "Criar novas redes",
	PermNetworkUpdate:  "Atualizar dados de redes existentes",
	PermNetworkDelete:  "Excluir redes",
	PermNetworkStatus:  "Alterar status (ativo/inativo) de redes",
	PermNetworkViewOwn: "Visualizar apenas as redes criadas pelo próprio usuário",

	PermCNPJView:   "Visualizar CNPJs cadastrados",
	PermCNPJCreate: "Cadastrar novos CNPJs",
	PermCNPJUpdate: "Atualizar CNPJs existentes",
	PermCNPJDelete: "Excluir CNPJs",

	PermUserCreate:        "Criar novos usuários no sistema",
	PermUserRead:          "Visualizar lista e detalhes de usuários",
	PermUserUpdate:        "Atualizar dados de usuários (exceto senha)",
	PermUserDelete:        "Desativar contas de usuários",
	PermUserResetPassword: "Redefinir a senha de outros usuários",
	PermUserUnlock:        "Desbloquear contas de usuários bloqueadas",
	PermUserManageRoles:   "Gerenciar roles e atribuir roles a usuários",

	PermRoleManage: "Criar/Editar/Excluir roles e suas permissões",

	PermExportData: "Exportar dados da aplicação",
	PermLogView:    "Visualizar logs de auditoria do sistema",

	PermImportExecute:    "Permite importar arquivos de dados (Direitos, Obrigações, etc.)",
	PermImportViewStatus: "Permite visualizar o status e histórico das importações",
}

// PermissionManager gerencia as permissões e suas associações com roles.
type PermissionManager struct {
	roleRepo repositories.RoleRepository
}

// NewPermissionManager cria uma nova instância do PermissionManager.
func NewPermissionManager(roleRepo repositories.RoleRepository) *PermissionManager {
	if roleRepo == nil {
		appLogger.Fatalf("RoleRepository não pode ser nil para PermissionManager")
	}
	return &PermissionManager{
		roleRepo: roleRepo,
	}
}

// GetAllDefinedPermissions retorna um mapa de todas as permissões definidas no código.
func (pm *PermissionManager) GetAllDefinedPermissions() map[Permission]string {
	return allDefinedPermissions
}

// IsPermissionDefined verifica se um nome de permissão é válido e definido no sistema.
func (pm *PermissionManager) IsPermissionDefined(permName Permission) bool {
	_, exists := allDefinedPermissions[permName]
	return exists
}

// HasPermission verifica se um usuário (representado por sua sessão) possui uma permissão específica.
// O parâmetro `resourceOwnerID` é opcional e usado para verificações baseadas em recursos.
func (pm *PermissionManager) HasPermission(userSession *SessionData, requiredPermission Permission, resourceOwnerID *string) (bool, error) {
	if userSession == nil {
		appLogger.Warn("Verificação de permissão falhou: sessão de usuário ausente.")
		return false, fmt.Errorf("%w: usuário não autenticado", appErrors.ErrUnauthorized)
	}

	if !pm.IsPermissionDefined(requiredPermission) {
		appLogger.Errorf("Permissão desconhecida '%s' solicitada por usuário '%s'.", requiredPermission, userSession.Username)
		return false, fmt.Errorf("%w: permissão '%s' não definida no sistema", appErrors.ErrPermissionConfig, requiredPermission)
	}

	for _, roleName := range userSession.Roles {
		if strings.ToLower(roleName) == "admin" {
			appLogger.Debugf("Permissão '%s' concedida (Admin bypass) para '%s'.", requiredPermission, userSession.Username)
			return true, nil
		}
	}

	for _, roleName := range userSession.Roles {
		role, err := pm.roleRepo.GetByName(roleName)
		if err != nil {
			if errors.Is(err, appErrors.ErrNotFound) {
				appLogger.Warnf("Role '%s' da sessão do usuário '%s' não encontrado no banco. Pulando.", roleName, userSession.Username)
				continue
			}
			appLogger.Errorf("Erro ao buscar role '%s' para usuário '%s': %v", roleName, userSession.Username, err)
			return false, fmt.Errorf("%w: erro ao verificar permissões do role '%s'", appErrors.ErrDatabase, roleName)
		}

		for _, permNameFromDB := range role.Permissions {
			if Permission(permNameFromDB) == requiredPermission {
				if requiredPermission == PermNetworkViewOwn {
					if resourceOwnerID == nil {
						appLogger.Warnf("Permissão '%s' requer resourceOwnerID, mas não foi fornecido para usuário '%s'. Negando.", requiredPermission, userSession.Username)
						return false, nil
					}
					// Exemplo simplificado de verificação de propriedade.
					// Uma implementação real pode comparar userSession.UserID com o ID do proprietário do recurso.
					isOwner := (userSession.Username == *resourceOwnerID) // Ou userSession.UserID.String() == *resourceOwnerID
					if isOwner {
						appLogger.Debugf("Permissão '%s' concedida para '%s' (owner).", requiredPermission, userSession.Username)
						return true, nil
					}
					continue // Não é o proprietário, continua verificando outros roles/permissões.
				}
				appLogger.Debugf("Permissão '%s' concedida para '%s' via role '%s'.", requiredPermission, userSession.Username, roleName)
				return true, nil
			}
		}
	}

	appLogger.Debugf("Permissão '%s' NEGADA para usuário '%s'.", requiredPermission, userSession.Username)
	return false, nil
}

// HasRole verifica se um usuário (representado por sua sessão) possui um role específico.
func (pm *PermissionManager) HasRole(userSession *SessionData, requiredRoleName string) (bool, error) {
	if userSession == nil {
		return false, fmt.Errorf("%w: usuário não autenticado", appErrors.ErrUnauthorized)
	}
	if requiredRoleName == "" {
		return false, errors.New("nome do role requerido não pode ser vazio")
	}

	normalizedRequiredRole := strings.ToLower(requiredRoleName)
	for _, userRoleName := range userSession.Roles {
		if strings.ToLower(userRoleName) == normalizedRequiredRole {
			return true, nil
		}
	}
	return false, nil
}

// CheckPermission é uma função helper para verificar permissão e retornar um erro apropriado.
func (pm *PermissionManager) CheckPermission(userSession *SessionData, requiredPermission Permission, resourceOwnerID *string) error {
	hasPerm, err := pm.HasPermission(userSession, requiredPermission, resourceOwnerID)
	if err != nil {
		return err
	}
	if !hasPerm {
		return fmt.Errorf("%w: permissão '%s' necessária", appErrors.ErrPermissionDenied, requiredPermission)
	}
	return nil
}

// CheckRole é uma função helper para verificar role e retornar um erro apropriado.
func (pm *PermissionManager) CheckRole(userSession *SessionData, requiredRoleName string) error {
	hasRole, err := pm.HasRole(userSession, requiredRoleName)
	if err != nil {
		return err
	}
	if !hasRole {
		return fmt.Errorf("%w: role '%s' necessário", appErrors.ErrPermissionDenied, requiredRoleName)
	}
	return nil
}

var globalPermissionManager *PermissionManager
var pmOnce sync.Once

// InitGlobalPermissionManager inicializa a instância global do PermissionManager.
// DEVE ser chamado uma vez durante a inicialização da aplicação.
func InitGlobalPermissionManager(roleRepo repositories.RoleRepository) {
	pmOnce.Do(func() {
		if roleRepo == nil {
			appLogger.Fatalf("Tentativa de inicializar GlobalPermissionManager com RoleRepository nil.")
		}
		globalPermissionManager = NewPermissionManager(roleRepo)
		appLogger.Info("PermissionManager global inicializado.")
	})
}

// GetPermissionManager retorna a instância global do PermissionManager.
// Panics se não for inicializado.
func GetPermissionManager() *PermissionManager {
	if globalPermissionManager == nil {
		appLogger.Fatalf("FATAL: PermissionManager global não foi inicializado. Chame InitGlobalPermissionManager primeiro.")
	}
	return globalPermissionManager
}

// SeedInitialRolesAndPermissions verifica e cria/atualiza roles e permissões padrão.
func SeedInitialRolesAndPermissions(roleRepo repositories.RoleRepository, permManager *PermissionManager) error {
	appLogger.Info("Verificando/Criando roles e permissões iniciais...")

	allDefinedPermsMap := permManager.GetAllDefinedPermissions()
	allDefinedPermNamesStr := make([]string, 0, len(allDefinedPermsMap))
	for pName := range allDefinedPermsMap {
		allDefinedPermNamesStr = append(allDefinedPermNamesStr, string(pName))
	}

	if len(allDefinedPermNamesStr) == 0 {
		appLogger.Error("Nenhuma permissão definida encontrada no PermissionManager! Seeding de roles abortado.")
		return errors.New("nenhuma permissão definida para o seeding de roles")
	}

	editorPermsStr := []string{
		string(PermNetworkView), string(PermNetworkCreate), string(PermNetworkUpdate), string(PermNetworkStatus),
		string(PermCNPJView), string(PermCNPJCreate), string(PermCNPJUpdate), string(PermCNPJDelete),
		string(PermExportData),
		string(PermImportExecute), string(PermImportViewStatus),
	}
	viewerPermsStr := []string{
		string(PermNetworkView), string(PermCNPJView),
		string(PermExportData),
		string(PermImportViewStatus),
	}

	defaultRolesConfig := []struct {
		Name        string
		Description string
		Permissions []string
		IsSystem    bool
	}{
		{"admin", "Administrador do Sistema (acesso total)", allDefinedPermNamesStr, true},
		{"editor", "Pode visualizar, criar/editar redes/CNPJs e importar dados.", editorPermsStr, true},
		{"viewer", "Pode apenas visualizar dados, exportar e ver status de importação.", viewerPermsStr, true},
		{"user", "Usuário básico (sem permissões por padrão).", []string{}, true},
	}

	createdCount := 0
	updatedPermsCount := 0

	for _, roleCfg := range defaultRolesConfig {
		roleNameLower := strings.ToLower(roleCfg.Name)
		existingRole, err := roleRepo.GetByName(roleNameLower)

		if err != nil && !errors.Is(err, appErrors.ErrNotFound) {
			return fmt.Errorf("erro ao verificar role '%s': %w", roleNameLower, err)
		}

		validPermsForRole := []string{}
		for _, pNameStr := range roleCfg.Permissions {
			if permManager.IsPermissionDefined(Permission(pNameStr)) {
				validPermsForRole = append(validPermsForRole, pNameStr)
			} else {
				appLogger.Warnf("Configuração de seed para role '%s' contém permissão inválida/não definida: '%s'. Ignorando.", roleNameLower, pNameStr)
			}
		}

		if errors.Is(err, appErrors.ErrNotFound) {
			appLogger.Infof("Criando role padrão: '%s'", roleNameLower)
			desc := roleCfg.Description // Copia para evitar problema com ponteiro em loop
			newRole := models.RoleCreate{
				Name:            roleNameLower,
				Description:     &desc,
				PermissionNames: validPermsForRole,
			}
			_, createErr := roleRepo.Create(newRole, true) // Passa isSystem=true
			if createErr != nil {
				return fmt.Errorf("erro ao criar role '%s': %w", roleNameLower, createErr)
			}
			createdCount++
		} else {
			if existingRole.IsSystemRole {
				currentPermsSet := make(map[string]bool)
				for _, p := range existingRole.Permissions {
					currentPermsSet[p] = true
				}
				targetPermsSet := make(map[string]bool)
				for _, p := range validPermsForRole {
					targetPermsSet[p] = true
				}

				permsChanged := false
				if len(currentPermsSet) != len(targetPermsSet) {
					permsChanged = true
				} else {
					for p := range currentPermsSet {
						if !targetPermsSet[p] {
							permsChanged = true
							break
						}
					}
					if !permsChanged { // Checa também se target tem algo que current não tem
						for p := range targetPermsSet {
							if !currentPermsSet[p] {
								permsChanged = true
								break
							}
						}
					}
				}

				if permsChanged {
					appLogger.Infof("Atualizando permissões para role de sistema '%s'...", roleNameLower)
					roleUpdateData := models.RoleUpdate{
						PermissionNames: &validPermsForRole,
					}
					_, updateErr := roleRepo.Update(existingRole.ID, roleUpdateData)
					if updateErr != nil {
						return fmt.Errorf("erro ao atualizar permissões do role '%s': %w", roleNameLower, updateErr)
					}
					updatedPermsCount++
				}
			}
		}
	}

	if createdCount > 0 || updatedPermsCount > 0 {
		appLogger.Infof("Seed de roles concluído: %d criados, %d tiveram permissões atualizadas.", createdCount, updatedPermsCount)
	} else {
		appLogger.Info("Roles padrão já existem e estão atualizados.")
	}
	return nil
}
