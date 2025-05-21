package pages

import (
	"fmt"
	"image"
	"image/color"
	"strings"

	// "time" // Descomentado se usar para algo como timestamp de última atividade na UI

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"golang.org/x/exp/shiny/materialdesign/icons" // Para ícones padrão

	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/auth"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/data/models" // Para UserPublic ao receber params
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/services"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui"

	// "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui/components" // Se MainAppLayout tivesse seu próprio spinner
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui/theme"
)

// ModuleConfig define a configuração para cada módulo/item na sidebar.
type ModuleConfig struct {
	ID                 ui.PageID       // ID da página/módulo para navegação no router principal.
	Title              string          // Título exibido na sidebar.
	Icon               *widget.Icon    // Ícone para o item da sidebar (usando material.Icon).
	RequiredPermission auth.Permission // Permissão necessária para ver/acessar este módulo.
	// ActionOnClick func() // Opcional: Se o item da sidebar executar uma ação direta em vez de navegar.
}

// MainAppLayout é a página principal da aplicação após o login, contendo a sidebar e a área de conteúdo.
type MainAppLayout struct {
	router *ui.Router
	cfg    *core.Config
	// Serviços necessários para as sub-páginas ou para a própria MainAppLayout.
	userService    services.UserService
	roleService    services.RoleService
	networkService services.NetworkService
	cnpjService    services.CNPJService
	importService  services.ImportService
	auditService   services.AuditLogService // Para logs de auditoria específicos do layout, se houver
	permManager    *auth.PermissionManager
	sessionManager *auth.SessionManager

	// Estado da UI
	currentModuleID ui.PageID          // ID do módulo atualmente ativo na área de conteúdo.
	sidebarModules  []ModuleConfig     // Configuração dos módulos da sidebar, filtrados por permissão.
	sidebarClicks   []widget.Clickable // Clickables para os botões da sidebar.
	logoutBtn       widget.Clickable   // Botão de logout.

	// Sub-páginas/layouts para cada módulo.
	// Estes são instanciados uma vez e reutilizados.
	modulePages map[ui.PageID]ui.Page

	// Para diálogo de confirmação de logout.
	showLogoutConfirm bool
	logoutYesBtn      widget.Clickable
	logoutNoBtn       widget.Clickable

	currentUserData *models.UserPublic // Dados do usuário logado, recebidos via OnNavigatedTo.
}

// NewMainAppLayout cria uma nova instância do layout principal da aplicação.
func NewMainAppLayout(
	router *ui.Router,
	cfg *core.Config,
	userSvc services.UserService,
	roleSvc services.RoleService,
	netSvc services.NetworkService,
	cnpjSvc services.CNPJService,
	importSvc services.ImportService,
	// auditSvc é acessado via router, se necessário
	permMan *auth.PermissionManager,
	sessMan *auth.SessionManager,
) *MainAppLayout {
	ml := &MainAppLayout{
		router:         router,
		cfg:            cfg,
		userService:    userSvc,
		roleService:    roleSvc,
		networkService: netSvc,
		cnpjService:    cnpjSvc,
		importService:  importSvc,
		auditService:   router.AuditLogService(), // Obtém via router para evitar passar muitos params
		permManager:    permMan,
		sessionManager: sessMan,
		modulePages:    make(map[ui.PageID]ui.Page),
	}

	// Pré-instanciar todas as páginas dos módulos para evitar recriação a cada navegação.
	// Isso também permite que elas mantenham seu estado interno.
	// Passar as dependências necessárias para cada página.
	// Os serviços e managers são acessados via `ml.router` para reduzir o número de parâmetros aqui.
	ml.modulePages[ui.PageCNPJ] = NewCNPJPage(ml.router, ml.cfg, ml.cnpjService, ml.networkService, ml.permManager, ml.sessionManager)
	ml.modulePages[ui.PageAdminPermissions] = NewAdminPermissionsPage(ml.router, ml.cfg, ml.userService, ml.roleService, ml.auditService, ml.permManager, ml.sessionManager)
	ml.modulePages[ui.PageRoleManagement] = NewRoleManagementPage(ml.router, ml.cfg, ml.roleService, ml.auditService, ml.permManager, ml.sessionManager)
	ml.modulePages[ui.PageImport] = NewImportPage(ml.router, ml.cfg, ml.importService, ml.permManager, ml.sessionManager)
	ml.modulePages[ui.PageNetworks] = &PlaceholderPage{Title: "Gerenciamento de Redes"} // Exemplo de placeholder
	// Adicionar outras páginas de módulo aqui.

	return ml
}

