package components

import (
	"image"
	"image/color"
	"math"
	"time"

	"gioui.org/f32"    // Para pontos flutuantes em coordenadas
	"gioui.org/layout" // Para layout
	"gioui.org/op"     // Para operações de desenho
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit" // Para unidades de display (dp, sp)

	"github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/ui/theme" // Para cores padrão
	// appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger" // Opcional, para debug
)

const (
	defaultSpinnerSize        = 50                    // dp
	defaultSpinnerTickRate    = 30 * time.Millisecond // Taxa de atualização da animação
	defaultSpinnerNumSegments = 8
	defaultSpinnerSegWidth    = 6   // dp
	defaultSpinnerSegLength   = 0.6 // Proporção do raio para o comprimento do segmento
	fadeDuration              = 250 * time.Millisecond
)

// LoadingSpinner é um widget que exibe uma animação de carregamento.
type LoadingSpinner struct {
	isActive bool // Controla se a animação de rotação deve rodar

	// Configurações do Spinner
	Color         color.NRGBA // Cor principal dos segmentos
	Size          unit.Dp     // Diâmetro do spinner
	NumSegments   int         // Número de segmentos no spinner
	SegmentWidth  unit.Dp     // Largura (espessura) de cada segmento
	SegmentLength float32     // Comprimento do segmento como proporção do raio

	// Estado interno da animação
	currentAngle   float32 // Ângulo de rotação atual em graus
	visibleOpacity float32 // Opacidade atual do spinner (0.0 a 1.0) para fade

	// Para animação de fade in/out
	isFading      bool      // True se estiver atualmente em transição de fade
	fadeStartTime time.Time // Momento em que o fade atual começou
	fadeTarget    float32   // Opacidade alvo do fade (0.0 para fade-out, 1.0 para fade-in)

	// Para animação de rotação contínua
	ticker     *time.Ticker  // Ticker para a animação de rotação
	stopTicker chan struct{} // Canal para sinalizar a parada da goroutine do ticker
	// lastFrameTime time.Time // Não é mais necessário se o incremento do ângulo for fixo
}

// NewLoadingSpinner cria um novo spinner com configurações padrão ou customizadas.
// `spinnerColor` é opcional; se fornecido, usa a primeira cor. Caso contrário, usa a cor primária do tema.
func NewLoadingSpinner(spinnerColor ...color.NRGBA) *LoadingSpinner {
	c := theme.Colors.Primary // Cor padrão
	if len(spinnerColor) > 0 {
		c = spinnerColor[0]
	}

	s := &LoadingSpinner{
		isActive: false, // Começa inativo

		Color:         c,
		Size:          unit.Dp(defaultSpinnerSize),
		NumSegments:   defaultSpinnerNumSegments,
		SegmentWidth:  unit.Dp(defaultSpinnerSegWidth),
		SegmentLength: defaultSpinnerSegLength,

		currentAngle:   0,
		visibleOpacity: 0.0, // Começa invisível
		stopTicker:     make(chan struct{}),
	}
	return s
}

// Start ativa o spinner, iniciando a animação de rotação e um fade-in.
// Requer um `layout.Context` para obter o tempo atual e solicitar redesenhos.
func (s *LoadingSpinner) Start(gtx layout.Context) {
	if s.isActive && s.fadeTarget == 1.0 && !s.isFading { // Já ativo e totalmente visível
		return
	}
	s.isActive = true // Ativa a lógica de rotação
	s.isFading = true
	s.fadeStartTime = gtx.Now
	s.fadeTarget = 1.0 // Fade in

	// Inicia o ticker para rotação contínua se ainda não estiver rodando
	if s.ticker == nil {
		s.ticker = time.NewTicker(defaultSpinnerTickRate)
		// A goroutine runRotation agora é iniciada apenas uma vez e controlada por s.isActive
		// ou pelo fechamento de s.stopTicker.
		// A chamada para runRotation pode ser feita aqui ou no primeiro Layout quando isActive.
		// Vamos iniciá-la aqui para garantir que a rotação comece.
		go s.runRotationLoop(gtx) // Passa gtx inicial para solicitar primeiro frame
	}
	op.InvalidateOp{}.Add(gtx.Ops) // Solicita um novo frame para iniciar a animação de fade
}

