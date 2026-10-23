// Package ui contiene la interfaz interactiva de la terminal.
package ui

import (
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// Paleta de Meet2Do. Dark es el color de marca (terminales oscuras); Light es un tono más
// oscuro de la misma familia para que se lea sobre fondo claro. Ningún estilo define fondo:
// se respeta el de la terminal, y el texto principal usa su color por defecto.
var (
	brand   = lipgloss.AdaptiveColor{Dark: "#3B82F6", Light: "#2563EB"} // marca, encabezado, preguntas
	accent  = lipgloss.AdaptiveColor{Dark: "#22D3EE", Light: "#0891B2"} // selección e interacción
	success = lipgloss.AdaptiveColor{Dark: "#22C55E", Light: "#16A34A"}
	warning = lipgloss.AdaptiveColor{Dark: "#F59E0B", Light: "#B45309"}
	danger  = lipgloss.AdaptiveColor{Dark: "#EF4444", Light: "#DC2626"}
	muted   = lipgloss.AdaptiveColor{Dark: "#9CA3AF", Light: "#6B7280"} // texto secundario
	faint   = lipgloss.AdaptiveColor{Dark: "#6B7280", Light: "#9CA3AF"} // aún más secundario

	brandStyle   = lipgloss.NewStyle().Foreground(brand).Bold(true)
	mutedStyle   = lipgloss.NewStyle().Foreground(muted)
	successStyle = lipgloss.NewStyle().Foreground(success).Bold(true)
	warnStyle    = lipgloss.NewStyle().Foreground(warning).Bold(true)
	errorStyle   = lipgloss.NewStyle().Foreground(danger).Bold(true)
)

// logo es el símbolo de grabación del encabezado: un glifo de texto, no un emoji, para que
// se vea igual en cualquier fuente.
const logo = "◉"

// theme es el tema de las preguntas: sin bordes ni fondos, pregunta en el color de marca,
// selección en el color de interacción y textos secundarios atenuados.
func theme() *huh.Theme {
	t := huh.ThemeBase()

	f := &t.Focused
	f.Base = f.Base.BorderStyle(lipgloss.HiddenBorder())
	f.Card = f.Base
	// Una línea en blanco entre la pregunta, su descripción y las opciones.
	f.Title = lipgloss.NewStyle().Foreground(brand).Bold(true).MarginBottom(1)
	f.NoteTitle = f.Title
	f.Description = lipgloss.NewStyle().Foreground(muted).MarginBottom(1)
	f.ErrorIndicator = lipgloss.NewStyle().Foreground(danger).SetString(" *")
	f.ErrorMessage = lipgloss.NewStyle().Foreground(danger)
	f.Directory = lipgloss.NewStyle().Foreground(brand)
	f.File = lipgloss.NewStyle()

	f.SelectSelector = lipgloss.NewStyle().Foreground(accent).Bold(true).SetString("› ")
	f.MultiSelectSelector = f.SelectSelector
	f.Option = lipgloss.NewStyle()
	f.NextIndicator = lipgloss.NewStyle().Foreground(accent).MarginLeft(1).SetString("→")
	f.PrevIndicator = lipgloss.NewStyle().Foreground(accent).MarginRight(1).SetString("←")
	f.SelectedOption = lipgloss.NewStyle().Foreground(accent)
	f.SelectedPrefix = lipgloss.NewStyle().Foreground(accent).SetString("✓ ")
	f.UnselectedOption = lipgloss.NewStyle()
	f.UnselectedPrefix = lipgloss.NewStyle().Foreground(muted).SetString("○ ")

	f.TextInput.Prompt = lipgloss.NewStyle().Foreground(accent).Bold(true)
	f.TextInput.Cursor = lipgloss.NewStyle().Foreground(accent)
	f.TextInput.Placeholder = lipgloss.NewStyle().Foreground(faint)
	f.TextInput.Text = lipgloss.NewStyle()

	button := lipgloss.NewStyle().Padding(0, 1).MarginRight(2)
	f.FocusedButton = button.Foreground(accent).Bold(true)
	f.BlurredButton = button.Foreground(muted)
	f.Next = f.FocusedButton

	// Sin foco: igual que con foco, pero sin selector ni indicadores.
	t.Blurred = t.Focused
	t.Blurred.MultiSelectSelector = lipgloss.NewStyle().SetString("  ")
	t.Blurred.NextIndicator = lipgloss.NewStyle()
	t.Blurred.PrevIndicator = lipgloss.NewStyle()

	t.Group.Title = f.Title
	t.Group.Description = f.Description

	t.Help.ShortKey = lipgloss.NewStyle().Foreground(muted)
	t.Help.ShortDesc = lipgloss.NewStyle().Foreground(faint)
	t.Help.ShortSeparator = lipgloss.NewStyle().Foreground(faint)
	t.Help.FullKey = t.Help.ShortKey
	t.Help.FullDesc = t.Help.ShortDesc
	t.Help.FullSeparator = t.Help.ShortSeparator
	t.Help.Ellipsis = t.Help.ShortSeparator
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

// bannerText arma el encabezado: logo, nombre y versión, y el subtítulo debajo.
func bannerText(version string) string {
	return fmt.Sprintf("\n%s %s %s\n  %s\n",
		brandStyle.Render(logo),
		brandStyle.Render("Meet2Do"),
		mutedStyle.Render("v"+version),
		mutedStyle.Render("Graba tu reunión y conviértela en una lista de tareas"))
}

// Banner imprime el encabezado de la aplicación.
func Banner(version string) { fmt.Println(bannerText(version)) }

// Success imprime una línea de éxito.
func Success(msg string) { fmt.Println(successStyle.Render("✔") + " " + msg) }

// Warn imprime una advertencia: algo salió distinto de lo esperado, pero el flujo sigue.
func Warn(msg string) { fmt.Println(warnStyle.Render("!") + " " + msg) }

// Error imprime una línea de error.
func Error(msg string) { fmt.Println(errorStyle.Render("✘") + " " + msg) }

// Info imprime una línea informativa.
func Info(msg string) { fmt.Println(brandStyle.Render("›") + " " + msg) }

// Subtle formatea texto secundario.
func Subtle(s string) string { return mutedStyle.Render(s) }

// hex devuelve la variante de c que corresponde al fondo de la terminal, para componentes
// que solo aceptan un color fijo (como la barra de progreso).
func hex(c lipgloss.AdaptiveColor) string {
	if lipgloss.HasDarkBackground() {
		return c.Dark
	}
	return c.Light
}
