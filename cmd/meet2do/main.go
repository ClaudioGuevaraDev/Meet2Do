// Meet2Do graba una reunión, la transcribe con whisper.cpp y genera un TODO con Claude o Codex.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/ClaudioGuevaraDev/Meet2Do/internal/ai"
	"github.com/ClaudioGuevaraDev/Meet2Do/internal/output"
	"github.com/ClaudioGuevaraDev/Meet2Do/internal/recorder"
	"github.com/ClaudioGuevaraDev/Meet2Do/internal/transcriber"
	"github.com/ClaudioGuevaraDev/Meet2Do/internal/ui"
)

// version es la versión de la app (A.B.C); ver la regla de versionado en CLAUDE.md.
const version = "0.2.3"

// errNoFFmpeg indica que ffmpeg no está en el PATH.
var errNoFFmpeg = errors.New("ffmpeg no está instalado (sudo apt install ffmpeg)")

func main() {
	model := flag.String("model", transcriber.DefaultModel, "modelo de whisper.cpp (tiny, base, small, medium, large-v3, ...)")
	lang := flag.String("lang", transcriber.DefaultLang, "idioma de la reunión para whisper (auto, es, en, ...)")
	showVersion := flag.Bool("version", false, "muestra la versión y termina")
	flag.Parse()

	if *showVersion {
		fmt.Println("meet2do", version)
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, *model, *lang)
	stop()
	if err != nil {
		if errors.Is(err, ui.ErrAborted) || errors.Is(err, context.Canceled) {
			fmt.Println(ui.Subtle("Cancelado."))
			os.Exit(130)
		}
		ui.Error(err.Error())
		os.Exit(1)
	}
}

// session agrupa el estado que comparten los pasos del flujo.
type session struct {
	provider ai.Provider
	whisper  *transcriber.Whisper
	tmpDir   string
	audio    string
	recorded time.Time
	kinds    []output.Kind
	dir      string
	written  []string
}

func run(ctx context.Context, model, lang string) error {
	ui.Banner()

	s := &session{}
	if err := s.checkDependencies(model, lang); err != nil {
		return err
	}
	if err := s.chooseProvider(); err != nil {
		return err
	}

	tmp, err := os.MkdirTemp("", "meet2do-")
	if err != nil {
		return fmt.Errorf("crear carpeta temporal: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }() // limpieza best-effort
	s.tmpDir = tmp

	if err := s.record(); err != nil {
		return err
	}
	if err := s.chooseDestination(); err != nil {
		return err
	}
	if err := s.writeOutputs(ctx); err != nil {
		return err
	}

	fmt.Println()
	for _, p := range s.written {
		ui.Success("Guardado " + p)
	}
	return nil
}

func (s *session) checkDependencies(model, lang string) error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return errNoFFmpeg
	}
	bin, err := transcriber.FindBinary()
	if err != nil {
		return err
	}
	s.whisper = &transcriber.Whisper{Binary: bin, Model: model, Lang: lang}
	return nil
}

func (s *session) chooseProvider() error {
	providers, err := ai.Available()
	if err != nil {
		return err
	}
	if len(providers) == 1 {
		s.provider = providers[0]
		ui.Info("Usando " + s.provider.Name() + " (único CLI disponible)")
		return nil
	}
	s.provider, err = ui.ChooseProvider(providers)
	return err
}

func (s *session) record() error {
	start, err := ui.Confirm("¿Comenzar la grabación?", "Se grabará tu micrófono y el audio del sistema", true)
	if err != nil {
		return err
	}
	if !start {
		return ui.ErrAborted
	}

	s.audio = filepath.Join(s.tmpDir, "reunion.wav")
	rec := recorder.New(s.audio)
	if err := rec.Start(); err != nil {
		return err
	}
	s.recorded = time.Now()
	elapsed, uiErr := ui.RecordingScreen()
	// Detener siempre la grabación, aunque la pantalla haya fallado.
	if err := rec.Stop(); err != nil {
		return err
	}
	if uiErr != nil {
		return uiErr
	}
	ui.Success(fmt.Sprintf("Grabación terminada (%s)", elapsed.Truncate(time.Second)))
	return nil
}

func (s *session) chooseDestination() error {
	kinds, err := ui.ChooseOutputs()
	if err != nil {
		return err
	}
	dir, err := ui.ChooseDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("crear carpeta %s: %w", dir, err)
	}

	existing := output.Existing(dir, kinds)
	if len(existing) > 0 {
		ok, err := ui.Confirm("Algunos archivos ya existen. ¿Sobrescribir?", strings.Join(existing, "\n"), false)
		if err != nil {
			return err
		}
		if !ok {
			return ui.ErrAborted
		}
	}
	s.kinds, s.dir = kinds, dir
	return nil
}

func (s *session) wants(k output.Kind) bool { return slices.Contains(s.kinds, k) }

func (s *session) writeOutputs(ctx context.Context) error {
	if s.wants(output.Audio) {
		if err := s.save(output.CopyAudio(s.dir, s.audio)); err != nil {
			return err
		}
	}
	if !s.wants(output.TODO) && !s.wants(output.Transcript) {
		return nil
	}

	transcript, err := s.transcribe(ctx)
	if err != nil {
		return err
	}
	if s.wants(output.Transcript) {
		if err = s.save(output.WriteTranscript(s.dir, transcript, s.recorded)); err != nil {
			return err
		}
	}
	if !s.wants(output.TODO) || transcript == "" {
		return nil
	}

	todo, err := s.generateTODO(ctx, transcript)
	if err != nil {
		return err
	}
	return s.save(output.WriteTODO(s.dir, todo))
}

// save registra la ruta escrita por un writer de output.
func (s *session) save(path string, err error) error {
	if err != nil {
		return err
	}
	s.written = append(s.written, path)
	return nil
}

func (s *session) transcribe(ctx context.Context) (string, error) {
	w := s.whisper
	if !w.ModelExists() {
		title := fmt.Sprintf("Descargando modelo de whisper (%s)…", w.Model)
		err := ui.Spin(ctx, title, "Modelo "+w.Model+" descargado", func(ctx context.Context, status ui.Status) error {
			return w.DownloadModel(ctx, func(done, total int64) {
				if total > 0 {
					status(fmt.Sprintf("Descargando modelo de whisper (%s)… %d%%", w.Model, done*100/total))
				}
			})
		})
		if err != nil {
			return "", err
		}
	}

	var transcript string
	err := ui.Spin(ctx, "Transcribiendo audio con whisper.cpp…", "Audio transcrito", func(ctx context.Context, _ ui.Status) error {
		var err error
		transcript, err = w.Transcribe(ctx, s.audio)
		return err
	})
	if err != nil {
		return "", err
	}
	if transcript == "" {
		ui.Error("La transcripción quedó vacía (¿había audio?)")
	}
	return transcript, nil
}

func (s *session) generateTODO(ctx context.Context, transcript string) (string, error) {
	name := s.provider.Name()
	var todo string
	err := ui.Spin(ctx, "Generando TODO con "+name+"…", "TODO generado con "+name, func(ctx context.Context, _ ui.Status) error {
		var err error
		todo, err = s.provider.GenerateTODO(ctx, transcript)
		return err
	})
	return todo, err
}
