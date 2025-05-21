package auth

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	appErrors "github.com/seu_usuario/riograndense_gio/internal/core/errors"
	appLogger "github.com/seu_usuario/riograndense_gio/internal/core/logger"
	"github.com/seu_usuario/riograndense_gio/internal/data/models"
	"github.com/seu_usuario/riograndense_gio/internal/data/repositories"
	// "github.com/seu_usuario/riograndense_gio/internal/services" // Pode não ser necessário aqui diretamente
)

// Permission representa uma permissão no sistema.
// No Python, era um dataclass. Em Go, será uma string ou um tipo customizado.
// Para simplicidade inicial, usaremos strings, mas um tipo enum-like seria mais robusto.
type Permission string

// Definição de todas as permissões possíveis no sistema.
// Estas são as "definições" de permissão. A associação a roles virá do banco de dados.
const (
	// Network Permissions
	PermNetworkView    Permission = "network:view"
	PermNetworkCreate  Permission = "network:create"
	PermNetworkUpdate  Permission = "network:update"
	PermNetworkDelete  Permission = "network:delete"
	PermNetworkStatus  Permission = "network:status"
	PermNetworkViewOwn Permission = "network:view_own" // Exemplo resource-based

	// CNPJ Permissions
	PermCNPJView   Permission = "cnpj:view"
	PermCNPJCreate Permission = "cnpj:create"
	PermCNPJDelete Permission = "cnpj:delete"
	// PermCNPJUpdate Permission = "cnpj:update" // Faltava no Python? Adicionei no seu .txt app_riograndense_completo_estrutura_codigo.txt

	// User Management Permissions
	PermUserCreate        Permission = "user:create"
	PermUserRead          Permission = "user:read"
	PermUserUpdate        Permission = "user:update"
	PermUserDelete        Permission = "user:delete" // No Python era desativar
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

	// Adicione outras permissões aqui
)

var allDefinedPermissions = map[Permission]string{
	PermNetworkView:    "Visualizar dados de redes",
	PermNetworkCreate:  "Criar novas redes",
	PermNetworkUpdate:  "Atualizar dados de redes existentes",
	PermNetworkDelete:  "Excluir redes",
	PermNetworkStatus:  "Alterar status (ativo/inativo) de redes",
	PermNetworkViewOwn: "Visualizar apenas as redes criadas pelo próprio usuário",

	PermCNPJView:   "Visualizar CNPJs cadastrados",
	PermCNPJCreate: "Cadastrar novos CNPJs",
	PermCNPJDelete: "Excluir CNPJs",
	// PermCNPJUpdate: "Atualizar CNPJs",

	PermUserCreate:        "Criar novos usuários no sistema",
	PermUserRead:          "Visualizar lista e detalhes de usuários",
	PermUserUpdate:        "Atualizar dados de usuários (exceto senha)",
	PermUserDelete:        "Desativar contas de usuários",
	PermUserResetPassword: "Redefinir a senha de outros usuários",
	PermUserUnlock:        "Desbloquear contas de usuários bloqueadas",
	PermUserManageRoles:   "Gerenciar roles e atribuir roles a usuários",
	PermRoleManage:        "Criar/Editar/Excluir roles e suas permissões",

	PermExportData: "Exportar dados da aplicação",
	PermLogView:    "Visualizar logs de auditoria do sistema",

	PermImportExecute:    "Permite importar arquivos de dados (Direitos, Obrigações, etc.)",
	PermImportViewStatus: "Permite visualizar o status e histórico das importações",
}

// PermissionManager gerencia as permissões e suas associações com roles.
// Em Go, isso envolverá interagir com o RoleRepository para obter as permissões de um role.
type PermissionManager struct {
	roleRepo repositories.RoleRepository // Interface
	// sessionManager *SessionManager // Pode ser útil para obter sessão atual
	// lock           sync.RWMutex    // Se houver cache interno de permissões de roles
}

// NewPermissionManager cria uma nova instância do PermissionManager.
func NewPermissionManager(roleRepo repositories.RoleRepository /*, sm *SessionManager*/) *PermissionManager {
	if roleRepo == nil {
		// Isso seria um erro de programação, então podemos usar panic ou log fatal.
		appLogger.Fatalf("RoleRepository não pode ser nil para PermissionManager")
	}
	return &PermissionManager{
		roleRepo: roleRepo,
		// sessionManager: sm,
	}
}

// GetAllDefinedPermissions retorna um mapa de todas as permissões definidas no código.
func (pm *PermissionManager) GetAllDefinedPermissions() map[Permission]string {
	// Retorna uma cópia para evitar modificação externa se desejado, mas para strings é ok.
	return allDefinedPermissions
}

