// Package transcriber transcribe audio localmente con whisper.cpp.
package transcriber

import (
	"bytes"
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

// InstallHelp explica cómo obtener whisper-cli.
const InstallHelp = `whisper.cpp no está instalado. Para compilarlo:

  git clone https://github.com/ggml-org/whisper.cpp
  cd whisper.cpp
  cmake -B build && cmake --build build -j --config Release
  sudo cp build/bin/whisper-cli /usr/local/bin/   # o agrégalo a tu PATH`

const modelURL = "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-%s.bin"

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
	return "", errors.New(InstallHelp)
}

// Whisper ejecuta whisper.cpp con un modelo ggml.
type Whisper struct {
	Binary string
	Model  string // nombre, p. ej. "small"
	Lang   string // "auto", "es", "en", ...
}

// ModelPath es la ruta local del modelo en la caché del usuario.
func (w *Whisper) ModelPath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "meet2do", "models", "ggml-"+w.Model+".bin"), nil
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
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(modelURL, w.Model), nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("descargando modelo: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("descargando modelo %q: HTTP %d", w.Model, resp.StatusCode)
	}

	tmp := dst + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	pw := &progressWriter{total: resp.ContentLength, fn: progress}
	if _, err := io.Copy(io.MultiWriter(f, pw), resp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("descargando modelo: %w", err)
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

// Args devuelve los argumentos de whisper-cli para transcribir audio a <outBase>.txt.
func (w *Whisper) Args(model, audio, outBase string) []string {
	return []string{
		"-m", model,
		"-f", audio,
		"-l", w.Lang,
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
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("whisper falló: %v\n%s", err, lastLines(stderr.String(), 10))
	}
	data, err := os.ReadFile(outBase + ".txt")
	if err != nil {
		return "", err
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