// loadSidebarModules configura os módulos da sidebar com base nas permissões do usuário atual.
// Também define o módulo inicial a ser exibido.
func (ml *MainAppLayout) loadSidebarModules() {
	session := ml.currentUserData // Usa os dados do usuário já passados para OnNavigatedTo
	if session == nil {           // Segurança: se por algum motivo não tiver dados do usuário
		appLogger.Error("MainAppLayout: Dados do usuário logado não disponíveis ao carregar módulos da sidebar. Forçando logout.")
		ml.router.GetAppWindow().HandleLogout()
		return
	}
	// Cria uma SessionData simulada para usar com PermissionManager, se necessário.
	// O ideal é que PermissionManager use UserPublic ou a lista de roles diretamente.
	// Assumindo que PermissionManager.HasPermission pode usar UserPublic.Roles.
	// Se não, uma SessionData precisa ser construída:
	simulatedSessionData := &auth.SessionData{
		UserID:   session.ID,
		Username: session.Username,
		Roles:    session.Roles,
		// IPAddress e UserAgent não são relevantes para checagem de permissão baseada em role.
	}

	// Definição de todos os módulos possíveis e seus requisitos.
	// Ícones são do pacote materialdesign/icons.
	type moduleDef struct {
		IconData []byte
		Cfg      ModuleConfig
	}
	allModuleDefs := []moduleDef{
		{IconData: icons.ActionList, Cfg: ModuleConfig{ID: ui.PageNetworks, Title: "Redes", RequiredPermission: auth.PermNetworkView}},
		{IconData: icons.ActionVerifiedUser, Cfg: ModuleConfig{ID: ui.PageCNPJ, Title: "CNPJs", RequiredPermission: auth.PermCNPJView}},
		{IconData: icons.ActionSupervisorAccount, Cfg: ModuleConfig{ID: ui.PageAdminPermissions, Title: "Usuários", RequiredPermission: auth.PermUserRead}},
		{IconData: icons.ActionLockOpen, Cfg: ModuleConfig{ID: ui.PageRoleManagement, Title: "Perfis", RequiredPermission: auth.PermRoleManage}},
		{IconData: icons.FileFileUpload, Cfg: ModuleConfig{ID: ui.PageImport, Title: "Importar Dados", RequiredPermission: auth.PermImportExecute}},
		// Adicionar outros módulos (ex: Logs de Auditoria)
		// {IconData: icons.ActionAssignment, Cfg: ModuleConfig{ID: ui.PageAuditLogs, Title: "Logs de Auditoria", RequiredPermission: auth.PermLogView}},
	}

	ml.sidebarModules = []ModuleConfig{}
	for _, modDef := range allModuleDefs {
		hasPerm, _ := ml.permManager.HasPermission(simulatedSessionData, modDef.Cfg.RequiredPermission, nil)
		if hasPerm {
			iconWidget, errIcon := widget.NewIcon(modDef.IconData)
			if errIcon != nil {
				appLogger.Warnf("Falha ao carregar ícone para módulo '%s': %v. Usando sem ícone.", modDef.Cfg.Title, errIcon)
				modDef.Cfg.Icon = nil
			} else {
				modDef.Cfg.Icon = iconWidget
			}
			ml.sidebarModules = append(ml.sidebarModules, modDef.Cfg)
		}
	}
	ml.sidebarClicks = make([]widget.Clickable, len(ml.sidebarModules))

	// Define o módulo inicial a ser exibido.
	// Se o `currentModuleID` já estiver definido e ainda for acessível, mantém.
	// Caso contrário, seleciona o primeiro módulo acessível.
	currentModuleStillAccessible := false
	if ml.currentModuleID != ui.PageNone {
		for _, mod := range ml.sidebarModules {
			if mod.ID == ml.currentModuleID {
				currentModuleStillAccessible = true
				break
			}
		}
	}

	if !currentModuleStillAccessible {
		if len(ml.sidebarModules) > 0 {
			ml.currentModuleID = ml.sidebarModules[0].ID
		} else {
			// Nenhum módulo acessível. Isso é um problema de configuração de permissões ou roles.
			appLogger.Errorf("Usuário '%s' não tem acesso a nenhum módulo configurado na sidebar.", session.Username)
			// Poderia navegar para uma página de "Acesso Negado" ou forçar logout.
			// Por agora, a área de conteúdo ficará vazia ou mostrará uma mensagem.
			ml.currentModuleID = ui.PageNone // Indica que nenhum módulo pode ser exibido.
			ml.router.GetAppWindow().ShowGlobalMessage("Acesso Negado", "Você não tem permissão para acessar nenhum módulo.", true, 0)
		}
	}
}

