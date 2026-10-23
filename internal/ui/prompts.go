package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/charmbracelet/huh"

	"github.com/ClaudioGuevaraDev/Meet2Do/internal/ai"
	"github.com/ClaudioGuevaraDev/Meet2Do/internal/output"
)

// ErrAborted indica que el usuario canceló un formulario (Ctrl+C / Esc).
var ErrAborted = huh.ErrUserAborted

func run(f huh.Field) error {
	th := theme()
	if _, ok := f.(*huh.Confirm); ok {
		th = confirmTheme()
	}
	return huh.NewForm(huh.NewGroup(f)).WithTheme(th).WithKeyMap(keymap()).Run()
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

// defaultLabel es la primera opción de modelo y esfuerzo: no pasa ningún flag al CLI.
const defaultLabel = "Por defecto"

// effortDesc describe en español los niveles de esfuerzo conocidos.
var effortDesc = map[string]string{
	"low":    "más rápido",
	"medium": "equilibrado",
	"high":   "más razonamiento",
	"xhigh":  "razonamiento extra",
	"max":    "máximo razonamiento",
	"ultra":  "máximo, con delegación",
}

// optionLabel alinea name en una columna y atenúa desc, como en el selector de archivos.
func optionLabel(name, desc string) string {
	if desc == "" {
		return name
	}
	return fmt.Sprintf("%-18s", name) + mutedStyle.Render(desc)
}

// settingDesc describe un valor por defecto como "valor · origen", o fallback si no se sabe.
func settingDesc(s ai.Setting, fallback string) string {
	switch {
	case s.Value == "":
		return fallback
	case s.Source == "":
		return s.Value
	}
	return s.Value + " · " + s.Source
}

// modelOptions arma las opciones de ChooseModel: "Por defecto" (0, el valor cero) y luego
// cada modelo como su índice + 1.
func modelOptions(provider string, cat ai.Catalog) []huh.Option[int] {
	desc := settingDesc(cat.DefaultModel, "el que elija "+provider+" según tu cuenta")
	opts := []huh.Option[int]{huh.NewOption(optionLabel(defaultLabel, desc), 0)}
	for i, m := range cat.Models {
		opts = append(opts, huh.NewOption(optionLabel(m.Name, m.ID), i+1))
	}
	return opts
}

// modelSelect arma la pregunta de modelo. Value va antes que Options: huh ubica el cursor
// y el scroll al recibir las opciones, y sin el valor real dejaría oculta la primera fila.
func modelSelect(provider string, cat ai.Catalog, idx *int) *huh.Select[int] {
	return huh.NewSelect[int]().
		Title("¿Qué modelo de " + provider + " quieres usar?").
		Value(idx).
		Options(modelOptions(provider, cat)...)
}

// ChooseModel pregunta qué modelo usar. Devuelve un Model vacío para "Por defecto" o si
// no hay modelos para elegir (en ese caso no pregunta).
func ChooseModel(provider string, cat ai.Catalog) (ai.Model, error) {
	if len(cat.Models) == 0 {
		return ai.Model{}, nil
	}
	var idx int
	if err := run(modelSelect(provider, cat, &idx)); err != nil || idx == 0 {
		return ai.Model{}, err
	}
	return cat.Models[idx-1], nil
}

// effortOptions arma las opciones de ChooseEffort: "Por defecto" ("") y luego cada nivel.
func effortOptions(efforts []string, def ai.Setting) []huh.Option[string] {
	desc := settingDesc(def, "el que use el modelo")
	opts := []huh.Option[string]{huh.NewOption(optionLabel(defaultLabel, desc), "")}
	for _, e := range efforts {
		opts = append(opts, huh.NewOption(optionLabel(e, effortDesc[e]), e))
	}
	return opts
}

// effortSelect arma la pregunta de esfuerzo (Value antes que Options, ver modelSelect).
func effortSelect(efforts []string, def ai.Setting, effort *string) *huh.Select[string] {
	return huh.NewSelect[string]().
		Title("¿Qué nivel de esfuerzo?").
		Value(effort).
		Options(effortOptions(efforts, def)...)
}

// ChooseEffort pregunta el nivel de esfuerzo; def es el que se usa con "Por defecto".
// Devuelve "" para "Por defecto" o si no hay niveles para elegir (en ese caso no pregunta).
func ChooseEffort(efforts []string, def ai.Setting) (string, error) {
	if len(efforts) == 0 {
		return "", nil
	}
	var effort string
	err := run(effortSelect(efforts, def, &effort))
	return effort, err
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
	kinds := slices.Clone(defaultOutputs)
	err := run(outputSelect(&kinds))
	return kinds, err
}

// defaultOutputs son los archivos marcados de entrada: los que genera la IA.
var defaultOutputs = []output.Kind{output.Tasks, output.Summary}

// outputSelect arma la pregunta de archivos, en el orden en que se muestran.
func outputSelect(kinds *[]output.Kind) *huh.MultiSelect[output.Kind] {
	opts := make([]huh.Option[output.Kind], 0, 4)
	for _, o := range []struct {
		kind output.Kind
		desc string
	}{
		{output.Tasks, "Tareas generadas por IA"},
		{output.Summary, "Resumen de la reunión por IA"},
		{output.Transcript, "Texto de la reunión"},
		{output.Audio, "Audio grabado"},
	} {
		opts = append(opts, huh.NewOption(outputLabel(o.kind, o.desc), o.kind).
			Selected(slices.Contains(defaultOutputs, o.kind)))
	}
	return huh.NewMultiSelect[output.Kind]().
		Title("¿Qué archivos quieres generar?").
		Options(opts...).
		Validate(func(k []output.Kind) error {
			if len(k) == 0 {
				return errors.New("elige al menos un archivo")
			}
			return nil
		}).
		Value(kinds)
}

// outputLabel alinea el nombre del archivo en una columna y atenúa su descripción, de modo
// que al marcarlo solo el nombre toma el color de selección.
func outputLabel(k output.Kind, desc string) string {
	return optionLabel(output.FileName(k), desc)
}

// ChooseDir pregunta dónde guardar los archivos (por defecto la carpeta actual).
func ChooseDir() (string, error) {
	dir := "."
	err := run(huh.NewInput().
		Title("¿Dónde quieres guardar la reunión?").
		Description("Se creará una carpeta reunion-<fecha> · vacío = carpeta actual").
		Prompt("› ").
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
