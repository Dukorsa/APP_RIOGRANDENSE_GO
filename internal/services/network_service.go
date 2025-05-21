package services

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/auth"
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
	GetNetworkByName(name string, userSession *auth.SessionData) (*models.NetworkPublic, error) // Adicionado
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
		appLogger.Fatalf("Dependências nulas fornecidas para NewNetworkService (repo, auditLog, permManager)")
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
		return nil, err // Erro já logado pelo repo.
	}
	return models.ToNetworkPublicList(dbNetworks), nil
}

// SearchNetworks busca redes por termo e/ou comprador.
func (s *networkServiceImpl) SearchNetworks(term string, buyer *string, includeInactive bool, userSession *auth.SessionData) ([]*models.NetworkPublic, error) {
	if err := s.permManager.CheckPermission(userSession, auth.PermNetworkView, nil); err != nil {
		return nil, err
	}
	// O repositório lida com a normalização de `term` e `buyer` para a busca.
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
		// Repo trata ErrNotFound e outros erros de DB.
		return nil, err
	}
	return models.ToNetworkPublic(dbNetwork), nil
}

// GetNetworkByName busca uma rede pelo nome.
func (s *networkServiceImpl) GetNetworkByName(name string, userSession *auth.SessionData) (*models.NetworkPublic, error) {
	if err := s.permManager.CheckPermission(userSession, auth.PermNetworkView, nil); err != nil {
		return nil, err
	}
	normalizedName := strings.ToLower(strings.TrimSpace(name))
	if normalizedName == "" {
		return nil, fmt.Errorf("%w: nome da rede para busca não pode ser vazio", appErrors.ErrInvalidInput)
	}
	dbNetwork, err := s.repo.GetByName(normalizedName)
	if err != nil {
		return nil, err
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
	// `CleanAndValidate` normaliza `Name` para minúsculas e `Buyer` para Title Case.
	if err := networkData.CleanAndValidate(); err != nil {
		appLogger.Warnf("Dados de criação de rede inválidos para '%s': %v", networkData.Name, err)
		return nil, err // Retorna o ValidationError.
	}

	// 3. Verificar Unicidade do Nome (case-insensitive, já que `networkData.Name` está em minúsculas).
	// O repositório também fará essa checagem, mas é bom ter no serviço para feedback mais rápido.
	if _, err := s.repo.GetByName(networkData.Name); err == nil {
		// Se não houve erro, significa que uma rede com esse nome já existe.
		return nil, fmt.Errorf("%w: já existe uma rede cadastrada com o nome '%s'", appErrors.ErrConflict, networkData.Name)
	} else if !errors.Is(err, appErrors.ErrNotFound) {
		// Se o erro for diferente de ErrNotFound, é um problema na consulta.
		return nil, appErrors.WrapErrorf(err, "erro ao verificar unicidade do nome da rede '%s'", networkData.Name)
	}
	// Se ErrNotFound, o nome está disponível.

	// 4. Chamar Repositório
	dbNetwork, err := s.repo.Create(networkData, userSession.Username)
	if err != nil {
		// Erros como ErrConflict (se houver race condition) ou ErrDatabase são tratados pelo repo.
		return nil, err
	}

	// 5. Log de Auditoria
	logEntry := models.AuditLogEntry{
		Action:      "NETWORK_CREATE",
		Description: fmt.Sprintf("Nova rede '%s' (Comprador: %s) criada.", dbNetwork.Name, dbNetwork.Buyer),
		Severity:    "INFO",
		Metadata:    map[string]interface{}{"network_id": dbNetwork.ID, "name": dbNetwork.Name, "buyer": dbNetwork.Buyer},
	}
	if logErr := s.auditLogService.LogAction(logEntry, userSession); logErr != nil {
		appLogger.Warnf("Falha ao registrar log de auditoria para criação da rede '%s': %v", dbNetwork.Name, logErr)
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
	// `CleanAndValidate` normaliza os campos fornecidos.
	if err := networkUpdateData.CleanAndValidate(); err != nil {
		appLogger.Warnf("Dados de atualização de rede inválidos para ID %d: %v", networkID, err)
		return nil, err
	}

	// 3. Verificar se há algo para atualizar (opcional, o repo pode lidar com isso).
	if networkUpdateData.Name == nil && networkUpdateData.Buyer == nil && networkUpdateData.Status == nil {
		appLogger.Infof("Nenhum campo fornecido para atualização da rede ID %d.", networkID)
		// Retornar a rede existente sem fazer nada.
		existingNetwork, errGet := s.repo.GetByID(networkID)
		if errGet != nil {
			return nil, errGet // Trata NotFound ou DB error.
		}
		return models.ToNetworkPublic(existingNetwork), nil
	}

	// 4. Se o nome estiver sendo alterado, verificar unicidade do novo nome.
	if networkUpdateData.Name != nil {
		// `*networkUpdateData.Name` já está em minúsculas devido a `CleanAndValidate`.
		existingByName, errGet := s.repo.GetByName(*networkUpdateData.Name)
		// Se encontrou uma rede com o novo nome E essa rede não é a que estamos atualizando.
		if errGet == nil && existingByName.ID != networkID {
			return nil, fmt.Errorf("%w: já existe outra rede com o nome '%s'", appErrors.ErrConflict, *networkUpdateData.Name)
		}
		if errGet != nil && !errors.Is(errGet, appErrors.ErrNotFound) {
			return nil, appErrors.WrapErrorf(errGet, "erro ao verificar novo nome para atualização da rede ID %d", networkID)
		}
	}

	// 5. Chamar Repositório
	dbNetwork, err := s.repo.Update(networkID, networkUpdateData, userSession.Username)
	if err != nil {
		return nil, err
	}

	// 6. Log de Auditoria
	updatedFieldsLog := []string{}
	meta := map[string]interface{}{"network_id": dbNetwork.ID, "name_after_update": dbNetwork.Name}
	if networkUpdateData.Name != nil {
		updatedFieldsLog = append(updatedFieldsLog, fmt.Sprintf("nome para '%s'", *networkUpdateData.Name))
		meta["new_name"] = *networkUpdateData.Name
	}
	if networkUpdateData.Buyer != nil {
		updatedFieldsLog = append(updatedFieldsLog, fmt.Sprintf("comprador para '%s'", *networkUpdateData.Buyer))
		meta["new_buyer"] = *networkUpdateData.Buyer
	}
	if networkUpdateData.Status != nil {
		statusStr := "Inativo"
		if *networkUpdateData.Status {
			statusStr = "Ativo"
		}
		updatedFieldsLog = append(updatedFieldsLog, fmt.Sprintf("status para %s", statusStr))
		meta["new_status"] = *networkUpdateData.Status
	}

	logEntry := models.AuditLogEntry{
		Action:      "NETWORK_UPDATE",
		Description: fmt.Sprintf("Rede ID %d ('%s') atualizada. Campos alterados: %s.", dbNetwork.ID, dbNetwork.Name, strings.Join(updatedFieldsLog, ", ")),
		Severity:    "INFO",
		Metadata:    meta,
	}
	if logErr := s.auditLogService.LogAction(logEntry, userSession); logErr != nil {
		appLogger.Warnf("Falha ao registrar log de auditoria para atualização da rede ID %d: %v", dbNetwork.ID, logErr)
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
		return nil, err // Erro já logado e formatado pelo repo.
	}

	// 3. Log de Auditoria
	newStatusStr := "Ativa"
	if !dbNetwork.Status {
		newStatusStr = "Inativa"
	}
	logEntry := models.AuditLogEntry{
		Action:      "NETWORK_STATUS_TOGGLE",
		Description: fmt.Sprintf("Status da rede ID %d ('%s') alterado para %s.", dbNetwork.ID, dbNetwork.Name, newStatusStr),
		Severity:    "INFO",
		Metadata:    map[string]interface{}{"network_id": dbNetwork.ID, "new_status": dbNetwork.Status, "network_name": dbNetwork.Name},
	}
	if logErr := s.auditLogService.LogAction(logEntry, userSession); logErr != nil {
		appLogger.Warnf("Falha ao registrar log de auditoria para alteração de status da rede ID %d: %v", dbNetwork.ID, logErr)
	}

	return models.ToNetworkPublic(dbNetwork), nil
}

// DeleteNetworks exclui redes em massa.
func (s *networkServiceImpl) DeleteNetworks(ids []uint64, userSession *auth.SessionData) (int64, error) {
	// 1. Verificar Permissão (ex: PermNetworkDelete ou uma permissão mais específica de admin/bulk).
	if err := s.permManager.CheckPermission(userSession, auth.PermNetworkDelete, nil); err != nil {
		return 0, err
	}
	// Adicionalmente, poderia verificar se o usuário tem um role de admin para operações em massa.
	// isAdmin, _ := s.permManager.HasRole(userSession, "admin")
	// if !isAdmin {
	// 	return 0, fmt.Errorf("%w: apenas administradores podem realizar exclusão em massa de redes", appErrors.ErrPermissionDenied)
	// }

	if len(ids) == 0 {
		return 0, appErrors.NewValidationError("Lista de IDs para exclusão de redes não pode ser vazia.", nil)
	}

	// 2. Chamar Repositório
	// O repositório lida com a exclusão física e pode retornar ErrConflict se houver FKs.
	deletedCount, err := s.repo.BulkDelete(ids)
	if err != nil {
		// Erros como ErrConflict (FK constraint) ou ErrDatabase são tratados pelo repo.
		return 0, err
	}

	// 3. Log de Auditoria (se algo foi deletado)
	if deletedCount > 0 {
		logEntry := models.AuditLogEntry{
			Action:      "NETWORK_BULK_DELETE",
			Description: fmt.Sprintf("%d redes excluídas fisicamente.", deletedCount),
			Severity:    "WARNING", // Exclusão física é geralmente um Warning ou mais alto.
			Metadata:    map[string]interface{}{"deleted_ids_count": len(ids), "actually_deleted_count": deletedCount, "ids_attempted_str": idsToStringForLog(ids)},
		}
		if logErr := s.auditLogService.LogAction(logEntry, userSession); logErr != nil {
			appLogger.Warnf("Falha ao registrar log de auditoria para exclusão em massa de redes: %v", logErr)
		}
	}
	return deletedCount, nil
}

// idsToStringForLog converte um slice de uint64 para uma string CSV para log.
// Limita o número de IDs na string para evitar logs excessivamente longos.
func idsToStringForLog(ids []uint64) string {
	const maxIDsInLog = 20
	if len(ids) == 0 {
		return ""
	}
	var sb strings.Builder
	count := 0
	for _, id := range ids {
		if count > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(fmt.Sprint(id))
		count++
		if count >= maxIDsInLog && len(ids) > maxIDsInLog {
			sb.WriteString(fmt.Sprintf("... (e mais %d IDs)", len(ids)-maxIDsInLog))
			break
		}
	}
	return sb.String()
}
