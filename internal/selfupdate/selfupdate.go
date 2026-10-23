// Package selfupdate detecta si hay una versión nueva de Meet2Do en GitHub Releases y
// reemplaza el ejecutable instalado por ella.
package selfupdate

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/ClaudioGuevaraDev/Meet2Do/internal/download"
	"github.com/ClaudioGuevaraDev/Meet2Do/internal/uninstall"
)

// releasesURL es la página de releases del repositorio. MEET2DO_RELEASES_URL la reemplaza
// para probar con un servidor local.
const releasesURL = "https://github.com/ClaudioGuevaraDev/Meet2Do/releases"

// Variables de entorno: NoUpdateEnv desactiva la auto-actualización; UpdatedEnv la marca el
// proceso reiniciado tras actualizarse, para no volver a revisar.
const (
	NoUpdateEnv = "MEET2DO_NO_UPDATE"
	UpdatedEnv  = "MEET2DO_UPDATED"
)

var (
	// ErrChecksum indica que el archivo descargado no coincide con checksums.txt.
	ErrChecksum = errors.New("el SHA-256 de la actualización no coincide")
	// ErrNoRelease indica que GitHub no informó una versión publicada.
	ErrNoRelease = errors.New("no se encontró la última versión publicada")
)

// maxBinarySize limita lo que se extrae del tar.gz (el binario pesa ~10 MB).
const maxBinarySize = 200 << 20

func baseURL() string {
	if u := os.Getenv("MEET2DO_RELEASES_URL"); u != "" {
		return strings.TrimRight(u, "/")
	}
	return releasesURL
}

// Enabled indica si corresponde buscar actualizaciones: no si el usuario lo desactivó, si
// este proceso ya es el reiniciado tras actualizar, o si es un binario de `go run`.
func Enabled() bool {
	if os.Getenv(NoUpdateEnv) != "" || os.Getenv(UpdatedEnv) != "" {
		return false
	}
	_, ok := uninstall.InstalledBinary()
	return ok
}

// Latest devuelve la última versión publicada ("1.2.3"). Lee la redirección de
// releases/latest → releases/tag/v1.2.3, sin usar el API de GitHub (que tiene límite).
func Latest(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, baseURL()+"/latest", nil)
	if err != nil {
		return "", fmt.Errorf("buscar actualizaciones: %w", err)
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("buscar actualizaciones: %w", err)
	}
	_ = resp.Body.Close() // HEAD: no hay cuerpo que leer
	loc := resp.Header.Get("Location")
	if resp.StatusCode < 300 || resp.StatusCode >= 400 || loc == "" {
		return "", fmt.Errorf("%w (HTTP %d)", ErrNoRelease, resp.StatusCode)
	}
	tag := path.Base(loc)
	v, ok := strings.CutPrefix(tag, "v")
	if !ok || parse(v) == nil {
		return "", fmt.Errorf("%w: %q", ErrNoRelease, loc)
	}
	return v, nil
}

// Newer indica si latest es una versión posterior a current (ambas "A.B.C").
func Newer(current, latest string) bool {
	c, l := parse(current), parse(latest)
	if c == nil || l == nil {
		return false
	}
	for i := range c {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

// parse convierte "A.B.C" en sus tres números, o nil si no tiene ese formato.
func parse(v string) []int {
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return nil
	}
	nums := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil
		}
		nums[i] = n
	}
	return nums
}

// Apply descarga la versión indicada, verifica su SHA-256 y reemplaza exe (el ejecutable
// real) y los otros nombres del programa que sean archivos en la misma carpeta (m2do
// instalado con go install; los symlinks ya apuntan a exe). El reemplazo es un rename, así
// que el binario nunca queda a medio escribir.
func Apply(ctx context.Context, exe, version string, progress download.ProgressFunc) error {
	tmp, err := os.MkdirTemp("", "meet2do-update-")
	if err != nil {
		return fmt.Errorf("crear carpeta temporal: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }() // limpieza best-effort

	asset := "meet2do-linux-" + runtime.GOARCH + ".tar.gz"
	base := baseURL() + "/download/v" + version + "/"
	sum, err := download.File(ctx, base+asset, filepath.Join(tmp, asset), progress)
	if err != nil {
		return err
	}
	sums := filepath.Join(tmp, "checksums.txt")
	if _, err = download.File(ctx, base+"checksums.txt", sums, nil); err != nil {
		return err
	}
	want, err := checksumFor(sums, asset)
	if err != nil {
		return err
	}
	if want != sum {
		return ErrChecksum
	}

	targets := []string{exe}
	for _, name := range uninstall.Commands {
		p := filepath.Join(filepath.Dir(exe), name)
		if info, err := os.Lstat(p); err == nil && info.Mode().IsRegular() && p != exe {
			targets = append(targets, p)
		}
	}
	for _, t := range targets {
		if err := replace(filepath.Join(tmp, asset), t); err != nil {
			return err
		}
	}
	return nil
}

// checksumFor busca en checksums.txt (formato de sha256sum) el SHA-256 de asset.
func checksumFor(path, asset string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("leer checksums: %w", err)
	}
	defer func() { _ = f.Close() }() // solo lectura
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if fields := strings.Fields(sc.Text()); len(fields) == 2 && fields[1] == asset {
			return fields[0], nil
		}
	}
	if err := sc.Err(); err != nil {
		return "", fmt.Errorf("leer checksums: %w", err)
	}
	return "", fmt.Errorf("checksums.txt no incluye %s", asset)
}

// replace extrae meet2do del archive junto a target y lo renombra encima de target.
func replace(archive, target string) (err error) {
	tmp := filepath.Join(filepath.Dir(target), ".meet2do-update")
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("escribir en %s: %w", filepath.Dir(target), err)
	}
	defer func() {
		if err != nil {
			_ = os.Remove(tmp) // no dejar el binario a medio escribir
		}
	}()
	if err = extractBinary(archive, out); err != nil {
		_ = out.Close() // el error relevante es el de la extracción
		return err
	}
	if err = out.Close(); err != nil {
		return fmt.Errorf("escribir %s: %w", tmp, err)
	}
	if err = os.Rename(tmp, target); err != nil {
		return fmt.Errorf("reemplazar %s: %w", target, err)
	}
	return nil
}

// extractBinary copia a w el archivo meet2do del tar.gz.
func extractBinary(archive string, w io.Writer) error {
	f, err := os.Open(archive)
	if err != nil {
		return fmt.Errorf("abrir %s: %w", archive, err)
	}
	defer func() { _ = f.Close() }() // solo lectura
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("leer %s: %w", filepath.Base(archive), err)
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("%s no contiene meet2do", filepath.Base(archive))
		}
		if err != nil {
			return fmt.Errorf("leer %s: %w", filepath.Base(archive), err)
		}
		if h.Name != "meet2do" || h.Typeflag != tar.TypeReg {
			continue
		}
		if h.Size > maxBinarySize {
			return fmt.Errorf("meet2do en %s es demasiado grande", filepath.Base(archive))
		}
		if _, err := io.Copy(w, io.LimitReader(tr, maxBinarySize)); err != nil {
			return fmt.Errorf("extraer meet2do: %w", err)
		}
		return nil
	}
}
