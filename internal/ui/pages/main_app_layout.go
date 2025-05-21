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
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui"

	// "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui/components" // Se MainAppLayout tivesse seu próprio spinner
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui"
)

// ModuleConfig define a configuração para cada módulo/item na sidebar.
type ModuleConfig struct {
	ID                 ui.PageID
	Title              string
	Icon               *widget.Icon
	RequiredPermission auth.Permission
}

// MainAppLayout é a página principal da aplicação após o login.
type MainAppLayout struct {
	router *ui.Router
	cfg    *core.Config

	userService    services.UserService
	roleService    services.RoleService
	networkService services.NetworkService
	cnpjService    services.CNPJService
	importService  services.ImportService
	auditService   services.AuditLogService
	permManager    *auth.PermissionManager
	sessionManager *auth.SessionManager

	currentModuleID ui.PageID
	sidebarModules  []ModuleConfig
	sidebarClicks   []widget.Clickable
	logoutBtn       widget.Clickable

	modulePages map[ui.PageID]ui.Page

	showLogoutConfirm bool
	logoutYesBtn      widget.Clickable
	logoutNoBtn       widget.Clickable

	currentUserData *models.UserPublic
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
		auditService:   router.AuditLogService(),
		permManager:    permMan,
		sessionManager: sessMan,
		modulePages:    make(map[ui.PageID]ui.Page),
	}

	ml.modulePages[ui.PageCNPJ] = NewCNPJPage(ml.router, ml.cfg, ml.cnpjService, ml.networkService, ml.permManager, ml.sessionManager)
	ml.modulePages[ui.PageAdminPermissions] = NewAdminPermissionsPage(ml.router, ml.cfg, ml.userService, ml.roleService, ml.auditService, ml.permManager, ml.sessionManager)
	ml.modulePages[ui.PageRoleManagement] = NewRoleManagementPage(ml.router, ml.cfg, ml.roleService, ml.auditService, ml.permManager, ml.sessionManager)
	ml.modulePages[ui.PageImport] = NewImportPage(ml.router, ml.cfg, ml.importService, ml.permManager, ml.sessionManager)
	ml.modulePages[ui.PageNetworks] = &PlaceholderPage{Title: "Gerenciamento de Redes"}

	return ml
}

func (ml *MainAppLayout) loadSidebarModules() {
	session := ml.currentUserData
	if session == nil {
		appLogger.Error("MainAppLayout: Dados do usuário logado não disponíveis ao carregar módulos da sidebar. Forçando logout.")
		ml.router.GetAppWindow().HandleLogout()
		return
	}
	simulatedSessionData := &auth.SessionData{
		UserID:   session.ID,
		Username: session.Username,
		Roles:    session.Roles,
	}

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
			appLogger.Errorf("Usuário '%s' não tem acesso a nenhum módulo configurado na sidebar.", session.Username)
			ml.currentModuleID = ui.PageNone
			ml.router.GetAppWindow().ShowGlobalMessage("Acesso Negado", "Você não tem permissão para acessar nenhum módulo.", true, 0)
		}
	}
}

func (ml *MainAppLayout) OnNavigatedTo(params interface{}) {
	appLogger.Info("Navegou para MainAppLayout")
	ml.showLogoutConfirm = false

	userData, ok := params.(*models.UserPublic)
	if !ok || userData == nil {
		appLogger.Error("MainAppLayout: Parâmetros inválidos ou nulos recebidos em OnNavigatedTo. Forçando logout.")
		ml.router.GetAppWindow().HandleLogout()
		return
	}
	ml.currentUserData = userData
	ml.loadSidebarModules()

	currentPage := ml.getModulePage(ml.currentModuleID)
	if currentPage == nil && ml.currentModuleID != ui.PageNone {
		appLogger.Errorf("Falha ao carregar o módulo ID %v em MainAppLayout.", ml.currentModuleID)
	}
}

func (ml *MainAppLayout) OnNavigatedFrom() {
	appLogger.Info("Navegando para fora da MainAppLayout")
	if page, ok := ml.modulePages[ml.currentModuleID]; ok && page != nil {
		page.OnNavigatedFrom()
	}
	ml.currentUserData = nil
}

