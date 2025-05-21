package components

import (
	"image"
	"image/color"
	"time"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"golang.org/x/exp/shiny/materialdesign/icons" // Para ícones de visibilidade

	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui/theme"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/utils"
)

const (
	passwordStrengthBarHeight = 4 // dp
	strengthAnimationDuration = 200 * time.Millisecond
)

// PasswordInput é um widget customizado para entrada de senha com
// ícone de alternância de visibilidade e barra de força opcional.
type PasswordInput struct {
	cfg *core.Config // Para obter PasswordMinLength

	Editor       widget.Editor
	isVisible    bool             // Controla se a senha está visível ou mascarada
	toggleButton widget.Clickable // Botão para alternar a visibilidade
	eyeIcon      *widget.Icon     // Ícone para "senha visível"
	eyeOffIcon   *widget.Icon     // Ícone para "senha mascarada"

	// Para a barra de força da senha
	showStrengthBar  bool        // Controla se a barra de força é exibida
	currentStrength  float32     // Pontuação de força atual (0.0 a 1.0) para largura da barra
	targetStrength   float32     // Pontuação alvo para animação da largura
	currentBarColor  color.NRGBA // Cor atual da barra de força
	targetBarColor   color.NRGBA // Cor alvo para animação da cor
	isAnimatingBar   bool        // True se a barra estiver animando cor/largura
	barAnimStartTime time.Time   // Momento de início da animação da barra

	// Callbacks e Eventos
	// OnChange é chamado sempre que o texto no editor muda.
	OnChange func(text string)
	// OnSubmit é chamado quando o usuário pressiona Enter/Return no editor.
	OnSubmit func(text string)

	isFocused bool // True se o editor de texto estiver em foco
	hint      string
}

// NewPasswordInput cria uma nova instância de PasswordInput.
func NewPasswordInput(th *material.Theme, cfg *core.Config) *PasswordInput {
	if cfg == nil {
		appLogger.Fatalf("Config não pode ser nil para NewPasswordInput")
	}
	pi := &PasswordInput{
		cfg: cfg,
		Editor: widget.Editor{
			SingleLine: true,
			Mask:       '*',  // Começa mascarado por padrão
			Submit:     true, // Para capturar Enter/Return
		},
		isVisible:       false,
		showStrengthBar: true,                // Mostrar barra de força por padrão
		currentBarColor: theme.Colors.Border, // Cor inicial da barra (cinza)
	}

	// Carregar ícones de visibilidade
	var err error
	pi.eyeIcon, err = widget.NewIcon(icons.ActionVisibility)
	if err != nil {
		appLogger.Errorf("Falha ao carregar ícone 'visibility': %v", err)
	}
	pi.eyeOffIcon, err = widget.NewIcon(icons.ActionVisibilityOff)
	if err != nil {
		appLogger.Errorf("Falha ao carregar ícone 'visibility_off': %v", err)
	}

	return pi
}

// SetHint define o texto de dica para o campo de senha.
func (pi *PasswordInput) SetHint(hint string) {
	pi.hint = hint // Armazena para uso no material.Editor
	pi.Editor.Hint = hint
}

// Text retorna o texto atual do editor.
func (pi *PasswordInput) Text() string {
	return pi.Editor.Text()
}

// SetText define o texto do editor e atualiza a barra de força.
func (pi *PasswordInput) SetText(txt string) {
	pi.Editor.SetText(txt)
	if pi.showStrengthBar {
		pi.updateStrengthVisuals(txt, false) // Atualiza a força, sem iniciar animação imediatamente
	}
	if pi.OnChange != nil {
		pi.OnChange(txt)
	}
}

// Clear limpa o texto do editor e reseta a barra de força.
func (pi *PasswordInput) Clear() {
	pi.Editor.SetText("")
	if pi.showStrengthBar {
		pi.updateStrengthVisuals("", false)
	}
	if pi.OnChange != nil {
		pi.OnChange("")
	}
}

// Focus solicita foco para o editor de texto.
func (pi *PasswordInput) Focus() {
	pi.Editor.Focus()
}

// Focused retorna true se o editor estiver em foco.
func (pi *PasswordInput) Focused() bool {
	return pi.Editor.Focused()
}

// ShowStrengthBar define se a barra de força da senha deve ser exibida.
func (pi *PasswordInput) ShowStrengthBar(show bool) {
	pi.showStrengthBar = show
	if !show { // Se esconder, reseta a animação e força para zero visualmente
		pi.isAnimatingBar = false
		pi.currentStrength = 0
	} else { // Se mostrar, recalcula a força baseada no texto atual
		pi.updateStrengthVisuals(pi.Editor.Text(), false)
	}
}

