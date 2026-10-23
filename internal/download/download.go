// Package download baja archivos por HTTP de forma atómica, informando el progreso y
// calculando el SHA-256 (lo usan el instalador de whisper.cpp y la auto-actualización).
package download

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// ProgressFunc recibe los bytes descargados y el total (0 o -1 si se desconoce).
type ProgressFunc func(done, total int64)

// File baja url a dst de forma atómica (vía dst.part) y devuelve el SHA-256 del contenido.
// Si algo falla no queda ningún archivo parcial.
func File(ctx context.Context, url, dst string, progress ProgressFunc) (string, error) {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", fmt.Errorf("crear carpeta %s: %w", filepath.Dir(dst), err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("crear petición %s: %w", url, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("descargar %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }() // cuerpo ya consumido
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("descargar %s: HTTP %d", url, resp.StatusCode)
	}

	tmp := dst + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return "", fmt.Errorf("crear %s: %w", tmp, err)
	}
	sum := sha256.New()
	pw := &progressWriter{total: resp.ContentLength, fn: progress}
	if _, err = io.Copy(io.MultiWriter(f, sum, pw), resp.Body); err != nil {
		_ = f.Close()      // el error relevante es el de la copia
		_ = os.Remove(tmp) // limpieza best-effort del archivo parcial
		return "", fmt.Errorf("descargar %s: %w", url, err)
	}
	if err = f.Close(); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("cerrar %s: %w", tmp, err)
	}
	if err = os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("guardar %s: %w", dst, err)
	}
	return hexSum(sum), nil
}

func hexSum(h hash.Hash) string { return hex.EncodeToString(h.Sum(nil)) }

type progressWriter struct {
	done, total int64
	fn          ProgressFunc
}

func (p *progressWriter) Write(b []byte) (int, error) {
	p.done += int64(len(b))
	if p.fn != nil {
		p.fn(p.done, p.total)
	}
	return len(b), nil
}
