package transcriber

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestArgs(t *testing.T) {
	w := &Whisper{Model: "small", Lang: "es"}
	args := w.Args("m.bin", "a.wav", "out")
	for _, pair := range [][2]string{{"-m", "m.bin"}, {"-f", "a.wav"}, {"-l", "es"}, {"-of", "out"}} {
		i := slices.Index(args, pair[0])
		if i < 0 || args[i+1] != pair[1] {
			t.Errorf("falta %s %s en %v", pair[0], pair[1], args)
		}
	}
}

func TestTranscribeWithFakeBinary(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	fake := filepath.Join(dir, "whisper-cli")
	// Escribe "hola mundo" en el archivo indicado por -of.
	script := "#!/bin/sh\nwhile [ $# -gt 0 ]; do [ \"$1\" = -of ] && out=$2; shift; done\necho ' hola mundo ' > \"$out.txt\"\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	w := &Whisper{Binary: fake, Model: "tiny", Lang: "auto"}
	got, err := w.Transcribe(t.Context(), filepath.Join(dir, "a.wav"))
	if err != nil {
		t.Fatal(err)
	}
	if got != "hola mundo" {
		t.Errorf("got %q", got)
	}
}
