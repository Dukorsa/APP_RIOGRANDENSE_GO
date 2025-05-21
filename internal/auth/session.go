package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core"
	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/errors"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/services" // Para AuditLogService, se usado
	"github.com/google/uuid"
)

// SessionData armazena informações sobre uma sessão de usuário ativa.
type SessionData struct {
	ID           string                 `json:"id"` // Session ID
	UserID       uuid.UUID              `json:"user_id"`
	Username     string                 `json:"username"`
	Roles        []string               `json:"roles"`
	IPAddress    string                 `json:"ip_address,omitempty"`
	UserAgent    string                 `json:"user_agent,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
	LastActivity time.Time              `json:"last_activity"`
	ExpiresAt    time.Time              `json:"expires_at"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// IsExpired verifica se a sessão expirou com base no tempo de inatividade.
func (s *SessionData) IsExpired(sessionTimeout time.Duration) bool {
	return time.Now().UTC().After(s.LastActivity.Add(sessionTimeout))
}

// UpdateActivity atualiza o timestamp da última atividade.
func (s *SessionData) UpdateActivity() {
	s.LastActivity = time.Now().UTC()
}

var (
	currentSessionID     string
	currentSessionIDLock sync.RWMutex
)

// SetCurrentSessionID define o ID da sessão ativa no contexto global.
func SetCurrentSessionID(sessionID string) {
	currentSessionIDLock.Lock()
	defer currentSessionIDLock.Unlock()
	currentSessionID = sessionID
	if sessionID != "" {
		appLogger.Debugf("Contexto de sessão definido para ID: %s...", sessionID[:8])
	} else {
		appLogger.Debug("Contexto de sessão limpo.")
	}
}

// GetCurrentSessionID obtém o ID da sessão ativa do contexto global.
func GetCurrentSessionID() string {
	currentSessionIDLock.RLock()
	defer currentSessionIDLock.RUnlock()
	return currentSessionID
}

// SessionManager gerencia sessões de usuário.
type SessionManager struct {
	cfg             *core.Config
	sessions        map[string]*SessionData
	lock            sync.RWMutex
	sessionFilePath string
	auditLogService services.AuditLogService // Pode ser nil se não usado ativamente
	shutdownChan    chan struct{}
	wg              sync.WaitGroup
}

// NewSessionManager cria uma nova instância do SessionManager.
// db é `interface{}` para flexibilidade, mas não é usado atualmente pois a persistência é em JSON.
func NewSessionManager(cfg *core.Config, db interface{}, auditLogService services.AuditLogService) *SessionManager {
	sessionsDir := filepath.Dir(cfg.SessionsJSONFile)
	absSessionsDir, _ := filepath.Abs(sessionsDir)
	if err := os.MkdirAll(absSessionsDir, os.ModePerm); err != nil {
		appLogger.Warnf("Não foi possível criar diretório para sessions.json '%s': %v", absSessionsDir, err)
	}
	absSessionFilePath, _ := filepath.Abs(cfg.SessionsJSONFile)

	sm := &SessionManager{
		cfg:             cfg,
		sessions:        make(map[string]*SessionData),
		sessionFilePath: absSessionFilePath,
		auditLogService: auditLogService, // Pode ser nil
		shutdownChan:    make(chan struct{}),
	}
	sm.loadSessionsFromFile()
	return sm
}

// StartCleanupGoroutine inicia uma goroutine para limpar sessões expiradas periodicamente.
func (sm *SessionManager) StartCleanupGoroutine() {
	if !sm.cfg.SessionCleanupEnabled {
		appLogger.Info("Limpeza de sessão em background desabilitada.")
		return
	}

	sm.wg.Add(1)
	go func() {
		defer sm.wg.Done()
		ticker := time.NewTicker(sm.cfg.SessionCleanupInterval)
		defer ticker.Stop()

		appLogger.Infof("Goroutine de limpeza de sessões iniciada (intervalo: %v).", sm.cfg.SessionCleanupInterval)
		for {
			select {
			case <-ticker.C:
				sm.cleanupExpiredSessions()
			case <-sm.shutdownChan:
				appLogger.Info("Goroutine de limpeza de sessões recebendo sinal de shutdown.")
				return
			}
		}
	}()
}

// Shutdown para o SessionManager, salvando sessões e parando a goroutine de cleanup.
func (sm *SessionManager) Shutdown() {
	appLogger.Info("Iniciando shutdown do SessionManager...")
	if sm.cfg.SessionCleanupEnabled {
		close(sm.shutdownChan)
		sm.wg.Wait()
		appLogger.Info("Goroutine de limpeza de sessões finalizada.")
	}
	sm.saveSessionsToFile()
	SetCurrentSessionID("")
	appLogger.Info("SessionManager shutdown concluído.")
}

// CreateSession cria uma nova sessão para um usuário.
func (sm *SessionManager) CreateSession(data SessionData) (string, error) {
	sm.lock.Lock()
	defer sm.lock.Unlock()

	sessionID := uuid.NewString()
	data.ID = sessionID
	// CreatedAt, LastActivity, e ExpiresAt já são definidos pelo Authenticator ao criar a SessionData
	if data.CreatedAt.IsZero() { // Segurança caso não tenha sido preenchido
		data.CreatedAt = time.Now().UTC()
	}
	if data.LastActivity.IsZero() {
		data.LastActivity = data.CreatedAt
	}
	if data.ExpiresAt.IsZero() {
		data.ExpiresAt = data.CreatedAt.Add(sm.cfg.SessionTimeout)
	}

	sm.sessions[sessionID] = &data
	appLogger.Infof("Sessão criada: ID=%s..., UserID=%s, Username=%s, Roles=%v",
		sessionID[:8], data.UserID, data.Username, data.Roles)

	return sessionID, nil
}

// GetSession recupera uma sessão por seu ID. Retorna ErrNotFound se não existir ou estiver expirada.
func (sm *SessionManager) GetSession(sessionID string) (*SessionData, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("%w: ID da sessão não pode ser vazio", appErrors.ErrInvalidInput)
	}
	sm.lock.RLock()
	session, exists := sm.sessions[sessionID]
	sm.lock.RUnlock()

	if !exists {
		return nil, fmt.Errorf("%w: sessão %s... não encontrada", appErrors.ErrNotFound, sessionID[:8])
	}

	now := time.Now().UTC()
	if now.After(session.ExpiresAt) || session.IsExpired(sm.cfg.SessionTimeout) {
		appLogger.Infof("Sessão %s... (Usuário: %s) expirada durante GetSession. Removendo.", sessionID[:8], session.Username)
		sm.lock.Lock()
		delete(sm.sessions, sessionID)
		if GetCurrentSessionID() == sessionID {
			SetCurrentSessionID("")
		}
		sm.lock.Unlock()
		return nil, fmt.Errorf("%w: sessão expirada", appErrors.ErrSessionExpired)
	}

	sm.lock.Lock()
	session.UpdateActivity()
	// Se a expiração for deslizante (rolling expiration), atualize ExpiresAt:
	// session.ExpiresAt = session.LastActivity.Add(sm.cfg.SessionTimeout)
	sm.lock.Unlock()

	return session, nil
}

