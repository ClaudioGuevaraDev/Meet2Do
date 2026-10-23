package ai_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ClaudioGuevaraDev/Meet2Do/internal/ai"
	"github.com/ClaudioGuevaraDev/Meet2Do/internal/testutil"
)

func TestClaudeCatalog(t *testing.T) {
	tests := []struct {
		name       string
		files      map[string]string // ruta relativa (home/, wd/, managed) → contenido
		env        map[string]string
		wantModel  ai.Setting
		wantEffort ai.Setting
	}{
		{name: "nothing configured"},
		{
			name:      "user settings",
			files:     map[string]string{"home/.claude/settings.json": `{"model":"opus"}`},
			wantModel: ai.Setting{Value: "Opus", Source: "~/.claude/settings.json"},
		},
		{
			name: "project overrides user",
			files: map[string]string{
				"home/.claude/settings.json": `{"model":"opus","effortLevel":"low"}`,
				"wd/.claude/settings.json":   `{"model":"sonnet"}`,
			},
			wantModel:  ai.Setting{Value: "Sonnet", Source: "wd/.claude/settings.json"},
			wantEffort: ai.Setting{Value: "low", Source: "~/.claude/settings.json"},
		},
		{
			name: "local overrides project",
			files: map[string]string{
				"wd/.claude/settings.json":       `{"effortLevel":"high"}`,
				"wd/.claude/settings.local.json": `{"effortLevel":"max"}`,
			},
			wantEffort: ai.Setting{Value: "max", Source: "wd/.claude/settings.local.json"},
		},
		{
			name: "managed wins and env block counts",
			files: map[string]string{
				"home/.claude/settings.json": `{"model":"opus"}`,
				"managed":                    `{"env":{"ANTHROPIC_MODEL":"claude-custom-1"}}`,
			},
			wantModel: ai.Setting{Value: "claude-custom-1", Source: "managed"},
		},
		{
			name:       "environment wins",
			files:      map[string]string{"home/.claude/settings.json": `{"model":"opus","effortLevel":"low"}`},
			env:        map[string]string{"ANTHROPIC_MODEL": "haiku", "CLAUDE_CODE_EFFORT_LEVEL": "xhigh"},
			wantModel:  ai.Setting{Value: "Haiku", Source: "ANTHROPIC_MODEL"},
			wantEffort: ai.Setting{Value: "xhigh", Source: "CLAUDE_CODE_EFFORT_LEVEL"},
		},
		{
			name:  "invalid json is ignored",
			files: map[string]string{"home/.claude/settings.json": `{model`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for rel, content := range tt.files {
				p := filepath.Join(root, rel)
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			ai.SetClaudeDirs(t, filepath.Join(root, "home"), filepath.Join(root, "wd"), filepath.Join(root, "managed"))
			t.Setenv("ANTHROPIC_MODEL", tt.env["ANTHROPIC_MODEL"])
			t.Setenv("CLAUDE_CODE_EFFORT_LEVEL", tt.env["CLAUDE_CODE_EFFORT_LEVEL"])

			cat := ai.Claude{}.Catalog(context.Background())
			// Las rutas fuera del home se muestran completas: se comparan sin el prefijo temporal.
			trim := func(s ai.Setting) ai.Setting {
				s.Source = strings.TrimPrefix(s.Source, root+string(filepath.Separator))
				return s
			}
			if got := trim(cat.DefaultModel); got != tt.wantModel {
				t.Errorf("model = %+v; want %+v", got, tt.wantModel)
			}
			if got := trim(cat.Effort); got != tt.wantEffort {
				t.Errorf("effort = %+v; want %+v", got, tt.wantEffort)
			}
			if len(cat.Models) != 4 || strings.Join(cat.DefaultEfforts, ",") != "low,medium,high,xhigh,max" {
				t.Errorf("models = %d, efforts = %v", len(cat.Models), cat.DefaultEfforts)
			}
		})
	}
}

// codexServer arma un `codex` falso que responde a app-server con modelList y config.
func codexServer(modelList, config string) string {
	return `[ "$1" = app-server ] || exit 2
echo '{"jsonrpc":"2.0","id":0,"result":{}}'
echo '{"jsonrpc":"2.0","method":"some/notification","params":{}}'
echo '{"jsonrpc":"2.0","id":2,"result":` + config + `}'
echo '{"jsonrpc":"2.0","id":1,"result":` + modelList + `}'
cat > /dev/null
`
}

func TestCodexCatalog(t *testing.T) {
	const models = `{"data":[
{"id":"a","displayName":"A","hidden":false,"isDefault":false,"defaultReasoningEffort":"medium","supportedReasoningEfforts":[{"reasoningEffort":"low"},{"reasoningEffort":"medium"}]},
{"id":"secret","displayName":"S","hidden":true,"isDefault":false},
{"id":"b","displayName":"B","hidden":false,"isDefault":true,"defaultReasoningEffort":"low","supportedReasoningEfforts":[{"reasoningEffort":"low"},{"reasoningEffort":"high"},{"reasoningEffort":"ultra"}]}
]}`
	models1 := strings.ReplaceAll(models, "\n", "")
	tests := []struct {
		name        string
		script      string
		wantModels  string
		wantDefault ai.Setting
		wantEffort  ai.Setting // DefaultEffort con "Por defecto"
		wantEfforts string     // DefaultEfforts
	}{
		{
			name:        "catalog default",
			script:      codexServer(models1, `{"config":{"model":null,"model_reasoning_effort":null}}`),
			wantModels:  "a,b",
			wantDefault: ai.Setting{Value: "B", Source: "predeterminado de Codex"},
			wantEffort:  ai.Setting{Value: "low", Source: "predeterminado del modelo"},
			wantEfforts: "low,high,ultra",
		},
		{
			name:        "configured model and effort",
			script:      codexServer(models1, `{"config":{"model":"a","model_reasoning_effort":"high"}}`),
			wantModels:  "a,b",
			wantDefault: ai.Setting{Value: "A", Source: "configurado en Codex"},
			wantEffort:  ai.Setting{Value: "high", Source: "configurado en Codex"},
			wantEfforts: "low,medium",
		},
		{
			name:        "configured model outside the list",
			script:      codexServer(models1, `{"config":{"model":"custom"}}`),
			wantModels:  "a,b",
			wantDefault: ai.Setting{Value: "custom", Source: "configurado en Codex"},
			wantEfforts: "low,medium,high",
		},
		{
			name:        "rpc error falls back",
			script:      "echo '{\"jsonrpc\":\"2.0\",\"id\":1,\"error\":{\"message\":\"nope\"}}'\ncat >/dev/null\n",
			wantEfforts: "low,medium,high",
		},
		{name: "exits without answering", script: "exit 1\n", wantEfforts: "low,medium,high"},
		{name: "never answers", script: "exec sleep 30\n", wantEfforts: "low,medium,high"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			testutil.FakeBin(t, dir, "codex", tt.script)
			testutil.IsolatedPath(t, dir)
			ai.SetCodexCatalogTimeout(t, 500*time.Millisecond)

			start := time.Now()
			cat := ai.Codex{}.Catalog(context.Background())
			if d := time.Since(start); d > 3*time.Second {
				t.Errorf("Catalog took %v; the timeout should stop it", d)
			}
			var ids []string
			for _, m := range cat.Models {
				ids = append(ids, m.ID)
			}
			if got := strings.Join(ids, ","); got != tt.wantModels {
				t.Errorf("models = %q; want %q", got, tt.wantModels)
			}
			if cat.DefaultModel != tt.wantDefault {
				t.Errorf("default model = %+v; want %+v", cat.DefaultModel, tt.wantDefault)
			}
			if got := cat.DefaultEffort(ai.Model{}); got != tt.wantEffort {
				t.Errorf("default effort = %+v; want %+v", got, tt.wantEffort)
			}
			if got := strings.Join(cat.DefaultEfforts, ","); got != tt.wantEfforts {
				t.Errorf("default efforts = %q; want %q", got, tt.wantEfforts)
			}
		})
	}
}

func TestCatalogDefaultEffort(t *testing.T) {
	t.Parallel()
	models := []ai.Model{{ID: "a", DefaultEffort: "medium"}, {ID: "b"}}
	configured := ai.Setting{Value: "high", Source: "config"}
	tests := []struct {
		name string
		cat  ai.Catalog
		m    ai.Model
		want ai.Setting
	}{
		{"configured wins", ai.NewCatalog(models, "a", configured), models[1], configured},
		{"chosen model default", ai.NewCatalog(models, "b", ai.Setting{}), models[0], ai.Setting{Value: "medium", Source: "predeterminado del modelo"}},
		{"default model when none chosen", ai.NewCatalog(models, "a", ai.Setting{}), ai.Model{}, ai.Setting{Value: "medium", Source: "predeterminado del modelo"}},
		{"unknown", ai.NewCatalog(models, "", ai.Setting{}), models[1], ai.Setting{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.cat.DefaultEffort(tt.m); got != tt.want {
				t.Errorf("DefaultEffort = %+v; want %+v", got, tt.want)
			}
		})
	}
}
