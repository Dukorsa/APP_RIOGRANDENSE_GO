package ui

import (
	"fmt"
	"image/color"

	// "gioui.org/font/gofont" // Já registrado em AppWindow ou main.go
	// Para font.Weight, font.Style
	// Para text.Shaper
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material" // Para material.Theme e material.Palette
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
)

// ColorPalette define a paleta de cores customizada da aplicação.
// Os nomes são inspirados em paletas comuns (ex: Bootstrap, Material Design).
type ColorPalette struct {
	// Cores Primárias (usadas para branding, ações principais)
	Primary      color.NRGBA // Cor principal da marca.
	PrimaryLight color.NRGBA // Variação mais clara da cor primária.
	PrimaryDark  color.NRGBA // Variação mais escura da cor primária.
	PrimaryText  color.NRGBA // Cor do texto para usar sobre fundos `Primary`.

	// Cores Neutras (tons de cinza e branco)
	White   color.NRGBA // Branco puro.
	Grey50  color.NRGBA // Cinza muito claro (quase branco).
	Grey100 color.NRGBA // Cinza claro.
	Grey200 color.NRGBA // Cinza um pouco mais escuro.
	Grey300 color.NRGBA // Cinza para bordas sutis, divisores.
	// Grey400 color.NRGBA // (Opcional)
	Grey500 color.NRGBA // Cinza médio, para texto secundário ou ícones.
	Grey600 color.NRGBA // Cinza mais escuro, para texto secundário com mais contraste.
	// Grey700 color.NRGBA // (Opcional)
	Grey800 color.NRGBA // Cinza escuro, para texto principal em fundos claros.
	Grey900 color.NRGBA // Cinza muito escuro (quase preto).
	Black   color.NRGBA // Preto puro.

	// Cores Semânticas para Feedback de UI (Alertas, Validação)
	Success       color.NRGBA // Verde para sucesso.
	SuccessText   color.NRGBA // Texto para usar sobre `SuccessBg`.
	SuccessBg     color.NRGBA // Fundo para alertas de sucesso.
	SuccessBorder color.NRGBA // Borda para alertas de sucesso.

	Warning       color.NRGBA // Amarelo/Laranja para avisos.
	WarningText   color.NRGBA
	WarningBg     color.NRGBA
	WarningBorder color.NRGBA

	Danger       color.NRGBA // Vermelho para erros e ações destrutivas.
	DangerText   color.NRGBA
	DangerBg     color.NRGBA
	DangerBorder color.NRGBA

	Info       color.NRGBA // Azul claro para informações.
	InfoText   color.NRGBA
	InfoBg     color.NRGBA
	InfoBorder color.NRGBA

	// Cores Base da UI
	Background    color.NRGBA // Fundo principal da janela/páginas (geralmente claro).
	BackgroundAlt color.NRGBA // Fundo alternativo (ex: para linhas de tabela zebradas).
	Surface       color.NRGBA // Fundo de elementos elevados (cards, diálogos, menus - geralmente branco).
	Text          color.NRGBA // Cor de texto principal (sobre `Background` ou `Surface`).
	TextMuted     color.NRGBA // Cor de texto secundário/sutil (menos importante).
	Border        color.NRGBA // Cor de borda padrão para inputs, tabelas, divisores.
	FocusRing     color.NRGBA // Cor para anel de foco em elementos interativos (Gio lida com foco de forma diferente).
	// Shadow     color.NRGBA // Cor para sombras (Gio usa elevação para sombras, mas a cor pode ser usada para opacidade).
}