// GetCurrentSession obtém a SessionData correspondente ao ID de sessão ativo no contexto global.
func (sm *SessionManager) GetCurrentSession() (*SessionData, error) {
	sessionID := GetCurrentSessionID()
	if sessionID == "" {
		return nil, nil // Nenhuma sessão no contexto, não é um erro.
	}
	return sm.GetSession(sessionID)
}

// DeleteSession remove uma sessão específica.
func (sm *SessionManager) DeleteSession(sessionID string) error {
	if sessionID == "" {
		appLogger.Warn("Tentativa de deletar sessão com ID vazio.")
		return nil // Não é um erro fatal, mas loga.
	}
	sm.lock.Lock()
	defer sm.lock.Unlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		// Não retorna erro se a sessão já não existe, pois o objetivo é garantir que ela não exista.
		appLogger.Debugf("Tentativa de deletar sessão %s... que não existe ou já foi removida.", sessionID[:8])
		return nil
	}

	delete(sm.sessions, sessionID)
	appLogger.Infof("Sessão %s... (Usuário: %s) removida.", sessionID[:8], session.Username)

	if GetCurrentSessionID() == sessionID {
		SetCurrentSessionID("")
	}
	return nil
}

// DeleteAllUserSessions remove todas as sessões de um usuário específico.
func (sm *SessionManager) DeleteAllUserSessions(userID uuid.UUID) (int, error) {
	sm.lock.Lock()
	defer sm.lock.Unlock()

	var sessionsToDelete []string
	var usernameForLog string
	for id, s := range sm.sessions {
		if s.UserID == userID {
			sessionsToDelete = append(sessionsToDelete, id)
			if usernameForLog == "" {
				usernameForLog = s.Username
			}
		}
	}

	if len(sessionsToDelete) == 0 {
		appLogger.Infof("Nenhuma sessão ativa encontrada para remover para userID: %s", userID)
		return 0, nil
	}

	for _, id := range sessionsToDelete {
		delete(sm.sessions, id)
		if GetCurrentSessionID() == id {
			SetCurrentSessionID("")
		}
	}
	appLogger.Infof("%d sessões removidas para userID: %s (Usuário: %s)", len(sessionsToDelete), userID, usernameForLog)
	return len(sessionsToDelete), nil
}