func (ml *MainAppLayout) getModulePage(moduleID ui.PageID) ui.Page {
	if moduleID == ui.PageNone {
		return &PlaceholderPage{Title: "Nenhum Módulo Selecionado"}
	}

	page, exists := ml.modulePages[moduleID]
	if !exists || page == nil {
		appLogger.Warnf("Página para módulo ID %v não pré-instanciada ou nula. Usando placeholder.", moduleID)
		title := fmt.Sprintf("Módulo %v", moduleID)
		for _, modCfg := range ml.sidebarModules {
			if modCfg.ID == moduleID {
				title = modCfg.Title
				break
			}
		}
		return &PlaceholderPage{Title: fmt.Sprintf("%s (Não Implementado/Erro)", title)}
	}
	return page
}

func (ml *MainAppLayout) Layout(gtx layout.Context) layout.Dimensions {
	th := ml.router.GetAppWindow().Theme()

	for i := range ml.sidebarModules {
		if ml.sidebarClicks[i].Clicked(gtx) {
			selectedModuleID := ml.sidebarModules[i].ID
			if ml.currentModuleID != selectedModuleID {
				if oldPage, ok := ml.modulePages[ml.currentModuleID]; ok && oldPage != nil {
					oldPage.OnNavigatedFrom()
				}
				ml.currentModuleID = selectedModuleID
				appLogger.Infof("Módulo da MainAppLayout alterado para: %s (ID: %v)", ml.sidebarModules[i].Title, selectedModuleID)
				if newPage, ok := ml.modulePages[ml.currentModuleID]; ok && newPage != nil {
					newPage.OnNavigatedTo(nil)
				} else {
					appLogger.Errorf("Tentativa de navegar para módulo ID %v que não tem página associada.", ml.currentModuleID)
				}
			}
		}
	}
	if ml.logoutBtn.Clicked(gtx) {
		ml.showLogoutConfirm = true
	}

	mainContentLayout := layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ml.layoutSidebar(gtx, th)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lineWidth := unit.Dp(1)
			lineColor := theme.Colors.Border
			rect := clip.Rect{Max: image.Pt(gtx.Dp(lineWidth), gtx.Constraints.Max.Y)}.Op()
			paint.FillShape(gtx.Ops, lineColor, rect)
			return layout.Dimensions{Size: image.Pt(gtx.Dp(lineWidth), gtx.Constraints.Max.Y)}
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(0)}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					currentPageToLayout := ml.getModulePage(ml.currentModuleID)
					if currentPageToLayout == nil {
						errMsg := fmt.Sprintf("Erro: Módulo ID %v não pôde ser carregado.", ml.currentModuleID)
						appLogger.Error(errMsg)
						return layout.Center.Layout(gtx, material.Body1(th, errMsg).Layout)
					}
					return layout.UniformInset(theme.PagePadding).Layout(gtx, currentPageToLayout.Layout)
				})
		}),
	)

	if ml.showLogoutConfirm {
		dialogLayout := ml.layoutLogoutConfirmDialog(gtx, th)
		return layout.Stack{}.Layout(gtx,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions { return mainContentLayout }),
			layout.Expanded(func(gtx layout.Context) layout.Dimensions { return dialogLayout }),
		)
	}

	return mainContentLayout
}