// IsPermissionDefined verifica se um nome de permissão é válido e definido no sistema.
func (pm *PermissionManager) IsPermissionDefined(permName Permission) bool {
	_, exists := allDefinedPermissions[permName]
	return exists
}

// HasPermission verifica se um usuário (representado por sua sessão) possui uma permissão específica.
// O parâmetro `resourceOwnerID` é opcional e usado para verificações baseadas em recursos (ex: "network:view_own").
func (pm *PermissionManager) HasPermission(userSession *SessionData, requiredPermission Permission, resourceOwnerID *string) (bool, error) {
	if userSession == nil {
		appLogger.Warn("Verificação de permissão falhou: sessão de usuário ausente.")
		return false, fmt.Errorf("%w: usuário não autenticado", appErrors.ErrUnauthorized)
	}

	// 1. Validar se a permissão requerida é conhecida pelo sistema
	if !pm.IsPermissionDefined(requiredPermission) {
		appLogger.Errorf("Permissão desconhecida '%s' solicitada por usuário '%s'.", requiredPermission, userSession.Username)
		return false, fmt.Errorf("%w: permissão '%s' não definida no sistema", appErrors.ErrPermissionConfig, requiredPermission)
	}

	// 2. Bypass para Administrador (role 'admin')
	// A SessionData deve conter os nomes dos roles do usuário.
	for _, roleName := range userSession.Roles {
		if strings.ToLower(roleName) == "admin" {
			appLogger.Debugf("Permissão '%s' concedida (Admin bypass) para '%s'.", requiredPermission, userSession.Username)
			// TODO: Implementar lógica resource-based para admin se necessário
			// if requiredPermission == PermNetworkViewOwn && resourceOwnerID != nil {
			// 	return *resourceOwnerID == userSession.Username, nil // Exemplo simplificado
			// }
			return true, nil
		}
	}

	// 3. Verificar permissões baseadas em roles do usuário
	// Itera sobre os roles do usuário na sessão e verifica se algum deles concede a permissão.
	for _, roleName := range userSession.Roles {
		role, err := pm.roleRepo.GetByName(roleName) // Obtém o DBRole e suas permissões
		if err != nil {
			if errors.Is(err, appErrors.ErrNotFound) {
				appLogger.Warnf("Role '%s' da sessão do usuário '%s' não encontrado no banco. Pulando.", roleName, userSession.Username)
				continue // Role da sessão pode estar desatualizado/removido
			}
			appLogger.Errorf("Erro ao buscar role '%s' para usuário '%s': %v", roleName, userSession.Username, err)
			return false, fmt.Errorf("%w: erro ao verificar permissões do role '%s'", appErrors.ErrDatabase, roleName)
		}

		// `role.Permissions` deve ser um slice/map de strings de nomes de permissão
		for _, permNameFromDB := range role.Permissions { // Supondo que role.Permissions é []string
			if Permission(permNameFromDB) == requiredPermission {
				// TODO: Implementar lógica resource-based
				// Exemplo para 'network:view_own'
				if requiredPermission == PermNetworkViewOwn {
					if resourceOwnerID == nil {
						appLogger.Warnf("Permissão '%s' requer resourceOwnerID, mas não foi fornecido para usuário '%s'. Negando.", requiredPermission, userSession.Username)
						return false, nil // Ou um erro específico
					}
					// Aqui você compararia o userSession.Username (ou UserID) com o resourceOwnerID
					// Se o userSession.Username é o dono, permite.
					// Esta é uma simplificação. Em Python, havia uma check_fn.
					// Em Go, você pode ter uma função helper ou lógica inline.
					isOwner := (userSession.Username == *resourceOwnerID) // Ou comparar IDs de usuário
					if isOwner {
						appLogger.Debugf("Permissão '%s' concedida para '%s' (owner).", requiredPermission, userSession.Username)
						return true, nil
					}
					// Se não for o dono, essa permissão específica não concede acesso.
					// O loop continua para verificar outras permissões/roles.
					continue // Não retorna false aqui, pois outro role/permissão pode conceder
				}
				appLogger.Debugf("Permissão '%s' concedida para '%s' via role '%s'.", requiredPermission, userSession.Username, roleName)
				return true, nil
			}
		}
	}

	appLogger.Debugf("Permissão '%s' NEGADA para usuário '%s'.", requiredPermission, userSession.Username)
	return false, nil // Permissão não encontrada em nenhum role
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

// --- Funções Helper para Verificação (Substitutos para Decoradores) ---

// CheckPermission é uma função helper para verificar permissão e retornar um erro apropriado.
// Uso: err := permManager.CheckPermission(session, PermNetworkCreate, nil)
//
//	if err != nil { return err // ou tratar o erro }
func (pm *PermissionManager) CheckPermission(userSession *SessionData, requiredPermission Permission, resourceOwnerID *string) error {
	hasPerm, err := pm.HasPermission(userSession, requiredPermission, resourceOwnerID)
	if err != nil {
		return err // Erro interno ou de configuração
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
		return err // Erro interno (improvável para HasRole simples)
	}
	if !hasRole {
		return fmt.Errorf("%w: role '%s' necessário", appErrors.ErrPermissionDenied, requiredRoleName)
	}
	return nil
}

// Global instance (Singleton-like pattern in Go is often just a package-level variable)
// Esta instância precisa ser inicializada em main.go ou onde os serviços são configurados.
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
		// Isso indica um erro de programação - InitGlobalPermissionManager não foi chamado.
		appLogger.Fatalf("FATAL: PermissionManager global não foi inicializado. Chame InitGlobalPermissionManager primeiro.")
	}
	return globalPermissionManager
}