// cleanupExpiredSessions é chamado pela goroutine de limpeza.
func (sm *SessionManager) cleanupExpiredSessions() {
	sm.lock.Lock()
	defer sm.lock.Unlock()

	now := time.Now().UTC()
	cleanedCount := 0
	for id, session := range sm.sessions {
		if now.After(session.ExpiresAt) || session.IsExpired(sm.cfg.SessionTimeout) {
			delete(sm.sessions, id)
			if GetCurrentSessionID() == id {
				SetCurrentSessionID("")
			}
			appLogger.Infof("Limpeza: Sessão %s... (Usuário: %s) expirada e removida.", id[:8], session.Username)
			cleanedCount++
		}
	}
	if cleanedCount > 0 {
		appLogger.Infof("Limpeza de sessões removeu %d sessões expiradas.", cleanedCount)
	} else {
		appLogger.Debug("Limpeza de sessões: Nenhuma sessão expirada encontrada.")
	}
}

// loadSessionsFromFile carrega sessões de um arquivo JSON.
func (sm *SessionManager) loadSessionsFromFile() {
	sm.lock.Lock()
	defer sm.lock.Unlock()

	if sm.sessionFilePath == "" {
		appLogger.Info("Caminho do arquivo de sessão não configurado. Nenhuma sessão carregada.")
		return
	}

	data, err := os.ReadFile(sm.sessionFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			appLogger.Infof("Arquivo de persistência de sessão '%s' não encontrado. Nenhuma sessão carregada.", sm.sessionFilePath)
		} else {
			appLogger.Errorf("Erro ao ler arquivo de sessão '%s': %v", sm.sessionFilePath, err)
		}
		return
	}

	if len(data) == 0 {
		appLogger.Infof("Arquivo de sessão '%s' está vazio.", sm.sessionFilePath)
		return
	}

	var loadedSessions map[string]*SessionData
	if err := json.Unmarshal(data, &loadedSessions); err != nil {
		appLogger.Errorf("Erro ao decodificar JSON do arquivo de sessão '%s': %v. Iniciando sem sessões carregadas.", sm.sessionFilePath, err)
		sm.sessions = make(map[string]*SessionData)
		return
	}

	validSessions := make(map[string]*SessionData)
	now := time.Now().UTC()
	loadedCount := 0
	expiredDuringLoad := 0

	for id, session := range loadedSessions {
		if session == nil {
			appLogger.Warnf("Sessão nula encontrada no arquivo para ID %s. Ignorando.", id)
			continue
		}
		if now.After(session.ExpiresAt) || session.IsExpired(sm.cfg.SessionTimeout) {
			appLogger.Debugf("Sessão %s... (Usuário: %s) carregada, mas já expirada. Descartando.", id[:8], session.Username)
			expiredDuringLoad++
		} else {
			validSessions[id] = session
			loadedCount++
		}
	}
	sm.sessions = validSessions
	appLogger.Infof("%d sessões válidas carregadas de '%s'. %d expiradas descartadas.", loadedCount, sm.sessionFilePath, expiredDuringLoad)
}

// saveSessionsToFile salva TODAS as sessões atualmente ativas no arquivo JSON.
func (sm *SessionManager) saveSessionsToFile() {
	sm.lock.RLock()
	sessionsToSave := make(map[string]*SessionData)
	now := time.Now().UTC()
	for id, session := range sm.sessions {
		if !(now.After(session.ExpiresAt) || session.IsExpired(sm.cfg.SessionTimeout)) {
			sessionsToSave[id] = session
		}
	}
	sm.lock.RUnlock()

	if sm.sessionFilePath == "" {
		appLogger.Warn("Caminho do arquivo de sessão não configurado. Não é possível salvar.")
		return
	}

	if len(sessionsToSave) == 0 {
		appLogger.Info("Nenhuma sessão ativa para salvar. Verificando se arquivo antigo deve ser removido.")
		if _, err := os.Stat(sm.sessionFilePath); err == nil {
			if err := os.Remove(sm.sessionFilePath); err == nil {
				appLogger.Infof("Arquivo de sessão %s removido pois não há sessões ativas.", sm.sessionFilePath)
			} else {
				appLogger.Errorf("Erro ao tentar remover arquivo de sessão vazio %s: %v", sm.sessionFilePath, err)
			}
		}
		return
	}

	data, err := json.MarshalIndent(sessionsToSave, "", "  ")
	if err != nil {
		appLogger.Errorf("Erro ao serializar sessões para JSON: %v", err)
		return
	}

	tempPath := sm.sessionFilePath + ".tmp"
	err = os.WriteFile(tempPath, data, 0644)
	if err != nil {
		appLogger.Errorf("Erro ao escrever arquivo de sessão temporário '%s': %v", tempPath, err)
		return
	}

	err = os.Rename(tempPath, sm.sessionFilePath)
	if err != nil {
		appLogger.Errorf("Erro ao renomear arquivo de sessão temporário para '%s': %v", sm.sessionFilePath, err)
		_ = os.Remove(tempPath)
		return
	}
	appLogger.Infof("%d sessões ativas salvas em %s", len(sessionsToSave), sm.sessionFilePath)
}
