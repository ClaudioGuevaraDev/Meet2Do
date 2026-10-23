// Package ui contiene la interfaz interactiva de la terminal.
package ui

import (
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

var (
	purple = lipgloss.Color("#A78BFA")
	red    = lipgloss.Color("#F87171")
	green  = lipgloss.Color("#34D399")
	gray   = lipgloss.Color("#9CA3AF")

	bannerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(purple).
			Padding(0, 2).
			MarginTop(1).
			MarginBottom(1)
	subtleStyle  = lipgloss.NewStyle().Foreground(gray)
	accentStyle  = lipgloss.NewStyle().Foreground(purple).Bold(true)
	successStyle = lipgloss.NewStyle().Foreground(green).Bold(true)
	errorStyle   = lipgloss.NewStyle().Foreground(red).Bold(true)
	recStyle     = lipgloss.NewStyle().Foreground(red).Bold(true)
	boxStyle     = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(purple).
			Padding(1, 3)
)

func theme() *huh.Theme { return huh.ThemeCharm() }

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
