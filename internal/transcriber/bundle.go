package transcriber

import (
	"archive/tar"
	"cmp"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

// BundleTag es la release de whisper.cpp que usa Meet2Do. Para actualizarla, cambia el tag
// y los SHA-256 de bundles (están en el digest de cada asset de la release en GitHub).
const BundleTag = "b5454"

// DefaultBundleBaseURL es de donde se descargan los builds oficiales de whisper.cpp.
const DefaultBundleBaseURL = "https://github.com/ggml-org/whisper.cpp/releases/download"

// binaryName es el ejecutable de whisper.cpp dentro del bundle.
const binaryName = "whisper-cli"

// maxBundleFile limita el tamaño de cada archivo extraído (defensa ante tar malicioso).
const maxBundleFile = 64 << 20

type bundle struct {
	asset  string
	sha256 string
	size   int64
}

// bundles son los builds oficiales de whisper.cpp para Linux, por arquitectura.
var bundles = map[string]bundle{
	"amd64": {"whisper-bin-ubuntu-x64.tar.gz", "a72becf15d7917f990f6313867a52638b82b7f9ef237fb0c980dac56a135781c", 10_364_195},
	"arm64": {"whisper-bin-ubuntu-arm64.tar.gz", "6b95ebfc60447df48e70ef00a73bdc3f41679ed2d01ef465827206a2ff325149", 4_608_377},
}

var (
	// ErrUnsupportedPlatform indica que no hay build de whisper.cpp para esta arquitectura.
	ErrUnsupportedPlatform = fmt.Errorf("whisper.cpp no tiene build para linux/%s", runtime.GOARCH)
	// ErrChecksum indica que el archivo descargado no coincide con el SHA-256 esperado.
	ErrChecksum = errors.New("el archivo descargado no coincide con el SHA-256 esperado")
	// ErrUnsafeArchive indica que el tar contiene rutas o enlaces fuera de su carpeta.
	ErrUnsafeArchive = errors.New("el archivo de whisper.cpp contiene rutas inseguras")
)

// Phase es una etapa de la instalación posterior a la descarga.
type Phase int

const (
	PhaseVerifying Phase = iota + 1
	PhaseExtracting
)

// BundleSize es el tamaño de descarga de whisper.cpp para esta arquitectura (0 si no hay build).
func BundleSize() int64 { return bundles[runtime.GOARCH].size }

// binaryDir es la carpeta en caché donde vive el bundle de esta versión.
func binaryDir() (string, error) {
	dir, err := WhisperDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, BundleTag), nil
}

// override es el whisper-cli indicado explícitamente (Whisper.Binary o MEET2DO_WHISPER).
func (w *Whisper) override() string { return cmp.Or(w.Binary, os.Getenv("MEET2DO_WHISPER")) }

// BinaryPath devuelve el whisper-cli a usar: Whisper.Binary, MEET2DO_WHISPER o el de la caché.
func (w *Whisper) BinaryPath() (string, error) {
	if p := w.override(); p != "" {
		return p, nil
	}
	dir, err := binaryDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, binaryName), nil
}

// NeedsBinary indica si hay que descargar whisper.cpp.
func (w *Whisper) NeedsBinary() bool {
	p, err := w.BinaryPath()
	if err != nil {
		return true
	}
	_, err = os.Stat(p)
	return err != nil
}

