// Package recorder graba micrófono + audio del sistema a un WAV 16 kHz mono, en Go puro
// (habla directamente con PulseAudio/PipeWire, sin ffmpeg).
package recorder

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

const (
	// SampleRate es la frecuencia de muestreo de la grabación (la que espera whisper).
	SampleRate = 16000

	// mixInterval es cada cuánto se mezclan y escriben a disco las muestras recibidas.
	mixInterval = 100 * time.Millisecond
	// maxLag es cuánto puede adelantarse una fuente a la otra antes de rellenar la atrasada
	// con silencio (p. ej. el monitor no entrega datos mientras no suena nada).
	maxLag = 2 * SampleRate
)

var (
	// ErrNotStarted se devuelve al llamar Stop, Pause o Resume sin un Start exitoso.
	ErrNotStarted = errors.New("la grabación no fue iniciada")
	// ErrEmptyRecording se devuelve cuando la grabación no contiene audio.
	ErrEmptyRecording = errors.New("la grabación quedó vacía")
)

// Source entrega muestras PCM int16 mono a SampleRate mientras está iniciada.
type Source interface {
	// Start comienza a llamar write con cada bloque de muestras (write copia los datos).
	Start(write func([]int16)) error
	// Close detiene la fuente; después no se vuelve a llamar write.
	Close() error
}

// Recorder mezcla dos fuentes (micrófono y sistema) y las escribe a un WAV.
type Recorder struct {
	Output string

	open func() (mic, monitor Source, release func(), err error)

	mu      sync.Mutex // protege bufs y paused
	bufs    [2][]int16
	paused  bool
	flushMu sync.Mutex // serializa flush: lo llaman mixLoop y Pause
	sources [2]Source
	release func()
	wav     *wavWriter
	quit    chan struct{}
	done    chan error
}

// New crea un Recorder que graba el micrófono y el audio del sistema por defecto.
func New(output string) *Recorder {
	return &Recorder{Output: output, open: openPulse}
}

// NewWithSources crea un Recorder con fuentes propias (útil para tests).
func NewWithSources(output string, mic, monitor Source) *Recorder {
	return &Recorder{Output: output, open: func() (Source, Source, func(), error) {
		return mic, monitor, func() {}, nil
	}}
}

// Start abre las fuentes y comienza a grabar.
func (r *Recorder) Start() error {
	mic, monitor, release, err := r.open()
	if err != nil {
		return err
	}
	wav, err := createWAV(r.Output, SampleRate)
	if err != nil {
		release()
		return err
	}

	r.bufs, r.paused = [2][]int16{}, false
	r.sources = [2]Source{mic, monitor}
	r.release, r.wav = release, wav
	for i, src := range r.sources {
		if err := src.Start(r.appender(i)); err != nil {
			r.abort()
			return fmt.Errorf("iniciar captura de audio: %w", err)
		}
	}

	r.quit, r.done = make(chan struct{}), make(chan error, 1)
	go r.mixLoop()
	return nil
}

// Stop detiene la grabación y cierra el WAV.
func (r *Recorder) Stop() error {
	if r.wav == nil {
		return ErrNotStarted
	}
	srcErr := r.closeSources()
	close(r.quit)
	mixErr := <-r.done
	if mixErr == nil {
		mixErr = r.flush(true)
	}
	samples := r.wav.samples
	closeErr := r.wav.close()
	r.wav = nil

	if err := errors.Join(mixErr, closeErr); err != nil {
		return fmt.Errorf("escribir %s: %w", r.Output, err)
	}
	if samples == 0 {
		if srcErr != nil {
			return fmt.Errorf("%w: %w", ErrEmptyRecording, srcErr)
		}
		return ErrEmptyRecording
	}
	return nil
}

// Pause deja de grabar sin cerrar las fuentes: lo que llegue hasta Resume se descarta.
// Antes escribe lo que quedaba en los buffers, para no perder el final previo a la pausa.
func (r *Recorder) Pause() error {
	if r.wav == nil {
		return ErrNotStarted
	}
	r.mu.Lock()
	wasPaused := r.paused
	r.paused = true
	r.mu.Unlock()
	if wasPaused {
		return nil
	}
	if err := r.flush(true); err != nil {
		return fmt.Errorf("escribir %s: %w", r.Output, err)
	}
	return nil
}

// Resume vuelve a grabar. Ambas fuentes retoman con los buffers vacíos, así que siguen
// alineadas.
func (r *Recorder) Resume() error {
	if r.wav == nil {
		return ErrNotStarted
	}
	r.mu.Lock()
	r.bufs, r.paused = [2][]int16{}, false
	r.mu.Unlock()
	return nil
}

func (r *Recorder) appender(i int) func([]int16) {
	return func(s []int16) {
		r.mu.Lock()
		if !r.paused {
			r.bufs[i] = append(r.bufs[i], s...)
		}
		r.mu.Unlock()
	}
}

func (r *Recorder) mixLoop() {
	t := time.NewTicker(mixInterval)
	defer t.Stop()
	for {
		select {
		case <-r.quit:
			r.done <- nil
			return
		case <-t.C:
			if err := r.flush(false); err != nil {
				r.done <- err
				return
			}
		}
	}
}

// flush mezcla lo que ambas fuentes tienen en común y lo escribe. Con final=true escribe todo,
// rellenando con silencio la fuente más corta.
func (r *Recorder) flush(final bool) error {
	r.flushMu.Lock()
	defer r.flushMu.Unlock()
	r.mu.Lock()
	a, b := r.bufs[0], r.bufs[1]
	n := min(len(a), len(b))
	if longest := max(len(a), len(b)); final {
		n = longest
	} else if longest-n > maxLag {
		n = longest - maxLag
	}
	out := Mix(make([]int16, n), head(a, n), head(b, n))
	r.bufs[0], r.bufs[1] = tail(a, n), tail(b, n)
	r.mu.Unlock()

	if n == 0 {
		return nil
	}
	return r.wav.write(out)
}

func (r *Recorder) closeSources() error {
	var errs []error
	for _, src := range r.sources {
		if src != nil {
			errs = append(errs, src.Close())
		}
	}
	r.sources = [2]Source{}
	if r.release != nil {
		r.release()
		r.release = nil
	}
	return errors.Join(errs...)
}

// abort deshace un Start a medias.
func (r *Recorder) abort() {
	_ = r.closeSources() // ya se informa el error de Start
	_ = r.wav.close()
	r.wav = nil
}

// Mix suma a y b muestra a muestra en dst con saturación int16 y devuelve dst[:n], con
// n = min(len(dst), len(a), len(b)).
func Mix(dst, a, b []int16) []int16 {
	n := min(len(dst), len(a), len(b))
	for i := range n {
		s := int32(a[i]) + int32(b[i])
		dst[i] = int16(max(-32768, min(32767, s)))
	}
	return dst[:n]
}

// head devuelve los primeros n elementos de s, rellenando con ceros si s es más corto.
func head(s []int16, n int) []int16 {
	if len(s) >= n {
		return s[:n]
	}
	return append(s[:len(s):len(s)], make([]int16, n-len(s))...)
}

// tail devuelve lo que queda de s después de los primeros n elementos.
func tail(s []int16, n int) []int16 {
	if len(s) <= n {
		return nil
	}
	return append([]int16(nil), s[n:]...)
}
