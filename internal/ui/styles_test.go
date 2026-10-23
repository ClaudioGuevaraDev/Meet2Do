package ui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestThemeUsesPrimaryColor(t *testing.T) {
	t.Parallel()
	th := theme()
	tests := []struct {
		name string
		got  lipgloss.TerminalColor
		want lipgloss.TerminalColor
	}{
		{"title", th.Focused.Title.GetForeground(), primary},
		{"blurred title", th.Blurred.Title.GetForeground(), primary},
		{"group title", th.Group.Title.GetForeground(), primary},
		{"select selector", th.Focused.SelectSelector.GetForeground(), primary},
		{"multiselect selector", th.Focused.MultiSelectSelector.GetForeground(), primary},
		{"focused button", th.Focused.FocusedButton.GetBackground(), primary},
		{"next button", th.Focused.Next.GetBackground(), primary},
		{"input prompt", th.Focused.TextInput.Prompt.GetForeground(), primary},
		{"banner", bannerStyle.GetBackground(), primary},
		{"accent", accentStyle.GetForeground(), primary},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.got != tt.want {
				t.Errorf("color = %v; want %v", tt.got, tt.want)
			}
		})
	}

	// Los colores semánticos de huh no se tocan.
	if th.Focused.SelectedOption.GetForeground() == primary {
		t.Error("selected option should keep its semantic green")
	}
	if th.Focused.ErrorMessage.GetForeground() == primary {
		t.Error("error message should keep its semantic red")
	}
}
