package ui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"

	"github.com/ClaudioGuevaraDev/Meet2Do/internal/ai"
	"github.com/ClaudioGuevaraDev/Meet2Do/internal/output"
)

// ErrAborted indica que el usuario canceló un formulario (Ctrl+C / Esc).
var ErrAborted = huh.ErrUserAborted

func run(f huh.Field) error {
	return huh.NewForm(huh.NewGroup(f)).WithTheme(theme()).Run()
}

// ChooseProvider pregunta qué IA usar entre las disponibles.
func ChooseProvider(providers []ai.Provider) (ai.Provider, error) {
	opts := make([]huh.Option[int], len(providers))
	for i, p := range providers {
		opts[i] = huh.NewOption(p.Name(), i)
	}
	var idx int
	err := run(huh.NewSelect[int]().
		Title("¿Qué IA quieres usar para generar las tareas?").
		Options(opts...).
		Value(&idx))
	if err != nil {
		return nil, err
	}
	return providers[idx], nil
}

// Confirm hace una pregunta de sí/no.
func Confirm(title, desc string, def bool) (bool, error) {
	v := def
	err := run(huh.NewConfirm().
		Title(title).
		Description(desc).
		Affirmative("Sí").
		Negative("No").
		Value(&v))
	return v, err
}

// ChooseOutputs pregunta qué archivos generar.
func ChooseOutputs() ([]output.Kind, error) {
	kinds := []output.Kind{output.Tasks}
	err := run(huh.NewMultiSelect[output.Kind]().
		Title("¿Qué archivos quieres generar?").
		Description("Espacio para marcar, Enter para continuar").
		Options(
			huh.NewOption("tareas.md — tareas generadas por la IA", output.Tasks).Selected(true),
			huh.NewOption("transcripcion.md — texto de la reunión", output.Transcript),
			huh.NewOption("reunion.wav — audio grabado", output.Audio),
		).
		Validate(func(k []output.Kind) error {
			if len(k) == 0 {
				return errors.New("elige al menos un archivo")
			}
			return nil
		}).
		Value(&kinds))
	return kinds, err
}

// ChooseDir pregunta dónde guardar los archivos (por defecto la carpeta actual).
func ChooseDir() (string, error) {
	dir := "."
	err := run(huh.NewInput().
		Title("¿Dónde quieres guardarlos?").
		Description("Deja vacío para usar la carpeta actual").
		Placeholder(".").
		Value(&dir).
		Validate(func(s string) error {
			s = expandHome(strings.TrimSpace(s))
			if s == "" {
				return nil
			}
			if info, err := os.Stat(s); err == nil && !info.IsDir() {
				return errors.New("la ruta existe y no es una carpeta")
			}
			return nil
		}))
	if err != nil {
		return "", err
	}
	dir = expandHome(strings.TrimSpace(dir))
	if dir == "" {
		dir = "."
	}
	return filepath.Abs(dir)
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}
