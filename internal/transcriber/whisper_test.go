package transcriber_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ClaudioGuevaraDev/Meet2Do/internal/testutil"
	"github.com/ClaudioGuevaraDev/Meet2Do/internal/transcriber"
)

// fakeWhisper escribe "hola mundo" en el archivo indicado por -of.
const fakeWhisper = `#!/bin/sh
while [ $# -gt 0 ]; do [ "$1" = -of ] && out=$2; shift; done
echo ' hola mundo ' > "$out.txt"
`

func TestModelDefaults(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)

	tests := []struct {
		name     string
		model    string
		wantName string
		wantFile string
		sized    bool
	}{
		{"zero value uses default model", "", "small", "ggml-small.bin", true},
		{"explicit model", "tiny", "tiny", "ggml-tiny.bin", true},
		{"unknown size", "custom", "custom", "ggml-custom.bin", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &transcriber.Whisper{Model: tt.model}
			if got := w.ModelName(); got != tt.wantName {
				t.Errorf("ModelName() = %q; want %q", got, tt.wantName)
			}
			if got := w.ModelSize() > 0; got != tt.sized {
				t.Errorf("ModelSize() > 0 = %v; want %v", got, tt.sized)
			}
			got, err := w.ModelPath()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if want := filepath.Join(cache, "meet2do", "models", tt.wantFile); got != want {
				t.Errorf("ModelPath() = %q; want %q", got, want)
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

			path, _ := w.ModelPath() // ya validado en TestModelDefaults
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

// tarEntry describe un archivo del tar.gz de prueba.
type tarEntry struct {
	name, body, link string
}

func makeBundle(t *testing.T, entries ...tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		hdr := &tar.Header{Name: e.name, Mode: 0o755, Size: int64(len(e.body)), Typeflag: tar.TypeReg}
		if e.link != "" {
			hdr = &tar.Header{Name: e.name, Linkname: e.link, Typeflag: tar.TypeSymlink}
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("tar header: %v", err)
		}
		if _, err := tw.Write([]byte(e.body)); err != nil {
			t.Fatalf("tar write: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

// serveBundle sirve data en cualquier ruta y cuenta las peticiones.
func serveBundle(t *testing.T, data []byte) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write(data)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func validBundle(t *testing.T) []byte {
	t.Helper()
	return makeBundle(t,
		tarEntry{name: "whisper-bin/whisper-cli", body: fakeWhisper},
		tarEntry{name: "whisper-bin/libwhisper.so.1.9.5", body: "lib"},
		tarEntry{name: "whisper-bin/libwhisper.so.1", link: "libwhisper.so.1.9.5"},
		tarEntry{name: "whisper-bin/README.md", body: "no se extrae"},
		tarEntry{name: "whisper-bin/sub/whisper-cli", body: "anidado: no se extrae"},
	)
}

func TestEnsureBinaryInstallsAndReusesCache(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("MEET2DO_WHISPER", "")
	data := validBundle(t)
	transcriber.SetTestBundle(t, data)
	srv, hits := serveBundle(t, data)

	w := &transcriber.Whisper{BundleBaseURL: srv.URL}
	if !w.NeedsBinary() {
		t.Fatal("NeedsBinary should be true before install")
	}
	var phases []transcriber.Phase
	var lastDone int64
	bin, err := w.EnsureBinary(context.Background(),
		func(done, _ int64) { lastDone = done },
		func(p transcriber.Phase) { phases = append(phases, p) })
	if err != nil {
		t.Fatalf("EnsureBinary: %v", err)
	}

	if lastDone != int64(len(data)) {
		t.Errorf("progress = %d; want %d", lastDone, len(data))
	}
	if want := []transcriber.Phase{transcriber.PhaseVerifying, transcriber.PhaseExtracting}; !slices.Equal(phases, want) {
		t.Errorf("phases = %v; want %v", phases, want)
	}
	dir := filepath.Dir(bin)
	if link, lerr := os.Readlink(filepath.Join(dir, "libwhisper.so.1")); lerr != nil || link != "libwhisper.so.1.9.5" {
		t.Errorf("symlink = %q, %v", link, lerr)
	}
	for _, name := range []string{"README.md", "sub"} {
		if _, serr := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(serr) {
			t.Errorf("%s should not be extracted", name)
		}
	}
	entries, err := os.ReadDir(filepath.Dir(dir))
	if err != nil {
		t.Fatalf("read cache: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("cache should only contain the version dir, got %d entries", len(entries))
	}

	// El binario instalado se usa para transcribir.
	text, err := (&transcriber.Whisper{}).Transcribe(context.Background(), filepath.Join(t.TempDir(), "a.wav"))
	if err != nil || text != "hola mundo" {
		t.Errorf("Transcribe with installed binary = %q, %v", text, err)
	}

	// Segunda vez: no descarga de nuevo.
	again := &transcriber.Whisper{BundleBaseURL: srv.URL}
	if again.NeedsBinary() {
		t.Error("NeedsBinary should be false after install")
	}
	if _, err := again.EnsureBinary(context.Background(), nil, nil); err != nil {
		t.Fatalf("second EnsureBinary: %v", err)
	}
	if hits.Load() != 1 {
		t.Errorf("bundle downloaded %d times; want 1", hits.Load())
	}
}

func TestEnsureBinaryErrors(t *testing.T) {
	good := validBundle(t)
	tests := []struct {
		name     string
		served   []byte
		expected []byte // bundle aceptado (nil = plataforma no soportada)
		status   int
		wantErr  error
		wantText string
	}{
		{name: "checksum mismatch", served: good, expected: []byte("otro"), wantErr: transcriber.ErrChecksum},
		{name: "unsupported platform", served: good, wantErr: transcriber.ErrUnsupportedPlatform},
		{name: "http error", served: good, expected: good, status: http.StatusNotFound, wantText: "HTTP 404"},
		{
			name:     "path traversal",
			served:   makeBundle(t, tarEntry{name: "../whisper-cli", body: "x"}),
			wantErr:  transcriber.ErrUnsafeArchive,
			expected: []byte("same"), // se reemplaza abajo por served
		},
		{
			name:     "symlink outside",
			served:   makeBundle(t, tarEntry{name: "whisper-bin/libevil.so", link: "/etc/passwd"}),
			wantErr:  transcriber.ErrUnsafeArchive,
			expected: []byte("same"),
		},
		{
			name:     "missing whisper-cli",
			served:   makeBundle(t, tarEntry{name: "whisper-bin/libwhisper.so", body: "lib"}),
			wantText: "no contiene whisper-cli",
			expected: []byte("same"),
		},
		{name: "not a gzip", served: []byte("basura"), expected: []byte("same"), wantText: "leer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("XDG_CACHE_HOME", t.TempDir())
			t.Setenv("MEET2DO_WHISPER", "")
			expected := tt.expected
			if string(expected) == "same" {
				expected = tt.served
			}
			transcriber.SetTestBundle(t, expected)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tt.status != 0 {
					w.WriteHeader(tt.status)
					return
				}
				_, _ = w.Write(tt.served)
			}))
			t.Cleanup(srv.Close)

			w := &transcriber.Whisper{BundleBaseURL: srv.URL}
			_, err := w.EnsureBinary(context.Background(), nil, nil)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("got %v; want %v", err, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantText) {
				t.Errorf("error %q should contain %q", err, tt.wantText)
			}
			if !w.NeedsBinary() {
				t.Error("nothing should be installed after a failed EnsureBinary")
			}
			assertNoLeftovers(t)
		})
	}
}

func TestEnsureBinaryOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	existing := testutil.FakeBin(t, dir, "my-whisper", "exit 0\n")

	tests := []struct {
		name    string
		env     string
		field   string
		want    string
		wantErr bool
	}{
		{name: "env var", env: existing, want: existing},
		{name: "field wins over env", env: "/no/existe", field: existing, want: existing},
		{name: "missing override", env: filepath.Join(dir, "nope"), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("MEET2DO_WHISPER", tt.env)
			w := &transcriber.Whisper{Binary: tt.field, BundleBaseURL: "http://127.0.0.1:1"}
			got, err := w.EnsureBinary(context.Background(), nil, nil)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("got %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

func TestBundleSize(t *testing.T) {
	data := validBundle(t)
	transcriber.SetTestBundle(t, data)
	if got := transcriber.BundleSize(); got != int64(len(data)) {
		t.Errorf("BundleSize() = %d; want %d", got, len(data))
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
		{name: "reads trimmed txt output", script: strings.TrimPrefix(fakeWhisper, "#!/bin/sh\n"), want: "hola mundo"},
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

// assertNoLeftovers verifica que en la caché de whisper no quedan archivos parciales.
func assertNoLeftovers(t *testing.T) {
	t.Helper()
	dir := filepath.Join(os.Getenv("XDG_CACHE_HOME"), "meet2do", "whisper")
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read %s: %v", dir, err)
	}
	for _, e := range entries {
		t.Errorf("leftover in cache: %s", e.Name())
	}
}
