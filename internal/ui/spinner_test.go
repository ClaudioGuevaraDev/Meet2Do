package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestSpinnerView(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("Transcribiendo reunión ", 10)
	tests := []struct {
		name      string
		msgs      []tea.Msg
		wantEmpty bool
		maxWidth  int
		wantErr   error
	}{
		{name: "running shows title"},
		{name: "narrow terminal truncates", msgs: []tea.Msg{tea.WindowSizeMsg{Width: 20}}, maxWidth: 20},
		{name: "done clears line", msgs: []tea.Msg{doneMsg{}}, wantEmpty: true},
		{name: "failure clears line", msgs: []tea.Msg{doneMsg{err: errBoom}}, wantEmpty: true, wantErr: errBoom},
		{name: "ctrl+c aborts", msgs: []tea.Msg{tea.KeyMsg{Type: tea.KeyCtrlC}}, wantEmpty: true, wantErr: ErrAborted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var m tea.Model = spinnerModel{sp: spinner.New(), title: long}
			for _, msg := range tt.msgs {
				m, _ = m.Update(msg)
			}
			sm, ok := m.(spinnerModel)
			if !ok {
				t.Fatalf("model = %T; want spinnerModel", m)
			}
			view := sm.View()
			if (view == "") != tt.wantEmpty {
				t.Errorf("view = %q; want empty = %v", view, tt.wantEmpty)
			}
			if tt.maxWidth > 0 && lipgloss.Width(view) > tt.maxWidth {
				t.Errorf("view width = %d; want <= %d", lipgloss.Width(view), tt.maxWidth)
			}
			if !errors.Is(sm.err, tt.wantErr) {
				t.Errorf("err = %v; want %v", sm.err, tt.wantErr)
			}
		})
	}
}

var errBoom = errors.New("boom")

func TestFail(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		err      error
		wantStep bool
	}{
		{"nil stays nil", nil, false},
		{"aborted passes through", ErrAborted, false},
		{"canceled passes through", fmt.Errorf("x: %w", context.Canceled), false},
		{"error is wrapped", errBoom, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := Fail("No se pudo.", tt.err)
			var step *StepError
			if errors.As(got, &step) != tt.wantStep {
				t.Fatalf("Fail(%v) = %v; want StepError = %v", tt.err, got, tt.wantStep)
			}
			if !errors.Is(got, tt.err) {
				t.Errorf("Fail should keep errors.Is(%v)", tt.err)
			}
			if tt.wantStep && step.Summary != "No se pudo." {
				t.Errorf("summary = %q", step.Summary)
			}
		})
	}
}
