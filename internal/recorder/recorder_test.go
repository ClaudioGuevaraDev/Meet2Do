package recorder

import (
	"slices"
	"testing"
)

func TestArgs(t *testing.T) {
	args := New("/tmp/out.wav").Args()
	for _, want := range []string{DefaultMic, DefaultMonitor, "16000", "pcm_s16le"} {
		if !slices.Contains(args, want) {
			t.Errorf("args sin %q: %v", want, args)
		}
	}
	if args[len(args)-1] != "/tmp/out.wav" {
		t.Errorf("el último argumento debe ser la salida: %v", args)
	}
}
