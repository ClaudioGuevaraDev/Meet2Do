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
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ClaudioGuevaraDev/Meet2Do/internal/output"
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
		{"selected prefix", th.Focused.SelectedPrefix.String(), "◉ "},
		{"unselected prefix", th.Focused.UnselectedPrefix.String(), "◯ "},
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
		wantDir bool
	}{
		{"current dir hides folder", cwd, false},
		{"other dir shows folder", "/otra/carpeta", true},
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
			if strings.Contains(got, "en "+tt.dir) != tt.wantDir {
				t.Errorf("folder shown = %v; want %v:\n%s", !tt.wantDir, tt.wantDir, got)
			}
		})
	}
}