// Stop desativa o spinner, parando a rotação (eventualmente) e iniciando um fade-out.
// Requer um `layout.Context` para o tempo e redesenho.
func (s *LoadingSpinner) Stop(gtx layout.Context) {
	if !s.isActive && s.fadeTarget == 0.0 && !s.isFading { // Já inativo e totalmente invisível
		return
	}
	s.isActive = false // Sinaliza para a goroutine de rotação parar (eventualmente)
	s.isFading = true
	s.fadeStartTime = gtx.Now
	s.fadeTarget = 0.0 // Fade out

	// O ticker será parado pela goroutine `runRotationLoop` quando `s.isActive` for false
	// e o fade-out terminar, ou se `s.stopTicker` for fechado (em `Destroy`).
	op.InvalidateOp{}.Add(gtx.Ops)
}

// SetVisibility controla diretamente a visibilidade do spinner com animação de fade.
func (s *LoadingSpinner) SetVisibility(gtx layout.Context, visible bool) {
	if visible {
		s.Start(gtx)
	} else {
		s.Stop(gtx)
	}
}

// runRotationLoop é uma goroutine que atualiza o ângulo de rotação do spinner.
// `initialGtx` é usado para solicitar o primeiro Invalidate.
func (s *LoadingSpinner) runRotationLoop(initialGtx layout.Context) {
	// Solicita um Invalidate para a UI principal para desenhar o estado inicial ou
	// se o spinner foi ativado e precisa ser renderizado.
	// A AppWindow deve fornecer uma maneira de invalidar a partir de uma goroutine.
	// Ex: appWindowInstance.Invalidate() ou appWindowInstance.Execute(func() { op.InvalidateOp{}.Add(...) })
	// Aqui, vamos assumir que o chamador de Start/Stop lida com a invalidação inicial.
	// A invalidação contínua durante a animação será feita no Layout.

	defer func() {
		if s.ticker != nil {
			s.ticker.Stop()
			s.ticker = nil // Importante para permitir que o ticker seja recriado se Start for chamado novamente
		}
		// appLogger.Debug("Goroutine de rotação do spinner finalizada.")
	}()

	// appLogger.Debug("Goroutine de rotação do spinner iniciada.")
	for {
		select {
		case <-s.stopTicker: // Canal para forçar parada (usado em Destroy)
			return
		case <-s.ticker.C: // A cada tick do temporizador
			// Se o spinner não está mais ativo (Stop foi chamado) E já está invisível (fade-out completo),
			// então a goroutine pode parar.
			if !s.isActive && s.visibleOpacity < 0.01 && !s.isFading {
				return
			}

			// Atualiza o ângulo para a rotação.
			// Um incremento fixo por tick resulta em velocidade constante.
			// O valor do incremento controla a "velocidade" da rotação visual.
			angleIncrement := float32(360.0 / float32(s.NumSegments*2)) // Ajuste este valor para mudar a velocidade.
			s.currentAngle = float32(math.Mod(float64(s.currentAngle)+float64(angleIncrement), 360.0))

			// A invalidação para redesenhar o spinner agora é feita no método Layout
			// se `s.isActive` ou `s.isFading` for true.
			// Isso evita chamar Invalidate de uma goroutine diretamente na janela,
			// o que pode ser problemático. O loop de eventos da UI cuidará disso.
		}
	}
}

