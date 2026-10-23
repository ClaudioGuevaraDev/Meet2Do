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

// Provider genera una lista de tareas en Markdown a partir de una transcripción.
type Provider interface {
	Name() string
	Binary() string
	GenerateTasks(ctx context.Context, transcript string) (string, error)
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

// Claude usa `claude -p`.
type Claude struct{}

func (Claude) Name() string   { return "Claude" }
func (Claude) Binary() string { return "claude" }

func (c Claude) GenerateTasks(ctx context.Context, transcript string) (string, error) {
	cmd := exec.CommandContext(ctx, c.Binary(), "-p", TasksPrompt)
	cmd.Stdin = strings.NewReader(transcript)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("claude falló: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return clean(stdout.String()), nil
}

// Codex usa `codex exec` y lee la respuesta final desde --output-last-message.
type Codex struct{}

func (Codex) Name() string   { return "Codex" }
func (Codex) Binary() string { return "codex" }

func (c Codex) GenerateTasks(ctx context.Context, transcript string) (string, error) {
	dir, err := os.MkdirTemp("", "meet2do-codex-")
	if err != nil {
		return "", fmt.Errorf("crear carpeta temporal: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }() // limpieza best-effort
	out := filepath.Join(dir, "tareas.md")

	cmd := exec.CommandContext(ctx, c.Binary(), "exec",
		"--skip-git-repo-check", "--ephemeral",
		"--sandbox", "read-only",
		"-C", dir,
		"--output-last-message", out,
		"-")
	cmd.Stdin = strings.NewReader(TasksPrompt + transcript)
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
