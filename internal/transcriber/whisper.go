// Package transcriber transcribe audio localmente con whisper.cpp. El ejecutable y los modelos
// se descargan solos en la caché del usuario la primera vez.
package transcriber

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const (
	// DefaultBaseURL es de donde se descargan los modelos ggml.
	DefaultBaseURL = "https://huggingface.co/ggerganov/whisper.cpp/resolve/main"
	// DefaultModel es el modelo usado cuando Whisper.Model está vacío.
	DefaultModel = "small"
	// DefaultLang es el idioma usado cuando Whisper.Lang está vacío.
	DefaultLang = "auto"
)

// modelSizes son los tamaños aproximados de descarga de los modelos más comunes.
var modelSizes = map[string]int64{
	"tiny":     77_691_713,
	"base":     147_951_465,
	"small":    487_601_967,
	"medium":   1_533_763_059,
	"large-v3": 3_095_033_483,
}

// Whisper ejecuta whisper.cpp con un modelo ggml. Los campos vacíos usan los valores por defecto.
type Whisper struct {
	Binary        string // ruta a whisper-cli; vacío → MEET2DO_WHISPER o el de la caché
	Model         string // nombre, p. ej. "small"
	Lang          string // "auto", "es", "en", ...
	BaseURL       string // origen de los modelos
	BundleBaseURL string // origen de los builds de whisper.cpp
}

// ModelName devuelve el modelo efectivo.
func (w *Whisper) ModelName() string { return cmp.Or(w.Model, DefaultModel) }

func (w *Whisper) lang() string { return cmp.Or(w.Lang, DefaultLang) }

// ModelSize es el tamaño aproximado de descarga del modelo (0 si se desconoce).
func (w *Whisper) ModelSize() int64 { return modelSizes[w.ModelName()] }

// ModelPath es la ruta local del modelo en la caché del usuario.
func (w *Whisper) ModelPath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("directorio de caché: %w", err)
	}
	return filepath.Join(dir, "meet2do", "models", "ggml-"+w.ModelName()+".bin"), nil
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

// DownloadModel descarga el modelo a la caché.
func (w *Whisper) DownloadModel(ctx context.Context, progress ProgressFunc) error {
	dst, err := w.ModelPath()
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/ggml-%s.bin", cmp.Or(w.BaseURL, DefaultBaseURL), w.ModelName())
	if _, err = download(ctx, url, dst, progress); err != nil {
		return fmt.Errorf("descargar modelo %q: %w", w.ModelName(), err)
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
	bin, err := w.BinaryPath()
	if err != nil {
		return "", err
	}
	model, err := w.ModelPath()
	if err != nil {
		return "", err
	}
	outBase := strings.TrimSuffix(audio, filepath.Ext(audio)) + "-transcript"
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, bin, w.Args(model, audio, outBase)...)
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

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