// Layout desenha o spinner.
func (s *LoadingSpinner) Layout(gtx layout.Context) layout.Dimensions {
	// Lógica de Fade In/Out
	if s.isFading {
		progress := float32(gtx.Now.Sub(s.fadeStartTime)) / float32(fadeDuration)
		if progress >= 1.0 {
			progress = 1.0
			s.isFading = false              // Terminou o fade
			s.visibleOpacity = s.fadeTarget // Garante que atinja o alvo
			if s.fadeTarget == 0.0 {
				s.isActive = false // Garante que está logicamente inativo após fade out completo
			}
		} else {
			if s.fadeTarget == 1.0 { // Fade in
				s.visibleOpacity = progress
			} else { // Fade out
				s.visibleOpacity = 1.0 - progress
			}
		}
		op.InvalidateOp{}.Add(gtx.Ops) // Continua animando o fade
	}

	// Se não está logicamente ativo, não está fazendo fade, e está totalmente invisível,
	// não desenha nada, mas ocupa o espaço definido por `s.Size`.
	if !s.isActive && !s.isFading && s.visibleOpacity < 0.01 {
		return layout.Dimensions{Size: image.Pt(gtx.Dp(s.Size), gtx.Dp(s.Size))}
	}

	// Se estiver ativo (rotação) ou fazendo fade (mudando opacidade),
	// solicita redesenho contínuo para a animação.
	if s.isActive || s.isFading {
		op.InvalidateOp{}.Add(gtx.Ops)
	}

	// Desenho do spinner
	spinnerDiameterPx := gtx.Dp(s.Size)
	// Garante que a área de desenho do clipe corresponda ao tamanho.
	defer clip.Rect{Max: image.Pt(spinnerDiameterPx, spinnerDiameterPx)}.Push(gtx.Ops).Pop()

	center := f32.Pt(float32(spinnerDiameterPx)/2, float32(spinnerDiameterPx)/2)
	segmentWidthPx := float32(gtx.Dp(s.SegmentWidth))
	// O raio efetivo considera a metade da largura do segmento para que as bordas não saiam do círculo.
	padding := segmentWidthPx/2 + 1 // Pequeno ajuste para evitar corte de bordas arredondadas
	effectiveRadius := center.X - padding
	if effectiveRadius <= 1 { // Muito pequeno para desenhar segmentos
		return layout.Dimensions{Size: image.Pt(spinnerDiameterPx, spinnerDiameterPx)}
	}

	// Salva e aplica a transformação de offset para o centro do spinner.
	offsetTransform := op.Offset(center).Push(gtx.Ops)
	defer offsetTransform.Pop()

	segmentAngleStepRad := 2.0 * math.Pi / float32(s.NumSegments) // Ângulo entre segmentos em radianos

	for i := 0; i < s.NumSegments; i++ {
		// Calcula o ângulo do segmento atual, considerando a rotação global `s.currentAngle`.
		segmentRotationRad := s.currentAngle * math.Pi / 180.0 // Converte `currentAngle` para radianos
		currentSegmentAngleRad := segmentRotationRad + (float32(i) * segmentAngleStepRad)

		// Calcula a opacidade do segmento individual (para efeito de "trilha")
		var segmentAlpha uint8 = 255
		// Opacidade decai para segmentos "mais antigos" na trilha.
		// O segmento `i=0` (após rotação) é o mais opaco.
		opacityFactor := math.Pow(1.0-(float64(i)/float64(s.NumSegments)), 1.8) // Ajuste o expoente para a força da trilha
		segmentAlpha = uint8(math.Max(20, 255*opacityFactor))                   // Mínimo de opacidade para visibilidade

		// Aplica a opacidade geral do fade (s.visibleOpacity)
		finalSegmentOpacity := uint8(float32(segmentAlpha) * s.visibleOpacity)
		if finalSegmentOpacity < 5 && s.visibleOpacity > 0.01 { // Garante um mínimo se o spinner estiver visível
			finalSegmentOpacity = 5
		}
		if s.visibleOpacity < 0.01 { // Se quase invisível, força opacidade zero para o segmento
			finalSegmentOpacity = 0
		}

		segmentColorWithOpacity := s.Color
		segmentColorWithOpacity.A = finalSegmentOpacity

		// Calcula os pontos inicial e final do segmento
		innerRadius := effectiveRadius * (1.0 - s.SegmentLength) // Ponto inicial do segmento (mais próximo ao centro)

		startX := innerRadius * float32(math.Cos(float64(currentSegmentAngleRad)))
		startY := innerRadius * float32(math.Sin(float64(currentSegmentAngleRad)))
		endX := effectiveRadius * float32(math.Cos(float64(currentSegmentAngleRad)))
		endY := effectiveRadius * float32(math.Sin(float64(currentSegmentAngleRad)))

		var path clip.Path
		path.Begin(gtx.Ops)
		path.MoveTo(f32.Pt(startX, startY))
		path.LineTo(f32.Pt(endX, endY))
		// Desenha o segmento como uma linha com espessura e extremidades arredondadas.
		paint.StrokeShape(gtx.Ops, segmentColorWithOpacity, clip.Stroke{
			Path:  path.End(),
			Width: segmentWidthPx,
			Cap:   clip.RoundCap, // Extremidades arredondadas
		}.Op())
	}

	return layout.Dimensions{Size: image.Pt(spinnerDiameterPx, spinnerDiameterPx)}
}

// Destroy deve ser chamado quando o spinner não for mais necessário para parar sua goroutine de animação.
// Isso é importante para evitar goroutines órfãs.
func (s *LoadingSpinner) Destroy() {
	// Tenta fechar o canal `stopTicker` de forma segura, apenas se não estiver já fechado.
	// Isso sinaliza para a goroutine `runRotationLoop` terminar.
	select {
	case <-s.stopTicker: // Canal já fechado ou sendo fechado.
	default:
		close(s.stopTicker)
	}
	// O ticker em si é parado dentro da goroutine `runRotationLoop` ao sair.
}
