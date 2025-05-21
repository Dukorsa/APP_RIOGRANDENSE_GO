package icons

import (
	"bytes"
	"embed"
	"image"
	_ "image/png" // Para decodificar PNGs embutidos

	// _ "image/svg" // Go padrão não decodifica SVG. Precisaria de lib externa.

	"gioui.org/op/paint"
	"gioui.org/widget" // Para widget.Icon (Material Icons)

	// Para ícones Material Design padrão do Gio
	// Você precisará encontrar os equivalentes ou os mais próximos.
	// Os nomes aqui são exemplos e podem não existir.
	"golang.org/x/exp/shiny/materialdesign/icons"

	appLogger "github.com/seu_usuario/riograndense_gio/internal/core/logger"
)

//go:embed assets_img_icons_png/*.png
var embeddedIconsFS embed.FS

const embeddedIconsDir = "assets_img_icons_png" // Supondo que você coloque PNGs aqui

// IconType define os tipos de ícones usados na aplicação,
// similar ao IconType Enum do Python.
type IconType int

const (
	IconNone IconType = iota // Para casos onde nenhum ícone é necessário
	IconSearch
	IconWindow
	IconRegister
	IconExcel
	IconPDF
	IconDelete
	IconEdit
	IconLogo // Logo pode ser uma imagem maior, tratada separadamente
	IconLog
	IconEye
	IconEyeOff
	IconComboBox // Geralmente um triângulo para baixo
	IconAdd
	IconUser
	IconNetwork
	IconCNPJ
	IconLock
	IconRefresh
	IconExport
	IconSettings
	IconEmail
	IconArrowDropDown // Exemplo de ícone comum
	IconArrowDropUp
	IconClose
	IconWarning
	IconInfo
	IconError
	// Adicione outros tipos conforme necessário
)

// iconCache armazena ícones já carregados/processados.
// Para widget.Icon (Material), não há muito o que cachear além do ponteiro.
// Para paint.ImageOp (PNG/SVG), o cache é mais útil.
var (
	iconCacheMaterial = make(map[IconType]*widget.Icon)
	iconCacheImageOp  = make(map[IconType]paint.ImageOp)
	// TODO: Adicionar um lock (sync.Mutex) se for preencher o cache de forma concorrente,
	// mas para UI geralmente é na thread principal.
)

// GetMaterialIcon retorna um *widget.Icon para o IconType especificado.
// Tenta mapear para os ícones Material Design disponíveis em 'golang.org/x/exp/shiny/materialdesign/icons'.
// Você precisará encontrar os equivalentes mais próximos.
func GetMaterialIcon(iconType IconType) (*widget.Icon, error) {
	if icon, ok := iconCacheMaterial[iconType]; ok {
		return icon, nil
	}

	var data []byte
	var err error

	switch iconType {
	case IconSearch:
		data = icons.ActionSearch // Exemplo, nome real pode variar
	case IconWindow: // Pode não ter um direto, usar um genérico
		data = icons.ActionViewModule // Ou similar
	case IconRegister:
		data = icons.ContentCreate // Ou ActionAssignmentInd
	case IconExcel: // Não há ícone de Excel direto, usar um genérico para "arquivo" ou "planilha"
		data = icons.FileFolder // Ou EditorInsertDriveFile
	case IconPDF:
		data = icons.ImagePictureAsPdf
	case IconDelete:
		data = icons.ActionDelete
	case IconEdit:
		data = icons.ImageEdit
	case IconLog:
		data = icons.ActionList // Ou ActionReceipt
	case IconEye:
		data = icons.ActionVisibility
	case IconEyeOff:
		data = icons.ActionVisibilityOff
	case IconComboBox: // Ícone de dropdown
		data = icons.NavigationArrowDropDown
	case IconAdd:
		data = icons.ContentAdd
	case IconUser:
		data = icons.SocialPerson
	case IconNetwork:
		data = icons.DeviceNetworkWifi // Ou ActionSettingsInputComponent, ContentLink
	case IconCNPJ: // Sem ícone específico, usar um genérico de "documento" ou "negócio"
		data = icons.ActionWork // Ou ActionDescription
	case IconLock:
		data = icons.ActionLock
	case IconRefresh:
		data = icons.NavigationRefresh
	case IconExport:
		data = icons.FileFileUpload // Ou ContentSend
	case IconSettings:
		data = icons.ActionSettings
	case IconEmail:
		data = icons.CommunicationEmail
	case IconArrowDropDown:
		data = icons.NavigationArrowDropDown
	case IconArrowDropUp:
		data = icons.NavigationArrowDropUp
	case IconClose:
		data = icons.NavigationClose
	case IconWarning:
		data = icons.AlertWarning
	case IconInfo:
		data = icons.ActionInfo
	case IconError:
		data = icons.AlertError
	case IconLogo: // Logo geralmente é uma imagem customizada, não um widget.Icon
		appLogger.Warn("GetMaterialIcon chamado para IconLogo. Logos devem ser tratados como ImageOp.")
		return nil, fmt.Errorf("IconLogo não é um ícone material padrão")
	default:
		appLogger.Warnf("Ícone material não mapeado para IconType: %d. Usando fallback (ActionHelp).", iconType)
		data = icons.ActionHelp // Um ícone de fallback
	}

	if data == nil { // Se o switch não encontrar um ícone (ex: nome errado)
		appLogger.Warnf("Dados do ícone material não encontrados para IconType: %d. Usando fallback (ActionHelp).", iconType)
		data = icons.ActionHelp
	}

	icon, err := widget.NewIcon(data)
	if err != nil {
		appLogger.Errorf("Erro ao criar widget.Icon para IconType %d: %v", iconType, err)
		return nil, fmt.Errorf("falha ao criar widget.Icon: %w", err)
	}

	iconCacheMaterial[iconType] = icon
	return icon, nil
}

