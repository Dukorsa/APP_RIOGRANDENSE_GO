package ui // Alterado de `package icons` para `package ui` para melhor organização se for parte do pacote ui

import (
	// "bytes"    // Não usado diretamente neste exemplo simplificado com Material Icons
	// "embed"    // Usado se embutir arquivos de ícone (PNG/SVG)
	"fmt"
	"image"
	"image/color" // Para `color.NRGBA`

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"          // Para widget.Icon
	"gioui.org/widget/material" // Para material.Theme e material.Icon

	// Pacote de ícones Material Design fornecido pelo Gio (ou um similar).
	// O caminho `golang.org/x/exp/shiny/materialdesign/icons` está obsoleto.
	// É preciso usar um fork ou uma alternativa. Para este exemplo, vamos manter
	// a referência, mas em um projeto real, isso precisaria ser atualizado.
	// Alternativas:
	// - Usar um fork como "github.com/gioui/gio-exp/shiny/materialdesign/icons"
	// - Embutir seus próprios SVGs e renderizá-los.
	// - Usar uma biblioteca de ícones SVG para Gio.
	// Por agora, vamos assumir que `icons` refere-se a um pacote válido.
	"golang.org/x/exp/shiny/materialdesign/icons" // ATENÇÃO: Pacote obsoleto.

	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	// "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui/theme" // Para cores do tema, se necessário
)

// ATENÇÃO: O pacote `golang.org/x/exp/shiny/materialdesign/icons` não é mais mantido
// e pode ter sido removido de dependências recentes do Go.
// Considere usar uma fonte alternativa de ícones Material Design ou embutir seus próprios SVGs.
// Para fins de compilação deste exemplo, os nomes de constantes de `icons` são mantidos.

// IconType define os tipos de ícones usados na aplicação.
// Mapeia para os ícones do Material Design ou para ícones customizados.
type IconType int

const (
	IconNone IconType = iota // Para casos onde nenhum ícone é necessário ou como fallback.

	// Ações Comuns
	IconSearch
	IconAdd
	IconEdit
	IconDelete
	IconRefresh
	IconClose
	IconSettings
	IconLogout // Adicionado para logout

	// Visibilidade e Estado
	IconVisibility
	IconVisibilityOff
	IconCheck // Adicionado para sucesso/selecionado
	IconWarning
	IconError
	IconInfo

	// Navegação e UI
	IconArrowDropDown
	IconArrowDropUp
	IconWindowMinimize // Exemplo de ícone de janela (pode não existir)
	IconMenu           // Ícone de menu Hamburguer

	// Entidades e Funções Específicas
	IconUser         // Usuário, perfil
	IconGroup        // Grupo de usuários, roles
	IconLock         // Senha, bloqueio, segurança
	IconUnlock       // Desbloqueio
	IconCNPJ         // Genérico para documento/empresa se não houver específico
	IconNetwork      // Redes, conexões
	IconFileUpload   // Para importação de arquivos
	IconFileDownload // Para exportação de dados
	IconExcelFile    // Específico para Excel (pode ser genérico de arquivo)
	IconPDFFile      // Específico para PDF
	IconAuditLog     // Para logs de auditoria
	IconRegistration // Para cadastro/registro
	IconEmail        // Para funcionalidade de e-mail

	// Logo (geralmente tratado como imagem, não ícone vetorial)
	// IconLogo // Comentado pois logos são geralmente imagens rasterizadas ou SVGs complexos.
)

// iconCacheMaterial armazena instâncias de `*widget.Icon` já criadas para reutilização.
// Isso evita a recriação desnecessária do widget de ícone.
var iconCacheMaterial = make(map[IconType]*widget.Icon)

