package services

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/auth"
	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/errors"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/models"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/repositories"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/utils" // Para IsValidCNPJ
)

// CNPJService define a interface para o serviço de CNPJ.
type CNPJService interface {
	// RegisterCNPJ registra um novo CNPJ.
	RegisterCNPJ(cnpjData models.CNPJCreate, userSession *auth.SessionData) (*models.CNPJPublic, error)

	// DeleteCNPJ exclui um CNPJ pelo ID.
	DeleteCNPJ(cnpjID uint64, userSession *auth.SessionData) error

	// GetAllCNPJs busca todos os CNPJs, com opção de incluir inativos.
	GetAllCNPJs(includeInactive bool, userSession *auth.SessionData) ([]*models.CNPJPublic, error)

	// GetCNPJsByNetwork busca CNPJs associados a um ID de rede específico.
	GetCNPJsByNetwork(networkID uint64, includeInactive bool, userSession *auth.SessionData) ([]*models.CNPJPublic, error)

	// UpdateCNPJ atualiza um CNPJ existente.
	UpdateCNPJ(cnpjID uint64, cnpjUpdateData models.CNPJUpdate, userSession *auth.SessionData) (*models.CNPJPublic, error)

	// GetCNPJByID busca um CNPJ pelo seu ID.
	GetCNPJByID(cnpjID uint64, userSession *auth.SessionData) (*models.CNPJPublic, error)

	// GetCNPJByNumber busca um CNPJ pelo seu número (14 dígitos).
	GetCNPJByNumber(cnpjNumber string, userSession *auth.SessionData) (*models.CNPJPublic, error)
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
		appLogger.Fatalf("Dependências nulas fornecidas para NewCNPJService (repo, networkRepo, auditLog, permManager)")
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
	// `CleanAndValidateCNPJ` da struct `CNPJCreate` limpa e valida o formato (14 dígitos).
	cleanedCNPJ, validationErr := cnpjData.CleanAndValidateCNPJ()
	if validationErr != nil {
		appLogger.Warnf("Dados de criação de CNPJ inválidos (formato): %v", validationErr)
		return nil, validationErr // Retorna o ValidationError
	}
	// A struct `cnpjData` não é modificada por `CleanAndValidateCNPJ`, então usamos `cleanedCNPJ`.
	// Passamos o `cleanedCNPJ` para o repositório.

	// Validação adicional de dígitos verificadores.
	if !utils.IsValidCNPJ(cleanedCNPJ) { // `utils.IsValidCNPJ` espera apenas dígitos.
		appLogger.Warnf("CNPJ '%s' (limpo: '%s') falhou na validação dos dígitos verificadores.", cnpjData.CNPJ, cleanedCNPJ)
		return nil, appErrors.NewValidationError("CNPJ inválido (dígitos verificadores não conferem).", map[string]string{"cnpj": "CNPJ inválido"})
	}

	// 3. Verificar se a NetworkID existe e está ativa (opcional, dependendo da regra de negócio)
	network, errNet := s.networkRepo.GetByID(cnpjData.NetworkID)
	if errNet != nil {
		if errors.Is(errNet, appErrors.ErrNotFound) {
			appLogger.Warnf("Network ID %d não encontrada ao tentar registrar CNPJ '%s': %v", cnpjData.NetworkID, cleanedCNPJ, errNet)
			return nil, fmt.Errorf("%w: rede com ID %d não encontrada para associar ao CNPJ", appErrors.ErrNotFound, cnpjData.NetworkID)
		}
		return nil, appErrors.WrapErrorf(errNet, "erro ao verificar Network ID %d", cnpjData.NetworkID)
	}
	if !network.Status { // Exemplo de regra: não permitir associar CNPJ a rede inativa.
		appLogger.Warnf("Tentativa de registrar CNPJ '%s' para rede inativa ID %d ('%s').", cleanedCNPJ, cnpjData.NetworkID, network.Name)
		return nil, fmt.Errorf("%w: não é possível associar CNPJ à rede inativa '%s' (ID: %d)", appErrors.ErrConflict, network.Name, cnpjData.NetworkID)
	}

	// 4. Preparar dados para o repositório e chamar.
	// O repositório espera o CNPJ limpo em `CNPJCreate.CNPJ`.
	dataForRepo := models.CNPJCreate{
		CNPJ:      cleanedCNPJ, // Usa o CNPJ limpo e validado.
		NetworkID: cnpjData.NetworkID,
	}
	dbCNPJ, err := s.repo.Add(dataForRepo)
	if err != nil {
		// Erros como ErrConflict (CNPJ já existe) ou ErrDatabase são tratados e logados pelo repo.
		return nil, err // Propaga o erro do repositório.
	}

	// 5. Log de Auditoria
	logEntry := models.AuditLogEntry{
		Action:      "CNPJ_CREATE",
		Description: fmt.Sprintf("CNPJ %s cadastrado para rede '%s' (ID: %d).", dbCNPJ.FormatCNPJ(), network.Name, dbCNPJ.NetworkID),
		Severity:    "INFO",
		Metadata:    map[string]interface{}{"cnpj_id": dbCNPJ.ID, "cnpj": dbCNPJ.CNPJ, "network_id": dbCNPJ.NetworkID, "network_name": network.Name},
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

	// 2. (Opcional) Buscar o CNPJ para logar o número antes de deletar e verificar existência.
	cnpjToLog := fmt.Sprintf("ID %d", cnpjID)
	existingCNPJ, errGet := s.repo.GetByID(cnpjID)
	if errGet != nil {
		if errors.Is(errGet, appErrors.ErrNotFound) {
			appLogger.Warnf("Tentativa de excluir CNPJ com ID %d que não existe.", cnpjID)
			return errGet // Retorna ErrNotFound.
		}
		// Outro erro ao buscar, não impede a tentativa de exclusão, mas loga.
		appLogger.Warnf("Erro ao buscar CNPJ ID %d para log antes da exclusão (prosseguindo com tentativa de exclusão): %v", cnpjID, errGet)
	} else if existingCNPJ != nil {
		cnpjToLog = existingCNPJ.FormatCNPJ()
	}

	// 3. Chamar Repositório
	if err := s.repo.Delete(cnpjID); err != nil {
		// ErrNotFound já é tratado e logado pelo repo (e verificado acima).
		// Outros erros (ex: FK constraint se o CNPJ estiver em uso) serão propagados.
		return err
	}

	// 4. Log de Auditoria
	logEntry := models.AuditLogEntry{
		Action:      "CNPJ_DELETE",
		Description: fmt.Sprintf("CNPJ %s (ID %d) excluído.", cnpjToLog, cnpjID),
		Severity:    "WARNING", // Exclusão é geralmente um Warning.
		Metadata:    map[string]interface{}{"deleted_cnpj_id": cnpjID, "deleted_cnpj_number_for_log": cnpjToLog},
	}
	if logErr := s.auditLogService.LogAction(logEntry, userSession); logErr != nil {
		appLogger.Warnf("Falha ao registrar log de auditoria para exclusão do CNPJ ID %d: %v", cnpjID, logErr)
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
		return nil, err // Erro já logado pelo repo.
	}
	return models.ToCNPJPublicList(dbCNPJs), nil
}

// GetCNPJsByNetwork busca CNPJs por ID da rede.
func (s *cnpjServiceImpl) GetCNPJsByNetwork(networkID uint64, includeInactive bool, userSession *auth.SessionData) ([]*models.CNPJPublic, error) {
	if err := s.permManager.CheckPermission(userSession, auth.PermCNPJView, nil); err != nil {
		return nil, err
	}
	// Verificar se a NetworkID existe antes de buscar CNPJs para ela.
	if _, err := s.networkRepo.GetByID(networkID); err != nil {
		if errors.Is(err, appErrors.ErrNotFound) {
			appLogger.Warnf("Network ID %d não encontrada ao tentar buscar CNPJs associados: %v", networkID, err)
			return nil, fmt.Errorf("%w: rede com ID %d não encontrada", appErrors.ErrNotFound, networkID)
		}
		return nil, appErrors.WrapErrorf(err, "erro ao verificar Network ID %d para buscar CNPJs", networkID)
	}

	dbCNPJs, err := s.repo.GetByNetworkID(networkID, includeInactive)
	if err != nil {
		return nil, err // Erro já logado pelo repo.
	}
	return models.ToCNPJPublicList(dbCNPJs), nil
}

// UpdateCNPJ atualiza um CNPJ existente.
func (s *cnpjServiceImpl) UpdateCNPJ(cnpjID uint64, cnpjUpdateData models.CNPJUpdate, userSession *auth.SessionData) (*models.CNPJPublic, error) {
	// 1. Verificar Permissão
	if err := s.permManager.CheckPermission(userSession, auth.PermCNPJUpdate, nil); err != nil {
		return nil, err
	}

	// 2. Validação da entrada (NetworkID > 0 se fornecido).
	// O modelo `CNPJUpdate` pode ter tags de validação para isso, ou validar aqui.
	if cnpjUpdateData.NetworkID != nil && *cnpjUpdateData.NetworkID == 0 {
		return nil, appErrors.NewValidationError("ID da Rede para atualização não pode ser zero.", map[string]string{"network_id": "ID inválido"})
	}

	// 3. Verificar se a nova NetworkID (se fornecida) existe e está ativa.
	var newNetworkName string
	if cnpjUpdateData.NetworkID != nil {
		network, errNet := s.networkRepo.GetByID(*cnpjUpdateData.NetworkID)
		if errNet != nil {
			if errors.Is(errNet, appErrors.ErrNotFound) {
				appLogger.Warnf("Nova Network ID %d não encontrada ao tentar atualizar CNPJ ID %d: %v", *cnpjUpdateData.NetworkID, cnpjID, errNet)
				return nil, fmt.Errorf("%w: nova rede com ID %d não encontrada para associar ao CNPJ", appErrors.ErrNotFound, *cnpjUpdateData.NetworkID)
			}
			return nil, appErrors.WrapErrorf(errNet, "erro ao verificar nova Network ID %d para CNPJ", *cnpjUpdateData.NetworkID)
		}
		if !network.Status {
			appLogger.Warnf("Tentativa de atualizar CNPJ ID %d para rede inativa ID %d ('%s').", cnpjID, *cnpjUpdateData.NetworkID, network.Name)
			return nil, fmt.Errorf("%w: não é possível associar CNPJ à rede inativa '%s' (ID: %d)", appErrors.ErrConflict, network.Name, *cnpjUpdateData.NetworkID)
		}
		newNetworkName = network.Name
	}

	// 4. Chamar Repositório
	dbCNPJ, err := s.repo.Update(cnpjID, cnpjUpdateData)
	if err != nil {
		return nil, err // Erro já logado e formatado pelo repo.
	}

	// 5. Log de Auditoria
	updatedFields := []string{}
	meta := map[string]interface{}{"updated_cnpj_id": dbCNPJ.ID, "cnpj": dbCNPJ.CNPJ}
	if cnpjUpdateData.NetworkID != nil {
		updatedFields = append(updatedFields, fmt.Sprintf("network_id para %d (%s)", *cnpjUpdateData.NetworkID, newNetworkName))
		meta["new_network_id"] = *cnpjUpdateData.NetworkID
		meta["new_network_name"] = newNetworkName
	}
	if cnpjUpdateData.Active != nil {
		statusStr := "Inativo"
		if *cnpjUpdateData.Active {
			statusStr = "Ativo"
		}
		updatedFields = append(updatedFields, fmt.Sprintf("status para %s", statusStr))
		meta["new_status"] = *cnpjUpdateData.Active
	}

	logEntry := models.AuditLogEntry{
		Action:      "CNPJ_UPDATE",
		Description: fmt.Sprintf("CNPJ %s (ID %d) atualizado. Campos alterados: %s.", dbCNPJ.FormatCNPJ(), dbCNPJ.ID, strings.Join(updatedFields, ", ")),
		Severity:    "INFO",
		Metadata:    meta,
	}
	if logErr := s.auditLogService.LogAction(logEntry, userSession); logErr != nil {
		appLogger.Warnf("Falha ao registrar log de auditoria para atualização do CNPJ ID %d: %v", dbCNPJ.ID, logErr)
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
		return nil, err // repo.GetByID já retorna ErrNotFound ou outro erro formatado.
	}
	return models.ToCNPJPublic(dbCNPJ), nil
}

// GetCNPJByNumber busca um CNPJ pelo seu número (14 dígitos).
func (s *cnpjServiceImpl) GetCNPJByNumber(cnpjNumber string, userSession *auth.SessionData) (*models.CNPJPublic, error) {
	if err := s.permManager.CheckPermission(userSession, auth.PermCNPJView, nil); err != nil {
		return nil, err
	}

	cleanedCNPJ := models.CleanCNPJ(cnpjNumber)
	if len(cleanedCNPJ) != 14 {
		return nil, appErrors.NewValidationError("CNPJ para busca deve ter 14 dígitos.", map[string]string{"cnpj": "formato inválido para busca"})
	}
	// Validação de dígitos verificadores pode ser opcional para busca, mas se o formato é inválido, não encontrará.
	// if !utils.IsValidCNPJ(cleanedCNPJ) {
	// 	return nil, appErrors.NewValidationError("CNPJ para busca inválido (dígitos verificadores).", map[string]string{"cnpj": "dígitos inválidos"})
	// }

	dbCNPJ, err := s.repo.GetByCNPJ(cleanedCNPJ) // Passa o CNPJ limpo
	if err != nil {
		return nil, err
	}
	return models.ToCNPJPublic(dbCNPJ), nil
}
