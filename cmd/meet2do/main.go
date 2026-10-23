// Meet2Do graba una reunión, la transcribe con whisper.cpp y genera una lista de tareas con Claude o Codex.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
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
const version = "0.4.4"

func main() {
	model := flag.String("model", transcriber.DefaultModel, "modelo de whisper.cpp (tiny, base, small, medium, large-v3, ...)")
	lang := flag.String("lang", transcriber.DefaultLang, "idioma de la reunión para whisper (auto, es, en, ...)")
	showVersion := flag.Bool("version", false, "muestra la versión y termina")
	setupOnly := flag.Bool("setup", false, "descarga los componentes necesarios (whisper.cpp y el modelo) y termina")
	flag.Parse()

	if *showVersion {
		fmt.Println("meet2do", version)
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, *model, *lang, *setupOnly)
	stop()
	if err != nil {
		if errors.Is(err, ui.ErrAborted) || errors.Is(err, context.Canceled) {
			ui.Canceled()
			os.Exit(130)
		}
		var step *ui.StepError
		if errors.As(err, &step) {
			ui.Failure(step.Summary, step.Err)
		} else {
			ui.Failure("No se pudo completar la operación.", err)
		}
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

func run(ctx context.Context, model, lang string, setupOnly bool) error {
	ui.Banner(version)

	s := &session{whisper: &transcriber.Whisper{Model: model, Lang: lang}}
	if setupOnly {
		return s.setup(ctx, true)
	}

	// Primero lo que no se puede descargar: sin claude/codex no tiene sentido instalar nada.
	providers, err := ai.Available()
	if err != nil {
		return ui.Fail("No se encontró una IA disponible.", err)
	}
	if err = s.setup(ctx, false); err != nil {
		return err
	}
	if err = s.chooseProvider(providers); err != nil {
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

	ui.Done(s.dir, s.written)
	return nil
}

// setup descarga lo que falte (whisper.cpp y el modelo) mostrando la pantalla de instalación.
func (s *session) setup(ctx context.Context, verbose bool) error {
	downloads := s.pendingDownloads()
	if len(downloads) == 0 {
		if verbose {
			ui.Success("Todo está instalado; Meet2Do está listo para usarse")
		}
		return nil
	}
	ui.Info(fmt.Sprintf("Primera ejecución: Meet2Do descargará los componentes necesarios (~%s). Esto ocurre una sola vez.",
		ui.FormatBytes(ui.TotalSize(downloads))))
	if err := ui.Install(ctx, downloads); err != nil {
		return ui.Fail("No se pudieron instalar los componentes.", err)
	}
	ui.Success("Componentes instalados")
	return nil
}

func (s *session) pendingDownloads() []ui.Download {
	w := s.whisper
	var downloads []ui.Download
	if w.NeedsBinary() {
		downloads = append(downloads, ui.Download{
			Name:   "whisper.cpp " + transcriber.BundleTag,
			Detail: "motor de transcripción",
			Size:   transcriber.BundleSize(),
			Run: func(ctx context.Context, progress func(done, total int64), phase func(string)) error {
				_, err := w.EnsureBinary(ctx, progress, func(p transcriber.Phase) { phase(phaseLabel(p)) })
				return err
			},
		})
	}
	if !w.ModelExists() {
		downloads = append(downloads, ui.Download{
			Name:   "Modelo de voz " + w.ModelName(),
			Detail: "reconocimiento de voz",
			Size:   w.ModelSize(),
			Run: func(ctx context.Context, progress func(done, total int64), _ func(string)) error {
				return w.DownloadModel(ctx, progress)
			},
		})
	}
	return downloads
}

func phaseLabel(p transcriber.Phase) string {
	switch p {
	case transcriber.PhaseVerifying:
		return "Verificando integridad (SHA-256)"
	case transcriber.PhaseExtracting:
		return "Extrayendo"
	}
	return "Procesando"
}

func (s *session) chooseProvider(providers []ai.Provider) error {
	if len(providers) == 1 {
		s.provider = providers[0]
		ui.Info("Usando " + s.provider.Name() + " (único CLI disponible)")
		return nil
	}
	var err error
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
		return ui.Fail("No se pudo iniciar la grabación.", err)
	}
	s.recorded = time.Now()
	elapsed, uiErr := ui.RecordingScreen()
	// Detener siempre la grabación, aunque la pantalla haya fallado.
	if err := rec.Stop(); err != nil {
		return ui.Fail("No se pudo guardar la grabación.", err)
	}
	if uiErr != nil {
		return uiErr
	}
	ui.Success(fmt.Sprintf("Grabación completada · %s", elapsed.Truncate(time.Second)))
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
	if !s.wants(output.Tasks) && !s.wants(output.Transcript) {
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
	if !s.wants(output.Tasks) || transcript == "" {
		return nil
	}

	tasks, err := s.generateTasks(ctx, transcript)
	if err != nil {
		return err
	}
	return s.save(output.WriteTasks(s.dir, tasks))
}

// save registra la ruta escrita por un writer de output.
func (s *session) save(path string, err error) error {
	if err != nil {
		return ui.Fail("No se pudieron guardar los archivos.", err)
	}
	s.written = append(s.written, path)
	return nil
}

func (s *session) transcribe(ctx context.Context) (string, error) {
	w := s.whisper // whisper.cpp y el modelo ya se descargaron en setup
	var transcript string
	err := ui.Spin(ctx, "Transcribiendo reunión…", "Transcripción completada", "No se pudo completar la transcripción.", func(ctx context.Context, _ ui.Status) error {
		var err error
		transcript, err = w.Transcribe(ctx, s.audio)
		return err
	})
	if err != nil {
		return "", err
	}
	if transcript == "" {
		ui.Warn("La transcripción quedó vacía (¿había audio?)")
	}
	return transcript, nil
}

func (s *session) generateTasks(ctx context.Context, transcript string) (string, error) {
	name := s.provider.Name()
	var tasks string
	err := ui.Spin(ctx, "Generando tareas con "+name+"…", "Tareas generadas correctamente", "No se pudieron generar las tareas.", func(ctx context.Context, _ ui.Status) error {
		var err error
		tasks, err = s.provider.GenerateTasks(ctx, transcript)
		return err
	})
	return tasks, err
}
