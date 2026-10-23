// Package output escribe los archivos finales elegidos por el usuario.
package output

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Kind identifica un tipo de archivo de salida.
type Kind string

const (
	Tasks      Kind = "tareas"
	Summary    Kind = "resumen"
	Transcript Kind = "transcript"
	Audio      Kind = "audio"
)

// FileName devuelve el nombre de archivo para cada tipo.
func FileName(k Kind) string {
	switch k {
	case Tasks:
		return "tareas.md"
	case Summary:
		return "resumen.md"
	case Transcript:
		return "transcripcion.md"
	case Audio:
		return "reunion.wav"
	}
	return string(k)
}

// MeetingDir crea en parent una carpeta nueva para la reunión, reunion-AAAA-MM-DD_HH-MM
// según t (con -2, -3… si ya existe), y devuelve su ruta. Nunca reutiliza una carpeta.
func MeetingDir(parent string, t time.Time) (string, error) {
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", fmt.Errorf("crear carpeta %s: %w", parent, err)
	}
	base := filepath.Join(parent, "reunion-"+t.Format("2006-01-02_15-04"))
	for n := 1; ; n++ {
		p := base
		if n > 1 {
			p = fmt.Sprintf("%s-%d", base, n)
		}
		err := os.Mkdir(p, 0o755)
		if err == nil {
			return p, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return "", fmt.Errorf("crear carpeta %s: %w", p, err)
		}
	}
}

// WriteTasks guarda la lista de tareas generada.
func WriteTasks(dir, content string) (string, error) {
	return writeMarkdown(dir, Tasks, content)
}

// WriteSummary guarda el resumen de la reunión generado.
func WriteSummary(dir, content string) (string, error) {
	return writeMarkdown(dir, Summary, content)
}

func writeMarkdown(dir string, k Kind, content string) (string, error) {
	p := filepath.Join(dir, FileName(k))
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
