package recorder_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ClaudioGuevaraDev/Meet2Do/internal/recorder"
)

var update = flag.Bool("update", false, "actualiza los golden files")

// fakeSource es una recorder.Source controlada por el test.
type fakeSource struct {
	mu       sync.Mutex
	write    func([]int16)
	startErr error
	closeErr error
	closed   bool
}

func (f *fakeSource) Start(write func([]int16)) error {
	if f.startErr != nil {
		return f.startErr
	}
	f.mu.Lock()
	f.write = write
	f.mu.Unlock()
	return nil
}

func (f *fakeSource) Close() error {
	f.mu.Lock()
	f.closed = true
	f.mu.Unlock()
	return f.closeErr
}

func (f *fakeSource) emit(s ...int16) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.write(s)
}

func TestMix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		a, b []int16
		want []int16
	}{
		{"sums samples", []int16{1, 2, 3}, []int16{10, 20, 30}, []int16{11, 22, 33}},
		{"saturates high", []int16{30000}, []int16{30000}, []int16{32767}},
		{"saturates low", []int16{-30000}, []int16{-30000}, []int16{-32768}},
		{"shorter input wins", []int16{1, 2, 3}, []int16{1}, []int16{2}},
		{"empty", nil, []int16{1}, []int16{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := recorder.Mix(make([]int16, 8), tt.a, tt.b)
			if !slices.Equal(got, tt.want) {
				t.Errorf("Mix(%v, %v) = %v; want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func FuzzMix(f *testing.F) {
	f.Add([]byte{0, 0, 255, 127}, []byte{1, 0, 255, 127})
	f.Fuzz(func(t *testing.T, ra, rb []byte) {
		a, b := samples(ra), samples(rb)
		got := recorder.Mix(make([]int16, len(a)), a, b)
		if len(got) != min(len(a), len(b)) {
			t.Fatalf("len = %d; want %d", len(got), min(len(a), len(b)))
		}
		for i, s := range got {
			want := max(-32768, min(32767, int32(a[i])+int32(b[i])))
			if int32(s) != want {
				t.Fatalf("sample %d = %d; want %d", i, s, want)
			}
		}
	})
}

func TestRecorderWritesMixedWAV(t *testing.T) {
	t.Parallel()
	mic, monitor := &fakeSource{}, &fakeSource{}
	out := filepath.Join(t.TempDir(), "out.wav")
	r := recorder.NewWithSources(out, mic, monitor)
	if err := r.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	mic.emit(1000, 2000)
	monitor.emit(1000, -500, 30000)
	mic.emit(30000)
	monitor.emit(7) // el monitor queda más largo: el micrófono se rellena con silencio
	if err := r.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if !mic.closed || !monitor.closed {
		t.Error("sources should be closed after Stop")
	}

	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if want := []int16{2000, 1500, 32767, 7}; !slices.Equal(samples(got[44:]), want) {
		t.Errorf("samples = %v; want %v", samples(got[44:]), want)
	}

	golden := filepath.Join("testdata", "mixed.wav.golden")
	if *update {
		if err = os.WriteFile(golden, got, 0o644); err != nil {
			t.Fatalf("update golden: %v", err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("WAV mismatch:\ngot  %x\nwant %x", got, want)
	}
}

func TestRecorderStreamsWhenOneSourceIsSilent(t *testing.T) {
	t.Parallel()
	mic, monitor := &fakeSource{}, &fakeSource{}
	out := filepath.Join(t.TempDir(), "out.wav")
	r := recorder.NewWithSources(out, mic, monitor)
	if err := r.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	// 3 s solo de micrófono: el monitor no entrega nada (p. ej. no suena nada en el equipo).
	const total = 3 * recorder.SampleRate
	mic.emit(make([]int16, total)...)

	// Lo que excede el desfase máximo se escribe a disco durante la grabación, sin esperar a Stop.
	waitFor(t, func() bool {
		info, err := os.Stat(out)
		return err == nil && info.Size() > 44
	})
	if err := r.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	info, err := os.Stat(out)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := (info.Size() - 44) / 2; got != total {
		t.Errorf("samples = %d; want %d", got, total)
	}
}

func TestRecorderStopErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		closeErr error
		wantErr  error
		wantText string
	}{
		{name: "no audio", wantErr: recorder.ErrEmptyRecording},
		{name: "no audio with source error", closeErr: errors.New("stream perdido"), wantErr: recorder.ErrEmptyRecording, wantText: "stream perdido"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := recorder.NewWithSources(filepath.Join(t.TempDir(), "out.wav"), &fakeSource{closeErr: tt.closeErr}, &fakeSource{})
			if err := r.Start(); err != nil {
				t.Fatalf("start: %v", err)
			}
			err := r.Stop()
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("got %v; want %v", err, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantText) {
				t.Errorf("error %q should contain %q", err, tt.wantText)
			}
			if err := r.Stop(); !errors.Is(err, recorder.ErrNotStarted) {
				t.Errorf("second Stop = %v; want ErrNotStarted", err)
			}
		})
	}
}

func TestRecorderStartErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		output  func(t *testing.T) string
		mic     *fakeSource
		wantErr string
	}{
		{
			name:    "source fails to start",
			output:  func(t *testing.T) string { return filepath.Join(t.TempDir(), "out.wav") },
			mic:     &fakeSource{startErr: errors.New("dispositivo ocupado")},
			wantErr: "dispositivo ocupado",
		},
		{
			name:    "output dir missing",
			output:  func(t *testing.T) string { return filepath.Join(t.TempDir(), "no", "out.wav") },
			mic:     &fakeSource{},
			wantErr: "crear",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			monitor := &fakeSource{}
			r := recorder.NewWithSources(tt.output(t), tt.mic, monitor)
			err := r.Start()
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("got %v; want error containing %q", err, tt.wantErr)
			}
			if err := r.Stop(); !errors.Is(err, recorder.ErrNotStarted) {
				t.Errorf("Stop after failed Start = %v; want ErrNotStarted", err)
			}
		})
	}
}

func TestStopWithoutStart(t *testing.T) {
	t.Parallel()
	r := recorder.New(filepath.Join(t.TempDir(), "out.wav"))
	if err := r.Stop(); !errors.Is(err, recorder.ErrNotStarted) {
		t.Errorf("got %v; want ErrNotStarted", err)
	}
}

// waitFor espera (como máximo 5 s) a que cond se cumpla.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met before deadline")
		}
		runtime.Gosched()
	}
}

// samples interpreta bytes little-endian como int16 (ignora un byte sobrante).
func samples(b []byte) []int16 {
	out := make([]int16, len(b)/2)
	for i := range out {
		out[i] = int16(binary.LittleEndian.Uint16(b[2*i:]))
	}
	return out
}
