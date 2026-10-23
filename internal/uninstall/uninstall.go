// Package uninstall encuentra y borra lo que Meet2Do instaló: su ejecutable, whisper.cpp y
// los modelos de voz. Las reuniones guardadas son del usuario y nunca se tocan.
package uninstall

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ClaudioGuevaraDev/Meet2Do/internal/transcriber"
)

// Component es algo instalado que se puede desinstalar.
type Component struct {
	Name   string // "Meet2Do", "whisper.cpp", "Modelos de voz"
	Detail string // información extra, p. ej. los modelos descargados ("small, tiny")
	Path   string // archivo o carpeta que se borra
	Size   int64  // bytes que ocupa
}

// Nombres de los componentes.
const (
	App     = "Meet2Do"
	Whisper = "whisper.cpp"
	Models  = "Modelos de voz"
)

// executable devuelve la ruta del ejecutable en uso; variable para los tests.
var executable = os.Executable

// Find devuelve lo que está instalado, en el orden en que se muestra.
func Find() ([]Component, error) {
	var cs []Component
	if exe, ok := installedBinary(); ok {
		if size, err := dirSize(exe); err == nil {
			cs = append(cs, Component{Name: App, Path: exe, Size: size})
		}
	}

	whisper, err := transcriber.WhisperDir()
	if err != nil {
		return nil, err
	}
	models, err := transcriber.ModelsDir()
	if err != nil {
		return nil, err
	}
	for _, c := range []Component{
		{Name: Whisper, Path: whisper},
		{Name: Models, Path: models, Detail: strings.Join(modelNames(models), ", ")},
	} {
		size, err := dirSize(c.Path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("leer %s: %w", c.Path, err)
		}
		c.Size = size
		cs = append(cs, c)
	}
	return cs, nil
}

// installedBinary devuelve el ejecutable de Meet2Do, salvo que sea uno temporal de `go run`.
func installedBinary() (string, bool) {
	exe, err := executable()
	if err != nil {
		return "", false
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	tmp := filepath.Clean(os.TempDir()) + string(filepath.Separator)
	if strings.HasPrefix(exe, tmp) || strings.Contains(exe, string(filepath.Separator)+"go-build") {
		return "", false
	}
	return exe, true
}

// modelNames devuelve los modelos descargados ("small" por ggml-small.bin), ordenados.
func modelNames(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if name, ok := strings.CutPrefix(e.Name(), "ggml-"); ok && strings.HasSuffix(name, ".bin") {
			names = append(names, strings.TrimSuffix(name, ".bin"))
		}
	}
	slices.Sort(names)
	return names
}

// dirSize suma el tamaño de los archivos de path (o el del archivo, si no es una carpeta).
func dirSize(path string) (int64, error) {
	var total int64
	err := filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			total += info.Size()
		}
		return nil
	})
	return total, err
}

// Remove borra el componente. En Linux se puede borrar el ejecutable que está corriendo.
func Remove(c Component) error {
	if err := os.RemoveAll(c.Path); err != nil {
		return fmt.Errorf("borrar %s: %w", c.Path, err)
	}
	return nil
}

// Cleanup borra la carpeta de caché de Meet2Do si quedó vacía.
func Cleanup() {
	if dir, err := transcriber.CacheDir(); err == nil {
		_ = os.Remove(dir) // falla si no está vacía, que es justo lo que queremos
	}
}