// EnsureBinary descarga, verifica y extrae whisper.cpp en la caché si aún no está.
// phase se llama al pasar a verificar y a extraer; ambos callbacks pueden ser nil.
func (w *Whisper) EnsureBinary(ctx context.Context, progress ProgressFunc, phase func(Phase)) (string, error) {
	if p := w.override(); p != "" {
		if _, err := os.Stat(p); err != nil {
			return "", fmt.Errorf("whisper-cli indicado no existe: %w", err)
		}
		return p, nil
	}
	bin, err := w.BinaryPath()
	if err != nil {
		return "", err
	}
	if !w.NeedsBinary() {
		return bin, nil
	}
	b, ok := bundles[runtime.GOARCH]
	if !ok {
		return "", ErrUnsupportedPlatform
	}

	dir := filepath.Dir(bin)
	parent := filepath.Dir(dir)
	archive := filepath.Join(parent, b.asset)
	url := fmt.Sprintf("%s/%s/%s", cmp.Or(w.BundleBaseURL, DefaultBundleBaseURL), BundleTag, b.asset)
	sum, err := download(ctx, url, archive, progress)
	if err != nil {
		return "", fmt.Errorf("descargar whisper.cpp: %w", err)
	}
	defer func() { _ = os.Remove(archive) }() // el tar ya no se necesita

	notify(phase, PhaseVerifying)
	if sum != b.sha256 {
		return "", fmt.Errorf("%w (%s)", ErrChecksum, b.asset)
	}

	notify(phase, PhaseExtracting)
	tmp, err := os.MkdirTemp(parent, ".extract-")
	if err != nil {
		return "", fmt.Errorf("crear carpeta temporal: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }() // vacía si el rename funcionó
	if err = extractBundle(archive, tmp); err != nil {
		return "", err
	}
	if _, err = os.Stat(filepath.Join(tmp, binaryName)); err != nil {
		return "", fmt.Errorf("el bundle de whisper.cpp no contiene %s: %w", binaryName, err)
	}
	if err = os.Rename(tmp, dir); err != nil {
		return "", fmt.Errorf("instalar whisper.cpp en %s: %w", dir, err)
	}
	w.Binary = bin
	return bin, nil
}

func notify(fn func(Phase), p Phase) {
	if fn != nil {
		fn(p)
	}
}

// extractBundle copia whisper-cli y las bibliotecas lib*.so* del primer nivel del tar a dst.
func extractBundle(archive, dst string) error {
	f, err := os.Open(archive)
	if err != nil {
		return fmt.Errorf("abrir %s: %w", archive, err)
	}
	defer func() { _ = f.Close() }() // solo lectura

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("leer %s: %w", archive, err)
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("leer %s: %w", archive, err)
		}
		name, ok, err := bundleEntry(hdr)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if err := writeEntry(tr, hdr, filepath.Join(dst, name)); err != nil {
			return err
		}
	}
}

// bundleEntry decide si una entrada del tar se extrae y con qué nombre.
func bundleEntry(hdr *tar.Header) (string, bool, error) {
	clean := path.Clean(hdr.Name)
	if path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", false, fmt.Errorf("%w: %s", ErrUnsafeArchive, hdr.Name)
	}
	// Solo archivos del primer nivel dentro de la carpeta raíz del tar.
	parts := strings.Split(clean, "/")
	if len(parts) != 2 {
		return "", false, nil
	}
	name := parts[1]
	isLib := strings.HasPrefix(name, "lib") && strings.Contains(name, ".so")
	if name != binaryName && !isLib {
		return "", false, nil
	}
	switch hdr.Typeflag {
	case tar.TypeReg:
		return name, true, nil
	case tar.TypeSymlink:
		// Los enlaces del bundle apuntan a otra biblioteca de la misma carpeta.
		if strings.Contains(hdr.Linkname, "/") || hdr.Linkname == ".." {
			return "", false, fmt.Errorf("%w: %s -> %s", ErrUnsafeArchive, hdr.Name, hdr.Linkname)
		}
		return name, true, nil
	}
	return "", false, nil
}

func writeEntry(r io.Reader, hdr *tar.Header, dst string) error {
	if hdr.Typeflag == tar.TypeSymlink {
		if err := os.Symlink(hdr.Linkname, dst); err != nil {
			return fmt.Errorf("crear enlace %s: %w", dst, err)
		}
		return nil
	}
	if hdr.Size > maxBundleFile {
		return fmt.Errorf("%w: %s es demasiado grande", ErrUnsafeArchive, hdr.Name)
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("crear %s: %w", dst, err)
	}
	if _, err = io.Copy(out, io.LimitReader(r, maxBundleFile)); err != nil {
		_ = out.Close() // el error relevante es el de la copia
		return fmt.Errorf("extraer %s: %w", dst, err)
	}
	if err = out.Close(); err != nil {
		return fmt.Errorf("cerrar %s: %w", dst, err)
	}
	return nil
}
