// Meet2Do graba una reunión, la transcribe con whisper.cpp y genera un TODO con Claude o Codex.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"meet2do/internal/ai"
	"meet2do/internal/output"
	"meet2do/internal/recorder"
	"meet2do/internal/transcriber"
	"meet2do/internal/ui"
)

func main() {
	model := flag.String("model", "small", "modelo de whisper.cpp (tiny, base, small, medium, large-v3, ...)")
	lang := flag.String("lang", "auto", "idioma de la reunión para whisper (auto, es, en, ...)")
	flag.Parse()

	if err := run(*model, *lang); err != nil {
		if errors.Is(err, ui.ErrAborted) {
			fmt.Println(ui.Subtle("Cancelado."))
			os.Exit(130)
		}
		ui.Error(err.Error())
		os.Exit(1)
	}
}

func run(model, lang string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ui.Banner()

	// 1. Dependencias.
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return errors.New("ffmpeg no está instalado (sudo apt install ffmpeg)")
	}
	whisperBin, err := transcriber.FindBinary()
	if err != nil {
		return err
	}
	providers := ai.Available()
	if len(providers) == 0 {
		return errors.New("no se encontró `claude` ni `codex` en el PATH")
	}

	// 2. Elegir IA.
	provider := providers[0]
	if len(providers) > 1 {
		if provider, err = ui.ChooseProvider(providers); err != nil {
			return err
		}
	} else {
		ui.Info("Usando " + provider.Name() + " (único CLI disponible)")
	}

	// 3. Grabar.
	start, err := ui.Confirm("¿Comenzar la grabación?", "Se grabará tu micrófono y el audio del sistema", true)
	if err != nil {
		return err
	}
	if !start {
		return ui.ErrAborted
	}

	tmp, err := os.MkdirTemp("", "meet2do-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	audio := filepath.Join(tmp, "reunion.wav")
	rec := recorder.New(audio)
	if err := rec.Start(); err != nil {
		return err
	}
	recordedAt := time.Now()
	elapsed, uiErr := ui.RecordingScreen()
	if err := rec.Stop(); err != nil {
		return err
	}
	if uiErr != nil {
		return uiErr
	}
	ui.Success(fmt.Sprintf("Grabación terminada (%s)", elapsed.Truncate(time.Second)))

	// 4. Elegir salidas y carpeta.
	kinds, err := ui.ChooseOutputs()
	if err != nil {
		return err
	}
	dir, err := ui.ChooseDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if existing := output.Existing(dir, kinds); len(existing) > 0 {
		ok, err := ui.Confirm("Algunos archivos ya existen. ¿Sobrescribir?", strings.Join(existing, "\n"), false)
		if err != nil {
			return err
		}
		if !ok {
			return ui.ErrAborted
		}
	}

	wantTODO := slices.Contains(kinds, output.TODO)
	wantTranscript := slices.Contains(kinds, output.Transcript)
	var written []string

	if slices.Contains(kinds, output.Audio) {
		p, err := output.CopyAudio(dir, audio)
		if err != nil {
			return err
		}
		written = append(written, p)
	}

	// 5. Transcribir.
	var transcript string
	if wantTODO || wantTranscript {
		w := &transcriber.Whisper{Binary: whisperBin, Model: model, Lang: lang}
		if !w.ModelExists() {
			err := ui.Spin("Descargando modelo de whisper ("+model+")…", "Modelo "+model+" descargado", func(status ui.Status) error {
				return w.DownloadModel(ctx, func(done, total int64) {
					if total > 0 {
						status(fmt.Sprintf("Descargando modelo de whisper (%s)… %d%%", model, done*100/total))
					}
				})
			})
			if err != nil {
				return err
			}
		}
		err := ui.Spin("Transcribiendo audio con whisper.cpp…", "Audio transcrito", func(ui.Status) error {
			var err error
			transcript, err = w.Transcribe(ctx, audio)
			return err
		})
		if err != nil {
			return err
		}
		if transcript == "" {
			ui.Error("La transcripción quedó vacía (¿había audio?)")
		}
		if wantTranscript {
			p, err := output.WriteTranscript(dir, transcript, recordedAt)
			if err != nil {
				return err
			}
			written = append(written, p)
		}
	}

	// 6. Generar TODO.
	if wantTODO && transcript != "" {
		var todo string
		err := ui.Spin("Generando TODO con "+provider.Name()+"…", "TODO generado con "+provider.Name(), func(ui.Status) error {
			var err error
			todo, err = provider.GenerateTODO(ctx, transcript)
			return err
		})
		if err != nil {
			return err
		}
		p, err := output.WriteTODO(dir, todo)
		if err != nil {
			return err
		}
		written = append(written, p)
	}

	// 7. Resumen.
	fmt.Println()
	for _, p := range written {
		ui.Success("Guardado " + p)
	}
	return nil
}
