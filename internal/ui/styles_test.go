package ui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
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
		{"muted", muted, "#9CA3AF"},
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