func (ml *MainAppLayout) layoutSidebar(gtx layout.Context, th *material.Theme) layout.Dimensions {
	sidebarWidth := gtx.Dp(unit.Dp(230))
	gtx.Constraints.Min.X = sidebarWidth
	gtx.Constraints.Max.X = sidebarWidth

	return layout.Background{Color: theme.Colors.Grey50}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(8)).Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceEnd}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if ml.currentUserData == nil {
								return layout.Dimensions{}
							}
							rolesStr := "N/A"
							if len(ml.currentUserData.Roles) > 0 {
								rolesStr = strings.Join(ml.currentUserData.Roles, ", ")
							}
							return material.Card(th, theme.Colors.Surface, theme.ElevationSmall, layout.UniformInset(unit.Dp(8)),
								func(gtx layout.Context) layout.Dimensions {
									return layout.Flex{Axis: layout.Vertical, Spacing: layout.Tight}.Layout(gtx,
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											lbl := material.Body1(th, "Usuário Logado:")
											lbl.Font.Weight = font.Bold
											return lbl.Layout(gtx)
										}),
										layout.Rigid(material.Body2(th, ml.currentUserData.Username).Layout),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											lbl := material.Caption(th, fmt.Sprintf("Perfis: %s", rolesStr))
											lbl.Color = theme.Colors.TextMuted
											return lbl.Layout(gtx)
										}),
									)
								}).Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Height: theme.LargeVSpacer}.Layout),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							list := layout.List{Axis: layout.Vertical}
							return list.Layout(gtx, len(ml.sidebarModules), func(gtx layout.Context, index int) layout.Dimensions {
								modCfg := ml.sidebarModules[index]
								btn := material.Button(th, &ml.sidebarClicks[index], "")
								btn.Background = color.NRGBA{}
								btn.Color = theme.Colors.Text
								btn.CornerRadius = theme.CornerRadius
								btn.Inset = layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(12), Right: unit.Dp(12)}

								contentColor := theme.Colors.Text
								if ml.currentModuleID == modCfg.ID {
									btn.Background = theme.Colors.PrimaryLight
									contentColor = theme.Colors.PrimaryText
								}

								buttonContent := layout.Flex{Alignment: layout.Middle}
								iconWidgetLayout := layout.Dimensions{}
								if modCfg.Icon != nil {
									icon := material.Icon(th, modCfg.Icon)
									icon.Color = contentColor
									iconWidgetLayout = layout.Inset{Right: unit.Dp(8)}.Layout(gtx, icon.Layout)
								}

								titleLabel := material.Body1(th, modCfg.Title)
								titleLabel.Color = contentColor
								if ml.currentModuleID == modCfg.ID {
									titleLabel.Font.Weight = font.Bold
								}

								return layout.Inset{Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return btn.Layout(gtx,
										buttonContent.Layout(gtx,
											layout.Rigid(iconWidgetLayout.Layout), // Correção: .Layout aqui
											layout.Rigid(titleLabel.Layout),
										)...,
									)
								})
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							logoutButton := material.Button(th, &ml.logoutBtn, "")
							logoutButton.Background = theme.Colors.DangerBg
							logoutButton.Color = theme.Colors.DangerText
							logoutButton.CornerRadius = theme.CornerRadius
							logoutButton.Inset = layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(12), Right: unit.Dp(12)}

							logoutIconWidget, _ := widget.NewIcon(icons.ActionExitToApp)
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

func (ml *MainAppLayout) layoutLogoutConfirmDialog(gtx layout.Context, th *material.Theme) layout.Dimensions {
	if ml.logoutYesBtn.Clicked(gtx) {
		ml.showLogoutConfirm = false
		ml.router.GetAppWindow().HandleLogout()
	}
	if ml.logoutNoBtn.Clicked(gtx) {
		ml.showLogoutConfirm = false
	}

	paint.FillShape(gtx.Ops, color.NRGBA{A: 180}, clip.Rect{Max: gtx.Constraints.Max}.Op())

	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return material.Dialog(th, "Confirmar Saída").Layout(gtx,
			material.Inset(theme.PagePadding, layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(material.Body1(th, "Tem certeza que deseja encerrar a sessão e sair da aplicação?").Layout),
				layout.Rigid(layout.Spacer{Height: theme.LargeVSpacer}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					btnNo := material.Button(th, &ml.logoutNoBtn, "Não, Cancelar")
					btnYes := material.Button(th, &ml.logoutYesBtn, "Sim, Sair")
					btnYes.Background = theme.Colors.Danger
					btnYes.Color = theme.Colors.DangerText

					return layout.Flex{Spacing: layout.SpaceBetween}.Layout(gtx,
						layout.Flexed(1, btnNo.Layout),
						layout.Flexed(1, btnYes.Layout),
					)
				}),
			)),
		)
	})
}

// PlaceholderPage é uma página simples para módulos não implementados ou com erro.
type PlaceholderPage struct {
	Title   string
	Message string
}

func (p *PlaceholderPage) Layout(gtx layout.Context) layout.Dimensions {
	th := material.NewTheme()
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(material.H6(th, p.Title).Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if p.Message != "" {
					return material.Body1(th, p.Message).Layout(gtx)
				}
				return layout.Dimensions{}
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
