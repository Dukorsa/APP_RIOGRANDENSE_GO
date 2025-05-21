package navigation

import (
	"time" // Para AppWindowInterface.ShowGlobalMessage

	"gioui.org/layout"
	"gioui.org/widget/material"
	configCore "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/config"
)

// PageID define um identificador único para cada página/view da aplicação.
type PageID int

const (
	PageNone PageID = iota
	PageLogin
	PageRegistration
	PageForgotPassword
	PageMain
	PageNetworks
	PageCNPJ
	PageAdminPermissions
	PageRoleManagement
	PageImport
)

// Page define a interface que cada página/view da aplicação deve implementar.
type Page interface {
	Layout(gtx layout.Context) layout.Dimensions
	OnNavigatedTo(params interface{})
	OnNavigatedFrom()
}

// AppWindowInterface define a interface mínima que o Router precisa da AppWindow.
// As páginas podem obter esta interface através do Router.
type AppWindowInterface interface {
	Invalidate()
	Execute(f func())
	ShowGlobalMessage(title, message string, isError bool, autoHideDuration time.Duration)
	// GetTheme() *material.Theme // Se as páginas precisarem e não importarem "theme" diretamente.
	// GetConfig() *core.Config
    Context() layout.Context // Adicionado para compatibilidade com o spinner atual
}

// RouterInterface define a interface que as páginas usarão para interagir com o router.
type RouterInterface interface {
	NavigateTo(id PageID, params interface{})
	NavigateBack(params interface{}) bool
	CurrentPageID() PageID
	GetAppWindow() AppWindowInterface
	GetTheme() *material.Theme // Para acesso ao tema global
	GetConfig() *configCore.Config   // Para acesso à configuração global
	// Adicione getters para serviços se as páginas os acessarem via router
	// Ex: UserService() services.UserService
}
