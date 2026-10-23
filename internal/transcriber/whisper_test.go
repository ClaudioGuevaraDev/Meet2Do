package transcriber_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ClaudioGuevaraDev/Meet2Do/internal/testutil"
	"github.com/ClaudioGuevaraDev/Meet2Do/internal/transcriber"
)

// fakeWhisper escribe "hola mundo" en el archivo indicado por -of.
const fakeWhisper = `while [ $# -gt 0 ]; do [ "$1" = -of ] && out=$2; shift; done
echo ' hola mundo ' > "$out.txt"
`

func TestFindBinary(t *testing.T) {
	tests := []struct {
		name    string
		env     string
		bins    []string
		want    string // nombre base esperado
		wantErr error
	}{
		{name: "env override wins", env: "/opt/whisper", bins: []string{"whisper-cli"}, want: "whisper"},
		{name: "whisper-cli", bins: []string{"whisper-cli"}, want: "whisper-cli"},
		{name: "alias whisper-cpp", bins: []string{"whisper-cpp"}, want: "whisper-cpp"},
		{name: "prefers whisper-cli", bins: []string{"whisper-cpp", "whisper-cli"}, want: "whisper-cli"},
		{name: "not installed", wantErr: transcriber.ErrNotInstalled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, b := range tt.bins {
				testutil.FakeBin(t, dir, b, "exit 0\n")
			}
			testutil.IsolatedPath(t, dir)
			t.Setenv("MEET2DO_WHISPER", tt.env)

			got, err := transcriber.FindBinary()
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("got error %v; want %v", err, tt.wantErr)
			}
			if err != nil {
				if !strings.Contains(err.Error(), "cmake") {
					t.Errorf("error should include install help, got %q", err)
				}
				return
			}
			if filepath.Base(got) != tt.want {
				t.Errorf("got %q; want base %q", got, tt.want)
			}
		})
	}
}

func TestModelPathDefaults(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)

	tests := []struct {
		name  string
		model string
		want  string
	}{
		{"zero value uses default model", "", "ggml-small.bin"},
		{"explicit model", "tiny", "ggml-tiny.bin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &transcriber.Whisper{Model: tt.model}
			got, err := w.ModelPath()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if want := filepath.Join(cache, "meet2do", "models", tt.want); got != want {
				t.Errorf("got %q; want %q", got, want)
			}
		})
	}
}

func TestDownloadModel(t *testing.T) {
	const body = "fake-model-bytes"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ggml-tiny.bin" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	tests := []struct {
		name    string
		model   string
		wantErr bool
	}{
		{name: "downloads and reports progress", model: "tiny"},
		{name: "http 404", model: "missing", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("XDG_CACHE_HOME", t.TempDir())
			w := &transcriber.Whisper{Model: tt.model, BaseURL: srv.URL}
			if w.ModelExists() {
				t.Fatal("model should not exist before download")
			}

			var lastDone int64
			err := w.DownloadModel(context.Background(), func(done, _ int64) { lastDone = done })

			path, _ := w.ModelPath() // ya validado en TestModelPathDefaults
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if w.ModelExists() {
					t.Error("model should not exist after failed download")
				}
				if _, statErr := os.Stat(path + ".part"); !os.IsNotExist(statErr) {
					t.Error("partial file should not remain")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !w.ModelExists() {
				t.Fatal("model should exist after download")
			}
			if lastDone != int64(len(body)) {
				t.Errorf("progress reported %d bytes; want %d", lastDone, len(body))
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != body {
				t.Errorf("model content = %q, %v; want %q", data, err, body)
			}
		})
	}
}

func TestDownloadModelCancelled(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := &transcriber.Whisper{Model: "tiny", BaseURL: "http://127.0.0.1:1"}
	if err := w.DownloadModel(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Errorf("got %v; want context.Canceled", err)
	}
}

func TestArgs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		lang     string
		wantLang string
	}{
		{"zero value lang is auto", "", "auto"},
		{"explicit lang", "es", "es"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := &transcriber.Whisper{Lang: tt.lang}
			args := w.Args("m.bin", "a.wav", "out")
			for flag, want := range map[string]string{"-m": "m.bin", "-f": "a.wav", "-l": tt.wantLang, "-of": "out"} {
				i := slices.Index(args, flag)
				if i < 0 || i+1 >= len(args) || args[i+1] != want {
					t.Errorf("want %s %s in %v", flag, want, args)
				}
			}
			if !slices.Contains(args, "-otxt") {
				t.Errorf("want -otxt in %v", args)
			}
		})
	}
}

func TestTranscribe(t *testing.T) {
	tests := []struct {
		name    string
		script  string
		want    string
		wantErr string
	}{
		{name: "reads trimmed txt output", script: fakeWhisper, want: "hola mundo"},
		{name: "binary fails", script: "echo 'modelo inválido' >&2\nexit 1\n", wantErr: "modelo inválido"},
		{name: "no txt output", script: "exit 0\n", wantErr: "leer transcripción"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("XDG_CACHE_HOME", dir)
			w := &transcriber.Whisper{Binary: testutil.FakeBin(t, dir, "whisper-cli", tt.script)}

			got, err := w.Transcribe(context.Background(), filepath.Join(dir, "a.wav"))
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
