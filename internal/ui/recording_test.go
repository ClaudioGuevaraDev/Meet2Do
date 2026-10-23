package ui

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// fakePauser registra las llamadas y puede fallar.
type fakePauser struct {
	calls []string
	err   error
}

func (f *fakePauser) Pause() error  { f.calls = append(f.calls, "pause"); return f.err }
func (f *fakePauser) Resume() error { f.calls = append(f.calls, "resume"); return f.err }

func keyMsg(s string) tea.Msg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestRecordingScreen(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	tests := []struct {
		name      string
		keys      []string
		pauseErr  error
		wantCalls string
		wantQuit  bool
		wantErr   error
		want      []string // en la vista (si no terminó)
	}{
		{name: "starts recording with pause focused", want: []string{"Grabando", "‖ Pausar", "■ Terminar", "←/→ mover", "p pausar"}},
		{name: "enter on pause pauses", keys: []string{"enter"}, wantCalls: "pause", want: []string{"En pausa", "▶ Continuar", "p continuar"}},
		{name: "enter again resumes", keys: []string{"enter", "enter"}, wantCalls: "pause,resume", want: []string{"Grabando", "‖ Pausar"}},
		{name: "p toggles from any focus", keys: []string{"right", "p"}, wantCalls: "pause", want: []string{"En pausa"}},
		{name: "arrows wrap around", keys: []string{"left", "left", "enter"}, wantCalls: "pause"},
		{name: "enter on stop finishes", keys: []string{"right", "enter"}, wantQuit: true},
		{name: "stop while paused", keys: []string{"enter", "right", "enter"}, wantCalls: "pause", wantQuit: true},
		{name: "t finishes", keys: []string{"t"}, wantQuit: true},
		{name: "esc finishes", keys: []string{"esc"}, wantQuit: true},
		{name: "ctrl+c finishes keeping the audio", keys: []string{"ctrl+c"}, wantQuit: true},
		{name: "pause error finishes with it", keys: []string{"enter"}, pauseErr: boom, wantCalls: "pause", wantQuit: true, wantErr: boom},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctl := &fakePauser{err: tt.pauseErr}
			var m tea.Model = newRecordingModel(ctl)
			quit := false
			for _, k := range tt.keys {
				var cmd tea.Cmd
				m, cmd = m.Update(keyMsg(k))
				if cmd != nil {
					if _, ok := cmd().(tea.QuitMsg); ok {
						quit = true
					}
				}
			}
			rm, ok := m.(recordingModel)
			if !ok {
				t.Fatalf("model = %T", m)
			}
			if got := strings.Join(ctl.calls, ","); got != tt.wantCalls {
				t.Errorf("calls = %q; want %q", got, tt.wantCalls)
			}
			if quit != tt.wantQuit || rm.done != tt.wantQuit {
				t.Errorf("quit = %v, done = %v; want %v", quit, rm.done, tt.wantQuit)
			}
			if !errors.Is(rm.err, tt.wantErr) {
				t.Errorf("err = %v; want %v", rm.err, tt.wantErr)
			}
			view := ansi.Strip(rm.View())
			if tt.wantQuit && view != "" {
				t.Errorf("view after finishing = %q; want empty", view)
			}
			for _, w := range tt.want {
				if !strings.Contains(view, w) {
					t.Errorf("view should contain %q:\n%s", w, view)
				}
			}
		})
	}
}

func TestRecordingStopwatchPauses(t *testing.T) {
	t.Parallel()
	var m tea.Model = newRecordingModel(&fakePauser{})
	m, cmd := m.Update(keyMsg("p"))
	if cmd == nil {
		t.Fatal("pausing should stop the stopwatch")
	}
	m, _ = m.Update(firstMsg(cmd)) // aplica el StartStopMsg del cronómetro
	if running(t, m) {
		t.Error("stopwatch should not run while paused")
	}
	m, cmd = m.Update(keyMsg("p"))
	m, _ = m.Update(firstMsg(cmd))
	if !running(t, m) {
		t.Error("stopwatch should run again after resuming")
	}
}

// firstMsg ejecuta cmd; si es una tea.Sequence (un slice de Cmd no exportado), ejecuta
// solo el primero, que en el cronómetro es el StartStopMsg.
func firstMsg(cmd tea.Cmd) tea.Msg {
	msg := cmd()
	if v := reflect.ValueOf(msg); v.Kind() == reflect.Slice && v.Len() > 0 {
		if first, ok := v.Index(0).Interface().(tea.Cmd); ok {
			return first()
		}
	}
	return msg
}

func running(t *testing.T, m tea.Model) bool {
	t.Helper()
	rm, ok := m.(recordingModel)
	if !ok {
		t.Fatalf("model = %T", m)
	}
	return rm.sw.Running()
}