// OnNavigatedTo é chamado quando o layout principal se torna a página ativa.
// `params` deve ser `*models.UserPublic` com os dados do usuário logado.
func (ml *MainAppLayout) OnNavigatedTo(params interface{}) {
	appLogger.Info("Navegou para MainAppLayout")
	ml.showLogoutConfirm = false // Garante que diálogo de logout esteja fechado

	userData, ok := params.(*models.UserPublic)
	if !ok || userData == nil {
		appLogger.Error("MainAppLayout: Parâmetros inválidos ou nulos recebidos em OnNavigatedTo (esperado *models.UserPublic). Forçando logout.")
		ml.router.GetAppWindow().HandleLogout() // Força logout se dados do usuário não forem recebidos
		return
	}
	ml.currentUserData = userData

	ml.loadSidebarModules() // Carrega/recarrega módulos da sidebar com base nas permissões do usuário atual

	// Notifica a sub-página (módulo) atual que ela se tornou ativa.
	// `getModulePage` também chama OnNavigatedTo da sub-página.
	currentPage := ml.getModulePage(ml.currentModuleID) // Isso também chama OnNavigatedTo da sub-página
	if currentPage == nil && ml.currentModuleID != ui.PageNone {
		appLogger.Errorf("Falha ao carregar o módulo ID %v em MainAppLayout.", ml.currentModuleID)
		// Se não conseguir carregar o módulo, pode mostrar uma mensagem de erro na área de conteúdo.
	}
}

// OnNavigatedFrom é chamado quando o router navega para fora do layout principal.
func (ml *MainAppLayout) OnNavigatedFrom() {
	appLogger.Info("Navegando para fora da MainAppLayout")
	// Notifica a sub-página (módulo) atual que ela está sendo desativada.
	if page, ok := ml.modulePages[ml.currentModuleID]; ok && page != nil {
		page.OnNavigatedFrom()
	}
	ml.currentUserData = nil // Limpa dados do usuário ao sair.
}

// getModulePage retorna (e instancia se necessário na primeira vez) a página para um ID de módulo.
func (ml *MainAppLayout) getModulePage(moduleID ui.PageID) ui.Page {
	if moduleID == ui.PageNone {
		return &PlaceholderPage{Title: "Nenhum Módulo Selecionado"}
	}

	page, exists := ml.modulePages[moduleID]
	if !exists || page == nil {
		appLogger.Warnf("Página para módulo ID %v não pré-instanciada ou nula. Usando placeholder.", moduleID)
		// Encontrar o título do módulo para o placeholder
		title := fmt.Sprintf("Módulo %v", moduleID)
		for _, modCfg := range ml.sidebarModules {
			if modCfg.ID == moduleID {
				title = modCfg.Title
				break
			}
		}
		return &PlaceholderPage{Title: fmt.Sprintf("%s (Não Implementado/Erro)", title)}
	}

	// Não precisa chamar OnNavigatedTo aqui, pois a lógica de mudança de módulo no Layout já faz isso.
	return page
}

