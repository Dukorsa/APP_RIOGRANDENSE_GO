package components

import (
	"image"
	"image/color"
	"math"
	"time"

	"gioui.org/f32" // Para pontos flutuantes em coordenadas
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui/theme" // Para cores padrão
	// appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger" // Opcional, para debug
)

const (
	defaultSpinnerSize        = 50 // dp
	defaultSpinnerSpeedMs     = 40
	defaultSpinnerNumSegments = 8
	defaultSpinnerSegWidth    = 6   // dp
	defaultSpinnerSegLength   = 0.6 // % do raio
	fadeDuration              = 250 * time.Millisecond
)

type LoadingSpinner struct {
	IsActive bool // Controla se a animação deve rodar

	// Configurações
	Color         color.NRGBA
	Size          unit.Dp // Tamanho do spinner (diâmetro)
	Speed         time.Duration
	TrailOpacity  bool
	NumSegments   int
	SegmentWidth  unit.Dp
	SegmentLength float32 // Proporção do raio para o comprimento do segmento

	// Estado interno da animação
	angle          float32 // Em graus
	visibleOpacity float32 // 0.0 (invisível) a 1.0 (totalmente visível)

	// Para animação de fade
	fading        bool
	fadeStartTime time.Time
	fadeTarget    float32 // 0.0 ou 1.0

	// Para animação de rotação
	lastFrameTime time.Time
	ticker        *time.Ticker  // Para animação contínua mesmo sem eventos da UI
	stopTicker    chan struct{} // Para parar o ticker
}

// NewLoadingSpinner cria um novo spinner com configurações padrão ou customizadas.
func NewLoadingSpinner(spinnerColor ...color.NRGBA) *LoadingSpinner {
	c := theme.Colors.Primary // Cor padrão
	if len(spinnerColor) > 0 {
		c = spinnerColor[0]
	}

	s := &LoadingSpinner{
		IsActive: false,

		Color:         c,
		Size:          unit.Dp(defaultSpinnerSize),
		Speed:         defaultSpinnerSpeedMs * time.Millisecond,
		TrailOpacity:  true,
		NumSegments:   defaultSpinnerNumSegments,
		SegmentWidth:  unit.Dp(defaultSpinnerSegWidth),
		SegmentLength: defaultSpinnerSegLength,

		angle:          0,
		visibleOpacity: 0.0, // Começa invisível
		stopTicker:     make(chan struct{}),
	}
	return s
}

// Start ativa o spinner e inicia a animação de fade-in.
func (s *LoadingSpinner) Start(gtx layout.Context) {
	if s.IsActive {
		return
	}
	s.IsActive = true
	s.fading = true
	s.fadeStartTime = gtx.Now
	s.fadeTarget = 1.0 // Fade in

	// Inicia ticker para rotação contínua
	if s.ticker == nil {
		s.ticker = time.NewTicker(s.Speed) // Usa a velocidade configurada
		s.lastFrameTime = gtx.Now
		go s.runRotation(gtx)
	}
	op.InvalidateOp{}.Add(gtx.Ops) // Solicita novo frame
}

// Stop desativa o spinner e inicia a animação de fade-out.
func (s *LoadingSpinner) Stop(gtx layout.Context) {
	if !s.IsActive && !s.fading { // Se já está inativo e não está fazendo fade out
		return
	}
	s.IsActive = false // Indica que não deve mais rodar ativamente
	s.fading = true
	s.fadeStartTime = gtx.Now
	s.fadeTarget = 0.0 // Fade out

	if s.ticker != nil {
		// O ticker será parado pela goroutine runRotation quando s.IsActive for false
		// ou quando s.stopTicker for fechado.
	}
	op.InvalidateOp{}.Add(gtx.Ops)
}

// SetVisibility controla diretamente a visibilidade com fade.
func (s *LoadingSpinner) SetVisibility(gtx layout.Context, visible bool) {
	if visible {
		s.Start(gtx)
	} else {
		s.Stop(gtx)
	}
}

// runRotation é uma goroutine que atualiza o ângulo de rotação.
func (s *LoadingSpinner) runRotation(initialGtx layout.Context) {
	// appLogger.Debug("Goroutine de rotação do spinner iniciada.")
	defer func() {
		if s.ticker != nil {
			s.ticker.Stop()
			s.ticker = nil
		}
		// appLogger.Debug("Goroutine de rotação do spinner finalizada.")
	}()

	for {
		select {
		case <-s.stopTicker: // Canal para forçar parada
			return
		case tickTime := <-s.ticker.C:
			if !s.IsActive && s.visibleOpacity < 0.01 { // Se parou e já está invisível
				return // Finaliza a goroutine
			}

			// Atualiza ângulo e solicita novo frame para a UI principal
			// A atualização do ângulo é baseada no tempo real para suavidade
			// Esta é uma forma de fazer. A outra é um incremento fixo por tick.
			// delta := tickTime.Sub(s.lastFrameTime)
			// s.angle += float32(delta.Seconds() * 360 / (float64(s.NumSegments) * s.Speed.Seconds() / 20)) // Ajuste a velocidade

			// Incremento fixo por tick (mais simples)
			s.angle = float32(math.Mod(float64(s.angle)+float64(360.0/float32(s.NumSegments)/2.0), 360.0)) // Similar ao Python
			// s.angle = float32(math.Mod(float64(s.angle) + 12, 360.0)) // Ou um valor fixo

			s.lastFrameTime = tickTime

			// Solicita um novo frame na thread da UI
			// Isso é feito chamando Invalidate na janela ou contexto da UI
			// O AppWindow precisaria expor um método para isso ou passar um canal.
			// Por agora, o Layout chamará InvalidateOp se s.isActive ou s.fading.
			// Se a janela principal puder ser acessada (não ideal diretamente daqui):
			// if globalAppWindow != nil { globalAppWindow.Invalidate() }
		}
	}
}

