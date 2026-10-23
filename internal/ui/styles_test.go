package ui

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/text/width"

	"github.com/ClaudioGuevaraDev/Meet2Do/internal/ai"
	"github.com/ClaudioGuevaraDev/Meet2Do/internal/output"
	"github.com/ClaudioGuevaraDev/Meet2Do/internal/uninstall"
)

func TestPalette(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		color    lipgloss.AdaptiveColor
		wantDark string
	}{
		{"brand", brand, "#3B82F6"},
		{"accent", accent, "#22D3EE"},
		{"success", success, "#22C55E"},
		{"warning", warning, "#F59E0B"},
		{"danger", danger, "#EF4444"},
		{"muted", muted, "#B4BAC4"},
		{"faint", faint, "#8B93A1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.color.Dark != tt.wantDark {
				t.Errorf("dark = %s; want %s", tt.color.Dark, tt.wantDark)
			}
			if tt.color.Light == "" || tt.color.Light == tt.color.Dark {
				t.Errorf("light variant %q should be a distinct color for light terminals", tt.color.Light)
			}
		})
	}
}

func TestThemeColors(t *testing.T) {
	t.Parallel()
	th := theme()
	tests := []struct {
		name string
		got  lipgloss.TerminalColor
		want lipgloss.TerminalColor
	}{
		{"title", th.Focused.Title.GetForeground(), brand},
		{"blurred title", th.Blurred.Title.GetForeground(), brand},
		{"group title", th.Group.Title.GetForeground(), brand},
		{"description", th.Focused.Description.GetForeground(), muted},
		{"select selector", th.Focused.SelectSelector.GetForeground(), accent},
		{"multiselect selector", th.Focused.MultiSelectSelector.GetForeground(), accent},
		{"selected option", th.Focused.SelectedOption.GetForeground(), accent},
		{"selected prefix", th.Focused.SelectedPrefix.GetForeground(), accent},
		{"unselected prefix", th.Focused.UnselectedPrefix.GetForeground(), muted},
		{"focused button", th.Focused.FocusedButton.GetForeground(), accent},
		{"blurred button", th.Focused.BlurredButton.GetForeground(), muted},
		{"input prompt", th.Focused.TextInput.Prompt.GetForeground(), accent},
		{"error message", th.Focused.ErrorMessage.GetForeground(), danger},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.got != tt.want {
				t.Errorf("color = %v; want %v", tt.got, tt.want)
			}
		})
	}
}

func TestNoBackgrounds(t *testing.T) {
	t.Parallel()
	th := theme()
	styles := map[string]lipgloss.Style{
		"title":          th.Focused.Title,
		"description":    th.Focused.Description,
		"focused button": th.Focused.FocusedButton,
		"blurred button": th.Focused.BlurredButton,
		"selected":       th.Focused.SelectedOption,
		"brand":          brandStyle,
		"muted":          mutedStyle,
		"success":        successStyle,
		"warn":           warnStyle,
		"error":          errorStyle,
	}
	for name, s := range styles {
		if _, isNone := s.GetBackground().(lipgloss.NoColor); !isNone {
			t.Errorf("%s should not set a background, got %v", name, s.GetBackground())
		}
	}
}

func TestThemeSpacing(t *testing.T) {
	t.Parallel()
	th, confirm := theme(), confirmTheme()
	tests := []struct {
		name string
		got  int
		want int
	}{
		{"title", th.Focused.Title.GetMarginBottom(), 1},
		{"blurred title", th.Blurred.Title.GetMarginBottom(), 1},
		{"description", th.Focused.Description.GetMarginBottom(), 1},
		{"blurred description", th.Blurred.Description.GetMarginBottom(), 1},
		{"confirm title", confirm.Focused.Title.GetMarginBottom(), 1},
		{"confirm description", confirm.Focused.Description.GetMarginBottom(), 0},
		{"confirm blurred description", confirm.Blurred.Description.GetMarginBottom(), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.got != tt.want {
				t.Errorf("margin bottom = %d; want %d", tt.got, tt.want)
			}
		})
	}
}

var emoji = regexp.MustCompile(`[\x{1F300}-\x{1FAFF}\x{2600}-\x{27BF}]`)

