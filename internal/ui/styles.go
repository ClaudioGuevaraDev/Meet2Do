// Package ui contiene la interfaz interactiva de la terminal.
package ui

import (
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

var (
	// primary es el color de marca de Meet2Do.
	primary = lipgloss.Color("#3B82F6")
	red     = lipgloss.Color("#F87171")
	green   = lipgloss.Color("#34D399")
	gray    = lipgloss.Color("#9CA3AF")

	bannerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(primary).
			Padding(0, 2).
			MarginTop(1).
			MarginBottom(1)
	subtleStyle  = lipgloss.NewStyle().Foreground(gray)
	accentStyle  = lipgloss.NewStyle().Foreground(primary).Bold(true)
	successStyle = lipgloss.NewStyle().Foreground(green).Bold(true)
	errorStyle   = lipgloss.NewStyle().Foreground(red).Bold(true)
	recStyle     = lipgloss.NewStyle().Foreground(red).Bold(true)
	boxStyle     = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(primary).
			Padding(1, 3)
)

// theme es el tema Charm de huh con el color primario en lugar de su índigo y fucsia.
// Verde (seleccionado) y rojo (errores) se mantienen como colores semánticos.
func theme() *huh.Theme {
	t := huh.ThemeCharm()
	f := &t.Focused
	// Una línea en blanco entre la pregunta, su descripción y las opciones.
	f.Title = f.Title.Foreground(primary).MarginBottom(1)
	f.Description = f.Description.MarginBottom(1)
	f.NoteTitle = f.NoteTitle.Foreground(primary)
	f.Directory = f.Directory.Foreground(primary)
	f.SelectSelector = f.SelectSelector.Foreground(primary)
	f.MultiSelectSelector = f.MultiSelectSelector.Foreground(primary)
	f.NextIndicator = f.NextIndicator.Foreground(primary)
	f.PrevIndicator = f.PrevIndicator.Foreground(primary)
	f.FocusedButton = f.FocusedButton.Background(primary)
	f.Next = f.FocusedButton
	f.TextInput.Prompt = f.TextInput.Prompt.Foreground(primary)

	// Igual que ThemeCharm: el estado sin foco hereda del enfocado sin borde ni indicadores.
	b := t.Blurred
	t.Blurred = t.Focused
	t.Blurred.Base, t.Blurred.Card = b.Base, b.Card
	t.Blurred.NextIndicator, t.Blurred.PrevIndicator = b.NextIndicator, b.PrevIndicator
	t.Group.Title = t.Focused.Title
	t.Group.Description = t.Focused.Description
	return t
}

// confirmTheme es theme() sin el margen de la descripción: Confirm ya deja una línea en
// blanco antes de sus botones.
func confirmTheme() *huh.Theme {
	t := theme()
	t.Focused.Description = t.Focused.Description.MarginBottom(0)
	t.Blurred.Description = t.Blurred.Description.MarginBottom(0)
	return t
}

// Banner imprime el encabezado de la aplicación.
func Banner() {
	fmt.Println(bannerStyle.Render("🎙  Meet2Do"))
	fmt.Println(subtleStyle.Render("  Graba tu reunión y conviértela en una lista de tareas\n"))
}

// Success imprime una línea de éxito.
func Success(msg string) { fmt.Println(successStyle.Render("✔ ") + msg) }

// Error imprime una línea de error.
func Error(msg string) { fmt.Println(errorStyle.Render("✘ ") + msg) }

// Info imprime una línea informativa.
func Info(msg string) { fmt.Println(accentStyle.Render("• ") + msg) }

// Subtle formatea texto secundario.
func Subtle(s string) string { return subtleStyle.Render(s) }
