package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/stopwatch"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	recDotStyle   = lipgloss.NewStyle().Foreground(danger)
	pausedStyle   = lipgloss.NewStyle().Foreground(warning).Bold(true)
	recTitleStyle = lipgloss.NewStyle().Bold(true)
)

// Pauser pausa y retoma la grabación (lo implementa *recorder.Recorder).
type Pauser interface {
	Pause() error
	Resume() error
}

// Botones de la pantalla de grabación.
const (
	btnPause = iota // Pausar / Continuar
	btnStop         // Terminar
	numButtons
)

type recordingModel struct {
	ctl    Pauser
	sw     stopwatch.Model
	blink  bool
	paused bool
	focus  int
	done   bool
	err    error
}

type blinkMsg struct{}

func blink() tea.Cmd {
	return tea.Tick(600*time.Millisecond, func(time.Time) tea.Msg { return blinkMsg{} })
}

func newRecordingModel(ctl Pauser) recordingModel {
	return recordingModel{ctl: ctl, sw: stopwatch.NewWithInterval(time.Second)}
}

func (m recordingModel) Init() tea.Cmd { return tea.Batch(m.sw.Init(), blink()) }

func (m recordingModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "left", "h", "shift+tab":
			m.focus = (m.focus + numButtons - 1) % numButtons
		case "right", "l", "tab":
			m.focus = (m.focus + 1) % numButtons
		case "enter", " ":
			if m.focus == btnPause {
				return m.togglePause()
			}
			return m.finish(nil)
		case "p":
			return m.togglePause()
		case "t", "q", "esc", "ctrl+c":
			// Ctrl+C también termina guardando: así no se pierde la reunión por accidente.
			return m.finish(nil)
		}
		return m, nil
	case blinkMsg:
		m.blink = !m.blink
		return m, blink()
	}
	var cmd tea.Cmd
	m.sw, cmd = m.sw.Update(msg)
	return m, cmd
}

// togglePause pausa o retoma la grabación y el cronómetro, que solo cuenta lo grabado.
func (m recordingModel) togglePause() (tea.Model, tea.Cmd) {
	if m.paused {
		if err := m.ctl.Resume(); err != nil {
			return m.finish(err)
		}
		m.paused = false
		return m, m.sw.Start()
	}
	if err := m.ctl.Pause(); err != nil {
		return m.finish(err)
	}
	m.paused = true
	return m, m.sw.Stop()
}

func (m recordingModel) finish(err error) (tea.Model, tea.Cmd) {
	m.done, m.err = true, err
	return m, tea.Quit
}

// View deja de dibujarse al terminar: la reemplaza la línea "Grabación completada".
func (m recordingModel) View() string {
	if m.done {
		return ""
	}
	elapsed := m.sw.Elapsed().Truncate(time.Second)
	h, mi, s := int(elapsed.Hours()), int(elapsed.Minutes())%60, int(elapsed.Seconds())%60
	clock := brandStyle.Render(fmt.Sprintf("%02d:%02d:%02d", h, mi, s))

	status := recDotStyle.Render("●") + " " + recTitleStyle.Render("Grabando")
	if m.blink {
		status = "  " + recTitleStyle.Render("Grabando")
	}
	pauseLabel := "‖ Pausar"
	if m.paused {
		status = pausedStyle.Render("‖ En pausa")
		pauseLabel = "▶ Continuar"
	}

	th := theme()
	buttons := make([]string, numButtons)
	for i, label := range []string{pauseLabel, "■ Terminar"} {
		style := th.Focused.BlurredButton
		if i == m.focus {
			style = th.Focused.FocusedButton
		}
		buttons[i] = style.Render(label)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "\n%s  %s\n  %s\n\n", status, clock, Subtle("Micrófono + audio del sistema"))
	b.WriteString(" " + lipgloss.JoinHorizontal(lipgloss.Top, buttons...) + "\n\n")
	b.WriteString(m.help() + "\n")
	return b.String()
}

// help arma la línea de ayuda con los mismos estilos que la de las preguntas.
func (m recordingModel) help() string {
	pause := "pausar"
	if m.paused {
		pause = "continuar"
	}
	h := help.New()
	h.Styles = theme().Help
	return h.ShortHelpView([]key.Binding{
		key.NewBinding(key.WithKeys("left", "right"), key.WithHelp("←/→", "mover")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "elegir")),
		key.NewBinding(key.WithKeys("p"), key.WithHelp("p", pause)),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "terminar")),
	})
}

// RecordingScreen muestra el cronómetro con los botones Pausar/Continuar y Terminar hasta
// que el usuario termina; devuelve el tiempo grabado (sin las pausas).
func RecordingScreen(ctl Pauser) (time.Duration, error) {
	final, err := tea.NewProgram(newRecordingModel(ctl)).Run()
	if err != nil {
		return 0, fmt.Errorf("pantalla de grabación: %w", err)
	}
	model, ok := final.(recordingModel)
	if !ok {
		return 0, fmt.Errorf("pantalla de grabación: modelo inesperado %T", final)
	}
	return model.sw.Elapsed(), Fail("No se pudo pausar o retomar la grabación.", model.err)
}
