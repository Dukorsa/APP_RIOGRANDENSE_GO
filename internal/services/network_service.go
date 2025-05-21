package services

import (
	"fmt"
	"strings"

	// "strings" // Se precisar de mais manipulação de string

	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/auth" // Para SessionData e PermissionManager
	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/errors"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/models"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/repositories"
)

// NetworkService define a interface para o serviço de Network.
type NetworkService interface {
	GetAllNetworks(includeInactive bool, userSession *auth.SessionData) ([]*models.NetworkPublic, error)
	SearchNetworks(term string, buyer *string, includeInactive bool, userSession *auth.SessionData) ([]*models.NetworkPublic, error)
	GetNetworkByID(networkID uint64, userSession *auth.SessionData) (*models.NetworkPublic, error)
	CreateNetwork(networkData models.NetworkCreate, userSession *auth.SessionData) (*models.NetworkPublic, error)
	UpdateNetwork(networkID uint64, networkUpdateData models.NetworkUpdate, userSession *auth.SessionData) (*models.NetworkPublic, error)
	ToggleNetworkStatus(networkID uint64, userSession *auth.SessionData) (*models.NetworkPublic, error)
	DeleteNetworks(ids []uint64, userSession *auth.SessionData) (deletedCount int64, err error)
}

// networkServiceImpl é a implementação de NetworkService.
type networkServiceImpl struct {
	repo            repositories.NetworkRepository
	auditLogService AuditLogService
	permManager     *auth.PermissionManager
}

// NewNetworkService cria uma nova instância de NetworkService.
func NewNetworkService(
	repo repositories.NetworkRepository,
	auditLog AuditLogService,
	pm *auth.PermissionManager,
) NetworkService {
	if repo == nil || auditLog == nil || pm == nil {
		appLogger.Fatalf("Dependências nulas fornecidas para NewNetworkService")
	}
	return &networkServiceImpl{
		repo:            repo,
		auditLogService: auditLog,
		permManager:     pm,
	}
}

// GetAllNetworks busca todas as redes.
func (s *networkServiceImpl) GetAllNetworks(includeInactive bool, userSession *auth.SessionData) ([]*models.NetworkPublic, error) {
	if err := s.permManager.CheckPermission(userSession, auth.PermNetworkView, nil); err != nil {
		return nil, err
	}
	dbNetworks, err := s.repo.GetAll(includeInactive)
	if err != nil {
		return nil, err // Erro já logado pelo repo
	}
	return models.ToNetworkPublicList(dbNetworks), nil
}

// SearchNetworks busca redes por termo e/ou comprador.
func (s *networkServiceImpl) SearchNetworks(term string, buyer *string, includeInactive bool, userSession *auth.SessionData) ([]*models.NetworkPublic, error) {
	if err := s.permManager.CheckPermission(userSession, auth.PermNetworkView, nil); err != nil {
		return nil, err
	}
	dbNetworks, err := s.repo.Search(term, buyer, includeInactive)
	if err != nil {
		return nil, err
	}
	return models.ToNetworkPublicList(dbNetworks), nil
}

// GetNetworkByID busca uma rede pelo ID.
func (s *networkServiceImpl) GetNetworkByID(networkID uint64, userSession *auth.SessionData) (*models.NetworkPublic, error) {
	if err := s.permManager.CheckPermission(userSession, auth.PermNetworkView, nil); err != nil {
		return nil, err
	}
	dbNetwork, err := s.repo.GetByID(networkID)
	if err != nil {
		return nil, err // Repo trata ErrNotFound
	}
	return models.ToNetworkPublic(dbNetwork), nil
}

