package components

import (
	"image"
	"image/color"
	"time"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core" // Para Config (PasswordMinLength)
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui/theme" // Para Cores

	// Para Ícones (se usar SVGs)
	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/utils" // Para SecurityValidator
)

const (
	passwordStrengthBarHeight = 4 // dp
	strengthAnimationDuration = 250 * time.Millisecond
)

// PasswordInput é um widget customizado para entrada de senha com barra de força.
type PasswordInput struct {
	cfg *core.Config // Para obter PasswordMinLength

	Editor    widget.Editor
	visible   bool
	toggleBtn widget.Clickable
	// TODO: Implementar ícones reais para o botão de toggle
	// eyeIcon       *icons.IconResource
	// eyeOffIcon    *icons.IconResource

	// Para a barra de força
	strengthScore    float32     // 0.0 (fraca) a 1.0 (forte)
	strengthBarColor color.NRGBA // Cor atual da barra
	targetBarColor   color.NRGBA // Cor alvo para animação
	targetScore      float32     // Score alvo para animação de largura
	animating        bool
	animStartTime    time.Time

	// Sinal (usando um canal para notificar o componente pai)
	// Em Gio, geralmente o estado é puxado pelo pai, ou callbacks são usados.
	// Um canal pode ser usado para eventos como "ReturnPressed".
	ReturnPressed chan bool         // true se return foi pressionado
	TextChanged   func(text string) // Callback opcional para quando o texto muda

	// Foco
	focused bool
}

// NewPasswordInput cria uma nova instância de PasswordInput.
func NewPasswordInput(th *material.Theme, cfg *core.Config) *PasswordInput {
	if cfg == nil {
		appLogger.Fatalf("Config não pode ser nil para NewPasswordInput")
		// Ou retornar um erro, mas para UI component, Fatalf pode ser ok na inicialização
	}
	pi := &PasswordInput{
		cfg: cfg,
		Editor: widget.Editor{
			SingleLine: true,
			Mask:       '*',  // Começa mascarado
			Submit:     true, // Para capturar Enter/Return
		},
		visible:          false,
		strengthBarColor: theme.Colors.Border, // Cor inicial da barra (cinza)
		ReturnPressed:    make(chan bool, 1),  // Canal bufferizado
	}

	// TODO: Carregar ícones
	// pi.eyeIcon = icons.GetIcon(icons.IconTypeEye)
	// pi.eyeOffIcon = icons.GetIcon(icons.IconTypeEyeOff)

	return pi
}

func (pi *PasswordInput) SetHint(hint string) {
	pi.Editor.Hint = hint
}

func (pi *PasswordInput) Text() string {
	return pi.Editor.Text()
}

func (pi *PasswordInput) SetText(txt string) {
	pi.Editor.SetText(txt)
	pi.updateStrength(txt) // Atualiza a força quando o texto é definido programaticamente
	if pi.TextChanged != nil {
		pi.TextChanged(txt)
	}
}

func (pi *PasswordInput) Clear() {
	pi.Editor.SetText("")
	pi.updateStrength("")
	if pi.TextChanged != nil {
		pi.TextChanged("")
	}
}

// Focus solicita foco para o editor de texto.
func (pi *PasswordInput) Focus() {
	pi.Editor.Focus()
}

func (pi *PasswordInput) updateStrength(text string) {
	var score float32
	var targetColor color.NRGBA

	// minLen := pi.cfg.PasswordMinLength // Obter de cfg
	minLen := 12 // Placeholder
	if pi.cfg != nil {
		minLen = pi.cfg.PasswordMinLength
	}

	validation := utils.ValidatePasswordStrength(text, minLen) // utils.SecurityValidator

	if text == "" {
		score = 0
		targetColor = theme.Colors.Border // Cinza claro para barra vazia
	} else if validation.IsValid {
		// Mapear entropia para score (exemplo)
		// Entropia em bits: < 40 (fraca), 40-70 (média), > 70 (forte)
		if validation.Entropy < 40 {
			score = 0.35
			targetColor = theme.Colors.Danger
		} else if validation.Entropy < 70 {
			score = 0.70
			targetColor = theme.Colors.Warning
		} else {
			score = 1.0
			targetColor = theme.Colors.Success
		}
	} else { // Senha inválida, mas não vazia (ex: muito curta)
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

		score = float32(checksPassed) * 0.15 // Um pequeno progresso para cada critério atendido
		if score > 0.30 {
			score = 0.30
		} // Limita o score para senhas ainda inválidas
		targetColor = theme.Colors.Danger
	}

	if pi.strengthScore != score || pi.strengthBarColor != targetColor {
		pi.targetScore = score
		pi.targetBarColor = targetColor
		pi.animating = true
		// pi.animStartTime = // Será definido no Layout se animStartTime for zero
	}
}

