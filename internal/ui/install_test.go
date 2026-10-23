package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestFormatBytes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   int64
		want string
	}{
		{0, "?"},
		{512, "512 B"},
		{1536, "1.5 KB"},
		{10_364_195, "9.9 MB"},
		{487_601_967, "465.0 MB"},
		{3_095_033_483, "2.88 GB"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			if got := FormatBytes(tt.in); got != tt.want {
				t.Errorf("FormatBytes(%d) = %q; want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestFormatSpeed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   float64
		want string
	}{
		{0, "calculando…"},
		{2 << 20, "2.0 MB/s"},
	}
	for _, tt := range tests {
		if got := FormatSpeed(tt.in); got != tt.want {
			t.Errorf("FormatSpeed(%v) = %q; want %q", tt.in, got, tt.want)
		}
	}
}

func TestTotalSize(t *testing.T) {
	t.Parallel()
	if got := TotalSize([]Download{{Size: 10}, {Size: 5}}); got != 15 {
		t.Errorf("TotalSize = %d; want 15", got)
	}
}

func testDownloads() []Download {
	return []Download{
		{Name: "whisper.cpp b5454", Detail: "motor de transcripción", Size: 10 << 20},
		{Name: "Modelo de voz small", Detail: "reconocimiento de voz", Size: 465 << 20},
	}
}

// step aplica msgs al modelo y devuelve el modelo final y si pidió salir.
func step(t *testing.T, m installModel, msgs ...tea.Msg) (installModel, bool) {
	t.Helper()
	quit := false
	for _, msg := range msgs {
		next, cmd := m.Update(msg)
		var ok bool
		if m, ok = next.(installModel); !ok {
			t.Fatalf("Update returned %T", next)
		}
		if cmd != nil {
			if _, isQuit := cmd().(tea.QuitMsg); isQuit {
				quit = true
			}
		}
	}
	return m, quit
}

func TestInstallModel(t *testing.T) {
	t.Parallel()
	t0 := time.Unix(0, 0)
	boom := errors.New("sin conexión")

	tests := []struct {
		name     string
		msgs     []tea.Msg
		wantQuit bool
		wantErr  error
		want     []string // fragmentos que deben aparecer en la vista
		notWant  []string
	}{
		{
			name: "waiting shows names and sizes",
			want: []string{"Instalando componentes", "whisper.cpp b5454", "motor de transcripción", "Modelo de voz small", "465.0 MB", "Ctrl+C"},
		},
		{
			name: "progress shows percent, bytes and speed",
			msgs: []tea.Msg{
				progressMsg{i: 0, done: 0, total: 10 << 20, at: t0},
				progressMsg{i: 0, done: 5 << 20, total: 10 << 20, at: t0.Add(time.Second)},
			},
			want: []string{"50%", "5.0 MB / 10.0 MB", "5.0 MB/s"},
		},
		{
			name: "phase label",
			msgs: []tea.Msg{phaseMsg{i: 0, label: "Verificando integridad"}},
			want: []string{"Verificando integridad…"},
		},
		{
			name: "first done keeps running",
			msgs: []tea.Msg{itemDoneMsg{i: 0}},
			want: []string{"✓ whisper.cpp"},
		},
		{
			name:     "all done quits",
			msgs:     []tea.Msg{itemDoneMsg{i: 0}, itemDoneMsg{i: 1}},
			wantQuit: true,
			want:     []string{"✓ whisper.cpp", "✓ Modelo de voz"},
		},
		{
			name:     "error quits with error",
			msgs:     []tea.Msg{itemDoneMsg{i: 0, err: boom}},
			wantQuit: true,
			wantErr:  boom,
			want:     []string{"✗ whisper.cpp"},
			notWant:  []string{"Ctrl+C"},
		},
		{
			name:     "ctrl+c aborts",
			msgs:     []tea.Msg{tea.KeyMsg{Type: tea.KeyCtrlC}},
			wantQuit: true,
			wantErr:  ErrAborted,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m, quit := step(t, newInstallModel(testDownloads()), tt.msgs...)
			if quit != tt.wantQuit {
				t.Errorf("quit = %v; want %v", quit, tt.wantQuit)
			}
			if !errors.Is(m.err, tt.wantErr) {
				t.Errorf("err = %v; want %v", m.err, tt.wantErr)
			}
			view := m.View()
			for _, w := range tt.want {
				if !strings.Contains(view, w) {
					t.Errorf("view should contain %q:\n%s", w, view)
				}
			}
			for _, w := range tt.notWant {
				if strings.Contains(view, w) {
					t.Errorf("view should not contain %q:\n%s", w, view)
				}
			}
		})
	}
}

func TestBarWidth(t *testing.T) {
	t.Parallel()
	tests := []struct{ term, want int }{
		{200, maxBarWidth},
		{60, 20},
		{30, minBarWidth},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.term), func(t *testing.T) {
			t.Parallel()
			if got := barWidth(tt.term); got != tt.want {
				t.Errorf("barWidth(%d) = %d; want %d", tt.term, got, tt.want)
			}
		})
	}
}

func TestInstallTitle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, title, note string
		want, notWant     string
	}{
		{"first install", "Instalando componentes de Meet2Do", "solo la primera vez", "Instalando componentes de Meet2Do · solo la primera vez", ""},
		{"update", "Actualizando Meet2Do", "", "Actualizando Meet2Do", "solo la primera vez"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := newInstallModel(testDownloads())
			m.title, m.note = tt.title, tt.note
			view := ansi.Strip(m.View())
			if !strings.Contains(view, tt.want) {
				t.Errorf("view should contain %q:\n%s", tt.want, view)
			}
			if tt.notWant != "" && strings.Contains(view, tt.notWant) {
				t.Errorf("view should not contain %q:\n%s", tt.notWant, view)
			}
		})
	}
}