// Layout é o método principal de desenho do layout principal.
func (ml *MainAppLayout) Layout(gtx layout.Context) layout.Dimensions {
	th := ml.router.GetAppWindow().Theme()

	// Processar cliques da sidebar para navegação entre módulos.
	for i := range ml.sidebarModules {
		if ml.sidebarClicks[i].Clicked(gtx) {
			selectedModuleID := ml.sidebarModules[i].ID
			if ml.currentModuleID != selectedModuleID {
				// Notifica a página antiga (módulo) que está sendo desativada.
				if oldPage, ok := ml.modulePages[ml.currentModuleID]; ok && oldPage != nil {
					oldPage.OnNavigatedFrom()
				}
				ml.currentModuleID = selectedModuleID
				appLogger.Infof("Módulo da MainAppLayout alterado para: %s (ID: %v)", ml.sidebarModules[i].Title, selectedModuleID)
				// Notifica a nova página (módulo) que ela se tornou ativa.
				// `getModulePage` não é chamado aqui para isso, pois a página já deve estar instanciada.
				// A notificação é feita ao obter a página para o layout.
				if newPage, ok := ml.modulePages[ml.currentModuleID]; ok && newPage != nil {
					newPage.OnNavigatedTo(nil) // Passa nil como params para a sub-página.
				} else {
					appLogger.Errorf("Tentativa de navegar para módulo ID %v que não tem página associada.", ml.currentModuleID)
				}
			}
		}
	}
	if ml.logoutBtn.Clicked(gtx) {
		ml.showLogoutConfirm = true
	}

	// Layout principal: Sidebar à esquerda, Área de Conteúdo à direita.
	mainContentLayout := layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(gtx C) D { // Sidebar
			return ml.layoutSidebar(gtx, th)
		}),
		layout.Rigid(func(gtx C) D { // Linha Divisória Vertical
			// Largura da linha e cor.
			lineWidth := unit.Dp(1)
			lineColor := theme.Colors.Border
			// Desenha um retângulo fino como linha.
			// Ocupa toda a altura disponível.
			rect := clip.Rect{Max: image.Pt(gtx.Dp(lineWidth), gtx.Constraints.Max.Y)}.Op()
			paint.FillShape(gtx.Ops, lineColor, rect)
			return layout.Dimensions{Size: image.Pt(gtx.Dp(lineWidth), gtx.Constraints.Max.Y)}
		}),
		layout.Flexed(1, func(gtx C) D { // Área de Conteúdo Principal
			return layout.Inset{Left: unit.Dp(0)}.Layout(gtx, // Sem inset extra após a linha.
				func(gtx layout.Context) layout.Dimensions {
					currentPageToLayout := ml.getModulePage(ml.currentModuleID)
					if currentPageToLayout == nil {
						// Fallback se o módulo não puder ser carregado.
						errMsg := fmt.Sprintf("Erro: Módulo ID %v não pôde ser carregado.", ml.currentModuleID)
						appLogger.Error(errMsg)
						return layout.Center.Layout(gtx, material.Body1(th, errMsg).Layout)
					}
					// Adiciona um padding padrão à área de conteúdo dos módulos.
					return layout.UniformInset(theme.PagePadding).Layout(gtx, currentPageToLayout.Layout)
				})
		}),
	)

	// Se o diálogo de confirmação de Logout estiver ativo, desenha-o sobreposto.
	if ml.showLogoutConfirm {
		// O diálogo deve ser centralizado e cobrir a tela.
		dialogLayout := ml.layoutLogoutConfirmDialog(gtx, th)
		return layout.Stack{}.Layout(gtx,
			layout.Expanded(func(gtx C) D { return mainContentLayout }), // Conteúdo principal por baixo
			layout.Expanded(func(gtx C) D { return dialogLayout }),      // Diálogo sobreposto
		)
	}

	return mainContentLayout
}