// GetImageOpIcon carrega um ícone PNG embutido e retorna uma paint.ImageOp.
// Útil para ícones customizados que não são do conjunto Material.
// `iconFileName` deve ser o nome do arquivo PNG (ex: "my_custom_icon.png")
// que está em `assets_img_icons_png/`.
func GetImageOpIcon(iconType IconType, iconFileName string) (paint.ImageOp, error) {
	if imgOp, ok := iconCacheImageOp[iconType]; ok {
		return imgOp, nil
	}

	filePath := filepath.Join(embeddedIconsDir, iconFileName)
	fileData, err := embeddedIconsFS.ReadFile(filePath)
	if err != nil {
		appLogger.Errorf("Erro ao ler arquivo de ícone embutido '%s': %v", filePath, err)
		// Tentar carregar um ícone de fallback do material
		fallbackIcon, fallbackErr := GetMaterialIcon(IconError)
		if fallbackErr == nil && fallbackIcon != nil {
			// Isso é um widget.Icon, não ImageOp. Precisaria de um PNG de fallback.
			// Por agora, retorna erro.
			appLogger.Warnf("Ícone PNG '%s' não encontrado, e fallback PNG não implementado.", iconFileName)
		}
		return paint.ImageOp{}, fmt.Errorf("falha ao ler ícone '%s': %w", iconFileName, err)
	}

	img, _, err := image.Decode(bytes.NewReader(fileData))
	if err != nil {
		appLogger.Errorf("Erro ao decodificar imagem do ícone '%s': %v", iconFileName, err)
		return paint.ImageOp{}, fmt.Errorf("falha ao decodificar imagem '%s': %w", iconFileName, err)
	}

	imgOp := paint.NewImageOp(img)
	iconCacheImageOp[iconType] = imgOp
	return imgOp, nil
}

// GetLogoOp retorna o logo da aplicação como uma paint.ImageOp.
// O logo.png deve estar em `assets_img_icons_png/logo.png`.
func GetLogoOp() (paint.ImageOp, error) {
	// O logo pode ser cacheado também se for usado frequentemente em diferentes partes.
	// Por simplicidade, aqui ele é carregado toda vez, mas o ideal seria cachear.
	return GetImageOpIcon(IconLogo, "logo.png") // Supondo que você tenha logo.png
}

// TODO: Implementar Theme-aware icon loading
// Se você tiver diretórios 'light' e 'dark' com PNGs/SVGs, precisaria de lógica
// para escolher o diretório correto com base no tema atual da aplicação.
// func GetThemedImageOpIcon(iconType IconType, iconNameWithoutExt string, currentTheme string) (paint.ImageOp, error) {
//    themeDir := "light"
//    if strings.ToLower(currentTheme) == "dark" {
//        themeDir = "dark"
//    }
//    fileName := fmt.Sprintf("%s.png", iconNameWithoutExt) // Ou .svg
//    filePath := filepath.Join(embeddedIconsDir, themeDir, fileName)
//    // ... lógica de carregamento similar a GetImageOpIcon ...
// }

// Helper para desenhar um widget.Icon (Material Icon)
func LayoutMaterialIcon(gtx layout.Context, th *material.Theme, icon *widget.Icon, size unit.Dp, c color.NRGBA) layout.Dimensions {
	if icon == nil {
		return layout.Dimensions{}
	}
	iconWidget := material.Icon(th, icon)
	iconWidget.Color = c
	iconWidget.Size = size
	return iconWidget.Layout(gtx)
}

// Helper para desenhar uma paint.ImageOp (PNG/SVG renderizado)
func LayoutImageOpIcon(gtx layout.Context, imgOp paint.ImageOp, size unit.Dp) layout.Dimensions {
	if imgOp.Size().Eq(image.Point{}) { // Imagem vazia
		return layout.Dimensions{Size: image.Pt(gtx.Dp(size), gtx.Dp(size))}
	}

	// Escalonar a imagem para o tamanho desejado
	imgWidth := imgOp.Size().X
	imgHeight := imgOp.Size().Y
	targetSizePx := gtx.Dp(size)

	var scale float32
	if imgWidth > imgHeight {
		scale = float32(targetSizePx) / float32(imgWidth)
	} else {
		scale = float32(targetSizePx) / float32(imgHeight)
	}

	finalWidth := int(float32(imgWidth) * scale)
	finalHeight := int(float32(imgHeight) * scale)

	// Desenhar a imagem
	macro := op.Record(gtx.Ops)
	imgOp.Add(gtx.Ops)
	call := macro.Stop()

	defer op.Affine(f32.Affine2D{}.Scale(f32.Point{}, f32.Pt(scale, scale))).Push(gtx.Ops).Pop()
	call.Add(gtx.Ops)

	return layout.Dimensions{Size: image.Pt(finalWidth, finalHeight)}
}