func (pi *PasswordInput) Layout(gtx layout.Context, th *material.Theme) layout.Dimensions {
	// Processar eventos do editor
	for _, e := range pi.Editor.Events(gtx) {
		switch ev := e.(type) {
		case widget.ChangeEvent:
			pi.updateStrength(pi.Editor.Text())
			if pi.TextChanged != nil {
				pi.TextChanged(pi.Editor.Text())
			}
			op.InvalidateOp{}.Add(gtx.Ops) // Solicita redesenho para barra de força
		case widget.SubmitEvent:
			// Enviar para o canal ReturnPressed
			// Usar select com default para não bloquear se o canal não estiver sendo lido
			select {
			case pi.ReturnPressed <- true:
			default:
			}
		}
	}

	// Eventos do botão de toggle
	if pi.toggleBtn.Clicked(gtx) {
		pi.visible = !pi.visible
		if pi.visible {
			pi.Editor.Mask = 0 // Sem máscara
		} else {
			pi.Editor.Mask = '*'
		}
	}

	// Atualizar estado de foco
	pi.focused = pi.Editor.Focused()

	// Layout principal (vertical: editor + barra de força)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// Linha 1: Editor e botão de toggle
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					// Desenha borda customizada em volta do editor
					border := widget.Border{
						Color:        theme.Colors.Border,
						CornerRadius: unit.Dp(5),
						Width:        unit.Dp(1),
					}
					if pi.focused {
						border.Color = theme.Colors.Primary
						border.Width = unit.Dp(1.5) // Borda mais grossa no foco
					}

					// Padding interno do editor
					// A altura do editor é controlada pelo tema e tamanho da fonte.
					// Para garantir altura mínima, poderíamos usar layout.ConstrainedBox.
					inputEditor := material.Editor(th, &pi.Editor, pi.Editor.Hint)
					inputEditor.Font.Weight = font.Normal // Ou outro peso
					inputEditor.TextSize = unit.Sp(14)    // Similar ao 10pt Python

					return border.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{
							Top:    unit.Dp(7),
							Bottom: unit.Dp(7),
							Left:   unit.Dp(10),
							Right:  unit.Dp(10), // Espaço para o botão
						}.Layout(gtx, inputEditor.Layout)
					})
				}),
				layout.Rigid(
					layout.Inset{Left: unit.Dp(-30), Right: unit.Dp(5)}.Layout(gtx, // Ajuste Left negativo para sobrepor um pouco
						func(gtx layout.Context) layout.Dimensions {
							// TODO: Usar ícone SVG ou do material.Theme
							// Por agora, um texto simples
							toggleLabel := "👁️"
							if pi.visible {
								toggleLabel = "🙈"
							}
							// IconButton para melhor interação
							btn := material.IconButton(th, &pi.toggleBtn, nil, "Toggle visibility")
							btn.Background = color.NRGBA{} // Transparente
							btn.Color = theme.Colors.TextMuted
							btn.Inset = layout.UniformInset(unit.Dp(2))
							// Se tiver ícones:
							// if pi.visible { btn.Icon = pi.eyeIcon.Resource() } else { btn.Icon = pi.eyeOffIcon.Resource() }
							// Ou material.Icon:
							// if pi.visible { btn.Icon = PularParaIcone(icons.Visibility) } else { btn.Icon = PularParaIcone(icons.VisibilityOff) }

							// Placeholder para o botão
							return material.Body2(th, toggleLabel).Layout(gtx) // Usando texto como placeholder
						},
					),
				),
			)
		}),
		// Linha 2: Barra de Força
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			// Pequeno espaço acima da barra
			return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, pi.layoutStrengthBar)
		}),
	)
}