// updateStrengthVisuals calcula a força da senha e prepara a animação da barra.
// `animate` define se a transição deve ser animada ou instantânea.
func (pi *PasswordInput) updateStrengthVisuals(text string, animate bool) {
	if !pi.showStrengthBar {
		return
	}

	var score float32
	var newTargetColor color.NRGBA
	var minLenRequired = 12 // Default se cfg for nil (improvável após NewPasswordInput)
	if pi.cfg != nil {
		minLenRequired = pi.cfg.PasswordMinLength
	}

	validation := utils.ValidatePasswordStrength(text, minLenRequired)

	if text == "" {
		score = 0
		newTargetColor = theme.Colors.Border // Cinza claro para barra vazia
	} else if validation.IsValid {
		// Mapeia pontuação de força baseada na entropia ou critérios.
		// Exemplo simples:
		if validation.Entropy < 40 { // Fraca, apesar de passar nos critérios básicos
			score = 0.35
			newTargetColor = theme.Colors.Danger
		} else if validation.Entropy < 70 { // Média
			score = 0.70
			newTargetColor = theme.Colors.Warning
		} else { // Forte
			score = 1.0
			newTargetColor = theme.Colors.Success
		}
	} else { // Senha inválida (ex: muito curta, não atende critérios)
		// Barra vermelha, com progresso parcial se alguns critérios forem atendidos.
		checksPassed := 0
		if validation.Length {
			checksPassed++
		}
		if validation.Uppercase {
			checksPassed++
		}
		if validation.Lowercase {
			checksPassed++
		}
		if validation.Digit {
			checksPassed++
		}
		if validation.SpecialChar {
			checksPassed++
		}
		// Não considera NotCommonPassword para progresso visual aqui, apenas para IsValid.

		score = float32(checksPassed) * 0.18    // Ajuste o fator para o progresso desejado
		if score > 0.30 && !validation.Length { // Limita se o comprimento ainda for o problema principal
			score = 0.30
		} else if score > 0.80 { // Limita o score máximo para senhas ainda inválidas
			score = 0.80
		}
		newTargetColor = theme.Colors.Danger
	}

	if pi.currentStrength != score || pi.currentBarColor != newTargetColor {
		pi.targetStrength = score
		pi.targetBarColor = newTargetColor
		if animate && !pi.isAnimatingBar { // Só inicia nova animação se não estiver animando ou se for para ser instantâneo
			pi.isAnimatingBar = true
			pi.barAnimStartTime = time.Time{} // Será definido no Layout
		} else if !animate { // Atualização instantânea
			pi.currentStrength = score
			pi.currentBarColor = newTargetColor
			pi.isAnimatingBar = false
		}
	}
}

// Layout desenha o componente PasswordInput.
func (pi *PasswordInput) Layout(gtx layout.Context, th *material.Theme) layout.Dimensions {
	// Processar eventos do editor de texto.
	for _, e := range pi.Editor.Events(gtx) {
		switch ev := e.(type) {
		case widget.ChangeEvent:
			if pi.showStrengthBar {
				pi.updateStrengthVisuals(pi.Editor.Text(), true) // Anima a barra ao digitar
			}
			if pi.OnChange != nil {
				pi.OnChange(pi.Editor.Text())
			}
			op.InvalidateOp{}.Add(gtx.Ops) // Solicita redesenho para barra de força e feedback.
		case widget.SubmitEvent:
			if pi.OnSubmit != nil {
				pi.OnSubmit(ev.Text)
			}
		}
	}

	// Processar clique no botão de alternar visibilidade.
	if pi.toggleButton.Clicked(gtx) {
		pi.isVisible = !pi.isVisible
		if pi.isVisible {
			pi.Editor.Mask = 0 // Sem máscara (senha visível)
		} else {
			pi.Editor.Mask = '*' // Mascarar com asterisco
		}
	}

	// Atualizar estado de foco.
	pi.isFocused = pi.Editor.Focused()

	// Layout principal (vertical: editor + barra de força opcional).
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// Linha 1: Editor e botão de alternar visibilidade.
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					// Desenha borda customizada em volta do editor.
					border := widget.Border{
						Color:        theme.Colors.Border,
						CornerRadius: theme.CornerRadius,
						Width:        theme.BorderWidthDefault,
					}
					if pi.isFocused {
						border.Color = theme.Colors.Primary
						border.Width = unit.Dp(1.5) // Borda mais grossa no foco.
					}

					// Editor de texto.
					inputEditor := material.Editor(th, &pi.Editor, pi.hint) // Usa o hint armazenado
					inputEditor.Font.Weight = font.Normal
					inputEditor.TextSize = unit.Sp(14) // Tamanho de texto padrão.

					return border.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						// Padding interno para o texto do editor.
						// O ícone de toggle é desenhado sobreposto, então o padding direito do texto
						// precisa acomodá-lo.
						return layout.Inset{
							Top: unit.Dp(8), Bottom: unit.Dp(8),
							Left: unit.Dp(10), Right: unit.Dp(36), // Espaço à direita para o ícone.
						}.Layout(gtx, inputEditor.Layout)
					})
				}),
				// Ícone de alternar visibilidade (desenhado "dentro" da área do editor).
				layout.Rigid(
					// Inset negativo para mover o ícone para dentro da borda.
					// O valor exato depende do tamanho do ícone e do padding.
					layout.Inset{Left: unit.Dp(-32), Right: unit.Dp(4)}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							iconToShow := pi.eyeOffIcon
							if pi.isVisible {
								iconToShow = pi.eyeIcon
							}
							if iconToShow == nil { // Fallback se ícones não carregarem.
								return layout.Dimensions{}
							}
							// Usar IconButton para área de clique maior e feedback visual.
							btn := material.IconButton(th, &pi.toggleButton, iconToShow, "Alternar visibilidade da senha")
							btn.Background = color.NRGBA{} // Botão transparente.
							btn.Color = theme.Colors.TextMuted
							btn.Inset = layout.UniformInset(unit.Dp(6)) // Padding do ícone dentro do botão.
							return btn.Layout(gtx)
						},
					),
				),
			)
		}),
		// Linha 2: Barra de Força (se habilitada).
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if pi.showStrengthBar {
				return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, pi.layoutStrengthBar)
			}
			return layout.Dimensions{}
		}),
	)
}

