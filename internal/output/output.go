// Package output escribe los archivos finales elegidos por el usuario.
package output

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Kind identifica un tipo de archivo de salida.
type Kind string

const (
	Tasks      Kind = "tareas"
	Transcript Kind = "transcript"
	Audio      Kind = "audio"
)

// FileName devuelve el nombre de archivo para cada tipo.
func FileName(k Kind) string {
	switch k {
	case Tasks:
		return "tareas.md"
	case Transcript:
		return "transcripcion.md"
	case Audio:
		return "reunion.wav"
	}
	return string(k)
}

// Existing devuelve las rutas que ya existen en dir para los tipos dados.
func Existing(dir string, kinds []Kind) []string {
	out := make([]string, 0, len(kinds))
	for _, k := range kinds {
		p := filepath.Join(dir, FileName(k))
		if _, err := os.Stat(p); err == nil {
			out = append(out, p)
		}
	}
	return out
}

// WriteTasks guarda la lista de tareas generada.
func WriteTasks(dir, content string) (string, error) {
	p := filepath.Join(dir, FileName(Tasks))
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("escribir %s: %w", p, err)
	}
	return p, nil
}

// WriteTranscript guarda la transcripción con un encabezado de fecha.
func WriteTranscript(dir, text string, date time.Time) (string, error) {
	p := filepath.Join(dir, FileName(Transcript))
	content := fmt.Sprintf("# Transcripción de la reunión\n\n_%s_\n\n%s\n", date.Format("2006-01-02 15:04"), text)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("escribir %s: %w", p, err)
	}
	return p, nil
}

// CopyAudio copia el WAV grabado al destino.
func CopyAudio(dir, src string) (p string, err error) {
	p = filepath.Join(dir, FileName(Audio))
	in, err := os.Open(src)
	if err != nil {
		return "", fmt.Errorf("abrir audio %s: %w", src, err)
	}
	defer func() { _ = in.Close() }() // solo lectura: el error de Close no afecta la copia

	out, err := os.Create(p)
	if err != nil {
		return "", fmt.Errorf("crear %s: %w", p, err)
	}
	defer func() {
		if cerr := out.Close(); cerr != nil && err == nil {
			p, err = "", fmt.Errorf("cerrar %s: %w", p, cerr)
		}
	}()

	if _, err = io.Copy(out, in); err != nil {
		return "", fmt.Errorf("copiar audio a %s: %w", p, err)
	}
	return p, nil
}
