package theme

import (
	"fmt"
	"image/color"

	"gioui.org/font"
	"gioui.org/unit"
	"gioui.org/widget/material" // Para acesso ao Shaper do tema se necessário
)

// ColorPalette define a paleta de cores da aplicação.
type ColorPalette struct {
	Primary      color.NRGBA
	PrimaryLight color.NRGBA
	PrimaryDark  color.NRGBA
	PrimaryText  color.NRGBA

	White   color.NRGBA
	Grey50  color.NRGBA
	Grey100 color.NRGBA
	Grey200 color.NRGBA
	Grey300 color.NRGBA
	Grey500 color.NRGBA
	Grey600 color.NRGBA
	Grey800 color.NRGBA
	Grey900 color.NRGBA

	Success       color.NRGBA
	SuccessBg     color.NRGBA // Background para alertas de sucesso
	SuccessBorder color.NRGBA
	SuccessText   color.NRGBA // Texto para alertas de sucesso

	Warning       color.NRGBA
	WarningText   color.NRGBA
	WarningBg     color.NRGBA
	WarningBorder color.NRGBA

	Danger       color.NRGBA
	DangerText   color.NRGBA // Texto para alertas de perigo
	DangerBg     color.NRGBA
	DangerBorder color.NRGBA

	Info       color.NRGBA
	InfoText   color.NRGBA
	InfoBg     color.NRGBA
	InfoBorder color.NRGBA

	// Cores semânticas da UI
	Background    color.NRGBA // Fundo principal da janela/páginas
	BackgroundAlt color.NRGBA // Fundo alternativo (ex: linhas de tabela)
	Surface       color.NRGBA // Fundo de elementos como cards, diálogos (geralmente branco)
	Text          color.NRGBA // Cor de texto principal
	TextMuted     color.NRGBA // Cor de texto secundário/sutil
	Border        color.NRGBA // Cor de borda padrão
	FocusRing     color.NRGBA // Cor para indicar foco (Gio lida com foco de forma diferente)
	Shadow        color.NRGBA // Cor para sombras (Gio usa elevação para sombras)

	// Cores específicas de tema (se houver dark/light)
	// Ex: SidebarBackground, ContentBackground
}

// hexToNRGBA converte uma string hexadecimal de cor (ex: "#RRGGBB" ou "#RGB") para color.NRGBA.
func hexToNRGBA(hex string) color.NRGBA {
	var r, g, b uint8
	var count int
	var err error

	if len(hex) == 0 || hex[0] != '#' {
		// Retornar uma cor padrão ou panic se o formato for essencial
		return color.NRGBA{R: 0, G: 0, B: 0, A: 0xFF} // Preto como fallback
	}

	hex = hex[1:] // Remove o '#'

	switch len(hex) {
	case 6: // #RRGGBB
		count, err = fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b)
	case 3: // #RGB
		count, err = fmt.Sscanf(hex, "%1x%1x%1x", &r, &g, &b)
		if count == 3 && err == nil {
			r *= 17 // 0xF * 17 = 0xFF
			g *= 17
			b *= 17
		}
	default:
		return color.NRGBA{R: 0, G: 0, B: 0, A: 0xFF} // Formato inválido
	}

	if err != nil || count != 3 {
		return color.NRGBA{R: 0, G: 0, B: 0, A: 0xFF} // Erro no parsing
	}
	return color.NRGBA{R: r, G: g, B: b, A: 0xFF} // Alpha 0xFF (opaco)
}

// Colors instancia a paleta de cores da aplicação.
var Colors = ColorPalette{
	Primary:      hexToNRGBA("#1A659E"),
	PrimaryLight: hexToNRGBA("#4D8DBC"),
	PrimaryDark:  hexToNRGBA("#0F4C7B"),
	PrimaryText:  hexToNRGBA("#FFFFFF"),

	White:   hexToNRGBA("#FFFFFF"),
	Grey50:  hexToNRGBA("#F8F9FA"),
	Grey100: hexToNRGBA("#F1F3F5"),
	Grey200: hexToNRGBA("#E9ECEF"),
	Grey300: hexToNRGBA("#DEE2E6"),
	Grey500: hexToNRGBA("#ADB5BD"),
	Grey600: hexToNRGBA("#6C757D"),
	Grey800: hexToNRGBA("#343A40"),
	Grey900: hexToNRGBA("#212529"),

	Success:       hexToNRGBA("#198754"),
	SuccessBg:     hexToNRGBA("#D1E7DD"),
	SuccessBorder: hexToNRGBA("#A3CFBB"),
	SuccessText:   hexToNRGBA("#0A3622"), // Texto mais escuro para contraste em fundo claro

	Warning:       hexToNRGBA("#FFC107"),
	WarningText:   hexToNRGBA("#664d03"), // Texto mais escuro para warning
	WarningBg:     hexToNRGBA("#FFF3CD"),
	WarningBorder: hexToNRGBA("#FFE69C"),

	Danger:       hexToNRGBA("#DC3545"),
	DangerText:   hexToNRGBA("#58151C"), // Texto mais escuro para danger
	DangerBg:     hexToNRGBA("#F8D7DA"),
	DangerBorder: hexToNRGBA("#F1AEB5"),

	Info:       hexToNRGBA("#0DCAF0"),
	InfoText:   hexToNRGBA("#055160"),
	InfoBg:     hexToNRGBA("#CFEFFC"),
	InfoBorder: hexToNRGBA("#9EEAF9"),

	Background:    hexToNRGBA("#FFFFFF"), // Fundo principal branco
	BackgroundAlt: hexToNRGBA("#F8F9FA"), // Cinza muito claro para alternância
	Surface:       hexToNRGBA("#FFFFFF"), // Cards, diálogos, etc., geralmente brancos
	Text:          hexToNRGBA("#212529"), // Texto principal escuro
	TextMuted:     hexToNRGBA("#6C757D"), // Texto sutil/secundário
	Border:        hexToNRGBA("#DEE2E6"), // Bordas de inputs, tabelas
	FocusRing:     hexToNRGBA("#86B7FE"), // Usado pelo Bootstrap, pode ser adaptado
	Shadow:        hexToNRGBA("#000000"), // Gio usa elevação, mas a cor pode ser usada para opacidade da sombra
}

