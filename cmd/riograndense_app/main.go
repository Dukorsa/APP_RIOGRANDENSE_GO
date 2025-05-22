package main

import (
	"log"
	"os"

	"gioui.org/app"
	"gioui.org/font/gofont"
	"gioui.org/widget/material"

	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/auth"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/config" // O pacote aqui é 'config'
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/repositories"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/services"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui"
)

func main() {
	go run()
	app.Main()
}

func run() {
	// --- 1. Carregar Configurações ---
	// CORREÇÃO: Usar 'config.LoadConfig' em vez de 'core.LoadConfig'
	cfg, err := config.LoadConfig(".env")
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
	roleRepo := repositories.NewGormRoleRepository(db)
	userRepo := repositories.NewGormUserRepository(db) // CORREÇÃO: userRepo é necessário para NewUserService

	auth.InitGlobalPermissionManager(roleRepo)
	permManager := auth.GetPermissionManager()

	if err := auth.SeedInitialRolesAndPermissions(roleRepo, permManager); err != nil {
		appLogger.Fatalf("Erro CRÍTICO ao semear roles e permissões iniciais: %v", err)
	}

	// --- 5. Inicializar Serviços ---
	var emailService services.EmailService
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

	auditLogRepo := repositories.NewGormAuditLogRepository(db)
	sessionManager := auth.NewSessionManager(cfg, db, nil) // Passando nil para AuditLogService aqui é aceitável se SessionManager não o usa ativamente.

	auditLogService := services.NewAuditLogService(auditLogRepo, sessionManager)

	sessionManager.StartCleanupGoroutine()
	defer sessionManager.Shutdown()

	authenticator := auth.NewAuthenticator(cfg, db, sessionManager, auditLogService)

	// Outros Repositórios
	networkRepo := repositories.NewGormNetworkRepository(db)
	cnpjRepo := repositories.NewGormCNPJRepository(db)
	importMetadataRepo := repositories.NewGormImportMetadataRepository(db)
	tituloDireitoRepo := repositories.NewGormTituloDireitoRepository(db)
	tituloObrigacaoRepo := repositories.NewGormTituloObrigacaoRepository(db)

	// Outros Serviços
	// CORREÇÃO: Ajustar a chamada para NewUserService para corresponder a uma assinatura provável de 7 argumentos
	// A assinatura inferida é: (cfg, userRepo, roleRepo, auditLogService, emailService, authenticator, sessionManager)
	userService := services.NewUserService(cfg, userRepo, roleRepo, auditLogService, emailService, authenticator, sessionManager)
	roleService := services.NewRoleService(roleRepo, auditLogService, permManager)
	networkService := services.NewNetworkService(networkRepo, auditLogService, permManager)
	cnpjService := services.NewCNPJService(cnpjRepo, networkRepo, auditLogService, permManager)
	importService := services.NewImportService(cfg, auditLogService, permManager, importMetadataRepo, tituloDireitoRepo, tituloObrigacaoRepo)

	appLogger.Info("Todos os serviços foram inicializados.")

	// --- 6. Inicializar Tema e UI ---
	gofont.Register()
	th := material.NewTheme() // Pode ser customizado em internal/ui/theme/theme.go

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
