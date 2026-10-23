package ui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
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
func Spin(title, doneMsgText string, task func(status Status) error) error {
	sp := spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(accentStyle))
	p := tea.NewProgram(spinnerModel{sp: sp, title: title})
	go func() {
		err := task(func(s string) { p.Send(statusMsg(s)) })
		p.Send(doneMsg{err: err})
	}()
	final, err := p.Run()
	if err != nil {
		return err
	}
	if err := final.(spinnerModel).err; err != nil {
		if err != ErrAborted {
			Error(title)
		}
		return err
	}
	Success(doneMsgText)
	return nil
}
