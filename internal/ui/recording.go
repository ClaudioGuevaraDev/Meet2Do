package ui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/stopwatch"
	tea "github.com/charmbracelet/bubbletea"
)

type recordingModel struct {
	sw    stopwatch.Model
	blink bool
}

type blinkMsg struct{}

func blink() tea.Cmd {
	return tea.Tick(600*time.Millisecond, func(time.Time) tea.Msg { return blinkMsg{} })
}

func (m recordingModel) Init() tea.Cmd { return tea.Batch(m.sw.Init(), blink()) }

func (m recordingModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter", "q", "esc", "ctrl+c", " ":
			return m, tea.Quit
		}
	case blinkMsg:
		m.blink = !m.blink
		return m, blink()
	}
	var cmd tea.Cmd
	m.sw, cmd = m.sw.Update(msg)
	return m, cmd
}

func (m recordingModel) View() string {
	dot := "●"
	if m.blink {
		dot = " "
	}
	elapsed := m.sw.Elapsed().Truncate(time.Second)
	h, mi, s := int(elapsed.Hours()), int(elapsed.Minutes())%60, int(elapsed.Seconds())%60
	body := fmt.Sprintf("%s %s  %s\n\n%s",
		recStyle.Render(dot),
		recStyle.Render("GRABANDO"),
		accentStyle.Render(fmt.Sprintf("%02d:%02d:%02d", h, mi, s)),
		Subtle("Micrófono + audio del sistema\nPresiona Enter o q para terminar"))
	return "\n" + boxStyle.Render(body) + "\n"
}

// RecordingScreen muestra el cronómetro hasta que el usuario termina; devuelve la duración.
func RecordingScreen() (time.Duration, error) {
	m := recordingModel{sw: stopwatch.NewWithInterval(time.Second)}
	final, err := tea.NewProgram(m).Run()
	if err != nil {
		return 0, err
	}
	return final.(recordingModel).sw.Elapsed(), nil
}