func TestBanner(t *testing.T) {
	t.Parallel()
	got := bannerText("1.2.3")
	for _, want := range []string{logo, "Meet2Do", "v1.2.3", "lista de tareas"} {
		if !strings.Contains(got, want) {
			t.Errorf("banner should contain %q:\n%s", want, got)
		}
	}
	if emoji.MatchString(got) {
		t.Errorf("banner should not use emoji (font-dependent):\n%s", got)
	}
	if lines := strings.Count(strings.TrimSpace(got), "\n") + 1; lines != 2 {
		t.Errorf("banner should be 2 lines, got %d", lines)
	}
}

func TestThemeGlyphs(t *testing.T) {
	t.Parallel()
	th := theme()
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"select selector", th.Focused.SelectSelector.String(), "❯ "},
		{"multiselect selector", th.Focused.MultiSelectSelector.String(), "❯ "},
		{"selected prefix", th.Focused.SelectedPrefix.String(), "[x] "},
		{"unselected prefix", th.Focused.UnselectedPrefix.String(), "[ ] "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ansi.Strip(tt.got); got != tt.want {
				t.Errorf("glyph = %q; want %q", got, tt.want)
			}
		})
	}
	// Sin foco, el selector debe ocupar lo mismo para que las opciones no se muevan.
	if f, b := lipgloss.Width(th.Focused.MultiSelectSelector.String()), lipgloss.Width(th.Blurred.MultiSelectSelector.String()); f != b {
		t.Errorf("blurred selector width = %d; want %d", b, f)
	}
}

func TestKeymapKeepsKeys(t *testing.T) {
	t.Parallel()
	def, k := huh.NewDefaultKeyMap(), keymap()
	tests := []struct {
		name     string
		got      key.Binding
		want     key.Binding
		wantHelp string
	}{
		{"multiselect toggle", k.MultiSelect.Toggle, def.MultiSelect.Toggle, "marcar"},
		{"multiselect next", k.MultiSelect.Next, def.MultiSelect.Next, "continuar"},
		{"multiselect up", k.MultiSelect.Up, def.MultiSelect.Up, "subir"},
		{"multiselect down", k.MultiSelect.Down, def.MultiSelect.Down, "bajar"},
		{"select next", k.Select.Next, def.Select.Next, "elegir"},
		{"confirm toggle", k.Confirm.Toggle, def.Confirm.Toggle, "cambiar"},
		{"input next", k.Input.Next, def.Input.Next, "continuar"},
		{"quit", k.Quit, def.Quit, "salir"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if !slices.Equal(tt.got.Keys(), tt.want.Keys()) {
				t.Errorf("keys = %v; want %v", tt.got.Keys(), tt.want.Keys())
			}
			if tt.got.Enabled() != tt.want.Enabled() {
				t.Errorf("enabled = %v; want %v", tt.got.Enabled(), tt.want.Enabled())
			}
			if got := tt.got.Help().Desc; got != tt.wantHelp {
				t.Errorf("help = %q; want %q", got, tt.wantHelp)
			}
		})
	}
}

func TestOutputLabelAligned(t *testing.T) {
	t.Parallel()
	a := ansi.Strip(outputLabel(output.Tasks, "x"))
	b := ansi.Strip(outputLabel(output.Transcript, "x"))
	if strings.Index(a, "x") != strings.Index(b, "x") {
		t.Errorf("descriptions should be aligned:\n%q\n%q", a, b)
	}
}

func TestFailureText(t *testing.T) {
	t.Parallel()
	got := ansi.Strip(failureText("No se pudo completar la transcripción.", errors.New("boom")))
	want := "✗ No se pudo completar la transcripción.\n  Motivo: boom\n"
	if got != want {
		t.Errorf("failureText = %q; want %q", got, want)
	}
}