// layoutStrengthBar desenha a barra de força da senha.
func (pi *PasswordInput) layoutStrengthBar(gtx layout.Context) layout.Dimensions {
	if pi.animating {
		if pi.animStartTime.IsZero() { // Inicia animação
			pi.animStartTime = gtx.Now
		}
		dt := gtx.Now.Sub(pi.animStartTime)
		progress := float32(dt) / float32(strengthAnimationDuration)

		if progress >= 1.0 {
			progress = 1.0
			pi.animating = false
			pi.animStartTime = time.Time{} // Reseta para a próxima animação
			pi.strengthScore = pi.targetScore
			pi.strengthBarColor = pi.targetBarColor
		} else {
			// Interpolação linear simples para cor e score
			pi.strengthScore = pi.targetScore*progress + pi.strengthScore*(1-progress) // Poderia ser mais suave com easing

			// Interpolar cor (R, G, B, A)
			r1, g1, b1, a1 := pi.strengthBarColor.RGBA()
			r2, g2, b2, a2 := pi.targetBarColor.RGBA()

			r := float32(r1>>8)*(1-progress) + float32(r2>>8)*progress
			g := float32(g1>>8)*(1-progress) + float32(g2>>8)*progress
			b := float32(b1>>8)*(1-progress) + float32(b2>>8)*progress
			a := float32(a1>>8)*(1-progress) + float32(a2>>8)*progress
			pi.strengthBarColor = color.NRGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: uint8(a)}
		}
		op.InvalidateOp{}.Add(gtx.Ops) // Continua animando
	}

	barHeight := gtx.Dp(passwordStrengthBarHeight)
	barWidth := gtx.Constraints.Max.X // Largura total do componente pai

	// Desenha o fundo da barra
	bgRect := clip.Rect{Max: image.Pt(barWidth, barHeight)}.Op()
	paint.FillShape(gtx.Ops, theme.Colors.Grey100, bgRect) // Cinza claro para fundo

	// Desenha a barra de progresso da força
	if pi.strengthScore > 0 {
		progressWidth := int(float32(barWidth) * pi.strengthScore)
		if progressWidth > 0 {
			fgRect := clip.RRect{
				Rect: image.Rect(0, 0, progressWidth, barHeight),
				SE:   gtx.Dp(2), SW: gtx.Dp(2), NW: gtx.Dp(2), NE: gtx.Dp(2), // Cantos arredondados
			}.Op(gtx.Ops)
			paint.FillShape(gtx.Ops, pi.strengthBarColor, fgRect)
		}
	}
	return layout.Dimensions{Size: image.Pt(barWidth, barHeight)}
}

// SetMaxLength define o comprimento máximo do texto.
func (pi *PasswordInput) SetMaxLength(length int) {
	// O widget.Editor do Gio não tem um MaxLength direto.
	// Isso precisaria ser tratado na lógica de entrada ou validação.
	appLogger.Warn("SetMaxLength não é diretamente suportado pelo widget.Editor do Gio; use validação.")
}

// SetReadOnly define se o campo é somente leitura.
func (pi *PasswordInput) SetReadOnly(readOnly bool) {
	// O widget.Editor não tem um modo ReadOnly direto.
	// Você pode desabilitar eventos de teclado ou mudar a aparência.
	// Para uma solução simples, podemos apenas impedir a edição.
	// pi.Editor.ReadOnly = readOnly // Se existisse algo assim
	if readOnly {
		pi.Editor.FocusPolicy = 0 // Impede foco
	} else {
		pi.Editor.FocusPolicy = widget.FocusPolicy(key.FocusFilter{})
	}
	// Aparência também precisaria mudar
	appLogger.Warn("SetReadOnly tem implementação limitada para PasswordInput em Gio.")
}

// AddInputListener permite que o componente pai ouça eventos do Editor.
// (Já temos o callback TextChanged e o canal ReturnPressed)
func (pi *PasswordInput) AddInputListener(gtx layout.Context, queue event.Queue) {
	pi.Editor.Add(gtx.Ops, queue)
}