// CreateNetwork cria uma nova rede.
func (s *networkServiceImpl) CreateNetwork(networkData models.NetworkCreate, userSession *auth.SessionData) (*models.NetworkPublic, error) {
	// 1. Verificar Permissão
	if err := s.permManager.CheckPermission(userSession, auth.PermNetworkCreate, nil); err != nil {
		return nil, err
	}

	// 2. Validar e Limpar Dados de Entrada
	if err := networkData.CleanAndValidate(); err != nil { // Chama método do modelo
		appLogger.Warnf("Dados de criação de rede inválidos para '%s': %v", networkData.Name, err)
		return nil, err // Retorna o ValidationError do CleanAndValidate
	}

	// 3. Chamar Repositório
	// O repositório é responsável por verificar a unicidade do nome (case-insensitive).
	dbNetwork, err := s.repo.Create(networkData, userSession.Username)
	if err != nil {
		// Erros como ErrConflict (nome já existe) ou ErrDatabase são tratados e logados pelo repo.
		return nil, err
	}

	// 4. Log de Auditoria
	logEntry := models.AuditLogEntry{
		Action:      "NETWORK_CREATE",
		Description: fmt.Sprintf("Nova rede '%s' (Comprador: %s) criada.", dbNetwork.Name, dbNetwork.Buyer),
		Severity:    "INFO",
		Metadata:    map[string]interface{}{"network_id": dbNetwork.ID, "name": dbNetwork.Name, "buyer": dbNetwork.Buyer},
	}
	if logErr := s.auditLogService.LogAction(logEntry, userSession); logErr != nil {
		appLogger.Warnf("Falha ao registrar log de auditoria para criação da rede %s: %v", dbNetwork.Name, logErr)
	}

	return models.ToNetworkPublic(dbNetwork), nil
}

// UpdateNetwork atualiza uma rede existente.
func (s *networkServiceImpl) UpdateNetwork(networkID uint64, networkUpdateData models.NetworkUpdate, userSession *auth.SessionData) (*models.NetworkPublic, error) {
	// 1. Verificar Permissão
	if err := s.permManager.CheckPermission(userSession, auth.PermNetworkUpdate, nil); err != nil {
		return nil, err
	}

	// 2. Validar e Limpar Dados de Entrada
	if err := networkUpdateData.CleanAndValidate(); err != nil {
		appLogger.Warnf("Dados de atualização de rede inválidos para ID %d: %v", networkID, err)
		return nil, err
	}

	// 3. Verificar se há algo para atualizar
	if networkUpdateData.Name == nil && networkUpdateData.Buyer == nil && networkUpdateData.Status == nil {
		appLogger.Infof("Nenhum campo fornecido para atualização da rede ID %d.", networkID)
		// Retornar a rede existente sem fazer nada ou um erro de input inválido?
		// Por ora, vamos buscar e retornar a rede existente.
		dbExistingNetwork, errGet := s.repo.GetByID(networkID)
		if errGet != nil {
			return nil, errGet // Trata NotFound ou DB error
		}
		return models.ToNetworkPublic(dbExistingNetwork), nil
	}

	// 4. Chamar Repositório
	// O repositório verificará a unicidade do nome se ele for alterado.
	dbNetwork, err := s.repo.Update(networkID, networkUpdateData, userSession.Username)
	if err != nil {
		return nil, err
	}

	// 5. Log de Auditoria
	updatedFields := []string{}
	if networkUpdateData.Name != nil {
		updatedFields = append(updatedFields, "name")
	}
	if networkUpdateData.Buyer != nil {
		updatedFields = append(updatedFields, "buyer")
	}
	if networkUpdateData.Status != nil {
		updatedFields = append(updatedFields, "status")
	}

	logEntry := models.AuditLogEntry{
		Action:      "NETWORK_UPDATE",
		Description: fmt.Sprintf("Rede ID %d ('%s') atualizada.", dbNetwork.ID, dbNetwork.Name),
		Severity:    "INFO",
		Metadata:    map[string]interface{}{"network_id": dbNetwork.ID, "updated_fields": updatedFields},
	}
	if logErr := s.auditLogService.LogAction(logEntry, userSession); logErr != nil {
		appLogger.Warnf("Falha ao registrar log de auditoria para atualização da rede ID %s: %v", dbNetwork.ID, logErr)
	}

	return models.ToNetworkPublic(dbNetwork), nil
}