// hexToNRGBA converte uma string hexadecimal de cor (ex: "#RRGGBB" ou "#RGB") para color.NRGBA.
// Retorna preto como fallback em caso de erro de parsing.
func hexToNRGBA(hex string) color.NRGBA {
	var r, g, b uint8
	var count int
	var err error

	if len(hex) == 0 || hex[0] != '#' {
		appLogger.Warnf("Formato de cor hexadecimal inválido (deve começar com #): '%s'. Usando preto.", hex)
		return color.NRGBA{R: 0, G: 0, B: 0, A: 0xFF} // Preto como fallback
	}

	hex = hex[1:] // Remove o '#'

	switch len(hex) {
	case 6: // Formato #RRGGBB
		count, err = fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b)
	case 3: // Formato #RGB (abreviado)
		count, err = fmt.Sscanf(hex, "%1x%1x%1x", &r, &g, &b)
		if count == 3 && err == nil {
			r *= 17 // Converte 0xF para 0xFF, 0xA para 0xAA, etc.
			g *= 17
			b *= 17
		}
	default:
		appLogger.Warnf("Comprimento de cor hexadecimal inválido (esperado 3 ou 6 caracteres após #): '%s'. Usando preto.", hex)
		return color.NRGBA{R: 0, G: 0, B: 0, A: 0xFF}
	}

	if err != nil || count != 3 {
		appLogger.Warnf("Erro ao parsear cor hexadecimal '%s' (Scanf count: %d, err: %v). Usando preto.", hex, count, err)
		return color.NRGBA{R: 0, G: 0, B: 0, A: 0xFF}
	}
	return color.NRGBA{R: r, G: g, B: b, A: 0xFF} // Alpha 0xFF (totalmente opaco)
}

// Colors é a instância global da paleta de cores da aplicação.
// Estes valores são usados para estilizar os componentes da UI.
var Colors = ColorPalette{
	// Paleta Azul Primária (Exemplo Riograndense)
	Primary:      hexToNRGBA("#1A659E"), // Azul Principal
	PrimaryLight: hexToNRGBA("#4D8DBC"), // Azul Claro (para hover, destaque sutil)
	PrimaryDark:  hexToNRGBA("#0F4C7B"), // Azul Escuro (para bordas ou variações)
	PrimaryText:  hexToNRGBA("#FFFFFF"), // Texto Branco (para usar sobre fundos primários)

	// Tons de Cinza e Branco
	White:   hexToNRGBA("#FFFFFF"),
	Grey50:  hexToNRGBA("#F8F9FA"), // Fundo muito claro, quase branco
	Grey100: hexToNRGBA("#F1F3F5"), // Fundo alternativo, divisores sutis
	Grey200: hexToNRGBA("#E9ECEF"), // Bordas de input, fundo de cabeçalho de tabela
	Grey300: hexToNRGBA("#DEE2E6"), // Bordas mais visíveis
	Grey500: hexToNRGBA("#ADB5BD"), // Texto sutil, ícones desabilitados
	Grey600: hexToNRGBA("#6C757D"), // Texto secundário
	Grey800: hexToNRGBA("#343A40"), // Texto principal escuro
	Grey900: hexToNRGBA("#212529"), // Texto muito escuro, quase preto
	Black:   hexToNRGBA("#000000"),

	// Cores Semânticas (Estilo Bootstrap)
	Success:       hexToNRGBA("#198754"), // Verde Sucesso
	SuccessText:   hexToNRGBA("#0A3622"), // Texto escuro para contraste em fundo claro de sucesso
	SuccessBg:     hexToNRGBA("#D1E7DD"), // Fundo claro para alertas de sucesso
	SuccessBorder: hexToNRGBA("#A3CFBB"),

	Warning:       hexToNRGBA("#FFC107"), // Amarelo Aviso
	WarningText:   hexToNRGBA("#664D03"),
	WarningBg:     hexToNRGBA("#FFF3CD"),
	WarningBorder: hexToNRGBA("#FFDA6A"), // Ajustado para melhor contraste com WarningBg

	Danger:       hexToNRGBA("#DC3545"), // Vermelho Perigo/Erro
	DangerText:   hexToNRGBA("#58151C"),
	DangerBg:     hexToNRGBA("#F8D7DA"),
	DangerBorder: hexToNRGBA("#F1AEB5"),

	Info:       hexToNRGBA("#0DCAF0"), // Azul Informação
	InfoText:   hexToNRGBA("#055160"),
	InfoBg:     hexToNRGBA("#CFF4FC"), // Ajustado para melhor contraste
	InfoBorder: hexToNRGBA("#9EEAF9"),

	// Cores Base da UI
	Background:    hexToNRGBA("#FFFFFF"), // Fundo principal da aplicação (branco)
	BackgroundAlt: hexToNRGBA("#F8F9FA"), // Fundo alternativo (ex: linhas de tabela zebradas)
	Surface:       hexToNRGBA("#FFFFFF"), // Fundo de cards, diálogos, menus (branco)
	Text:          hexToNRGBA("#212529"), // Cor de texto principal (escuro, sobre fundos claros)
	TextMuted:     hexToNRGBA("#6C757D"), // Texto secundário/sutil
	Border:        hexToNRGBA("#DEE2E6"), // Bordas de inputs, tabelas, divisores
	FocusRing:     hexToNRGBA("#86B7FE"), // Cor para anel de foco (estilo Bootstrap, adaptar para Gio)
}

