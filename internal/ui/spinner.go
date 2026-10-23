package ui

import (
	"context"
	"errors"
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Status permite a una tarea actualizar el texto del spinner.
type Status func(msg string)

// StepError es un error de un paso del flujo: Summary dice qué no se pudo hacer (para el
// usuario) y Err es el motivo.
type StepError struct {
	Summary string
	Err     error
}

func (e *StepError) Error() string { return e.Summary + ": " + e.Err.Error() }
func (e *StepError) Unwrap() error { return e.Err }

// Fail envuelve err en un StepError con summary. Deja pasar nil y las cancelaciones tal
// cual, para que no se muestren como fallos.
func Fail(summary string, err error) error {
	if err == nil || errors.Is(err, ErrAborted) || errors.Is(err, context.Canceled) {
		return err
	}
	return &StepError{Summary: summary, Err: err}
}

type statusMsg string
type doneMsg struct{ err error }

type spinnerModel struct {
	sp    spinner.Model
	title string
	width int
	done  bool
	err   error
}

func (m spinnerModel) Init() tea.Cmd { return m.sp.Tick }

func (m spinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case statusMsg:
		m.title = string(msg)
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case doneMsg:
		m.done, m.err = true, msg.err
		return m, tea.Quit
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.done, m.err = true, ErrAborted
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.sp, cmd = m.sp.Update(msg)
	return m, cmd
}

// View deja de dibujarse al terminar: la línea del spinner la reemplaza la de resultado.
// Se recorta al ancho de la terminal porque una línea partida se repetiría al redibujar.
func (m spinnerModel) View() string {
	if m.done {
		return ""
	}
	line := m.sp.View() + m.title // los frames de spinner.Dot ya terminan en espacio
	if m.width > 0 {
		line = lipgloss.NewStyle().MaxWidth(m.width).Render(line)
	}
	return line + "\n"
}

// Spin ejecuta task mostrando un spinner con title. Si termina bien imprime ✓ doneText;
// si falla devuelve el error envuelto con failText (sin imprimirlo, lo hace quien lo recibe).
// Si el usuario presiona Ctrl+C, el ctx que recibe task se cancela y devuelve ErrAborted.
func Spin(ctx context.Context, title, doneText, failText string, task func(ctx context.Context, status Status) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sp := spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(lipgloss.NewStyle().Foreground(brand)))
	p := tea.NewProgram(spinnerModel{sp: sp, title: title}, tea.WithContext(ctx))
	go func() {
		err := task(ctx, func(s string) { p.Send(statusMsg(s)) })
		p.Send(doneMsg{err: err})
	}()

	final, err := p.Run()
	if err != nil {
		return fmt.Errorf("spinner: %w", err)
	}
	model, ok := final.(spinnerModel)
	if !ok {
		return fmt.Errorf("spinner: modelo inesperado %T", final)
	}
	if model.err != nil {
		return Fail(failText, model.err)
	}
	Success(doneText)
	return nil
}