// GetMaterialIcon retorna um `*widget.Icon` para o `IconType` especificado.
// Tenta mapear para os ícones Material Design disponíveis no pacote `icons`.
// Retorna um ícone de fallback (ajuda/erro) se o ícone solicitado não for encontrado ou houver erro.
func GetMaterialIcon(iconType IconType) (*widget.Icon, error) {
	if icon, ok := iconCacheMaterial[iconType]; ok && icon != nil {
		return icon, nil
	}

	var iconData []byte // Dados SVG/vetoriais do ícone.
	var err error

	switch iconType {
	// Ações Comuns
	case IconSearch:
		iconData = icons.ActionSearch
	case IconAdd:
		iconData = icons.ContentAdd
	case IconEdit:
		iconData = icons.ImageEdit // Ou icons.ContentCreate para "criar/editar"
	case IconDelete:
		iconData = icons.ActionDelete
	case IconRefresh:
		iconData = icons.NavigationRefresh
	case IconClose:
		iconData = icons.NavigationClose
	case IconSettings:
		iconData = icons.ActionSettings
	case IconLogout:
		iconData = icons.ActionExitToApp // Ícone de sair/logout

	// Visibilidade e Estado
	case IconVisibility:
		iconData = icons.ActionVisibility
	case IconVisibilityOff:
		iconData = icons.ActionVisibilityOff
	case IconCheck:
		iconData = icons.NavigationCheck // Para sucesso ou seleção
	case IconWarning:
		iconData = icons.AlertWarning
	case IconError:
		iconData = icons.AlertError
	case IconInfo:
		iconData = icons.ActionInfo

	// Navegação e UI
	case IconArrowDropDown:
		iconData = icons.NavigationArrowDropDown
	case IconArrowDropUp:
		iconData = icons.NavigationArrowDropUp
	case IconWindowMinimize:
		iconData = icons.NavigationRemove // Subtrair/Minimizar
	case IconMenu:
		iconData = icons.NavigationMenu

	// Entidades e Funções Específicas
	case IconUser:
		iconData = icons.SocialPerson
	case IconGroup:
		iconData = icons.SocialGroup // Para roles/grupos de usuários
	case IconLock:
		iconData = icons.ActionLockOutline // Ou ActionLock para preenchido
	case IconUnlock:
		iconData = icons.ActionLockOpen
	case IconCNPJ:
		iconData = icons.ActionAssignment // Genérico para documento/CNPJ
	case IconNetwork:
		iconData = icons.DeviceNetworkCell // Ou icons.ActionSettingsInputComponent
	case IconFileUpload:
		iconData = icons.FileFileUpload
	case IconFileDownload:
		iconData = icons.FileFileDownload
	case IconExcelFile:
		iconData = icons.EditorInsertDriveFile // Genérico para arquivo
	case IconPDFFile:
		iconData = icons.ImagePictureAsPdf
	case IconAuditLog:
		iconData = icons.ActionReceipt // Para logs/registros
	case IconRegistration:
		iconData = icons.ActionAssignmentInd // Ícone de identidade/registro
	case IconEmail:
		iconData = icons.CommunicationEmail

	case IconNone: // Nenhum ícone
		return nil, nil // Retorna nil para indicar que nenhum ícone deve ser desenhado.
	default:
		appLogger.Warnf("Ícone material não mapeado para IconType: %d. Usando ícone de ajuda como fallback.", iconType)
		iconData = icons.ActionHelp // Ícone de fallback genérico.
	}

	// Cria o widget.Icon a partir dos dados vetoriais.
	iconWidget, err := widget.NewIcon(iconData)
	if err != nil {
		appLogger.Errorf("Erro ao criar widget.Icon para IconType %d: %v. Usando ícone de erro.", iconType, err)
		// Tenta usar um ícone de erro como fallback se a criação falhar.
		errorIconData := icons.AlertError
		iconWidget, err = widget.NewIcon(errorIconData) // Tenta criar o ícone de erro
		if err != nil {
			// Se até o ícone de erro falhar, retorna o erro original e um ícone nil.
			appLogger.Errorf("Falha crítica ao criar ícone de fallback (erro): %v", err)
			return nil, fmt.Errorf("falha ao criar widget.Icon para tipo %d e fallback: %w", iconType, err)
		}
		// Se o ícone de erro foi criado com sucesso, armazena-o no cache para este tipo.
		iconCacheMaterial[iconType] = iconWidget
		return iconWidget, nil // Retorna o ícone de erro, mas sem erro na função GetMaterialIcon.
	}

	iconCacheMaterial[iconType] = iconWidget // Armazena no cache para uso futuro.
	return iconWidget, nil
}