// --- Exemplo de seeding de roles e permissões (chamado em main.go ou em um script de setup) ---
// Esta função é uma tradução conceitual do _seed_initial_roles_and_permissions do Python.
func SeedInitialRolesAndPermissions(roleRepo repositories.RoleRepository, permManager *PermissionManager) error {
	appLogger.Info("Verificando/Criando roles e permissões iniciais...")

	allDefinedPermsMap := permManager.GetAllDefinedPermissions()
	allDefinedPermNames := make([]Permission, 0, len(allDefinedPermsMap))
	for pName := range allDefinedPermsMap {
		allDefinedPermNames = append(allDefinedPermNames, pName)
	}

	if len(allDefinedPermNames) == 0 {
		appLogger.Error("Nenhuma permissão definida encontrada no PermissionManager! Seeding de roles abortado.")
		return errors.New("nenhuma permissão definida para o seeding de roles")
	}

	// Converte para []string para o modelo DBRole
	allDefinedPermNamesStr := make([]string, len(allDefinedPermNames))
	for i, p := range allDefinedPermNames {
		allDefinedPermNamesStr[i] = string(p)
	}

	// Permissões como strings para DBRole
	editorPermsStr := []string{
		string(PermNetworkView), string(PermNetworkCreate), string(PermNetworkUpdate), string(PermNetworkStatus),
		string(PermCNPJView), string(PermCNPJCreate), string(PermCNPJDelete), // string(PermCNPJUpdate) se existir
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
		Permissions []string // Nomes das permissões
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
		existingRole, err := roleRepo.GetByName(roleNameLower) // GetByName deve ser case-insensitive

		if err != nil && !errors.Is(err, appErrors.ErrNotFound) {
			return fmt.Errorf("erro ao verificar role '%s': %w", roleNameLower, err)
		}

		// Validar se as permissões da config existem no sistema
		validPermsForRole := []string{}
		for _, pNameStr := range roleCfg.Permissions {
			if permManager.IsPermissionDefined(Permission(pNameStr)) {
				validPermsForRole = append(validPermsForRole, pNameStr)
			} else {
				appLogger.Warnf("Configuração de seed para role '%s' contém permissão inválida/não definida: '%s'. Ignorando.", roleNameLower, pNameStr)
			}
		}

		if errors.Is(err, appErrors.ErrNotFound) { // Role não existe, criar
			appLogger.Infof("Criando role padrão: '%s'", roleNameLower)
			newRole := models.RoleCreate{ // Supondo que RoleCreate aceite []string para Permissions
				Name:            roleNameLower,
				Description:     &roleCfg.Description, // RoleCreate pode esperar ponteiro para string opcional
				PermissionNames: validPermsForRole,    // Passa os nomes validados
				// IsSystemRole é definido internamente pelo repositório ou serviço ao criar system roles
			}
			// O método Create do RoleRepository deve lidar com a criação do DBRole e a associação das permissões
			_, createErr := roleRepo.Create(newRole, true) // Passa isSystem=true
			if createErr != nil {
				return fmt.Errorf("erro ao criar role '%s': %w", roleNameLower, createErr)
			}
			createdCount++
		} else { // Role existe, verificar e atualizar permissões se for system role
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
				}

				if permsChanged {
					appLogger.Infof("Atualizando permissões para role de sistema '%s'...", roleNameLower)
					// O método Update do RoleRepository deve lidar com a atualização das permissões
					// RoleUpdate pode precisar de um campo para todas as permissões
					roleUpdateData := models.RoleUpdate{
						PermissionNames: &validPermsForRole, // Passa o conjunto completo de permissões desejadas
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