// ToggleNetworkStatus alterna o status (ativo/inativo) de uma rede.
func (s *networkServiceImpl) ToggleNetworkStatus(networkID uint64, userSession *auth.SessionData) (*models.NetworkPublic, error) {
	// 1. Verificar Permissão
	if err := s.permManager.CheckPermission(userSession, auth.PermNetworkStatus, nil); err != nil {
		return nil, err
	}

	// 2. Chamar Repositório
	dbNetwork, err := s.repo.ToggleStatus(networkID, userSession.Username)
	if err != nil {
		return nil, err
	}

	// 3. Log de Auditoria
	newStatusStr := "Ativo"
	if !dbNetwork.Status {
		newStatusStr = "Inativo"
	}
	logEntry := models.AuditLogEntry{
		Action:      "NETWORK_STATUS_TOGGLE",
		Description: fmt.Sprintf("Status da rede ID %d ('%s') alterado para %s.", dbNetwork.ID, dbNetwork.Name, newStatusStr),
		Severity:    "INFO",
		Metadata:    map[string]interface{}{"network_id": dbNetwork.ID, "new_status": dbNetwork.Status},
	}
	if logErr := s.auditLogService.LogAction(logEntry, userSession); logErr != nil {
		appLogger.Warnf("Falha ao registrar log de auditoria para alteração de status da rede ID %s: %v", dbNetwork.ID, logErr)
	}

	return models.ToNetworkPublic(dbNetwork), nil
}

// DeleteNetworks exclui redes em massa.
func (s *networkServiceImpl) DeleteNetworks(ids []uint64, userSession *auth.SessionData) (int64, error) {
	// 1. Verificar Permissão
	// No Python, era require_role('admin'). Em Go, podemos mapear isso para uma permissão se quisermos,
	// ou manter a checagem de role aqui. Usar uma permissão é mais flexível.
	// Supondo que exista uma permissão como PermNetworkBulkDelete ou que PermNetworkDelete cubra isso
	// com uma verificação adicional de que é uma operação de admin se necessário.
	// Para este exemplo, vamos usar a permissão PermNetworkDelete.
	// O repositório pode ter uma camada adicional de segurança para bulk operations.
	// Ou, como no Python, usar uma permissão específica de admin.
	// Vamos simular o require_role('admin') com uma verificação de permissão mais forte se existir,
	// ou cair para PermNetworkDelete.

	// Idealmente: if err := s.permManager.CheckPermission(userSession, auth.PermNetworkBulkDelete, nil); err != nil { return 0, err}
	// Alternativa (se PermNetworkDelete for suficiente e o método do repo só puder ser chamado por admins de qualquer forma):
	if err := s.permManager.CheckPermission(userSession, auth.PermNetworkDelete, nil); err != nil {
		return 0, err
	}
	// Se for estritamente admin, pode-se verificar o role também:
	// hasAdminRole, _ := s.permManager.HasRole(userSession, "admin")
	// if !hasAdminRole {
	//  return 0, appErrors.ErrPermissionDenied // Ou um erro mais específico
	// }

	if len(ids) == 0 {
		return 0, appErrors.NewValidationError("Lista de IDs para exclusão não pode ser vazia.", nil)
	}

	// 2. Chamar Repositório
	deletedCount, err := s.repo.BulkDelete(ids)
	if err != nil {
		// Erros como ErrConflict (FK constraint) ou ErrDatabase são tratados e logados pelo repo.
		return 0, err
	}

	// 3. Log de Auditoria (se algo foi deletado)
	if deletedCount > 0 {
		logEntry := models.AuditLogEntry{
			Action:      "NETWORK_BULK_DELETE",
			Description: fmt.Sprintf("%d redes excluídas fisicamente.", deletedCount),
			Severity:    "WARNING", // Exclusão física é geralmente um Warning ou mais alto
			Metadata:    map[string]interface{}{"deleted_ids_count": len(ids), "actually_deleted_count": deletedCount, "ids_attempted": idsToString(ids)},
		}
		if logErr := s.auditLogService.LogAction(logEntry, userSession); logErr != nil {
			appLogger.Warnf("Falha ao registrar log de auditoria para exclusão em massa de redes: %v", logErr)
		}
	}
	return deletedCount, nil
}

// Helper para converter slice de uint64 para string para log
func idsToString(ids []uint64) string {
	strIds := make([]string, len(ids))
	for i, id := range ids {
		strIds[i] = fmt.Sprint(id)
	}
	return strings.Join(strIds, ", ")
}
