package main

import (
	"log"
	"os"

	"gioui.org/app"         // Para criar e gerenciar a janela da aplicação.
	"gioui.org/font/gofont" // Registra uma coleção de fontes Go padrão.

	// "gioui.org/io/system"   // Para eventos do sistema, como o fechamento da janela.
	// "gioui.org/layout"      // Para organizar widgets.
	// "gioui.org/op"          // Para operações de desenho de baixo nível.
	"gioui.org/widget/material" // Um tema de widgets Material Design.

	// Imports internos do seu projeto
	"github.com/seu_usuario/riograndense_gio/internal/auth"                  // Exemplo de import de pacote de autenticação
	"github.com/seu_usuario/riograndense_gio/internal/core"                  // Para configurações
	appLogger "github.com/seu_usuario/riograndense_gio/internal/core/logger" // Alias para o seu logger customizado
	"github.com/seu_usuario/riograndense_gio/internal/data"                  // Para configuração do banco de dados
	"github.com/seu_usuario/riograndense_gio/internal/services"              // Para os serviços da aplicação
	"github.com/seu_usuario/riograndense_gio/internal/ui"                    // Para a UI principal e o router
)

func main() {
	// Garante que a função run() seja chamada na thread principal do OS,
	// o que é necessário para muitas bibliotecas de GUI.
	// A função eventLoop() então gerencia o loop de eventos da UI.
	go run()
	app.Main()
}

func run() {
	// --- 1. Carregar Configurações ---
	cfg, err := core.LoadConfig(".env") // Assume .env na raiz do projeto Go
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
		if err := data.CloseDB(db); err != nil { // Supondo uma função CloseDB
			appLogger.Errorf("Erro ao fechar conexão com banco de dados: %v", err)
		} else {
			appLogger.Info("Conexão com banco de dados fechada.")
		}
	}()
	appLogger.Info("Banco de dados inicializado com sucesso.")

	// --- (Opcional) Criar/Migrar Tabelas ---
	// Chame isso apenas se você quiser criar tabelas na inicialização.
	// Em produção, migrações são geralmente feitas separadamente.
	// data.CreateDatabaseTables(db) // Supondo que esta função exista e use o 'db' inicializado

	// --- 4. Inicializar Serviços ---
	// Os serviços precisam da conexão com o banco (ou da sessão/tx factory) e do logger.
	auditLogService := services.NewAuditLogService(db) // Baseado no seu log_service.py

	// EmailService (pode falhar graciosamente se a config não estiver completa)
	var emailService *services.EmailService
	if cfg.EmailSMTPServer != "" && cfg.EmailUser != "" { // Checagem mínima
		es, err := services.NewEmailService(cfg)
		if err != nil {
			appLogger.Warnf("Falha ao inicializar EmailService: %v. Funcionalidades de email estarão desabilitadas.", err)
		} else {
			emailService = es
			appLogger.Info("EmailService inicializado.")
		}
	} else {
		appLogger.Info("Configuração de Email incompleta. EmailService não será inicializado.")
	}

	// SessionManager (precisa do config para timeouts, etc.)
	sessionManager := auth.NewSessionManager(cfg, db, auditLogService)
	sessionManager.StartCleanupGoroutine() // Inicia a limpeza de sessões em background
	defer sessionManager.Shutdown()        // Garante que as sessões sejam salvas ao sair

	// Authenticator (precisa do config para tentativas de login, etc., e SessionManager)
	authenticator := auth.NewAuthenticator(cfg, db, sessionManager, auditLogService)

	// Outros serviços
	// Nota: os repositórios são geralmente instanciados DENTRO dos serviços.
	userService := services.NewUserService(db, auditLogService, emailService, cfg, authenticator, sessionManager)
	roleService := services.NewRoleService(db, auditLogService)
	networkService := services.NewNetworkService(db, auditLogService)
	cnpjService := services.NewCNPJService(db, auditLogService)
	importService := services.NewImportService(db, auditLogService, cfg)

	appLogger.Info("Todos os serviços foram inicializados.")

	// --- 5. Inicializar Tema e UI ---
	gofont.Register() // Registra a coleção de fontes Go padrão.
	// `th` será compartilhado entre a janela principal e todas as suas "páginas".
	th := material.NewTheme() // Você pode customizar o tema mais tarde em `internal/ui/theme/theme.go`

	// Cria a instância da Janela Principal da Aplicação
	// Passa todas as dependências necessárias para a AppWindow, que então as passará para o Router e Páginas.
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
		// Adicione outros serviços conforme necessário
	)

	appLogger.Info("Interface do usuário (AppWindow) pronta para iniciar.")

	// --- 6. Iniciar o Loop de Eventos da UI ---
	// A função Run da AppWindow conterá o loop de eventos do Gio.
	// Esta função bloqueará até que a janela seja fechada.
	if err := appWindow.Run(); err != nil {
		appLogger.Fatalf("Erro ao executar a janela da aplicação: %v", err)
		os.Exit(1) // Garante que a aplicação saia em caso de erro fatal na UI.
	}

	appLogger.Info("Aplicação encerrada normalmente.")
	os.Exit(0) // Saída limpa
}
