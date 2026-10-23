package output_test

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ClaudioGuevaraDev/Meet2Do/internal/output"
)

var update = flag.Bool("update", false, "actualiza los golden files")

var meetingDate = time.Date(2026, 10, 23, 11, 30, 0, 0, time.UTC)

func TestFileName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		kind output.Kind
		want string
	}{
		{output.Tasks, "tareas.md"},
		{output.Transcript, "transcripcion.md"},
		{output.Audio, "reunion.wav"},
		{output.Kind("otro"), "otro"},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			t.Parallel()
			if got := output.FileName(tt.kind); got != tt.want {
				t.Errorf("FileName(%q) = %q; want %q", tt.kind, got, tt.want)
			}
		})
	}
}

func TestExisting(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "tareas.md"), "x")
	writeFile(t, filepath.Join(dir, "reunion.wav"), "x")

	tests := []struct {
		name  string
		kinds []output.Kind
		want  []string
	}{
		{"none requested", nil, []string{}},
		{"none exist", []output.Kind{output.Transcript}, []string{}},
		{"some exist", []output.Kind{output.Tasks, output.Transcript, output.Audio}, []string{"tareas.md", "reunion.wav"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := output.Existing(dir, tt.kinds)
			names := make([]string, 0, len(got))
			for _, p := range got {
				names = append(names, filepath.Base(p))
			}
			if !slices.Equal(names, tt.want) {
				t.Errorf("got %v; want %v", names, tt.want)
			}
		})
	}
}

func TestWriteTasks(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p, err := output.WriteTasks(dir, "# Tareas\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p != filepath.Join(dir, "tareas.md") {
		t.Errorf("got path %q", p)
	}
	assertFile(t, p, []byte("# Tareas\n"))
}

func TestWriteTranscriptGolden(t *testing.T) {
	t.Parallel()
	p, err := output.WriteTranscript(t.TempDir(), "Hola, revisamos el login.", meetingDate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}

	golden := filepath.Join("testdata", "transcripcion.golden")
	if *update {
		writeFile(t, golden, string(got))
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden file: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("output mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestCopyAudio(t *testing.T) {
	t.Parallel()
	src := filepath.Join(t.TempDir(), "in.wav")
	writeFile(t, src, "RIFF....WAVE")
	dir := t.TempDir()

	p, err := output.CopyAudio(dir, src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p != filepath.Join(dir, "reunion.wav") {
		t.Errorf("got path %q", p)
	}
	assertFile(t, p, []byte("RIFF....WAVE"))
}

func TestWriteErrors(t *testing.T) {
	t.Parallel()
	missing := filepath.Join(t.TempDir(), "no", "existe")
	src := filepath.Join(t.TempDir(), "in.wav")
	writeFile(t, src, "x")

	tests := []struct {
		name string
		fn   func() (string, error)
	}{
		{"tasks in missing dir", func() (string, error) { return output.WriteTasks(missing, "x") }},
		{"transcript in missing dir", func() (string, error) { return output.WriteTranscript(missing, "x", meetingDate) }},
		{"audio in missing dir", func() (string, error) { return output.CopyAudio(missing, src) }},
		{"audio missing source", func() (string, error) { return output.CopyAudio(t.TempDir(), missing) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, err := tt.fn()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if p != "" {
				t.Errorf("path should be empty on error, got %q", p)
			}
		})
	}
}

func FuzzWriteTranscript(f *testing.F) {
	f.Add("hola mundo")
	f.Add("")
	f.Add("línea 1\nlínea 2\n\n# no es un título")
	f.Fuzz(func(t *testing.T, text string) {
		p, err := output.WriteTranscript(t.TempDir(), text, meetingDate)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if !strings.HasSuffix(string(data), "\n\n"+text+"\n") {
			t.Errorf("transcript text not preserved: %q", data)
		}
	})
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertFile(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s = %q; want %q", filepath.Base(path), got, want)
	}
}
