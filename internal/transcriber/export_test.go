package transcriber

import (
	"crypto/sha256"
	"encoding/hex"
	"runtime"
	"testing"
)

// SetTestBundle hace que EnsureBinary acepte data como el bundle de esta arquitectura.
// Con data == nil no hay bundle (plataforma no soportada). Modifica estado global:
// los tests que lo usan no pueden ser paralelos.
func SetTestBundle(t *testing.T, data []byte) {
	t.Helper()
	orig := bundles
	t.Cleanup(func() { bundles = orig })
	if data == nil {
		bundles = map[string]bundle{}
		return
	}
	sum := sha256.Sum256(data)
	bundles = map[string]bundle{runtime.GOARCH: {
		asset:  "whisper-test.tar.gz",
		sha256: hex.EncodeToString(sum[:]),
		size:   int64(len(data)),
	}}
}
