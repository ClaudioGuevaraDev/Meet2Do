package recorder

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// wavHeaderSize es el tamaño de la cabecera de un WAV PCM canónico.
const wavHeaderSize = 44

// wavWriter escribe PCM 16 bits mono; los tamaños de la cabecera se corrigen al cerrar.
type wavWriter struct {
	f       *os.File
	bw      *bufio.Writer
	rate    int
	samples int64
}

func createWAV(path string, rate int) (*wavWriter, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("crear %s: %w", path, err)
	}
	w := &wavWriter{f: f, bw: bufio.NewWriter(f), rate: rate}
	if err = w.header(); err != nil {
		_ = f.Close() // el error relevante es el de la cabecera
		return nil, fmt.Errorf("escribir cabecera de %s: %w", path, err)
	}
	return w, nil
}

// header escribe la cabecera con los tamaños actuales.
func (w *wavWriter) header() error {
	data := uint32(w.samples * 2)
	h := make([]byte, 0, wavHeaderSize)
	h = append(h, "RIFF"...)
	h = binary.LittleEndian.AppendUint32(h, 36+data)
	h = append(h, "WAVEfmt "...)
	h = binary.LittleEndian.AppendUint32(h, 16)             // tamaño del bloque fmt
	h = binary.LittleEndian.AppendUint16(h, 1)              // PCM
	h = binary.LittleEndian.AppendUint16(h, 1)              // mono
	h = binary.LittleEndian.AppendUint32(h, uint32(w.rate)) // muestras por segundo
	h = binary.LittleEndian.AppendUint32(h, uint32(w.rate*2))
	h = binary.LittleEndian.AppendUint16(h, 2)  // bytes por muestra
	h = binary.LittleEndian.AppendUint16(h, 16) // bits por muestra
	h = append(h, "data"...)
	h = binary.LittleEndian.AppendUint32(h, data)
	_, err := w.bw.Write(h)
	return err
}

func (w *wavWriter) write(s []int16) error {
	if err := binary.Write(w.bw, binary.LittleEndian, s); err != nil {
		return err
	}
	w.samples += int64(len(s))
	return nil
}

// close vacía el búfer, reescribe la cabecera con los tamaños finales y cierra el archivo.
func (w *wavWriter) close() error {
	err := w.bw.Flush()
	if err == nil {
		_, err = w.f.Seek(0, io.SeekStart)
	}
	if err == nil {
		w.bw.Reset(w.f)
		err = w.header()
	}
	if err == nil {
		err = w.bw.Flush()
	}
	if cerr := w.f.Close(); err == nil {
		err = cerr
	}
	return err
}