// Unidades de Medida Padrão para consistência na UI.
var (
	// Espaçamento e Padding
	TightVSpacer   = unit.Dp(4)  // Espaçador vertical pequeno
	DefaultVSpacer = unit.Dp(8)  // Espaçador vertical padrão
	LargeVSpacer   = unit.Dp(16) // Espaçador vertical grande
	PagePadding    = unit.Dp(16) // Padding geral para conteúdo de página

	// Tamanhos de Componentes (alturas mínimas, raios)
	ButtonMinHeight    = unit.Dp(36) // Altura mínima para botões
	InputMinHeight     = unit.Dp(38) // Altura mínima para campos de input (inclui padding interno)
	ListItemHeight     = unit.Dp(48) // Altura padrão para itens de lista
	CornerRadius       = unit.Dp(4)  // Raio de canto padrão para botões, inputs, cards
	BorderWidthDefault = unit.Dp(1)  // Largura de borda padrão

	// Elevação (usada por Cards, Diálogos para simular profundidade/sombra)
	ElevationNone   = unit.Dp(0)
	ElevationSmall  = unit.Dp(2)
	ElevationMedium = unit.Dp(4)
	ElevationLarge  = unit.Dp(8)

	// Dimensões da Janela
	WindowDefaultWidth  = unit.Dp(1024)
	WindowDefaultHeight = unit.Dp(768)
	WindowMinWidth      = unit.Dp(800) // Aumentado para acomodar layouts mais complexos
	WindowMinHeight     = unit.Dp(600)
)

// NewAppTheme cria uma instância customizada de `material.Theme` para a aplicação.
// Configura a paleta de cores do tema e, opcionalmente, fontes.
func NewAppTheme() *material.Theme {
	// `gofont.Register()` já deve ter sido chamado em `main.go` ou `appwindow.go`
	// para registrar as fontes padrão do Go, que o `material.NewTheme()` usará
	// por padrão se nenhum `text.Shaper` customizado for fornecido.

	// Cria um novo tema Material.
	th := material.NewTheme() // Usa o shaper padrão (gofont).

	// Sobrescreve as cores da paleta padrão do tema com as cores da `ColorPalette` customizada.
	// Isso afeta como os widgets `material.*` são renderizados por padrão.
	th.Palette = material.Palette{
		Fg:         Colors.Text,        // Cor de texto principal.
		Bg:         Colors.Background,  // Cor de fundo principal.
		ContrastFg: Colors.PrimaryText, // Cor de texto sobre `ContrastBg` (ex: texto em botões primários).
		ContrastBg: Colors.Primary,     // Cor de fundo para elementos de contraste (ex: botões primários).

		// Cores para estados de erro e desabilitado (opcional, pode ser ajustado).
		// Error:      Colors.Danger,
		// DisabledFg: Colors.TextMuted,
		// DisabledBg: Colors.Grey200,
	}
	// Configurações de fonte padrão para o tema (opcional).
	// th.TextSize = unit.Sp(14) // Tamanho de texto padrão para material.Body1, etc.
	// th.Face = "GoRegular" // Nome da fonte padrão (se usando gofont).

	// Configurações adicionais do tema (ex: para `material.CheckBox`, `material.RadioButton`).
	th.CheckBoxStyle.IconColor = Colors.Primary
	th.RadioButtonStyle.IconColor = Colors.Primary

	// Outras customizações do tema podem ser feitas aqui.
	// Ex: `th.ButtonDefault.Background = ...`, `th.InputStyle.Border.Color = ...`
	// No entanto, é comum aplicar estilos diretamente nos widgets usando a `ColorPalette`
	// para maior controle, como visto em `PrimaryButton` ou `Card` (se fossem implementados).

	return th
}

