package main

import (
	"log"
	"os"

	"gioui.org/app"
	"gioui.org/font/gofont"
	"gioui.org/widget/material"

	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/auth"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/repositories"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/services"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui"
)

func main() {
	go run()
	app.Main()
}

func run() {
	// --- 1. Carregar Configurações ---
	cfg, err := core.LoadConfig(".env")
	if err != nil {
		log.Fatalf("Erro CRÍTICO ao carregar configuração: %v", err)
	}

	// --- 2. Configurar Logger ---
	if err := appLogger.SetupLogger(cfg); err != nil {
		log.Fatalf("Erro CRÍTICO ao configurar logger: %v", err)
	}
	appLogger.Info("=====================================================")
	appLogger.Infof("Iniciando %s v%s...", cfg.AppName, cfg.AppVersion)
	appLogger.Debugf("Modo Debug: %t", cfg.AppDebug)
	appLogger.Info("=====================================================")

	// --- 3. Inicializar Banco de Dados ---
	// InitializeDB retorna *gorm.DB
	db, err := data.InitializeDB(cfg)
	if err != nil {
		appLogger.Fatalf("Erro CRÍTICO ao inicializar banco de dados: %v", err)
	}
	defer func() {
		if err := data.CloseDB(db); err != nil {
			appLogger.Errorf("Erro ao fechar conexão com banco de dados: %v", err)
		} else {
			appLogger.Info("Conexão com banco de dados fechada.")
		}
	}()
	appLogger.Info("Banco de dados inicializado com sucesso.")

	// --- 4. Inicializar Repositórios e PermissionManager Global ---
	// RoleRepository é necessário para o PermissionManager e RoleService
	roleRepo := repositories.NewGormRoleRepository(db)

	// Inicializar PermissionManager Global (deve ser feito antes dos serviços que dependem dele)
	auth.InitGlobalPermissionManager(roleRepo)
	permManager := auth.GetPermissionManager() // Obter a instância global

	// Seed de Roles e Permissões Iniciais (após o PermissionManager estar pronto)
	if err := auth.SeedInitialRolesAndPermissions(roleRepo, permManager); err != nil {
		appLogger.Fatalf("Erro CRÍTICO ao semear roles e permissões iniciais: %v", err)
	}

	// --- 5. Inicializar Serviços ---

	// EmailService (pode falhar graciosamente)
	var emailService services.EmailService // Declarado como interface
	if cfg.EmailSMTPServer != "" && cfg.EmailUser != "" {
		esInstance, errMail := services.NewEmailService(cfg)
		if errMail != nil {
			appLogger.Warnf("Falha ao inicializar EmailService: %v. Funcionalidades de email estarão desabilitadas.", errMail)
		} else {
			emailService = esInstance
			appLogger.Info("EmailService inicializado.")
		}
	} else {
		appLogger.Info("Configuração de Email incompleta. EmailService não será inicializado.")
	}

	// AuditLogRepository
	auditLogRepo := repositories.NewGormAuditLogRepository(db)

	// SessionManager
	// NewSessionManager foi ajustado para não exigir AuditLogService na construção para evitar ciclo.
	// Se SessionManager precisar logar, ele pode usar appLogger ou ter o AuditLogService injetado depois.
	// O construtor do SessionManager aceita `db interface{}` e `auditLogService services.AuditLogService`.
	// Passando nil para auditLogService se não for usado na construção ou se a assinatura permitir.
	// A assinatura em session.go é: NewSessionManager(cfg *core.Config, db interface{}, auditLogService services.AuditLogService)
	// Vamos passar nil para auditLogService para sessionManager e criar AuditLogService depois.
	// A implementação atual do SessionManager armazena o auditLogService mas não o usa ativamente.
	// Para AuditLogService precisar do SessionManager:
	sessionManager := auth.NewSessionManager(cfg, db, nil) // Passando nil para AuditLogService

	// AuditLogService (agora pode receber o SessionManager)
	auditLogService := services.NewAuditLogService(auditLogRepo, sessionManager)

	// Se SessionManager realmente precisasse do AuditLogService, você poderia injetá-lo agora:
	// sessionManager.SetAuditLogService(auditLogService) // (se tal método existisse)
	// Ou, o SessionManager usa o appLogger global para seus próprios logs internos.

	sessionManager.StartCleanupGoroutine()
	defer sessionManager.Shutdown()

	// Authenticator
	// NewAuthenticator precisa ser capaz de lidar com *gorm.DB ou seu UserRepository interno precisa ser GORM-compatível.
	// Assumindo que NewAuthenticator foi ajustado para instanciar NewGormUserRepository(db)
	authenticator := auth.NewAuthenticator(cfg, db, sessionManager, auditLogService)

	// Outros Repositórios
	userRepo := repositories.NewGormUserRepository(db)
	networkRepo := repositories.NewGormNetworkRepository(db)
	cnpjRepo := repositories.NewGormCNPJRepository(db)
	importMetadataRepo := repositories.NewGormImportMetadataRepository(db)
	tituloDireitoRepo := repositories.NewGormTituloDireitoRepository(db)
	tituloObrigacaoRepo := repositories.NewGormTituloObrigacaoRepository(db)

	// Outros Serviços (agora passando os repositórios e o permManager global)
	userService := services.NewUserService(db, auditLogService, emailService, cfg, authenticator, sessionManager)
	roleService := services.NewRoleService(roleRepo, auditLogService, permManager)
	networkService := services.NewNetworkService(networkRepo, auditLogService, permManager)
	cnpjService := services.NewCNPJService(cnpjRepo, networkRepo, auditLogService, permManager)
	importService := services.NewImportService(cfg, auditLogService, permManager, importMetadataRepo, tituloDireitoRepo, tituloObrigacaoRepo)

	appLogger.Info("Todos os serviços foram inicializados.")

	// --- 6. Inicializar Tema e UI ---
	gofont.Register()
	th := material.NewTheme() // Pode ser customizado em internal/ui/theme/theme.go

	// Cria a instância da Janela Principal da Aplicação
	appWindow := ui.NewAppWindow(
		th,
		cfg,
		authenticator,
		sessionManager,
		userService,
		roleService,
		networkService,
		cnpjService,
		importService,
		auditLogService,
	)

	appLogger.Info("Interface do usuário (AppWindow) pronta para iniciar.")

	// --- 7. Iniciar o Loop de Eventos da UI ---
	if err := appWindow.Run(); err != nil {
		appLogger.Fatalf("Erro ao executar a janela da aplicação: %v", err)
		os.Exit(1)
	}

	appLogger.Info("Aplicação encerrada normalmente.")
	os.Exit(0)
}