// FontCollection define as fontes usadas.
// Gio usa gofont por padrão. Para fontes customizadas, você precisaria registrá-las.
var FontCollection = []font.FontFace{
	// Exemplo de como registrar fontes customizadas (se você as embutir):
	// {
	// 	Font: font.Font{Typeface: "MyCustomFont-Regular"},
	// 	Face: loadMyCustomFontRegular(), // Função que retorna opentype.Face
	// },
	// {
	// 	Font: font.Font{Typeface: "MyCustomFont-Bold", Weight: font.Bold},
	// 	Face: loadMyCustomFontBold(),
	// },
}

// Unidades de Medida Padrão
var (
	// Espaçamento
	TightVSpacer   = unit.Dp(4)
	DefaultVSpacer = unit.Dp(8)
	LargeVSpacer   = unit.Dp(16)
	PagePadding    = unit.Dp(16) // Padding geral para conteúdo de página

	// Tamanhos de Componentes
	ButtonMinHeight = unit.Dp(36)
	InputMinHeight  = unit.Dp(38) // Incluindo padding interno
	ListItemHeight  = unit.Dp(48)

	// Bordas e Elevação
	BorderWidthDefault = unit.Dp(1)
	CornerRadius       = unit.Dp(4)
	ElevationSmall     = unit.Dp(2)
	ElevationMedium    = unit.Dp(4)
	ElevationLarge     = unit.Dp(8)

	// Tamanhos de Janela
	WindowDefaultWidth  = unit.Dp(1024)
	WindowDefaultHeight = unit.Dp(768)
	WindowMinWidth      = unit.Dp(600)
	WindowMinHeight     = unit.Dp(400)
)

// NewTheme cria uma instância customizada de material.Theme.
// Por enquanto, retorna o tema padrão, mas aqui você pode configurar fontes, etc.
func NewAppTheme() *material.Theme {
	// Carrega as fontes padrão do Go se nenhuma customizada for adicionada
	// gofont.Register() // Já chamado em NewAppWindow

	// Cria um novo shaper de texto. Gio usa um por padrão.
	// Se FontCollection estiver preenchido, você pode querer criar um shaper com ele.
	// shaper := text.NewShaper(text.NoSystemFonts(), text.WithCollection(FontCollection))
	// th := material.NewTheme(shaper)

	th := material.NewTheme() // Usa o shaper padrão (gofont)

	// Você pode sobrescrever cores padrão do tema material se desejar,
	// embora seja mais comum usar sua `ColorPalette` diretamente.
	// Ex:
	// th.Fg = Colors.Text
	// th.Bg = Colors.Background
	// th.ContrastFg = Colors.PrimaryText
	// th.ContrastBg = Colors.Primary
	// th.Palette.Fg = Colors.Text
	// th.Palette.Bg = Colors.Background
	// th.Palette.ContrastFg = Colors.PrimaryText
	// th.Palette.ContrastBg = Colors.Primary

	return th
}

// --- Funções Helper para Estilos (Opcional) ---

// PrimaryButton retorna um widget de botão com o estilo primário.
// func PrimaryButton(th *material.Theme, clickable *widget.Clickable, text string) material.ButtonStyle {
// 	btn := material.Button(th, clickable, text)
// 	btn.Background = Colors.Primary
// 	btn.Color = Colors.PrimaryText
// 	btn.CornerRadius = CornerRadius
// 	// btn.Inset = layout.Inset{ ... } // Definir padding padrão
// 	return btn
// }

// Card é um helper para criar um layout de card com padding e sombra (elevação).
// O conteúdo do card é fornecido pela função `contentWidget`.
func Card(th *material.Theme, elevation unit.Dp, inset unit.Dp, contentWidget layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return material.Card(th, Colors.Surface, elevation, layout.UniformInset(inset), contentWidget).Layout(gtx)
	}
}

// LabeledEditor é um helper para um label acima de um editor.
func LabeledEditor(th *material.Theme, label string, editor *widget.Editor, hint string, feedbackText string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		ed := material.Editor(th, editor, hint)
		return layout.Flex{Axis: layout.Vertical, Spacing: layout.Tight}.Layout(gtx,
			layout.Rigid(material.Body2(th, label).Layout),
			layout.Rigid(layout.Spacer{Height: unit.Dp(2)}.Layout),
			layout.Rigid(ed.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if feedbackText != "" {
					lbl := material.Caption(th, feedbackText) // Caption para texto menor
					lbl.Color = Colors.Danger
					return layout.Inset{Top: unit.Dp(2)}.Layout(gtx, lbl.Layout)
				}
				return layout.Dimensions{}
			}),
		)
	}
}
