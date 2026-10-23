package uninstall

import "testing"

// SetExecutable hace que Find vea path como el ejecutable en uso. Estado global: no paralelo.
func SetExecutable(t *testing.T, path string) {
	t.Helper()
	orig := executable
	t.Cleanup(func() { executable = orig })
	executable = func() (string, error) { return path, nil }
}
