//go:build integration

package recorder_test

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ClaudioGuevaraDev/Meet2Do/internal/recorder"
)

// Graba con el servidor de audio real. Ejecutar con: go test -tags integration ./internal/recorder
func TestRecordWithPulse(t *testing.T) {
	out := filepath.Join(t.TempDir(), "real.wav")
	r := recorder.New(out)
	if err := r.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	time.Sleep(3 * time.Second) // grabación real: no hay condición que esperar
	if err := r.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		t.Fatalf("invalid WAV header: %q", data[:12])
	}
	rate := binary.LittleEndian.Uint32(data[24:28])
	samples := len(data[44:]) / 2
	secs := float64(samples) / float64(rate)
	t.Logf("rate=%d samples=%d (%.2fs)", rate, samples, secs)
	if rate != recorder.SampleRate || secs < 2.5 {
		t.Errorf("want ~3s at %d Hz, got %.2fs at %d Hz", recorder.SampleRate, secs, rate)
	}
}
