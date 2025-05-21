package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/seu_usuario/riograndense_gio/internal/core"
	appErrors "github.com/seu_usuario/riograndense_gio/internal/core/errors"
	appLogger "github.com/seu_usuario/riograndense_gio/internal/core/logger" // Para AuditLog se SessionManager logar diretamente
	"github.com/seu_usuario/riograndense_gio/internal/services"              // Para AuditLogService
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
	ExpiresAt    time.Time              `json:"expires_at"` // Adicionado para clareza e persistência
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// IsExpired verifica se a sessão expirou.
func (s *SessionData) IsExpired(sessionTimeout time.Duration) bool {
	// Uma sessão pode expirar por inatividade ou por um tempo de expiração absoluto.
	// Aqui, vamos priorizar a inatividade se ExpiresAt não estiver muito no futuro.
	// O Python usava inatividade.
	return time.Now().UTC().After(s.LastActivity.Add(sessionTimeout))
	// Ou, se você quiser uma expiração absoluta:
	// return time.Now().UTC().After(s.ExpiresAt)
}

// UpdateActivity atualiza o timestamp da última atividade.
func (s *SessionData) UpdateActivity() {
	s.LastActivity = time.Now().UTC()
}

// --- Gerenciamento de Sessão Atual (Global) ---
// Similar ao contextvars do Python, mas mais simples para uma app desktop.
// CUIDADO: Variáveis globais com mutex podem ser complexas em cenários de alta concorrência.
// Para Gio, que geralmente tem uma thread principal de UI, isso pode ser gerenciável.
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
		appLogger.Debugf("Contexto de sessão definido para ID: %s", sessionID)
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
	sessions        map[string]*SessionData // In-memory store: sessionID -> SessionData
	lock            sync.RWMutex            // Protege o mapa de sessões
	sessionFilePath string
	// db *sql.DB // Se for persistir sessões no banco
	auditLogService services.AuditLogService // Para logar eventos de sessão, se necessário
	shutdownChan    chan struct{}            // Para sinalizar a goroutine de cleanup para parar
	wg              sync.WaitGroup           // Para esperar a goroutine de cleanup terminar
}

// NewSessionManager cria uma nova instância do SessionManager.
func NewSessionManager(cfg *core.Config, db interface{}, auditLogService services.AuditLogService) *SessionManager {
	// db pode ser *sql.DB ou *gorm.DB, etc.
	// Por enquanto, focaremos na persistência em arquivo JSON.

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
		auditLogService: auditLogService,
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
		close(sm.shutdownChan) // Sinaliza a goroutine para parar
		sm.wg.Wait()           // Espera a goroutine terminar
		appLogger.Info("Goroutine de limpeza de sessões finalizada.")
	}
	sm.saveSessionsToFile()
	SetCurrentSessionID("") // Limpa o contexto global ao desligar
	appLogger.Info("SessionManager shutdown concluído.")
}

// CreateSession cria uma nova sessão para um usuário.
func (sm *SessionManager) CreateSession(data SessionData) (string, error) {
	sm.lock.Lock()
	defer sm.lock.Unlock()

	sessionID := uuid.NewString()
	data.ID = sessionID
	data.CreatedAt = time.Now().UTC()
	data.LastActivity = data.CreatedAt
	data.ExpiresAt = data.CreatedAt.Add(sm.cfg.SessionTimeout) // Expiração inicial baseada no timeout

	sm.sessions[sessionID] = &data
	appLogger.Infof("Sessão criada: ID=%s, UserID=%s, Username=%s, Roles=%v",
		sessionID, data.UserID, data.Username, data.Roles)

	// Opcional: logar criação de sessão
	// sm.auditLogService.LogAction(...)

	return sessionID, nil
}

// GetSession recupera uma sessão por seu ID. Retorna ErrNotFound se não existir ou estiver expirada.
func (sm *SessionManager) GetSession(sessionID string) (*SessionData, error) {
	sm.lock.RLock()
	session, exists := sm.sessions[sessionID]
	sm.lock.RUnlock() // Libera RLock antes de potencialmente pegar Lock para update

	if !exists {
		return nil, fmt.Errorf("%w: sessão não encontrada", appErrors.ErrNotFound)
	}

	if session.IsExpired(sm.cfg.SessionTimeout) {
		appLogger.Infof("Sessão %s (Usuário: %s) expirada durante GetSession. Removendo.", sessionID, session.Username)
		// Deletar sessão expirada
		sm.lock.Lock()
		delete(sm.sessions, sessionID)
		if GetCurrentSessionID() == sessionID { // Limpa contexto global se for a sessão atual
			SetCurrentSessionID("")
		}
		sm.lock.Unlock()
		return nil, fmt.Errorf("%w: sessão expirada", appErrors.ErrSessionExpired)
	}

	// Atualiza LastActivity (requer Lock de escrita)
	sm.lock.Lock()
	session.UpdateActivity()
	// Se você quiser que a expiração seja deslizante (rolling expiration):
	// session.ExpiresAt = session.LastActivity.Add(sm.cfg.SessionTimeout)
	sm.lock.Unlock()

	return session, nil
}

