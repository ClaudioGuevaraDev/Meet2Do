package download_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ClaudioGuevaraDev/Meet2Do/internal/download"
)

func TestFile(t *testing.T) {
	t.Parallel()
	const body = "contenido del archivo"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			_, _ = w.Write([]byte(body))
		case "/cut":
			// Promete más bytes de los que envía: la copia falla a mitad.
			w.Header().Set("Content-Length", "1000")
			_, _ = w.Write([]byte("incompleto"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	sum := sha256.Sum256([]byte(body))

	tests := []struct {
		name    string
		path    string
		wantErr string
	}{
		{name: "downloads and returns the sha256", path: "/ok"},
		{name: "http error", path: "/missing", wantErr: "HTTP 404"},
		{name: "connection cut", path: "/cut", wantErr: "descargar"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dst := filepath.Join(t.TempDir(), "sub", "archivo")
			var last, total int64
			got, err := download.File(context.Background(), srv.URL+tt.path, dst, func(done, tot int64) { last, total = done, tot })

			if _, statErr := os.Stat(dst + ".part"); !os.IsNotExist(statErr) {
				t.Errorf(".part should never remain (stat err %v)", statErr)
			}
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v; want one containing %q", err, tt.wantErr)
				}
				if _, statErr := os.Stat(dst); !os.IsNotExist(statErr) {
					t.Errorf("nothing should be written on error (stat err %v)", statErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != hex.EncodeToString(sum[:]) {
				t.Errorf("sha256 = %s", got)
			}
			if data, _ := os.ReadFile(dst); string(data) != body {
				t.Errorf("content = %q", data)
			}
			if last != int64(len(body)) || total != int64(len(body)) {
				t.Errorf("progress = %d/%d; want %d/%d", last, total, len(body), len(body))
			}
		})
	}
}

func TestFileCanceled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := download.File(ctx, "http://127.0.0.1:1/x", filepath.Join(t.TempDir(), "x"), nil); err == nil {
		t.Fatal("expected an error for a canceled context")
	}
}
