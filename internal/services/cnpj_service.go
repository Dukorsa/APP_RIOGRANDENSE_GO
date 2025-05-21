package services

import (
	"errors"
	"fmt"

	"github.com/seu_usuario/riograndense_gio/internal/auth" // Para SessionData e PermissionManager
	appErrors "github.com/seu_usuario/riograndense_gio/internal/core/errors"
	appLogger "github.com/seu_usuario/riograndense_gio/internal/core/logger"
	"github.com/seu_usuario/riograndense_gio/internal/data/models"
	"github.com/seu_usuario/riograndense_gio/internal/data/repositories"
	"github.com/seu_usuario/riograndense_gio/internal/utils" // Para IsValidCNPJ
)

// CNPJService define a interface para o serviço de CNPJ.
type CNPJService interface {
	RegisterCNPJ(cnpjData models.CNPJCreate, userSession *auth.SessionData) (*models.CNPJPublic, error)
	DeleteCNPJ(cnpjID uint64, userSession *auth.SessionData) error
	GetAllCNPJs(includeInactive bool, userSession *auth.SessionData) ([]*models.CNPJPublic, error)
	GetCNPJsByNetwork(networkID uint64, includeInactive bool, userSession *auth.SessionData) ([]*models.CNPJPublic, error)
	UpdateCNPJ(cnpjID uint64, cnpjUpdateData models.CNPJUpdate, userSession *auth.SessionData) (*models.CNPJPublic, error)
	GetCNPJByID(cnpjID uint64, userSession *auth.SessionData) (*models.CNPJPublic, error)
}

// cnpjServiceImpl é a implementação de CNPJService.
type cnpjServiceImpl struct {
	repo            repositories.CNPJRepository
	networkRepo     repositories.NetworkRepository // Para verificar existência de NetworkID
	auditLogService AuditLogService
	permManager     *auth.PermissionManager
}

// NewCNPJService cria uma nova instância de CNPJService.
func NewCNPJService(
	repo repositories.CNPJRepository,
	networkRepo repositories.NetworkRepository,
	auditLogService AuditLogService,
	permManager *auth.PermissionManager,
) CNPJService {
	if repo == nil || networkRepo == nil || auditLogService == nil || permManager == nil {
		appLogger.Fatalf("Dependências nulas fornecidas para NewCNPJService")
	}
	return &cnpjServiceImpl{
		repo:            repo,
		networkRepo:     networkRepo,
		auditLogService: auditLogService,
		permManager:     permManager,
	}
}

// RegisterCNPJ registra um novo CNPJ.
func (s *cnpjServiceImpl) RegisterCNPJ(cnpjData models.CNPJCreate, userSession *auth.SessionData) (*models.CNPJPublic, error) {
	// 1. Verificar Permissão
	if err := s.permManager.CheckPermission(userSession, auth.PermCNPJCreate, nil); err != nil {
		return nil, err
	}

	// 2. Validar e Limpar Dados de Entrada
	// O método CleanAndValidateCNPJ da struct CNPJCreate já faz a limpeza e validação de formato.
	cleanedCNPJ, validationErr := cnpjData.CleanAndValidateCNPJ()
	if validationErr != nil {
		appLogger.Warnf("Dados de criação de CNPJ inválidos: %v", validationErr)
		// O erro retornado por CleanAndValidateCNPJ já deve ser um appErrors.ValidationError
		return nil, validationErr
	}
	// Substitui o CNPJ original pelos dígitos limpos para passar ao repositório
	cnpjData.CNPJ = cleanedCNPJ // Embora o repo também possa limpar, é bom ter aqui.

	// Validação adicional de dígitos verificadores
	if !utils.IsValidCNPJ(cleanedCNPJ) { // Supondo que IsValidCNPJ está em utils
		return nil, appErrors.NewValidationError("CNPJ inválido (dígitos verificadores não conferem).", map[string]string{"cnpj": "CNPJ inválido."})
	}

	// 3. Verificar se a NetworkID existe
	if _, err := s.networkRepo.GetByID(cnpjData.NetworkID); err != nil {
		// Se networkRepo.GetByID retorna appErrors.ErrNotFound, isso será propagado.
		appLogger.Warnf("Network ID %d não encontrada ao tentar registrar CNPJ %s: %v", cnpjData.NetworkID, cleanedCNPJ, err)
		return nil, fmt.Errorf("%w: rede com ID %d não encontrada para associar ao CNPJ", appErrors.ErrNotFound, cnpjData.NetworkID)
	}

	// 4. Chamar Repositório
	dbCNPJ, err := s.repo.Add(cnpjData)
	if err != nil {
		// Erros como ErrConflict (CNPJ já existe) ou ErrDatabase já são tratados e logados pelo repo.
		return nil, err // Propaga o erro do repositório
	}

	// 5. Log de Auditoria
	logEntry := models.AuditLogEntry{
		Action:      "CNPJ_CREATE",
		Description: fmt.Sprintf("CNPJ %s cadastrado para network ID %d.", dbCNPJ.FormatCNPJ(), dbCNPJ.NetworkID),
		Severity:    "INFO",
		Metadata:    map[string]interface{}{"cnpj_id": dbCNPJ.ID, "cnpj": dbCNPJ.CNPJ, "network_id": dbCNPJ.NetworkID},
	}
	if logErr := s.auditLogService.LogAction(logEntry, userSession); logErr != nil {
		appLogger.Warnf("Falha ao registrar log de auditoria para criação do CNPJ %s: %v", dbCNPJ.CNPJ, logErr)
	}

	return models.ToCNPJPublic(dbCNPJ), nil
}