// LayoutMaterialIcon é um helper para desenhar um `*widget.Icon` (Material Icon) com tamanho e cor.
// `th` é o tema atual, `icon` é o widget de ícone obtido de `GetMaterialIcon`.
// `size` é o tamanho desejado em Dp, `iconColor` é a cor do ícone.
func LayoutMaterialIcon(gtx layout.Context, th *material.Theme, icon *widget.Icon, size unit.Dp, iconColor color.NRGBA) layout.Dimensions {
	if icon == nil {
		// Se o ícone for nil (ex: IconNone), retorna dimensões vazias, ocupando o espaço `size` se necessário.
		// Isso pode ser útil para alinhamento se um ícone puder estar ausente.
		// Se não quiser ocupar espaço, retorne `layout.Dimensions{}`.
		placeholderSize := gtx.Dp(size)
		return layout.Dimensions{Size: image.Pt(placeholderSize, placeholderSize)}
	}
	// Cria um `material.Icon` a partir do `widget.Icon` para aplicar estilo do tema.
	iconWidget := material.Icon(th, icon)
	iconWidget.Color = iconColor
	iconWidget.Size = size // Define o tamanho do ícone.
	return iconWidget.Layout(gtx)
}

// GetIcon é uma função wrapper que tenta obter um ícone material.
// Simplifica a chamada para as páginas, lidando com o erro internamente (logando).
// Retorna nil se o ícone não puder ser carregado, permitindo que o layout decida como lidar.
func GetIcon(iconType IconType) *widget.Icon {
	icon, err := GetMaterialIcon(iconType)
	if err != nil {
		// O erro já é logado por GetMaterialIcon.
		// Retorna nil para que o chamador possa decidir não desenhar nada ou usar um placeholder.
		return nil
	}
	return icon
}

// Se você decidir usar SVGs embutidos no futuro:
// 1. `//go:embed assets/icons/custom/*.svg`
//    `var embeddedSVGIconsFS embed.FS`
// 2. Uma função como `GetSVGIconOp(iconName string) (paint.ImageOp, error)`
//    que lê o SVG, rasteriza-o para uma `image.Image` (usando uma biblioteca SVG como `github.com/srwiley/oksvg`),
//    e então cria uma `paint.ImageOp` a partir da imagem rasterizada.
// 3. Um cache para `paint.ImageOp` similar ao `iconCacheImageOp` do exemplo anterior.
// 4. Um helper `LayoutSVGIcon` para desenhar e escalar a `paint.ImageOp`.

// Exemplo de como poderia ser o `LayoutSVGIcon` (semelhante ao `LayoutImageOpIcon` do exemplo anterior):
/*
func LayoutSVGIcon(gtx layout.Context, imgOp paint.ImageOp, size unit.Dp, iconColor color.NRGBA) layout.Dimensions {
	if imgOp.Size().Eq(image.Point{}) { // Imagem vazia
		placeholderSize := gtx.Dp(size)
		return layout.Dimensions{Size: image.Pt(placeholderSize, placeholderSize)}
	}

	// Escalonar a imagem para o tamanho desejado
	imgWidth := imgOp.Size().X
	imgHeight := imgOp.Size().Y
	targetSizePx := gtx.Dp(size)

	var scale float32
	if imgWidth > imgHeight { // Escala pela maior dimensão para caber
		scale = float32(targetSizePx) / float32(imgWidth)
	} else {
		scale = float32(targetSizePx) / float32(imgHeight)
	}

	finalWidth := int(float32(imgWidth) * scale)
	finalHeight := int(float32(imgHeight) * scale)

	// Salva o estado da opacidade e da transformação
	areaStack := clip.Rect{Max: image.Pt(finalWidth, finalHeight)}.Push(gtx.Ops)
	paintStack := paint.ColorOp{Color: iconColor}.Push(gtx.Ops) // Aplica a cor (para SVGs monocromáticos)
	transformStack := op.Affine(f32.Affine2D{}.Scale(f32.Point{}, f32.Pt(scale, scale))).Push(gtx.Ops)

	imgOp.Add(gtx.Ops) // Adiciona a operação de desenho da imagem (já transformada pela stack)

	// Restaura os estados
	transformStack.Pop()
	paintStack.Pop()
	areaStack.Pop()

	return layout.Dimensions{Size: image.Pt(finalWidth, finalHeight)}
}
*/