// layoutStrengthBar desenha a barra de força da senha.
func (pi *PasswordInput) layoutStrengthBar(gtx layout.Context) layout.Dimensions {
	if pi.isAnimatingBar {
		if pi.barAnimStartTime.IsZero() { // Inicia animação se ainda não começou.
			pi.barAnimStartTime = gtx.Now
		}
		dt := gtx.Now.Sub(pi.barAnimStartTime)
		progress := float32(dt) / float32(strengthAnimationDuration)

		if progress >= 1.0 {
			progress = 1.0
			pi.isAnimatingBar = false
			pi.barAnimStartTime = time.Time{} // Reseta para a próxima animação.
			pi.currentStrength = pi.targetStrength
			pi.currentBarColor = pi.targetBarColor
		} else {
			// Interpolação linear simples para suavizar a transição da largura da barra.
			pi.currentStrength = pi.targetStrength*progress + pi.currentStrength*(1-progress)

			// Interpolação linear para a cor da barra.
			r1, g1, b1, a1 := pi.currentBarColor.RGBA()
			r2, g2, b2, a2 := pi.targetBarColor.RGBA()
			r := uint8(float32(r1>>8)*(1-progress) + float32(r2>>8)*progress)
			g := uint8(float32(g1>>8)*(1-progress) + float32(g2>>8)*progress)
			b := uint8(float32(b1>>8)*(1-progress) + float32(b2>>8)*progress)
			a := uint8(float32(a1>>8)*(1-progress) + float32(a2>>8)*progress)
			pi.currentBarColor = color.NRGBA{R: r, G: g, B: b, A: a}
		}
		op.InvalidateOp{}.Add(gtx.Ops) // Continua animando.
	}

	barHeightPx := gtx.Dp(passwordStrengthBarHeight)
	totalBarWidthPx := gtx.Constraints.Max.X // Largura total do componente pai (editor).

	// Desenha o fundo da barra (trilha cinza).
	backgroundRect := clip.Rect{Max: image.Pt(totalBarWidthPx, barHeightPx)}
	// Cantos arredondados para o fundo da barra.
	// clip.RRect{Rect: backgroundRect.Rect, SE: barHeightPx / 2, SW: barHeightPx / 2, NW: barHeightPx / 2, NE: barHeightPx / 2}.Add(gtx.Ops)
	// paint.Fill(gtx.Ops, theme.Colors.Grey100)
	// Ou mais simples, um retângulo:
	paint.FillShape(gtx.Ops, theme.Colors.Grey200, backgroundRect.Op())

	// Desenha a barra de progresso da força.
	if pi.currentStrength > 0 {
		progressWidthPx := int(float32(totalBarWidthPx) * pi.currentStrength)
		if progressWidthPx > 0 {
			// Garante que a barra de progresso não exceda a largura total.
			if progressWidthPx > totalBarWidthPx {
				progressWidthPx = totalBarWidthPx
			}

			strengthRect := image.Rect(0, 0, progressWidthPx, barHeightPx)
			// Cantos arredondados para a barra de progresso.
			// Se a barra for muito curta, os raios dos cantos podem ser problemáticos.
			cornerRadiusPx := float32(barHeightPx) / 2.0
			clip.RRect{
				Rect: strengthRect,
				NW:   cornerRadiusPx, NE: cornerRadiusPx,
				SW: cornerRadiusPx, SE: cornerRadiusPx,
			}.Add(gtx.Ops)
			paint.Fill(gtx.Ops, pi.currentBarColor)
		}
	}
	return layout.Dimensions{Size: image.Pt(totalBarWidthPx, barHeightPx)}
}

// AddInputListener permite que o componente pai ouça eventos do Editor.
// Isso é útil para integração com sistemas de formulário mais complexos.
func (pi *PasswordInput) AddInputListener(gtx layout.Context, queue event.Queue) {
	pi.Editor.Add(gtx.Ops, queue)
}
