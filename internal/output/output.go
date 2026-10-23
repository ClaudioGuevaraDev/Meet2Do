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
	TODO       Kind = "todo"
	Transcript Kind = "transcript"
	Audio      Kind = "audio"
)

// FileName devuelve el nombre de archivo para cada tipo.
func FileName(k Kind) string {
	switch k {
	case TODO:
		return "TODO.md"
	case Transcript:
		return "transcripcion.md"
	case Audio:
		return "reunion.wav"
	}
	return string(k)
}

// Existing devuelve las rutas que ya existen en dir para los tipos dados.
func Existing(dir string, kinds []Kind) []string {
	var out []string
	for _, k := range kinds {
		p := filepath.Join(dir, FileName(k))
		if _, err := os.Stat(p); err == nil {
			out = append(out, p)
		}
	}
	return out
}

// WriteTODO guarda el TODO generado.
func WriteTODO(dir, content string) (string, error) {
	p := filepath.Join(dir, FileName(TODO))
	return p, os.WriteFile(p, []byte(content), 0o644)
}

// WriteTranscript guarda la transcripción con un encabezado de fecha.
func WriteTranscript(dir, text string, date time.Time) (string, error) {
	p := filepath.Join(dir, FileName(Transcript))
	content := fmt.Sprintf("# Transcripción de la reunión\n\n_%s_\n\n%s\n", date.Format("2006-01-02 15:04"), text)
	return p, os.WriteFile(p, []byte(content), 0o644)
}

// CopyAudio copia el WAV grabado al destino.
func CopyAudio(dir, src string) (string, error) {
	p := filepath.Join(dir, FileName(Audio))
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()
	out, err := os.Create(p)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return "", err
	}
	return p, out.Close()
}
