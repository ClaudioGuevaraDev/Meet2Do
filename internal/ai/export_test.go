package ai

import (
	"testing"
	"time"
)

// SetClaudeDirs hace que Claude lea su configuración de home, wd y managed (rutas de
// prueba). Modifica estado global: los tests que lo usan no pueden ser paralelos.
func SetClaudeDirs(t *testing.T, home, wd, managed string) {
	t.Helper()
	origHome, origWd, origManaged := userHomeDir, workDir, claudeManagedSettings
	t.Cleanup(func() { userHomeDir, workDir, claudeManagedSettings = origHome, origWd, origManaged })
	userHomeDir = func() (string, error) { return home, nil }
	workDir = func() (string, error) { return wd, nil }
	claudeManagedSettings = managed
}

// SetCodexCatalogTimeout acorta la espera a `codex app-server`. Estado global: no paralelo.
func SetCodexCatalogTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	orig := codexCatalogTimeout
	t.Cleanup(func() { codexCatalogTimeout = orig })
	codexCatalogTimeout = d
}

// NewCatalog arma un Catalog con modelo por defecto defaultID, para probar DefaultEffort.
func NewCatalog(models []Model, defaultID string, effort Setting) Catalog {
	return Catalog{Models: models, Effort: effort, defaultID: defaultID}
}
