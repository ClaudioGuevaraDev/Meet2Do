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

func TestClaudeGenerateTasks(t *testing.T) {
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

			got, err := ai.Claude{}.GenerateTasks(context.Background(), transcript)
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

func TestClaudeReceivesPromptAndTranscript(t *testing.T) {
	dir := t.TempDir()
	stdin := filepath.Join(dir, "stdin.txt")
	args := filepath.Join(dir, "args.txt")
	testutil.FakeBin(t, dir, "claude", `printf '%s\n' "$@" > `+args+"\ncat > "+stdin+"\necho ok\n")
	testutil.IsolatedPath(t, dir)

	if _, err := (ai.Claude{}).GenerateTasks(context.Background(), transcript); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertFileContains(t, stdin, transcript)
	assertFileContains(t, args, "-p")
	assertFileContains(t, args, "Transcripción:")
}

func TestCodexGenerateTasks(t *testing.T) {
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

			got, err := ai.Codex{}.GenerateTasks(context.Background(), transcript)
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
	dir := t.TempDir()
	stdin := filepath.Join(dir, "stdin.txt")
	testutil.FakeBin(t, dir, "codex", "cat > "+stdin+"\nexit 1\n")
	testutil.IsolatedPath(t, dir)

	_, _ = ai.Codex{}.GenerateTasks(context.Background(), transcript) // solo interesa lo recibido
	assertFileContains(t, stdin, "Transcripción:")
	assertFileContains(t, stdin, transcript)
}

func TestGenerateTasksCancelled(t *testing.T) {
	dir := t.TempDir()
	testutil.FakeBin(t, dir, "claude", "exec sleep 10\n")
	testutil.IsolatedPath(t, dir)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (ai.Claude{}).GenerateTasks(ctx, transcript); err == nil {
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
