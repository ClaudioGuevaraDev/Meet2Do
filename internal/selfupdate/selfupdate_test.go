package selfupdate_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/ClaudioGuevaraDev/Meet2Do/internal/selfupdate"
)

var asset = "meet2do-linux-" + runtime.GOARCH + ".tar.gz"

// tarball arma un tar.gz con un archivo meet2do de contenido body.
func tarball(t *testing.T, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "meet2do", Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// fakeGitHub imita GitHub Releases: /latest redirige al tag y /download/<tag>/ sirve files.
// Apunta MEET2DO_RELEASES_URL al servidor (por eso los tests no son paralelos).
func fakeGitHub(t *testing.T, latest string, files map[string][]byte) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/latest", func(w http.ResponseWriter, r *http.Request) {
		if latest == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		http.Redirect(w, r, "/tag/"+latest, http.StatusFound)
	})
	mux.HandleFunc("/download/", func(w http.ResponseWriter, r *http.Request) {
		data, ok := files[filepath.Base(r.URL.Path)]
		if !ok || filepath.Base(filepath.Dir(r.URL.Path)) != latest {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(data)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Setenv("MEET2DO_RELEASES_URL", srv.URL)
}

// release devuelve los archivos de un release con el binario body.
func release(t *testing.T, body string) map[string][]byte {
	t.Helper()
	tgz := tarball(t, body)
	sum := sha256.Sum256(tgz)
	return map[string][]byte{
		asset:           tgz,
		"checksums.txt": []byte(hex.EncodeToString(sum[:]) + "  " + asset + "\n"),
	}
}

func TestLatest(t *testing.T) {
	tests := []struct {
		name    string
		latest  string
		want    string
		wantErr error
	}{
		{name: "reads version from redirect", latest: "v1.2.3", want: "1.2.3"},
		{name: "no release", latest: "", wantErr: selfupdate.ErrNoRelease},
		{name: "tag without semver", latest: "nightly", wantErr: selfupdate.ErrNoRelease},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeGitHub(t, tt.latest, nil)
			got, err := selfupdate.Latest(context.Background())
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v; want %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("Latest = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestLatestTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("MEET2DO_RELEASES_URL", srv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := selfupdate.Latest(ctx); err == nil {
		t.Fatal("expected a timeout error")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("Latest took %v; the context should cut it", d)
	}
}

func TestNewer(t *testing.T) {
	t.Parallel()
	tests := []struct {
		current, latest string
		want            bool
	}{
		{"0.9.0", "0.10.0", true},
		{"0.10.0", "0.10.1", true},
		{"0.10.0", "1.0.0", true},
		{"0.10.0", "0.10.0", false},
		{"0.10.1", "0.10.0", false},
		{"0.11.0", "0.10.9", false},
		{"0.10.0", "0.10", false},
		{"dev", "0.10.0", false},
		{"0.10.0", "v0.11.0", false},
		{"0.10.0", "0.-1.0", false},
	}
	for _, tt := range tests {
		t.Run(tt.current+"→"+tt.latest, func(t *testing.T) {
			t.Parallel()
			if got := selfupdate.Newer(tt.current, tt.latest); got != tt.want {
				t.Errorf("Newer(%q, %q) = %v; want %v", tt.current, tt.latest, got, tt.want)
			}
		})
	}
}

func TestApply(t *testing.T) {
	good := release(t, "binario nuevo")
	badSum := release(t, "binario nuevo")
	badSum["checksums.txt"] = []byte("0000  " + asset + "\n")
	missingSum := release(t, "binario nuevo")
	missingSum["checksums.txt"] = []byte("abcd  otro.tar.gz\n")
	notMeet2do := map[string][]byte{asset: nil, "checksums.txt": nil}
	{
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gz)
		_ = tw.WriteHeader(&tar.Header{Name: "otro", Mode: 0o644, Size: 1, Typeflag: tar.TypeReg})
		_, _ = tw.Write([]byte("x"))
		_ = tw.Close()
		_ = gz.Close()
		sum := sha256.Sum256(buf.Bytes())
		notMeet2do[asset] = buf.Bytes()
		notMeet2do["checksums.txt"] = []byte(hex.EncodeToString(sum[:]) + "  " + asset + "\n")
	}

	tests := []struct {
		name     string
		files    map[string][]byte
		version  string
		m2do     string // "file", "symlink" o "" (no existe)
		wantErr  error
		anyErr   bool
		wantBody string // contenido esperado de meet2do (y de m2do si es archivo)
	}{
		{name: "replaces the binary", files: good, version: "1.2.3", wantBody: "binario nuevo"},
		{name: "also replaces m2do installed as a file", files: good, version: "1.2.3", m2do: "file", wantBody: "binario nuevo"},
		{name: "keeps m2do symlink", files: good, version: "1.2.3", m2do: "symlink", wantBody: "binario nuevo"},
		{name: "bad checksum keeps the old binary", files: badSum, version: "1.2.3", wantErr: selfupdate.ErrChecksum, wantBody: "viejo"},
		{name: "checksum missing", files: missingSum, version: "1.2.3", anyErr: true, wantBody: "viejo"},
		{name: "archive without meet2do", files: notMeet2do, version: "1.2.3", anyErr: true, wantBody: "viejo"},
		{name: "release not found", files: good, version: "9.9.9", anyErr: true, wantBody: "viejo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeGitHub(t, "v1.2.3", tt.files)
			dir := t.TempDir()
			exe := filepath.Join(dir, "meet2do")
			if err := os.WriteFile(exe, []byte("viejo"), 0o755); err != nil {
				t.Fatal(err)
			}
			switch tt.m2do {
			case "file":
				if err := os.WriteFile(filepath.Join(dir, "m2do"), []byte("viejo"), 0o755); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Symlink("meet2do", filepath.Join(dir, "m2do")); err != nil {
					t.Fatal(err)
				}
			}

			err := selfupdate.Apply(context.Background(), exe, tt.version, nil)
			switch {
			case tt.wantErr != nil && !errors.Is(err, tt.wantErr):
				t.Fatalf("err = %v; want %v", err, tt.wantErr)
			case tt.anyErr && err == nil:
				t.Fatal("expected an error")
			case tt.wantErr == nil && !tt.anyErr && err != nil:
				t.Fatalf("unexpected error: %v", err)
			}

			assertBinary(t, exe, tt.wantBody)
			switch tt.m2do {
			case "file":
				assertBinary(t, filepath.Join(dir, "m2do"), tt.wantBody)
			case "symlink":
				if target, err := os.Readlink(filepath.Join(dir, "m2do")); err != nil || target != "meet2do" {
					t.Errorf("m2do should still be a symlink to meet2do (%q, %v)", target, err)
				}
			}
			if _, err := os.Stat(filepath.Join(dir, ".meet2do-update")); !os.IsNotExist(err) {
				t.Errorf("temporary file should not remain (stat err %v)", err)
			}
		})
	}
}

func TestApplyReadOnlyDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignora los permisos")
	}
	fakeGitHub(t, "v1.2.3", release(t, "nuevo"))
	dir := t.TempDir()
	exe := filepath.Join(dir, "meet2do")
	if err := os.WriteFile(exe, []byte("viejo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	if err := selfupdate.Apply(context.Background(), exe, "1.2.3", nil); err == nil {
		t.Fatal("expected a permission error")
	}
	assertBinary(t, exe, "viejo")
}

func TestEnabled(t *testing.T) {
	tests := []struct {
		name string
		env  string
	}{
		{"disabled by the user", selfupdate.NoUpdateEnv},
		{"already updated in this run", selfupdate.UpdatedEnv},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.env, "1")
			if selfupdate.Enabled() {
				t.Errorf("Enabled() = true with %s set", tt.env)
			}
		})
	}
	// Los tests corren como binario temporal de `go test` (bajo os.TempDir/go-build): no se
	// actualiza, igual que con `go run`.
	t.Run("go test binary", func(t *testing.T) {
		t.Setenv(selfupdate.NoUpdateEnv, "")
		t.Setenv(selfupdate.UpdatedEnv, "")
		if selfupdate.Enabled() {
			t.Error("Enabled() = true for a temporary go build binary")
		}
	})
}

func assertBinary(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(data) != want {
		t.Errorf("%s = %q; want %q", filepath.Base(path), data, want)
	}
	if info, err := os.Stat(path); err == nil && info.Mode().Perm()&0o111 == 0 {
		t.Errorf("%s should be executable, mode %v", filepath.Base(path), info.Mode())
	}
}