// DeleteCNPJ exclui um CNPJ.
func (s *cnpjServiceImpl) DeleteCNPJ(cnpjID uint64, userSession *auth.SessionData) error {
	// 1. Verificar Permissão
	if err := s.permManager.CheckPermission(userSession, auth.PermCNPJDelete, nil); err != nil {
		return err
	}

	// 2. (Opcional) Buscar o CNPJ para logar o número antes de deletar
	cnpjToLog := fmt.Sprintf("ID %d", cnpjID)
	existingCNPJ, err := s.repo.GetByID(cnpjID)
	if err == nil && existingCNPJ != nil {
		cnpjToLog = existingCNPJ.FormatCNPJ() // Usa o CNPJ formatado se conseguir buscar
	} else if !errors.Is(err, appErrors.ErrNotFound) {
		// Se houve um erro diferente de NotFound ao tentar buscar para log, não impede a exclusão
		appLogger.Warnf("Erro ao buscar CNPJ ID %d para log antes da exclusão (prosseguindo com exclusão): %v", cnpjID, err)
	}
	// Se for ErrNotFound, o repo.Delete() tratará isso e retornará ErrNotFound.

	// 3. Chamar Repositório
	if err := s.repo.Delete(cnpjID); err != nil {
		// ErrNotFound já é tratado e logado pelo repo.
		return err
	}

	// 4. Log de Auditoria
	logEntry := models.AuditLogEntry{
		Action:      "CNPJ_DELETE",
		Description: fmt.Sprintf("CNPJ %s (ID %d) excluído.", cnpjToLog, cnpjID),
		Severity:    "INFO", // Ou WARNING, dependendo da política
		Metadata:    map[string]interface{}{"deleted_cnpj_id": cnpjID, "deleted_cnpj_number_for_log": cnpjToLog},
	}
	if logErr := s.auditLogService.LogAction(logEntry, userSession); logErr != nil {
		appLogger.Warnf("Falha ao registrar log de auditoria para exclusão do CNPJ ID %s: %v", cnpjID, logErr)
	}
	return nil
}

// GetAllCNPJs busca todos os CNPJs.
func (s *cnpjServiceImpl) GetAllCNPJs(includeInactive bool, userSession *auth.SessionData) ([]*models.CNPJPublic, error) {
	if err := s.permManager.CheckPermission(userSession, auth.PermCNPJView, nil); err != nil {
		return nil, err
	}
	dbCNPJs, err := s.repo.GetAll(includeInactive)
	if err != nil {
		return nil, err
	}
	return models.ToCNPJPublicList(dbCNPJs), nil
}

// GetCNPJsByNetwork busca CNPJs por ID da rede.
func (s *cnpjServiceImpl) GetCNPJsByNetwork(networkID uint64, includeInactive bool, userSession *auth.SessionData) ([]*models.CNPJPublic, error) {
	if err := s.permManager.CheckPermission(userSession, auth.PermCNPJView, nil); err != nil {
		return nil, err
	}
	// Verificar se a NetworkID existe
	if _, err := s.networkRepo.GetByID(networkID); err != nil {
		appLogger.Warnf("Network ID %d não encontrada ao tentar buscar CNPJs: %v", networkID, err)
		return nil, fmt.Errorf("%w: rede com ID %d não encontrada", appErrors.ErrNotFound, networkID)
	}

	dbCNPJs, err := s.repo.GetByNetworkID(networkID, includeInactive)
	if err != nil {
		return nil, err
	}
	return models.ToCNPJPublicList(dbCNPJs), nil
}

