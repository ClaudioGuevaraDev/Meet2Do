// Package testutil contiene helpers para tests que reemplazan binarios externos por scripts.
package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

// FakeBin escribe un script de shell ejecutable llamado name en dir y devuelve su ruta.
func FakeBin(t *testing.T, dir, name, script string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatalf("crear binario falso %s: %v", name, err)
	}
	return p
}

// IsolatedPath deja el PATH apuntando solo a dir (más /usr/bin y /bin para las utilidades
// que usan los scripts), para controlar qué binarios encuentra exec.LookPath.
// Usa t.Setenv, así que el test no puede ser paralelo.
func IsolatedPath(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+"/usr/bin"+string(os.PathListSeparator)+"/bin")
}
