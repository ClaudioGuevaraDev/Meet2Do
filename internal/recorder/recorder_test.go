package recorder_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ClaudioGuevaraDev/Meet2Do/internal/recorder"
	"github.com/ClaudioGuevaraDev/Meet2Do/internal/testutil"
)

// Los scripts reciben la ruta de salida como último argumento (ver Recorder.Args).
const lastArg = `for a in "$@"; do out=$a; done
`

func TestArgs(t *testing.T) {
	t.Parallel()
	args := recorder.New("/tmp/out.wav").Args()
	tests := []struct {
		flag, value string
	}{
		{"-i", recorder.DefaultMic},
		{"-ar", "16000"},
		{"-ac", "1"},
		{"-c:a", "pcm_s16le"},
	}
	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			t.Parallel()
			i := slices.Index(args, tt.flag)
			if i < 0 || i+1 >= len(args) || args[i+1] != tt.value {
				t.Errorf("want %s %s in %v", tt.flag, tt.value, args)
			}
		})
	}
	if !slices.Contains(args, recorder.DefaultMonitor) {
		t.Errorf("want system monitor input in %v", args)
	}
	if args[len(args)-1] != "/tmp/out.wav" {
		t.Errorf("last arg must be the output file: %v", args)
	}
}

func TestStartStop(t *testing.T) {
	tests := []struct {
		name         string
		script       string
		wantStartErr string
		wantStopErr  error
	}{
		{
			// Graba hasta recibir "q" por stdin, como ffmpeg.
			name:   "records until q",
			script: lastArg + "head -c 100 /dev/zero > \"$out\"\nread -r q\n",
		},
		{
			name:         "ffmpeg dies on start",
			script:       "echo 'Unknown input format: pulse' >&2\nexit 1\n",
			wantStartErr: "Unknown input format",
		},
		{
			name:        "empty recording",
			script:      lastArg + "head -c 44 /dev/zero > \"$out\"\nread -r q\n",
			wantStopErr: recorder.ErrEmptyRecording,
		},
		{
			name:        "no output file",
			script:      "read -r q\n",
			wantStopErr: recorder.ErrEmptyRecording,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			testutil.FakeBin(t, dir, "ffmpeg", tt.script)
			testutil.IsolatedPath(t, dir)

			r := recorder.New(filepath.Join(dir, "out.wav"))
			err := r.Start()
			if tt.wantStartErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantStartErr) {
					t.Fatalf("got start error %v; want one containing %q", err, tt.wantStartErr)
				}
				if !errors.Is(r.Stop(), recorder.ErrNotStarted) {
					t.Error("Stop after failed Start should return ErrNotStarted")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected start error: %v", err)
			}

			err = r.Stop()
			if !errors.Is(err, tt.wantStopErr) {
				t.Fatalf("got stop error %v; want %v", err, tt.wantStopErr)
			}
			if err == nil {
				if info, statErr := os.Stat(r.Output); statErr != nil || info.Size() != 100 {
					t.Errorf("output not written correctly: %v", statErr)
				}
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

func TestStartMissingFFmpeg(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir) // sin ffmpeg

	r := recorder.New(filepath.Join(dir, "out.wav"))
	if err := r.Start(); err == nil {
		t.Fatal("expected error when ffmpeg is missing")
	}
}
