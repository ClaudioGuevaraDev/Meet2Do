package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
)

// Download es un componente que se descarga en la pantalla de instalación.
type Download struct {
	Name   string // p. ej. "whisper.cpp b5454"
	Detail string // para qué sirve
	Size   int64  // tamaño aproximado, para mostrarlo antes de empezar
	// Run descarga el componente; progress informa bytes y phase una etapa posterior
	// (p. ej. "verificando"). Debe respetar la cancelación de ctx.
	Run func(ctx context.Context, progress func(done, total int64), phase func(label string)) error
}

// TotalSize suma los tamaños aproximados de downloads.
func TotalSize(downloads []Download) int64 {
	var total int64
	for _, d := range downloads {
		total += d.Size
	}
	return total
}

// Install muestra la pantalla de instalación y descarga los componentes en orden.
// Con Ctrl+C cancela la descarga en curso y devuelve ErrAborted.
func Install(ctx context.Context, downloads []Download) error {
	if len(downloads) == 0 {
		return nil
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	p := tea.NewProgram(newInstallModel(downloads))
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		for i, d := range downloads {
			r := &reporter{p: p, i: i}
			err := d.Run(ctx, r.progress, func(label string) { p.Send(phaseMsg{i: i, label: label}) })
			p.Send(itemDoneMsg{i: i, err: err})
			if err != nil {
				return
			}
		}
	}()

	final, err := p.Run()
	cancel()
	<-finished // espera a que la descarga cancelada limpie sus archivos parciales
	if err != nil {
		return fmt.Errorf("pantalla de instalación: %w", err)
	}
	m, ok := final.(installModel)
	if !ok {
		return fmt.Errorf("pantalla de instalación: modelo inesperado %T", final)
	}
	return m.err
}

// reporter limita los mensajes de progreso para no saturar la interfaz.
type reporter struct {
	p    *tea.Program
	i    int
	last time.Time
}

func (r *reporter) progress(done, total int64) {
	now := time.Now()
	if done != total && now.Sub(r.last) < 80*time.Millisecond {
		return
	}
	r.last = now
	r.p.Send(progressMsg{i: r.i, done: done, total: total, at: now})
}

type itemState int

const (
	stateWaiting itemState = iota
	stateDownloading
	stateWorking
	stateDone
	stateFailed
)

type installItem struct {
	Download
	state   itemState
	done    int64
	total   int64
	label   string
	started time.Time
	speed   float64 // bytes por segundo
}

type installModel struct {
	items []installItem
	bar   progress.Model
	err   error
}

type progressMsg struct {
	i           int
	done, total int64
	at          time.Time
}

type phaseMsg struct {
	i     int
	label string
}

type itemDoneMsg struct {
	i   int
	err error
}

func newInstallModel(downloads []Download) installModel {
	items := make([]installItem, len(downloads))
	for i, d := range downloads {
		items[i] = installItem{Download: d, total: d.Size}
	}
	return installModel{
		items: items,
		bar:   progress.New(progress.WithSolidFill(string(primary)), progress.WithWidth(32), progress.WithoutPercentage()),
	}
}

func (m installModel) Init() tea.Cmd { return nil }

func (m installModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.err = ErrAborted
			return m, tea.Quit
		}
	case progressMsg:
		it := &m.items[msg.i]
		if it.started.IsZero() {
			it.started = msg.at
		}
		it.state, it.done = stateDownloading, msg.done
		if msg.total > 0 {
			it.total = msg.total
		}
		if secs := msg.at.Sub(it.started).Seconds(); secs > 0 {
			it.speed = float64(msg.done) / secs
		}
	case phaseMsg:
		m.items[msg.i].state, m.items[msg.i].label = stateWorking, msg.label
	case itemDoneMsg:
		it := &m.items[msg.i]
		if msg.err != nil {
			it.state, m.err = stateFailed, msg.err
			return m, tea.Quit
		}
		it.state = stateDone
		if msg.i == len(m.items)-1 {
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m installModel) View() string {
	var b strings.Builder
	b.WriteString("\n" + accentStyle.Render("Instalando componentes de Meet2Do") + Subtle(" · solo la primera vez") + "\n\n")
	for _, it := range m.items {
		b.WriteString(m.itemView(it))
	}
	if m.err == nil {
		b.WriteString(Subtle("\nCtrl+C para cancelar") + "\n")
	}
	return b.String()
}

func (m installModel) itemView(it installItem) string {
	title := fmt.Sprintf("%s %s", it.Name, Subtle("· "+it.Detail))
	switch it.state {
	case stateWaiting:
		return fmt.Sprintf("  %s %s  %s\n", Subtle("○"), title, Subtle(FormatBytes(it.total)))
	case stateDone:
		return fmt.Sprintf("  %s %s  %s\n", successStyle.Render("✔"), title, Subtle(FormatBytes(it.total)))
	case stateFailed:
		return fmt.Sprintf("  %s %s\n", errorStyle.Render("✘"), title)
	case stateWorking:
		return fmt.Sprintf("  %s %s\n    %s %s\n", accentStyle.Render("↓"), title, m.bar.ViewAs(1), Subtle(it.label+"…"))
	}
	pct := 0.0
	if it.total > 0 {
		pct = min(1, float64(it.done)/float64(it.total))
	}
	return fmt.Sprintf("  %s %s\n    %s %3.0f%%  %s / %s · %s\n",
		accentStyle.Render("↓"), title, m.bar.ViewAs(pct), pct*100,
		FormatBytes(it.done), FormatBytes(it.total), FormatSpeed(it.speed))
}

// FormatBytes muestra un tamaño en KB/MB/GB (base 1024).
func FormatBytes(n int64) string {
	const unit = 1024
	switch {
	case n <= 0:
		return "?"
	case n < unit:
		return fmt.Sprintf("%d B", n)
	case n < unit*unit:
		return fmt.Sprintf("%.1f KB", float64(n)/unit)
	case n < unit*unit*unit:
		return fmt.Sprintf("%.1f MB", float64(n)/(unit*unit))
	}
	return fmt.Sprintf("%.2f GB", float64(n)/(unit*unit*unit))
}

// FormatSpeed muestra una velocidad de descarga.
func FormatSpeed(bytesPerSec float64) string {
	if bytesPerSec <= 0 {
		return "calculando…"
	}
	return FormatBytes(int64(bytesPerSec)) + "/s"
}
