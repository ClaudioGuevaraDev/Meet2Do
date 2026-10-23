package ai_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ClaudioGuevaraDev/Meet2Do/internal/ai"
	"github.com/ClaudioGuevaraDev/Meet2Do/internal/testutil"
)

const transcript = "Juan dijo que el login falla."

func TestClaudeGenerate(t *testing.T) {
	tests := []struct {
		name    string
		script  string
		want    string
		wantErr bool
	}{
		{
			name:   "plain markdown",
			script: "echo '# Tareas'\necho '- [ ] arreglar login'\n",
			want:   "# Tareas\n- [ ] arreglar login\n",
		},
		{
			name:   "fenced markdown is unwrapped",
			script: "echo '```markdown'\necho '# Tareas'\necho '```'\n",
			want:   "# Tareas\n",
		},
		{
			name:   "surrounding whitespace is trimmed",
			script: "echo\necho '  # Tareas  '\necho\n",
			want:   "# Tareas\n",
		},
		{
			name:    "non-zero exit",
			script:  "echo boom >&2\nexit 3\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			testutil.FakeBin(t, dir, "claude", tt.script)
			testutil.IsolatedPath(t, dir)

			got, err := ai.Claude{}.Generate(context.Background(), ai.TasksPrompt, transcript)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), "boom") {
					t.Errorf("error should include stderr, got %q", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q; want %q", got, tt.want)
			}
		})
	}
}

// prompts son los prompts reales y una línea que solo aparece en cada uno.
var prompts = []struct {
	name, prompt, marker string
}{
	{"tasks", ai.TasksPrompt, "# Tareas — "},
	{"summary", ai.SummaryPrompt, "# Resumen — "},
}

func TestClaudeReceivesPromptAndTranscript(t *testing.T) {
	for _, tt := range prompts {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			stdin := filepath.Join(dir, "stdin.txt")
			args := filepath.Join(dir, "args.txt")
			testutil.FakeBin(t, dir, "claude", `printf '%s\n' "$@" > `+args+"\ncat > "+stdin+"\necho ok\n")
			testutil.IsolatedPath(t, dir)

			if _, err := (ai.Claude{}).Generate(context.Background(), tt.prompt, transcript); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertFileContains(t, stdin, transcript)
			assertFileContains(t, args, "-p")
			assertFileContains(t, args, tt.marker)
			assertFileContains(t, args, "Transcripción:")
		})
	}
}

func TestCodexGenerate(t *testing.T) {
	// El script busca el valor de --output-last-message y escribe ahí la respuesta.
	const writeOut = `while [ $# -gt 0 ]; do [ "$1" = --output-last-message ] && out=$2; shift; done
cat > /dev/null
`
	tests := []struct {
		name    string
		script  string
		want    string
		wantErr string
	}{
		{
			name:   "reads last message file",
			script: writeOut + "echo 'log noise'\nprintf '# Tareas\\n- [ ] a\\n' > \"$out\"\n",
			want:   "# Tareas\n- [ ] a\n",
		},
		{
			name:   "fenced markdown is unwrapped",
			script: writeOut + "printf '```md\\n# Tareas\\n```' > \"$out\"\n",
			want:   "# Tareas\n",
		},
		{
			name:    "no output file",
			script:  writeOut,
			wantErr: "no devolvió respuesta",
		},
		{
			name:    "non-zero exit",
			script:  "echo kaput >&2\nexit 1\n",
			wantErr: "kaput",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			testutil.FakeBin(t, dir, "codex", tt.script)
			testutil.IsolatedPath(t, dir)

			got, err := ai.Codex{}.Generate(context.Background(), ai.TasksPrompt, transcript)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("got error %v; want one containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q; want %q", got, tt.want)
			}
		})
	}
}

func TestCodexReceivesPromptAndTranscriptOnStdin(t *testing.T) {
	for _, tt := range prompts {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			stdin := filepath.Join(dir, "stdin.txt")
			testutil.FakeBin(t, dir, "codex", "cat > "+stdin+"\nexit 1\n")
			testutil.IsolatedPath(t, dir)

			_, _ = ai.Codex{}.Generate(context.Background(), tt.prompt, transcript) // solo interesa lo recibido
			assertFileContains(t, stdin, tt.marker)
			assertFileContains(t, stdin, "Transcripción:\n"+transcript)
		})
	}
}