// layoutSidebar desenha a barra lateral de navegação.
func (ml *MainAppLayout) layoutSidebar(gtx layout.Context, th *material.Theme) layout.Dimensions {
	sidebarWidth := gtx.Dp(unit.Dp(230)) // Largura da sidebar.
	gtx.Constraints.Min.X = sidebarWidth
	gtx.Constraints.Max.X = sidebarWidth

	return layout.Background{Color: theme.Colors.Grey50}.Layout(gtx, // Cor de fundo da sidebar.
		func(gtx layout.Context) layout.Dimensions {
			// Padding interno da sidebar.
			return layout.UniformInset(unit.Dp(8)).Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceEnd}.Layout(gtx,
						// Seção de Informações do Usuário (Topo da Sidebar)
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if ml.currentUserData == nil {
								return layout.Dimensions{} // Não desenha se não houver dados do usuário.
							}
							rolesStr := "N/A"
							if len(ml.currentUserData.Roles) > 0 {
								rolesStr = strings.Join(ml.currentUserData.Roles, ", ")
							}
							// Usar um Card ou GroupBox para melhor agrupamento visual.
							return material.Card(th, theme.Colors.Surface, theme.ElevationSmall, layout.UniformInset(unit.Dp(8)),
								func(gtx C) D {
									return layout.Flex{Axis: layout.Vertical, Spacing: layout.Tight}.Layout(gtx,
										layout.Rigid(func(gtx C) D {
											lbl := material.Body1(th, "Usuário Logado:")
											lbl.Font.Weight = font.Bold
											return lbl.Layout(gtx)
										}),
										layout.Rigid(material.Body2(th, ml.currentUserData.Username).Layout),
										layout.Rigid(func(gtx C) D {
											lbl := material.Caption(th, fmt.Sprintf("Perfis: %s", rolesStr)) // Caption para texto menor
											lbl.Color = theme.Colors.TextMuted
											return lbl.Layout(gtx)
										}),
									)
								}).Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Height: theme.LargeVSpacer}.Layout), // Espaço após info do usuário.

						// Lista de Módulos Navegáveis
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							list := layout.List{Axis: layout.Vertical}
							return list.Layout(gtx, len(ml.sidebarModules), func(gtx layout.Context, index int) layout.Dimensions {
								modCfg := ml.sidebarModules[index]
								// Botão para cada item do módulo.
								// O material.Button pode ser estilizado para parecer um item de lista.
								btn := material.Button(th, &ml.sidebarClicks[index], "") // Texto vazio, será um Flex com Ícone e Título

								// Estilo do botão da sidebar:
								btn.Background = color.NRGBA{} // Transparente
								btn.Color = theme.Colors.Text  // Cor do texto padrão
								btn.CornerRadius = theme.CornerRadius
								btn.Inset = layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(12), Right: unit.Dp(12)}

								contentColor := theme.Colors.Text
								if ml.currentModuleID == modCfg.ID { // Módulo ativo
									btn.Background = theme.Colors.PrimaryLight
									contentColor = theme.Colors.PrimaryText
								}

								// Layout interno do botão (Ícone + Título)
								buttonContent := layout.Flex{Alignment: layout.Middle}
								iconWidget := layout.Dimensions{}
								if modCfg.Icon != nil {
									icon := material.Icon(th, modCfg.Icon)
									icon.Color = contentColor
									iconWidget = layout.Inset{Right: unit.Dp(8)}.Layout(gtx, icon.Layout)
								}

								titleLabel := material.Body1(th, modCfg.Title)
								titleLabel.Color = contentColor
								if ml.currentModuleID == modCfg.ID {
									titleLabel.Font.Weight = font.Bold
								}

								return layout.Inset{Bottom: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
									return btn.Layout(gtx,
										buttonContent.Layout(gtx,
											layout.Rigid(iconWidget.Layout),
											layout.Rigid(titleLabel.Layout),
										)...,
									)
								})
							})
						}),
						// Botão de Logout (Fundo da Sidebar)
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							logoutButton := material.Button(th, &ml.logoutBtn, "") // Texto vazio, usar Flex
							logoutButton.Background = theme.Colors.DangerBg
							logoutButton.Color = theme.Colors.DangerText
							logoutButton.CornerRadius = theme.CornerRadius
							logoutButton.Inset = layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(12), Right: unit.Dp(12)}

							// Ícone de Logout
							logoutIconWidget, _ := widget.NewIcon(icons.ActionExitToApp) // Ícone de saída
							logoutIcon := material.Icon(th, logoutIconWidget)
							logoutIcon.Color = theme.Colors.DangerText

							logoutLabel := material.Body1(th, "Sair / Logout")
							logoutLabel.Color = theme.Colors.DangerText
							logoutLabel.Font.Weight = font.SemiBold

							return logoutButton.Layout(gtx,
								layout.Flex{Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(layout.Inset{Right: unit.Dp(8)}.Layout(gtx, logoutIcon.Layout)),
									layout.Rigid(logoutLabel.Layout),
								)...,
							)
						}),
					)
				})
		})
}