func TestDoneText(t *testing.T) {
	t.Parallel()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	files := []string{filepath.Join(cwd, "tareas.md"), filepath.Join(cwd, "transcripcion.md")}
	tests := []struct {
		name    string
		dir     string
		wantDir string // "" = no se muestra la carpeta
	}{
		{"current dir hides folder", cwd, ""},
		{"folder inside current dir is relative", filepath.Join(cwd, "reunion-2026-10-23_13-07"), "en reunion-2026-10-23_13-07/"},
		{"other dir is absolute", "/otra/carpeta", "en /otra/carpeta/"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ansi.Strip(doneText(tt.dir, files))
			for _, w := range []string{"✓ Archivos generados correctamente", "\n  tareas.md\n", "\n  transcripcion.md\n"} {
				if !strings.Contains(got, w) {
					t.Errorf("should contain %q:\n%s", w, got)
				}
			}
			if tt.wantDir == "" && strings.Contains(got, "  en ") {
				t.Errorf("folder should not be shown:\n%s", got)
			}
			if tt.wantDir != "" && !strings.Contains(got, "\n  "+tt.wantDir+"\n") {
				t.Errorf("should show %q:\n%s", tt.wantDir, got)
			}
		})
	}
}

func TestModelOptions(t *testing.T) {
	t.Parallel()
	opts := modelOptions("Claude", ai.Catalog{Models: []ai.Model{{ID: "opus", Name: "Opus"}, {ID: "haiku", Name: "Haiku"}}})
	if len(opts) != 3 {
		t.Fatalf("got %d options; want 3", len(opts))
	}
	if !strings.HasPrefix(ansi.Strip(opts[0].Key), defaultLabel) || opts[0].Value != 0 {
		t.Errorf("first option = %q (%d); want %q (0)", opts[0].Key, opts[0].Value, defaultLabel)
	}
	if got := ansi.Strip(opts[1].Key); !strings.Contains(got, "Opus") || !strings.Contains(got, "opus") || opts[1].Value != 1 {
		t.Errorf("second option = %q (%d)", got, opts[1].Value)
	}
}

func TestEffortOptions(t *testing.T) {
	t.Parallel()
	opts := effortOptions([]string{"low", "weird"}, ai.Setting{})
	tests := []struct {
		name      string
		opt       huh.Option[string]
		wantValue string
		wantKey   string
	}{
		{"default first", opts[0], "", defaultLabel},
		{"known level is described", opts[1], "low", "más rápido"},
		{"unknown level is shown as is", opts[2], "weird", "weird"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.opt.Value != tt.wantValue || !strings.Contains(ansi.Strip(tt.opt.Key), tt.wantKey) {
				t.Errorf("option = %q (%q); want key with %q and value %q", tt.opt.Key, tt.opt.Value, tt.wantKey, tt.wantValue)
			}
		})
	}
	if ansi.Strip(opts[2].Key) != "weird" {
		t.Errorf("unknown level should have no description padding: %q", opts[2].Key)
	}
}

// Regresión: huh ubica el cursor y el scroll al recibir las opciones; la primera fila
// ("Por defecto", con el cursor) debe verse al abrir la pregunta.
func TestSelectShowsDefaultFirst(t *testing.T) {
	t.Parallel()
	cat := ai.Catalog{Models: []ai.Model{{ID: "fable", Name: "Fable"}, {ID: "opus", Name: "Opus"}}}
	var idx int
	var effort string
	tests := []struct {
		name  string
		field huh.Field
	}{
		{"model", modelSelect("Claude", cat, &idx)},
		{"effort", effortSelect([]string{"low", "high"}, ai.Setting{}, &effort)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := huh.NewForm(huh.NewGroup(tt.field)).WithTheme(theme()).WithKeyMap(keymap())
			f.Init()
			view := ansi.Strip(f.View())
			if !strings.Contains(view, "❯ "+defaultLabel) {
				t.Errorf("the cursor should be on %q:\n%s", defaultLabel, view)
			}
		})
	}
}

func TestDefaultOptionShowsResolvedValue(t *testing.T) {
	t.Parallel()
	known := ai.Setting{Value: "Opus", Source: "~/.claude/settings.json"}
	tests := []struct {
		name string
		key  string
		want string
	}{
		{"known model", modelOptions("Claude", ai.Catalog{DefaultModel: known})[0].Key, "Opus · ~/.claude/settings.json"},
		{"unknown model", modelOptions("Claude", ai.Catalog{})[0].Key, "el que elija Claude según tu cuenta"},
		{"known effort", effortOptions(nil, ai.Setting{Value: "low", Source: "predeterminado del modelo"})[0].Key, "low · predeterminado del modelo"},
		{"effort without source", effortOptions(nil, ai.Setting{Value: "high"})[0].Key, "high"},
		{"unknown effort", effortOptions(nil, ai.Setting{})[0].Key, "el que use el modelo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ansi.Strip(tt.key)
			if !strings.HasPrefix(got, defaultLabel) || !strings.HasSuffix(got, tt.want) {
				t.Errorf("default option = %q; want %q … %q", got, defaultLabel, tt.want)
			}
		})
	}
}

