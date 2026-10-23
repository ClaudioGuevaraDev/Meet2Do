// Package transcriber transcribe audio localmente con whisper.cpp.
package transcriber

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// Binaries son los nombres con los que se suele instalar whisper.cpp.
var Binaries = []string{"whisper-cli", "whisper-cpp", "whisper.cpp"}

// ErrNotInstalled indica que no se encontró el ejecutable de whisper.cpp.
var ErrNotInstalled = errors.New("whisper.cpp no está instalado")

// InstallHelp explica cómo obtener whisper-cli.
const InstallHelp = `Para compilarlo:

  git clone https://github.com/ggml-org/whisper.cpp
  cd whisper.cpp
  cmake -B build && cmake --build build -j --config Release
  sudo cp build/bin/whisper-cli /usr/local/bin/   # o agrégalo a tu PATH

También puedes indicar la ruta con MEET2DO_WHISPER=/ruta/a/whisper-cli`

const (
	// DefaultBaseURL es de donde se descargan los modelos ggml.
	DefaultBaseURL = "https://huggingface.co/ggerganov/whisper.cpp/resolve/main"
	// DefaultModel es el modelo usado cuando Whisper.Model está vacío.
	DefaultModel = "small"
	// DefaultLang es el idioma usado cuando Whisper.Lang está vacío.
	DefaultLang = "auto"
)

// FindBinary devuelve la ruta al ejecutable de whisper.cpp.
func FindBinary() (string, error) {
	if p := os.Getenv("MEET2DO_WHISPER"); p != "" {
		return p, nil
	}
	for _, name := range Binaries {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("%w\n\n%s", ErrNotInstalled, InstallHelp)
}

// Whisper ejecuta whisper.cpp con un modelo ggml. Los campos vacíos usan los valores por defecto.
type Whisper struct {
	Binary  string
	Model   string // nombre, p. ej. "small"
	Lang    string // "auto", "es", "en", ...
	BaseURL string // origen de los modelos
}

func (w *Whisper) model() string { return cmp.Or(w.Model, DefaultModel) }
func (w *Whisper) lang() string  { return cmp.Or(w.Lang, DefaultLang) }

// ModelPath es la ruta local del modelo en la caché del usuario.
func (w *Whisper) ModelPath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("directorio de caché: %w", err)
	}
	return filepath.Join(dir, "meet2do", "models", "ggml-"+w.model()+".bin"), nil
}

// ModelExists indica si el modelo ya está descargado.
func (w *Whisper) ModelExists() bool {
	p, err := w.ModelPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

// DownloadModel descarga el modelo; progress recibe (bytes descargados, total).
func (w *Whisper) DownloadModel(ctx context.Context, progress func(done, total int64)) error {
	dst, err := w.ModelPath()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("crear carpeta de modelos: %w", err)
	}
	url := fmt.Sprintf("%s/ggml-%s.bin", cmp.Or(w.BaseURL, DefaultBaseURL), w.model())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("crear petición %s: %w", url, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("descargar modelo %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }() // cuerpo ya consumido
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("descargar modelo %q: HTTP %d", w.model(), resp.StatusCode)
	}

	tmp := dst + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("crear %s: %w", tmp, err)
	}
	pw := &progressWriter{total: resp.ContentLength, fn: progress}
	if _, err := io.Copy(io.MultiWriter(f, pw), resp.Body); err != nil {
		_ = f.Close()      // el error relevante es el de la copia
		_ = os.Remove(tmp) // limpieza best-effort del archivo parcial
		return fmt.Errorf("descargar modelo %q: %w", w.model(), err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("cerrar %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		return fmt.Errorf("guardar modelo %s: %w", dst, err)
	}
	return nil
}

// Args devuelve los argumentos de whisper-cli para transcribir audio a <outBase>.txt.
func (w *Whisper) Args(model, audio, outBase string) []string {
	return []string{
		"-m", model,
		"-f", audio,
		"-l", w.lang(),
		"-t", strconv.Itoa(max(1, runtime.NumCPU()-1)),
		"-nt", "-otxt",
		"-of", outBase,
	}
}

// Transcribe devuelve el texto del audio (WAV 16 kHz mono).
func (w *Whisper) Transcribe(ctx context.Context, audio string) (string, error) {
	model, err := w.ModelPath()
	if err != nil {
		return "", err
	}
	outBase := strings.TrimSuffix(audio, filepath.Ext(audio)) + "-transcript"
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, w.Binary, w.Args(model, audio, outBase)...)
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if err = cmd.Run(); err != nil {
		return "", fmt.Errorf("whisper falló: %w\n%s", err, lastLines(stderr.String(), 10))
	}
	data, err := os.ReadFile(outBase + ".txt")
	if err != nil {
		return "", fmt.Errorf("leer transcripción: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

type progressWriter struct {
	done, total int64
	fn          func(done, total int64)
}

func (p *progressWriter) Write(b []byte) (int, error) {
	p.done += int64(len(b))
	if p.fn != nil {
		p.fn(p.done, p.total)
	}
	return len(b), nil
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