// --- Funções Helper para Estilos (Opcionais, mas úteis para consistência) ---

// PrimaryButton (Exemplo de como criar um helper para um botão primário estilizado).
// Retorna um `material.ButtonStyle` configurado.
func PrimaryButton(th *material.Theme, clickable *widget.Clickable, text string) material.ButtonStyle {
	btn := material.Button(th, clickable, text)
	btn.Background = Colors.Primary // Cor de fundo primária.
	btn.Color = Colors.PrimaryText  // Cor do texto sobre o fundo primário.
	btn.CornerRadius = CornerRadius // Raio de canto padrão.
	// btn.Inset = layout.UniformInset(unit.Dp(10)) // Padding interno padrão para botões.
	return btn
}

// Card (Exemplo de helper para criar um layout de card com padding e elevação).
// `contentWidget` é a função que desenha o conteúdo interno do card.
func Card(th *material.Theme, elevation unit.Dp, internalPadding unit.Dp, contentWidget layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		// `material.Card` já usa `th.Palette.Surface` por padrão, mas pode ser sobrescrito.
		// Aqui, usamos `theme.Colors.Surface` explicitamente para consistência com a paleta.
		card := material.Card(th, Colors.Surface, elevation)
		// `material.Card` não tem um método para definir padding interno diretamente.
		// O padding é aplicado ao `contentWidget` usando `layout.Inset`.
		return card.Layout(gtx, func(gtx C) D {
			return layout.UniformInset(internalPadding).Layout(gtx, contentWidget)
		})
	}
}

// LabeledInputLayout é um helper para um layout comum de [Label Acima, Input Abaixo, Feedback Abaixo].
// `inputWidgetLayout` é a função que desenha o widget de input (ex: material.Editor, components.PasswordInput).
// `feedbackText` é a mensagem de erro/validação para o input.
// `enabled` pode ser usado para alterar visualmente o label se o input estiver desabilitado.
func LabeledInputLayout(th *material.Theme, labelText string, inputWidgetLayout layout.Widget, feedbackText string, enabled bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		label := material.Body1(th, labelText) // Label para o input.
		if !enabled {
			label.Color = Colors.TextMuted // Cor sutil se o input estiver "desabilitado".
		}

		return layout.Flex{Axis: layout.Vertical, Spacing: layout.Tight}.Layout(gtx,
			layout.Rigid(label.Layout),
			layout.Rigid(layout.Spacer{Height: unit.Dp(2)}.Layout), // Pequeno espaço entre label e input.
			layout.Rigid(inputWidgetLayout),                        // O widget de input.
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { // Feedback de erro abaixo do input.
				if feedbackText != "" {
					feedbackLabel := material.Body2(th, feedbackText) // Texto menor para feedback.
					feedbackLabel.Color = Colors.Danger               // Cor vermelha para erro.
					return layout.Inset{Top: unit.Dp(2)}.Layout(gtx, feedbackLabel.Layout)
				}
				return layout.Dimensions{} // Sem feedback, sem espaço.
			}),
		)
	}
}