// GetCurrentSession obtém a SessionData correspondente ao ID de sessão ativo no contexto global.
func (sm *SessionManager) GetCurrentSession() (*SessionData, error) {
	sessionID := GetCurrentSessionID()
	if sessionID == "" {
		return nil, nil // Nenhuma sessão no contexto, não é um erro, apenas não há sessão
	}
	return sm.GetSession(sessionID)
}

// DeleteSession remove uma sessão específica.
func (sm *SessionManager) DeleteSession(sessionID string) error {
	sm.lock.Lock()
	defer sm.lock.Unlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return fmt.Errorf("%w: sessão %s não encontrada para exclusão", appErrors.ErrNotFound, sessionID)
	}

	delete(sm.sessions, sessionID)
	appLogger.Infof("Sessão %s (Usuário: %s) removida.", sessionID, session.Username)

	if GetCurrentSessionID() == sessionID {
		SetCurrentSessionID("") // Limpa o contexto global se era a sessão ativa
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
	sm.lock.Lock() // Bloqueio total durante a limpeza
	defer sm.lock.Unlock()

	now := time.Now().UTC()
	cleanedCount := 0
	for id, session := range sm.sessions {
		// Usa IsExpired que checa LastActivity + SessionTimeout
		if session.IsExpired(sm.cfg.SessionTimeout) || now.After(session.ExpiresAt) {
			delete(sm.sessions, id)
			if GetCurrentSessionID() == id {
				SetCurrentSessionID("")
			}
			appLogger.Infof("Limpeza: Sessão %s (Usuário: %s) expirada e removida.", id, session.Username)
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
		sm.sessions = make(map[string]*SessionData) // Garante que comece vazio
		return
	}

	validSessions := make(map[string]*SessionData)
	now := time.Now().UTC()
	loadedCount := 0
	expiredDuringLoad := 0

	for id, session := range loadedSessions {
		if session == nil { // Checagem de segurança
			appLogger.Warnf("Sessão nula encontrada no arquivo para ID %s. Ignorando.", id)
			continue
		}
		// Verifica expiração baseada no ExpiresAt persistido ou na inatividade
		if now.After(session.ExpiresAt) || session.IsExpired(sm.cfg.SessionTimeout) {
			appLogger.Debugf("Sessão %s (Usuário: %s) carregada, mas já expirada. Descartando.", id, session.Username)
			expiredDuringLoad++
		} else {
			validSessions[id] = session
			loadedCount++
		}
	}
	sm.sessions = validSessions
	appLogger.Infof("%d sessões válidas carregadas de '%s'. %d expiradas descartadas.", loadedCount, sm.sessionFilePath, expiredDuringLoad)
}

// saveSessionsToFile salva TODAS as sessões atualmente ativas (não expiradas) no arquivo JSON.
func (sm *SessionManager) saveSessionsToFile() {
	sm.lock.RLock() // Só precisa de RLock para ler, mas cria uma cópia para evitar race no json.Marshal

	// Filtra sessões expiradas ANTES de salvar
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

	// Escrita atômica: escreve em temp e depois renomeia
	tempPath := sm.sessionFilePath + ".tmp"
	err = os.WriteFile(tempPath, data, 0644) // Permissões padrão
	if err != nil {
		appLogger.Errorf("Erro ao escrever arquivo de sessão temporário '%s': %v", tempPath, err)
		return
	}

	err = os.Rename(tempPath, sm.sessionFilePath)
	if err != nil {
		appLogger.Errorf("Erro ao renomear arquivo de sessão temporário para '%s': %v", sm.sessionFilePath, err)
		// Tenta remover o arquivo temporário se a renomeação falhar
		_ = os.Remove(tempPath)
		return
	}
	appLogger.Infof("%d sessões ativas salvas em %s", len(sessionsToSave), sm.sessionFilePath)
}