// Layout desenha o spinner.
func (s *LoadingSpinner) Layout(gtx layout.Context) layout.Dimensions {
	if s.fading {
		progress := float32(gtx.Now.Sub(s.fadeStartTime)) / float32(fadeDuration)
		if progress >= 1.0 {
			progress = 1.0
			s.fading = false // Terminou o fade
			if s.fadeTarget == 0.0 {
				s.IsActive = false // Garante que está inativo após fade out
			}
		}
		if s.fadeTarget == 1.0 { // Fade in
			s.visibleOpacity = progress
		} else { // Fade out
			s.visibleOpacity = 1.0 - progress
		}
		op.InvalidateOp{}.Add(gtx.Ops) // Continua animando o fade
	}

	// Se não está ativo e não está fazendo fade out, não desenha
	if !s.IsActive && !s.fading && s.visibleOpacity < 0.01 {
		return layout.Dimensions{Size: image.Pt(gtx.Dp(s.Size), gtx.Dp(s.Size))} // Ocupa espaço mas não desenha
	}

	// Se estiver ativo ou fazendo fade, solicita redesenho contínuo
	if s.IsActive || s.fading {
		op.InvalidateOp{}.Add(gtx.Ops)
	}

	// Desenho do spinner
	sz := gtx.Dp(s.Size)
	defer op.Save(gtx.Ops).Load()
	clip.Rect{Max: image.Pt(sz, sz)}.Add(gtx.Ops) // Define a área de desenho

	center := f32.Pt(float32(sz)/2, float32(sz)/2)
	padding := float32(gtx.Dp(s.SegmentWidth))/2 + 1 // Metade da largura + pequeno extra
	effectiveRadius := center.X - padding
	if effectiveRadius <= 1 {
		return layout.Dimensions{Size: image.Pt(sz, sz)}
	}

	transform := op.Affine(f32.Affine2D{}.Offset(center))
	defer transform.Push(gtx.Ops).Pop()

	segmentStepAngle := 2.0 * math.Pi / float32(s.NumSegments) // Em radianos

	for i := 0; i < s.NumSegments; i++ {
		currentAngleRad := (s.angle * math.Pi / 180.0) + (float32(i) * segmentStepAngle)

		alpha := uint8(255)
		if s.TrailOpacity {
			// Cálculo de opacidade não linear para trail mais acentuado
			// O (1.0 - ...) faz o primeiro segmento (i=0, após rotação) ser o mais opaco
			opacityFactor := math.Pow(1.0-(float64(i)/float64(s.NumSegments)), 1.5)
			alpha = uint8(math.Max(5, 255*opacityFactor))
		}

		// Aplicar a opacidade geral do fade
		finalAlpha := uint8(float32(alpha) * s.visibleOpacity)
		if finalAlpha < 5 && s.visibleOpacity > 0 { // Garante um mínimo se visível
			finalAlpha = 5
		}

		segColor := s.Color
		segColor.A = finalAlpha // Define a opacidade do segmento

		// Posição inicial e final do segmento
		startRadius := effectiveRadius * (1.0 - s.SegmentLength)

		// Rotacionar pontos
		startX := startRadius * float32(math.Cos(float64(currentAngleRad)))
		startY := startRadius * float32(math.Sin(float64(currentAngleRad)))
		endX := effectiveRadius * float32(math.Cos(float64(currentAngleRad)))
		endY := effectiveRadius * float32(math.Sin(float64(currentAngleRad)))

		var path clip.Path
		path.Begin(gtx.Ops)
		path.MoveTo(f32.Pt(startX, startY))
		path.LineTo(f32.Pt(endX, endY))

		// Desenhar a linha com espessura e extremidades arredondadas
		paint.StrokeShape(gtx.Ops, segColor, clip.Stroke{
			Path:  path.End(),
			Width: float32(gtx.Dp(s.SegmentWidth)),
			Cap:   clip.RoundCap,
		}.Op())
	}

	return layout.Dimensions{Size: image.Pt(sz, sz)}
}

// Destroy deve ser chamado quando o spinner não for mais necessário para parar a goroutine.
func (s *LoadingSpinner) Destroy() {
	if s.ticker != nil {
		// Tenta fechar o canal se não estiver já fechado (seguro com select)
		select {
		case <-s.stopTicker: // Já fechado
		default:
			close(s.stopTicker)
		}
	}
}
