// Package ai genera la lista de tareas usando el CLI de Claude o de Codex.
package ai

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrNoProvider indica que no hay ningún CLI de IA instalado.
var ErrNoProvider = errors.New("no se encontró `claude` ni `codex` en el PATH")

// Provider genera documentos en Markdown (tareas, resumen) a partir de una transcripción.
type Provider interface {
	Name() string
	Binary() string
	// Label es Name más el modelo y el esfuerzo elegidos, si no son los por defecto.
	Label() string
	// Catalog devuelve los modelos que se pueden elegir y lo que el CLI usa por defecto.
	Catalog(ctx context.Context) Catalog
	// With devuelve una copia que usa model y effort ("" = el por defecto del CLI).
	With(model, effort string) Provider
	// Generate ejecuta prompt (TasksPrompt, SummaryPrompt…) sobre transcript.
	Generate(ctx context.Context, prompt, transcript string) (string, error)
}

// Model es un modelo que se puede elegir y los niveles de esfuerzo que admite.
type Model struct {
	ID            string
	Name          string
	Efforts       []string
	DefaultEffort string // el que usa el modelo si no se elige otro; "" si no se sabe
}

// Setting es un valor que el CLI usa por defecto y de dónde sale. Value vacío = no se sabe.
type Setting struct {
	Value  string
	Source string
}

// Catalog es lo que se puede elegir de un proveedor y lo que usa si no se elige nada.
type Catalog struct {
	Models         []Model
	DefaultModel   Setting  // modelo que usa el CLI sin --model
	Effort         Setting  // esfuerzo configurado en el CLI (para cualquier modelo)
	DefaultEfforts []string // esfuerzos que se ofrecen con el modelo por defecto
	defaultID      string   // ID del modelo por defecto, si está en Models
}

// DefaultEffort es el esfuerzo que se usará con m si no se elige uno: el configurado en el
// CLI o, si no hay, el del modelo (el modelo por defecto si m está vacío).
func (c Catalog) DefaultEffort(m Model) Setting {
	if c.Effort.Value != "" {
		return c.Effort
	}
	if m.ID == "" {
		for _, dm := range c.Models {
			if dm.ID == c.defaultID {
				m = dm
				break
			}
		}
	}
	if m.DefaultEffort == "" {
		return Setting{}
	}
	return Setting{Value: m.DefaultEffort, Source: "predeterminado del modelo"}
}

// label arma "Nombre (modelo · esfuerzo)" omitiendo lo que esté por defecto.
func label(name, model, effort string) string {
	var parts []string
	for _, p := range []string{model, effort} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return name
	}
	return name + " (" + strings.Join(parts, " · ") + ")"
}

// All devuelve los proveedores soportados.
func All() []Provider { return []Provider{Claude{}, Codex{}} }

// Available devuelve los proveedores cuyo CLI está en el PATH, o ErrNoProvider.
func Available() ([]Provider, error) {
	all := All()
	out := make([]Provider, 0, len(all))
	for _, p := range all {
		if _, err := exec.LookPath(p.Binary()); err == nil {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil, ErrNoProvider
	}
	return out, nil
}

// Claude usa `claude -p`. Model y Effort vacíos usan la configuración del CLI.
type Claude struct {
	Model  string
	Effort string
}

func (Claude) Name() string    { return "Claude" }
func (Claude) Binary() string  { return "claude" }
func (c Claude) Label() string { return label(c.Name(), c.Model, c.Effort) }
func (c Claude) With(model, effort string) Provider {
	c.Model, c.Effort = model, effort
	return c
}

func (c Claude) Generate(ctx context.Context, prompt, transcript string) (string, error) {
	args := []string{"-p", prompt}
	if c.Model != "" {
		args = append(args, "--model", c.Model)
	}
	if c.Effort != "" {
		args = append(args, "--effort", c.Effort)
	}
	cmd := exec.CommandContext(ctx, c.Binary(), args...)
	cmd.Stdin = strings.NewReader(transcript)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("claude falló: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return clean(stdout.String()), nil
}

// Codex usa `codex exec` y lee la respuesta final desde --output-last-message. Model y
// Effort vacíos usan la configuración del CLI.
type Codex struct {
	Model  string
	Effort string
}

func (Codex) Name() string    { return "Codex" }
func (Codex) Binary() string  { return "codex" }
func (c Codex) Label() string { return label(c.Name(), c.Model, c.Effort) }
func (c Codex) With(model, effort string) Provider {
	c.Model, c.Effort = model, effort
	return c
}

func (c Codex) Generate(ctx context.Context, prompt, transcript string) (string, error) {
	dir, err := os.MkdirTemp("", "meet2do-codex-")
	if err != nil {
		return "", fmt.Errorf("crear carpeta temporal: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }() // limpieza best-effort
	out := filepath.Join(dir, "respuesta.md")

	args := []string{"exec",
		"--skip-git-repo-check", "--ephemeral",
		"--sandbox", "read-only",
		"-C", dir,
		"--output-last-message", out}
	if c.Model != "" {
		args = append(args, "-m", c.Model)
	}
	if c.Effort != "" {
		args = append(args, "-c", fmt.Sprintf("model_reasoning_effort=%q", c.Effort))
	}
	cmd := exec.CommandContext(ctx, c.Binary(), append(args, "-")...)
	cmd.Stdin = strings.NewReader(prompt + transcript)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err = cmd.Run(); err != nil {
		return "", fmt.Errorf("codex falló: %w: %s", err, lastLines(stderr.String(), 10))
	}
	data, err := os.ReadFile(out)
	if err != nil {
		return "", fmt.Errorf("codex no devolvió respuesta: %w", err)
	}
	return clean(string(data)), nil
}

// clean quita espacios y un posible bloque ```markdown envolvente.
func clean(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") && strings.HasSuffix(s, "```") {
		if i := strings.Index(s, "\n"); i >= 0 {
			s = strings.TrimSpace(strings.TrimSuffix(s[i+1:], "```"))
		}
	}
	return s + "\n"
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