func TestGenerateCancelled(t *testing.T) {
	dir := t.TempDir()
	testutil.FakeBin(t, dir, "claude", "exec sleep 10\n")
	testutil.IsolatedPath(t, dir)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (ai.Claude{}).Generate(ctx, ai.TasksPrompt, transcript); err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestAvailable(t *testing.T) {
	tests := []struct {
		name      string
		installed []string
		want      []string
		wantErr   error
	}{
		{name: "none", wantErr: ai.ErrNoProvider},
		{name: "only claude", installed: []string{"claude"}, want: []string{"Claude"}},
		{name: "only codex", installed: []string{"codex"}, want: []string{"Codex"}},
		{name: "both", installed: []string{"codex", "claude"}, want: []string{"Claude", "Codex"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, bin := range tt.installed {
				testutil.FakeBin(t, dir, bin, "exit 0\n")
			}
			testutil.IsolatedPath(t, dir)

			got, err := ai.Available()
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("got error %v; want %v", err, tt.wantErr)
			}
			names := make([]string, 0, len(got))
			for _, p := range got {
				names = append(names, p.Name())
			}
			if strings.Join(names, ",") != strings.Join(tt.want, ",") {
				t.Errorf("got %v; want %v", names, tt.want)
			}
		})
	}
}

func TestProviderMetadata(t *testing.T) {
	t.Parallel()
	tests := []struct {
		provider  ai.Provider
		name, bin string
	}{
		{ai.Claude{}, "Claude", "claude"},
		{ai.Codex{}, "Codex", "codex"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.provider.Name() != tt.name || tt.provider.Binary() != tt.bin {
				t.Errorf("got (%q, %q); want (%q, %q)", tt.provider.Name(), tt.provider.Binary(), tt.name, tt.bin)
			}
		})
	}
	if n := len(ai.All()); n != len(tests) {
		t.Errorf("All() has %d providers; want %d", n, len(tests))
	}
}

func assertFileContains(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.Contains(string(data), want) {
		t.Errorf("%s does not contain %q:\n%s", filepath.Base(path), want, data)
	}
}

func TestProviderArgs(t *testing.T) {
	// Cada script guarda sus argumentos (uno por línea) y responde algo válido.
	const record = `printf '%s\n' "$@" > "$(dirname "$0")/args.txt"
while [ $# -gt 0 ]; do [ "$1" = --output-last-message ] && out=$2; shift; done
cat > /dev/null
[ -n "$out" ] && echo ok > "$out"
echo ok
`
	tests := []struct {
		name    string
		bin     string
		p       ai.Provider
		want    []string
		notWant []string
	}{
		{name: "claude default passes no flags", bin: "claude", p: ai.Claude{}, notWant: []string{"--model", "--effort"}},
		{name: "claude model and effort", bin: "claude", p: ai.Claude{}.With("haiku", "low"), want: []string{"--model\nhaiku\n", "--effort\nlow\n"}},
		{name: "claude only effort", bin: "claude", p: ai.Claude{}.With("", "max"), want: []string{"--effort\nmax\n"}, notWant: []string{"--model"}},
		{name: "codex default passes no flags", bin: "codex", p: ai.Codex{}, notWant: []string{"-m\n", "model_reasoning_effort"}},
		{name: "codex model and effort", bin: "codex", p: ai.Codex{}.With("gpt-x", "high"), want: []string{"-m\ngpt-x\n", "-c\nmodel_reasoning_effort=\"high\"\n"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			testutil.FakeBin(t, dir, tt.bin, record)
			testutil.IsolatedPath(t, dir)

			if _, err := tt.p.Generate(context.Background(), ai.TasksPrompt, transcript); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			data, err := os.ReadFile(filepath.Join(dir, "args.txt"))
			if err != nil {
				t.Fatal(err)
			}
			for _, w := range tt.want {
				if !strings.Contains(string(data), w) {
					t.Errorf("args should contain %q:\n%s", w, data)
				}
			}
			for _, w := range tt.notWant {
				if strings.Contains(string(data), w) {
					t.Errorf("args should not contain %q:\n%s", w, data)
				}
			}
		})
	}
}

func TestLabel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		p    ai.Provider
		want string
	}{
		{ai.Claude{}, "Claude"},
		{ai.Claude{}.With("opus", "high"), "Claude (opus · high)"},
		{ai.Codex{}.With("gpt-x", ""), "Codex (gpt-x)"},
		{ai.Codex{}.With("", "low"), "Codex (low)"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			if got := tt.p.Label(); got != tt.want {
				t.Errorf("Label() = %q; want %q", got, tt.want)
			}
		})
	}
}
