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

type statusMsg string
type doneMsg struct{ err error }

type spinnerModel struct {
	sp    spinner.Model
	title string
	err   error
}

func (m spinnerModel) Init() tea.Cmd { return m.sp.Tick }

func (m spinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case statusMsg:
		m.title = string(msg)
	case doneMsg:
		m.err = msg.err
		return m, tea.Quit
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.err = ErrAborted
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.sp, cmd = m.sp.Update(msg)
	return m, cmd
}

func (m spinnerModel) View() string {
	return fmt.Sprintf("%s %s\n", m.sp.View(), m.title)
}

// Spin ejecuta task mostrando un spinner con title. Al terminar imprime ✔ o ✘.
// Si el usuario presiona Ctrl+C, el ctx que recibe task se cancela.
func Spin(ctx context.Context, title, doneText string, task func(ctx context.Context, status Status) error) error {
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
		if !errors.Is(model.err, ErrAborted) {
			Error(title)
		}
		return model.err
	}
	Success(doneText)
	return nil
}
