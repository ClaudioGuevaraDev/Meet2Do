// Package recorder captura micrófono + audio del sistema con ffmpeg (PulseAudio/PipeWire).
package recorder

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

const (
	// DefaultMic es la fuente de entrada por defecto del servidor Pulse.
	DefaultMic = "default"
	// DefaultMonitor es el monitor del sink por defecto (lo que suena en parlantes/audífonos).
	DefaultMonitor = "@DEFAULT_MONITOR@"
)

// Recorder controla un proceso ffmpeg que graba a un WAV 16 kHz mono.
type Recorder struct {
	Mic     string
	Monitor string
	Output  string

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stderr bytes.Buffer
	done   chan error
}

// New crea un Recorder con las fuentes por defecto.
func New(output string) *Recorder {
	return &Recorder{Mic: DefaultMic, Monitor: DefaultMonitor, Output: output}
}

// Args devuelve los argumentos de ffmpeg.
func (r *Recorder) Args() []string {
	return []string{
		"-hide_banner", "-loglevel", "error", "-nostats", "-y",
		"-f", "pulse", "-i", r.Mic,
		"-f", "pulse", "-i", r.Monitor,
		"-filter_complex", "amix=inputs=2:duration=longest:normalize=0",
		"-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le",
		r.Output,
	}
}

// Start lanza ffmpeg. Devuelve error si el proceso muere en el primer instante
// (p. ej. dispositivo inexistente).
func (r *Recorder) Start() error {
	r.cmd = exec.Command("ffmpeg", r.Args()...)
	r.cmd.Stderr = &r.stderr
	// Evita que Ctrl+C en la terminal mate ffmpeg antes de cerrar el WAV.
	r.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdin, err := r.cmd.StdinPipe()
	if err != nil {
		return err
	}
	r.stdin = stdin
	if err := r.cmd.Start(); err != nil {
		return fmt.Errorf("no se pudo iniciar ffmpeg: %w", err)
	}
	r.done = make(chan error, 1)
	go func() { r.done <- r.cmd.Wait() }()

	select {
	case err := <-r.done:
		return fmt.Errorf("ffmpeg terminó inesperadamente: %v: %s", err, strings.TrimSpace(r.stderr.String()))
	case <-time.After(500 * time.Millisecond):
		return nil
	}
}

// Stop pide a ffmpeg que termine limpiamente ("q") para que el WAV quede bien cerrado.
func (r *Recorder) Stop() error {
	if r.cmd == nil {
		return errors.New("la grabación no fue iniciada")
	}
	_, _ = io.WriteString(r.stdin, "q")
	_ = r.stdin.Close()

	select {
	case <-r.done:
	case <-time.After(5 * time.Second):
		_ = r.cmd.Process.Signal(os.Interrupt)
		select {
		case <-r.done:
		case <-time.After(3 * time.Second):
			_ = r.cmd.Process.Kill()
			<-r.done
		}
	}

	info, err := os.Stat(r.Output)
	if err != nil || info.Size() <= 44 {
		return fmt.Errorf("la grabación quedó vacía: %s", strings.TrimSpace(r.stderr.String()))
	}
	return nil
}