// layoutLogoutConfirmDialog desenha um diálogo de confirmação para logout.
func (ml *MainAppLayout) layoutLogoutConfirmDialog(gtx layout.Context, th *material.Theme) layout.Dimensions {
	// Fundo semi-transparente para o efeito modal.
	paint.FillShape(gtx.Ops, color.NRGBA{A: 180}, clip.Rect{Max: gtx.Constraints.Max}.Op())

	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		// Limitar a largura do diálogo.
		// gtx.Constraints.Max.X = gtx.Dp(unit.Dp(300))
		// if gtx.Constraints.Max.X > gtx.Constraints.Min.X {
		// 	gtx.Constraints.Min.X = gtx.Constraints.Max.X
		// }

		return material.Dialog(th, "Confirmar Saída").Layout(gtx,
			material.Inset(theme.PagePadding, layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(material.Body1(th, "Tem certeza que deseja encerrar a sessão e sair da aplicação?").Layout),
				layout.Rigid(layout.Spacer{Height: theme.LargeVSpacer}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					// Botões do diálogo.
					btnNo := material.Button(th, &ml.logoutNoBtn, "Não, Cancelar")
					btnYes := material.Button(th, &ml.logoutYesBtn, "Sim, Sair")
					btnYes.Background = theme.Colors.Danger // Destaca o botão de confirmação.
					btnYes.Color = theme.Colors.DangerText  // Texto branco ou claro para contraste.

					return layout.Flex{Spacing: layout.SpaceBetween}.Layout(gtx,
						layout.Flexed(1, btnNo.Layout),  // Botão "Não"
						layout.Flexed(1, btnYes.Layout), // Botão "Sim"
					)
				}),
			)),
		)
	})
}

// PlaceholderPage é uma página simples para módulos não implementados ou com erro.
type PlaceholderPage struct {
	Title   string
	Message string // Mensagem adicional, se houver
}

func (p *PlaceholderPage) Layout(gtx layout.Context) layout.Dimensions {
	th := material.NewTheme() // Pode obter do router se precisar de um tema consistente.
	return layout.Center.Layout(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(material.H6(th, p.Title).Layout),
			layout.Rigid(func(gtx C) D {
				if p.Message != "" {
					return material.Body1(th, p.Message).Layout(gtx)
				}
				return D{}
			}),
			layout.Rigid(material.Body2(th, "(Módulo em Desenvolvimento ou Erro ao Carregar)").Layout),
		)
	})
}
func (p *PlaceholderPage) OnNavigatedTo(params interface{}) {
	appLogger.Debugf("PlaceholderPage '%s' ativada. Mensagem: %s", p.Title, p.Message)
}
func (p *PlaceholderPage) OnNavigatedFrom() {
	appLogger.Debugf("PlaceholderPage '%s' desativada", p.Title)
}
