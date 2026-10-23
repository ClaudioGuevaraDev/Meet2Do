package uninstall_test

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/ClaudioGuevaraDev/Meet2Do/internal/uninstall"
)

// setup arma un home de prueba: caché en root/cache y temporales de Go en root/tmp.
// Devuelve root. Usa t.Setenv, así que los tests no pueden ser paralelos.
func setup(t *testing.T, files map[string]int) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	t.Setenv("TMPDIR", filepath.Join(root, "tmp"))
	for rel, size := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, make([]byte, size), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestFind(t *testing.T) {
	all := map[string]int{
		"bin/meet2do": 100,
		"cache/meet2do/whisper/b5454/whisper-cli":     30,
		"cache/meet2do/whisper/b5454/libwhisper.so.1": 20,
		"cache/meet2do/models/ggml-small.bin":         400,
		"cache/meet2do/models/ggml-tiny.bin":          70,
		"cache/meet2do/models/ggml-base.bin.part":     5, // descarga a medias: cuenta en el tamaño
		"tmp/go-build123/b001/exe/meet2do":            100,
	}
	tests := []struct {
		name  string
		files map[string]int
		exe   string
		want  string // "Nombre|Detalle|tamaño" separados por ;
	}{
		{
			name:  "everything installed",
			files: all,
			exe:   "bin/meet2do",
			want:  "Meet2Do||100;whisper.cpp||50;Modelos de voz|small, tiny|475",
		},
		{
			name:  "go run binary is not offered",
			files: all,
			exe:   "tmp/go-build123/b001/exe/meet2do",
			want:  "whisper.cpp||50;Modelos de voz|small, tiny|475",
		},
		{
			name:  "only the binary",
			files: map[string]int{"bin/meet2do": 100},
			exe:   "bin/meet2do",
			want:  "Meet2Do||100",
		},
		{name: "nothing installed", exe: "bin/missing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := setup(t, tt.files)
			uninstall.SetExecutable(t, filepath.Join(root, tt.exe))

			cs, err := uninstall.Find()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			var got []string
			for _, c := range cs {
				got = append(got, c.Name+"|"+c.Detail+"|"+strconv.FormatInt(c.Size, 10))
				if !strings.HasPrefix(c.Path, root) {
					t.Errorf("%s path %q is outside the test root", c.Name, c.Path)
				}
			}
			if s := strings.Join(got, ";"); s != tt.want {
				t.Errorf("Find = %q; want %q", s, tt.want)
			}
		})
	}
}

func TestRemoveAndCleanup(t *testing.T) {
	tests := []struct {
		name     string
		remove   []string // componentes a borrar
		wantRoot bool     // si la carpeta de caché sigue existiendo
	}{
		{name: "keeps cache root while something is left", remove: []string{"Meet2Do", "whisper.cpp"}, wantRoot: true},
		{name: "removes empty cache root", remove: []string{"Meet2Do", "whisper.cpp", "Modelos de voz"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := setup(t, map[string]int{
				"bin/meet2do": 10,
				"cache/meet2do/whisper/b5454/whisper-cli": 10,
				"cache/meet2do/models/ggml-small.bin":     10,
			})
			uninstall.SetExecutable(t, filepath.Join(root, "bin/meet2do"))
			cs, err := uninstall.Find()
			if err != nil {
				t.Fatal(err)
			}
			for _, c := range cs {
				if !slices.Contains(tt.remove, c.Name) {
					continue
				}
				if rmErr := uninstall.Remove(c); rmErr != nil {
					t.Fatalf("remove %s: %v", c.Name, rmErr)
				}
				if _, statErr := os.Stat(c.Path); !os.IsNotExist(statErr) {
					t.Errorf("%s should be gone (stat err %v)", c.Path, statErr)
				}
			}
			uninstall.Cleanup()
			_, err = os.Stat(filepath.Join(root, "cache/meet2do"))
			if exists := err == nil; exists != tt.wantRoot {
				t.Errorf("cache root exists = %v; want %v", exists, tt.wantRoot)
			}
			if _, err := os.Stat(filepath.Join(root, "cache")); err != nil {
				t.Errorf("the user's cache dir must never be removed: %v", err)
			}
		})
	}
}

func TestRemoveError(t *testing.T) {
	dir := t.TempDir()
	locked := filepath.Join(dir, "locked")
	if err := os.MkdirAll(filepath.Join(locked, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o500); err != nil { // sin permiso de escritura: no se puede borrar sub
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	if os.Geteuid() == 0 {
		t.Skip("root ignora los permisos")
	}
	err := uninstall.Remove(uninstall.Component{Name: "x", Path: filepath.Join(locked, "sub")})
	if err == nil || !strings.Contains(err.Error(), "borrar") {
		t.Errorf("Remove = %v; want a wrapped error", err)
	}
}
