package ai

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// claudeEfforts son los valores que acepta `claude --effort`.
var claudeEfforts = []string{"low", "medium", "high", "xhigh", "max"}

// claudeModels son los alias de `claude --model`, que siempre apuntan al último modelo de
// cada familia (el CLI no tiene un comando para listarlos).
var claudeModels = []Model{
	{ID: "fable", Name: "Fable", Efforts: claudeEfforts},
	{ID: "opus", Name: "Opus", Efforts: claudeEfforts},
	{ID: "sonnet", Name: "Sonnet", Efforts: claudeEfforts},
	{ID: "haiku", Name: "Haiku", Efforts: claudeEfforts},
}

// Rutas de configuración de Claude Code; variables para poder probarlas.
var (
	claudeManagedSettings = "/etc/claude-code/managed-settings.json"
	userHomeDir           = os.UserHomeDir
	workDir               = os.Getwd
)

// Variables de entorno y claves de settings con las que Claude Code elige modelo y esfuerzo.
const (
	envClaudeModel  = "ANTHROPIC_MODEL"
	envClaudeEffort = "CLAUDE_CODE_EFFORT_LEVEL"
)

// claudeSettings es la parte que usamos de un settings.json de Claude Code.
type claudeSettings struct {
	Model       string            `json:"model"`
	EffortLevel string            `json:"effortLevel"`
	Env         map[string]string `json:"env"`
}

// Catalog devuelve los alias y el modelo/esfuerzo que Claude usará sin flags, leyendo su
// configuración (el entorno y luego los settings, de mayor a menor precedencia). Sin nada
// configurado, Claude elige según la cuenta y no se puede saber cuál.
func (Claude) Catalog(context.Context) Catalog {
	model, effort := claudeEnv(os.Getenv, "")
	for _, path := range claudeSettingsPaths() {
		s, ok := readClaudeSettings(path)
		if !ok {
			continue
		}
		src := displayPath(path)
		env := func(k string) string { return s.Env[k] }
		m, e := claudeEnv(env, src)
		model, effort = firstSet(model, m), firstSet(effort, e)
		model = firstSet(model, setting(s.Model, src))
		effort = firstSet(effort, setting(s.EffortLevel, src))
	}
	if model.Value != "" {
		model.Value = claudeModelName(model.Value)
	}
	return Catalog{Models: claudeModels, DefaultModel: model, Effort: effort, DefaultEfforts: claudeEfforts}
}

// claudeEnv lee las variables de modelo y esfuerzo; src es su origen ("" = el entorno).
func claudeEnv(getenv func(string) string, src string) (model, effort Setting) {
	modelSrc, effortSrc := src, src
	if src == "" {
		modelSrc, effortSrc = envClaudeModel, envClaudeEffort
	}
	return setting(getenv(envClaudeModel), modelSrc), setting(getenv(envClaudeEffort), effortSrc)
}

// setting arma un Setting; sin valor no hay origen que mostrar.
func setting(value, src string) Setting {
	if value == "" {
		return Setting{}
	}
	return Setting{Value: value, Source: src}
}

// claudeSettingsPaths devuelve los settings de mayor a menor precedencia.
func claudeSettingsPaths() []string {
	paths := []string{claudeManagedSettings}
	if wd, err := workDir(); err == nil {
		paths = append(paths,
			filepath.Join(wd, ".claude", "settings.local.json"),
			filepath.Join(wd, ".claude", "settings.json"))
	}
	if home, err := userHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".claude", "settings.json"))
	}
	return paths
}

func readClaudeSettings(path string) (claudeSettings, bool) {
	var s claudeSettings
	data, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(data, &s) != nil {
		return s, false
	}
	return s, true
}

// firstSet devuelve a si ya tiene valor; si no, b.
func firstSet(a, b Setting) Setting {
	if a.Value != "" {
		return a
	}
	return b
}

// claudeModelName muestra un alias conocido con su nombre ("opus" → "Opus").
func claudeModelName(v string) string {
	for _, m := range claudeModels {
		if strings.EqualFold(m.ID, v) {
			return m.Name
		}
	}
	return v
}

// displayPath abrevia el home como ~.
func displayPath(p string) string {
	if home, err := userHomeDir(); err == nil && home != "" {
		if rel, err := filepath.Rel(home, p); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.Join("~", rel)
		}
	}
	return p
}