// UpdateCNPJ atualiza um CNPJ existente.
func (s *cnpjServiceImpl) UpdateCNPJ(cnpjID uint64, cnpjUpdateData models.CNPJUpdate, userSession *auth.SessionData) (*models.CNPJPublic, error) {
	// 1. Verificar Permissão (Supondo que cnpj:update exista, se não, use uma mais genérica)
	// Adicione PermCNPJUpdate às suas constantes de permissão se necessário
	// if err := s.permManager.CheckPermission(userSession, auth.PermCNPJUpdate, nil); err != nil {
	// 	return nil, err
	// }
	// Se não houver PermCNPJUpdate, pode-se usar PermCNPJCreate ou uma mais geral se apropriado.
	// Por agora, vamos assumir que uma permissão de "gerenciamento" mais ampla cobre isso, ou que view + delete cobre a UI.
	// Se a UI permitir update, uma permissão específica é melhor.
	// Vamos usar PermCNPJCreate como placeholder se PermCNPJUpdate não foi definida.
	var updatePermission auth.Permission = "cnpj:update" // Idealmente definido como constante
	if !s.permManager.IsPermissionDefined(updatePermission) {
		updatePermission = auth.PermCNPJCreate // Fallback se cnpj:update não estiver definida.
	}
	if err := s.permManager.CheckPermission(userSession, updatePermission, nil); err != nil {
		return nil, err
	}

	// 2. Validação (opcional, mas boa prática)
	// if cnpjUpdateData.NetworkID != nil && *cnpjUpdateData.NetworkID == 0 {
	// 	return nil, appErrors.NewValidationError("ID da Rede para atualização não pode ser zero.", map[string]string{"network_id": "ID inválido"})
	// }

	// 3. Verificar se a nova NetworkID (se fornecida) existe
	if cnpjUpdateData.NetworkID != nil {
		if _, err := s.networkRepo.GetByID(*cnpjUpdateData.NetworkID); err != nil {
			appLogger.Warnf("Nova Network ID %d não encontrada ao tentar atualizar CNPJ ID %d: %v", *cnpjUpdateData.NetworkID, cnpjID, err)
			return nil, fmt.Errorf("%w: nova rede com ID %d não encontrada para associar ao CNPJ", appErrors.ErrNotFound, *cnpjUpdateData.NetworkID)
		}
	}

	// 4. Chamar Repositório
	dbCNPJ, err := s.repo.Update(cnpjID, cnpjUpdateData)
	if err != nil {
		return nil, err
	}

	// 5. Log de Auditoria
	updatedFields := []string{}
	if cnpjUpdateData.NetworkID != nil {
		updatedFields = append(updatedFields, "network_id")
	}
	if cnpjUpdateData.Active != nil {
		updatedFields = append(updatedFields, "active")
	}

	logEntry := models.AuditLogEntry{
		Action:      "CNPJ_UPDATE",
		Description: fmt.Sprintf("CNPJ %s (ID %d) atualizado.", dbCNPJ.FormatCNPJ(), dbCNPJ.ID),
		Severity:    "INFO",
		Metadata:    map[string]interface{}{"updated_cnpj_id": dbCNPJ.ID, "fields": updatedFields},
	}
	if logErr := s.auditLogService.LogAction(logEntry, userSession); logErr != nil {
		appLogger.Warnf("Falha ao registrar log de auditoria para atualização do CNPJ ID %s: %v", dbCNPJ.ID, logErr)
	}

	return models.ToCNPJPublic(dbCNPJ), nil
}

// GetCNPJByID busca um CNPJ pelo ID.
func (s *cnpjServiceImpl) GetCNPJByID(cnpjID uint64, userSession *auth.SessionData) (*models.CNPJPublic, error) {
	if err := s.permManager.CheckPermission(userSession, auth.PermCNPJView, nil); err != nil {
		return nil, err
	}
	dbCNPJ, err := s.repo.GetByID(cnpjID)
	if err != nil {
		return nil, err // repo.GetByID já retorna ErrNotFound se for o caso
	}
	return models.ToCNPJPublic(dbCNPJ), nil
}
