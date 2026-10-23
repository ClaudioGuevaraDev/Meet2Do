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
	Name   string   // "Meet2Do", "whisper.cpp", "Modelos de voz"
	Detail string   // información extra, p. ej. los modelos descargados ("small, tiny")
	Paths  []string // archivos o carpetas que se borran
	Size   int64    // bytes que ocupa
}

// Commands son los nombres del ejecutable: meet2do y su atajo m2do (el mismo programa).
var Commands = []string{"meet2do", "m2do"}

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
		if c, ok := appComponent(exe); ok {
			cs = append(cs, c)
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
		{Name: Whisper, Paths: []string{whisper}},
		{Name: Models, Paths: []string{models}, Detail: strings.Join(modelNames(models), ", ")},
	} {
		size, err := dirSize(c.Paths[0])
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("leer %s: %w", c.Paths[0], err)
		}
		c.Size = size
		cs = append(cs, c)
	}
	return cs, nil
}

// appComponent arma el componente Meet2Do: el ejecutable en uso y, si están en la misma
// carpeta, los demás nombres del programa (meet2do ↔ m2do).
func appComponent(exe string) (Component, bool) {
	c := Component{Name: App}
	paths := []string{exe}
	for _, name := range Commands {
		if p := filepath.Join(filepath.Dir(exe), name); p != exe {
			paths = append(paths, p)
		}
	}
	for _, p := range paths {
		size, err := dirSize(p)
		if err != nil {
			continue // el atajo no está instalado (o no se puede leer): no se ofrece
		}
		c.Paths = append(c.Paths, p)
		c.Size += size
	}
	return c, len(c.Paths) > 0
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

// Remove borra todas las rutas del componente. En Linux se puede borrar el ejecutable que
// está corriendo.
func Remove(c Component) error {
	var errs []error
	for _, p := range c.Paths {
		if err := os.RemoveAll(p); err != nil {
			errs = append(errs, fmt.Errorf("borrar %s: %w", p, err))
		}
	}
	return errors.Join(errs...)
}

// Cleanup borra la carpeta de caché de Meet2Do si quedó vacía.
func Cleanup() {
	if dir, err := transcriber.CacheDir(); err == nil {
		_ = os.Remove(dir) // falla si no está vacía, que es justo lo que queremos
	}
}