// Los íconos de ancho "ambiguo" (◯, ●, ↓…) se dibujan más grandes en muchas fuentes y pisan
// el texto: todos los de la interfaz deben ser de ancho estrecho.
func TestGlyphsAreNarrow(t *testing.T) {
	t.Parallel()
	glyphs := []string{
		glyphSuccess, glyphError, glyphWarn, glyphInfo, glyphCursor, glyphChecked, glyphUnchecked,
		glyphPending, glyphDownload, glyphRec, glyphPause, glyphResume, glyphStop, logo,
	}
	for _, g := range glyphs {
		for _, r := range g {
			switch width.LookupRune(r).Kind() {
			case width.Neutral, width.EastAsianNarrow, width.EastAsianHalfwidth:
			default:
				t.Errorf("glyph %q: %U is not narrow (%v)", g, r, width.LookupRune(r).Kind())
			}
		}
	}
	// Las casillas deben medir lo mismo para que los nombres queden alineados.
	if a, b := lipgloss.Width(glyphChecked), lipgloss.Width(glyphUnchecked); a != b {
		t.Errorf("checkbox widths differ: %d vs %d", a, b)
	}
}

func TestOutputSelectDefaults(t *testing.T) {
	t.Parallel()
	kinds := slices.Clone(defaultOutputs)
	f := huh.NewForm(huh.NewGroup(outputSelect(&kinds))).WithTheme(theme()).WithKeyMap(keymap())
	f.Init()
	// Sin terminal, el formulario no conoce su alto: se lo damos como lo haría bubbletea.
	f.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	view := ansi.Strip(f.View())
	want := []string{"[x] tareas.md", "[x] resumen.md", "[ ] transcripcion.md", "[ ] reunion.wav"}
	last := -1
	for _, w := range want {
		i := strings.Index(view, w)
		if i < 0 {
			t.Fatalf("view should contain %q:\n%s", w, view)
		}
		if i < last {
			t.Errorf("%q is out of order:\n%s", w, view)
		}
		last = i
	}
	if !slices.Equal(kinds, []output.Kind{output.Tasks, output.Summary}) {
		t.Errorf("default kinds = %v", kinds)
	}
}

func TestUninstallSelect(t *testing.T) {
	t.Parallel()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("sin home")
	}
	cs := []uninstall.Component{
		{Name: uninstall.App, Paths: []string{"/usr/local/bin/meet2do", "/usr/local/bin/m2do"}, Size: 10 << 20},
		{Name: uninstall.Whisper, Paths: []string{filepath.Join(home, ".cache/meet2do/whisper")}, Size: 20 << 20},
		{Name: uninstall.Models, Paths: []string{filepath.Join(home, ".cache/meet2do/models")}, Detail: "small", Size: 465 << 20},
	}
	var idx []int
	f := huh.NewForm(huh.NewGroup(uninstallSelect(cs, &idx))).WithTheme(theme()).WithKeyMap(keymap())
	f.Init()
	f.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := ansi.Strip(f.View())
	for _, w := range []string{
		"Tus reuniones guardadas no se borran",
		"[x] Meet2Do", "/usr/local/bin/meet2do, m2do · 10.0 MB",
		"[x] whisper.cpp", "~/.cache/meet2do/whisper · 20.0 MB",
		"[x] Modelos de voz", "~/.cache/meet2do/models (small) · 465.0 MB",
	} {
		if !strings.Contains(view, w) {
			t.Errorf("view should contain %q:\n%s", w, view)
		}
	}
	if !slices.Equal(idx, []int{0, 1, 2}) {
		t.Errorf("all components should be selected by default, got %v", idx)
	}
}
