// Package cli es el programa Meet2Do: lo usan los comandos meet2do y m2do (su atajo).
// Graba una reunión, la transcribe con whisper.cpp y genera tareas y resumen con Claude o Codex.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
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
	"github.com/ClaudioGuevaraDev/Meet2Do/internal/selfupdate"
	"github.com/ClaudioGuevaraDev/Meet2Do/internal/transcriber"
	"github.com/ClaudioGuevaraDev/Meet2Do/internal/ui"
	"github.com/ClaudioGuevaraDev/Meet2Do/internal/uninstall"
)

// version es la versión de la app (A.B.C); ver la regla de versionado en CLAUDE.md.
const version = "0.11.1"

// Main es el punto de entrada de meet2do y m2do.
func Main() {
	model := flag.String("model", transcriber.DefaultModel, "modelo de whisper.cpp (tiny, base, small, medium, large-v3, ...)")
	lang := flag.String("lang", transcriber.DefaultLang, "idioma de la reunión para whisper (auto, es, en, ...)")
	showVersion := flag.Bool("version", false, "muestra la versión y termina")
	setupOnly := flag.Bool("setup", false, "descarga los componentes necesarios (whisper.cpp y el modelo) y termina")
	flag.Usage = usage
	flag.Parse()

	// Subcomandos: por ahora solo "uninstall".
	uninstallApp := false
	switch args := flag.Args(); {
	case len(args) == 0:
	case len(args) == 1 && args[0] == "uninstall":
		uninstallApp = true
	default:
		fmt.Fprintf(os.Stderr, "comando desconocido: %s\n\n", strings.Join(args, " "))
		usage()
		os.Exit(2)
	}

	if *showVersion {
		fmt.Println("meet2do", version)
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	var err error
	if uninstallApp {
		err = runUninstall()
	} else if err = checkUpdate(ctx); err == nil {
		err = run(ctx, *model, *lang, *setupOnly)
	}
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

// updateCheckTimeout limita la consulta a GitHub: sin red, Meet2Do arranca igual.
const updateCheckTimeout = 1500 * time.Millisecond

// checkUpdate busca una versión nueva y, si la hay, la instala y reinicia el programa con
// ella (mismos argumentos). Cualquier problema se informa y se sigue con la versión actual;
// solo devuelve error si el usuario cancela.
func checkUpdate(ctx context.Context) error {
	if !selfupdate.Enabled() {
		return nil
	}
	exe, _ := uninstall.InstalledBinary() // Enabled ya comprobó que existe
	checkCtx, cancel := context.WithTimeout(ctx, updateCheckTimeout)
	latest, err := selfupdate.Latest(checkCtx)
	cancel()
	if err != nil || !selfupdate.Newer(version, latest) {
		return nil // sin red o sin versión nueva: nada que mostrar
	}

	ui.Info(fmt.Sprintf("Hay una versión nueva de Meet2Do: v%s (tienes v%s)", latest, version))
	err = ui.Update(ctx, ui.Download{
		Name:   "Meet2Do v" + latest,
		Detail: "versión nueva",
		Run: func(ctx context.Context, progress func(done, total int64), _ func(string)) error {
			return selfupdate.Apply(ctx, exe, latest, progress)
		},
	})
	if errors.Is(err, ui.ErrAborted) || errors.Is(err, context.Canceled) {
		return err
	}
	if err != nil {
		hint := ""
		if errors.Is(err, fs.ErrPermission) {
			hint = " (vuelve a ejecutar el instalador para actualizar)"
		}
		ui.Warn(fmt.Sprintf("No se pudo actualizar a v%s: %v%s. Sigues usando v%s", latest, err, hint, version))
		return nil
	}
	ui.Success("Meet2Do actualizado a v" + latest)
	fmt.Println()

	// Reemplaza este proceso por la versión nueva; la marca evita volver a revisar.
	env := append(os.Environ(), selfupdate.UpdatedEnv+"=1")
	if err := syscall.Exec(exe, os.Args, env); err != nil {
		// Se sigue con la versión que ya está en memoria; la próxima vez arranca la nueva.
		ui.Warn(fmt.Sprintf("Se actualizó, pero no se pudo reiniciar (%v): la versión nueva se usará la próxima vez", err))
	}
	return nil
}

// usage muestra la ayuda (-h): el subcomando y los flags.
func usage() {
	// Si no se puede escribir la ayuda no hay nada más que hacer.
	_, _ = fmt.Fprint(flag.CommandLine.Output(), "Uso (meet2do o su atajo m2do):\n"+
		"  meet2do [flags]       graba una reunión y genera tareas y resumen\n"+
		"  meet2do uninstall     desinstala Meet2Do y lo que descargó (whisper.cpp y los modelos)\n\nFlags:\n")
	flag.PrintDefaults()
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
	if err = s.chooseModel(ctx); err != nil {
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

	if len(s.written) == 0 {
		// Solo se pidieron archivos de la IA y la transcripción quedó vacía.
		_ = os.Remove(s.dir) // la carpeta está vacía; si no se puede borrar, no es grave
		ui.Warn("No se generó ningún archivo")
		return nil
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

// chooseModel pregunta el modelo y el esfuerzo; "Por defecto" deja la configuración del CLI.
func (s *session) chooseModel(ctx context.Context) error {
	cat := s.provider.Catalog(ctx)
	model, err := ui.ChooseModel(s.provider.Name(), cat)
	if err != nil {
		return err
	}
	efforts := model.Efforts
	if model.ID == "" {
		efforts = cat.DefaultEfforts
	}
	effort, err := ui.ChooseEffort(efforts, cat.DefaultEffort(model))
	if err != nil {
		return err
	}
	s.provider = s.provider.With(model.ID, effort)
	return nil
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
	elapsed, uiErr := ui.RecordingScreen(rec)
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
	// Una carpeta nueva por reunión: nunca se sobrescribe nada.
	if dir, err = output.MeetingDir(dir, s.recorded); err != nil {
		return ui.Fail("No se pudo crear la carpeta de la reunión.", err)
	}
	s.kinds, s.dir = kinds, dir
	return nil
}

// aiDoc es un archivo que genera la IA a partir de la transcripción.
type aiDoc struct {
	kind       output.Kind
	prompt     string
	what       string // "tareas", "resumen": para el texto del spinner
	done, fail string
	write      func(dir, content string) (string, error)
}

var aiDocs = []aiDoc{
	{output.Tasks, ai.TasksPrompt, "tareas", "Tareas generadas correctamente", "No se pudieron generar las tareas.", output.WriteTasks},
	{output.Summary, ai.SummaryPrompt, "resumen", "Resumen generado correctamente", "No se pudo generar el resumen.", output.WriteSummary},
}

func (s *session) wants(k output.Kind) bool { return slices.Contains(s.kinds, k) }

func (s *session) writeOutputs(ctx context.Context) error {
	if s.wants(output.Audio) {
		if err := s.save(output.CopyAudio(s.dir, s.audio)); err != nil {
			return err
		}
	}
	needsAI := slices.ContainsFunc(aiDocs, func(d aiDoc) bool { return s.wants(d.kind) })
	if !needsAI && !s.wants(output.Transcript) {
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
	if transcript == "" {
		return nil
	}
	for _, d := range aiDocs {
		if !s.wants(d.kind) {
			continue
		}
		content, err := s.generate(ctx, d, transcript)
		if err != nil {
			return err
		}
		if err := s.save(d.write(s.dir, content)); err != nil {
			return err
		}
	}
	return nil
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

func (s *session) generate(ctx context.Context, d aiDoc, transcript string) (string, error) {
	title := "Generando " + d.what + " con " + s.provider.Label() + "…"
	var content string
	err := ui.Spin(ctx, title, d.done, d.fail, func(ctx context.Context, _ ui.Status) error {
		var err error
		content, err = s.provider.Generate(ctx, d.prompt, transcript)
		return err
	})
	return content, err
}

// runUninstall muestra lo instalado, pregunta qué borrar y lo borra.
func runUninstall() error {
	ui.Banner(version)
	cs, err := uninstall.Find()
	if err != nil {
		return ui.Fail("No se pudo revisar qué está instalado.", err)
	}
	if len(cs) == 0 {
		ui.Info("No hay nada de Meet2Do instalado")
		return nil
	}
	chosen, err := ui.ChooseUninstall(cs)
	if err != nil {
		return err
	}
	ok, err := ui.Confirm("¿Desinstalar lo seleccionado?", "Esto no se puede deshacer.", false)
	if err != nil {
		return err
	}
	if !ok {
		return ui.ErrAborted
	}

	var freed int64
	var errs []error
	binaryRemoved := false
	for _, c := range chosen {
		if err := uninstall.Remove(c); err != nil {
			if errors.Is(err, fs.ErrPermission) {
				err = fmt.Errorf("%w (bórralo con: sudo rm -r %s)", err, strings.Join(c.Paths, " "))
			}
			errs = append(errs, err)
			continue
		}
		freed += c.Size
		binaryRemoved = binaryRemoved || c.Name == uninstall.App
		ui.Success("Eliminado " + c.Name + " · " + ui.FormatBytes(c.Size))
	}
	uninstall.Cleanup()
	if len(errs) > 0 {
		return ui.Fail("No se pudo desinstalar todo.", errors.Join(errs...))
	}
	fmt.Println()
	if binaryRemoved {
		ui.Success("Meet2Do se desinstaló · se liberaron " + ui.FormatBytes(freed))
	} else {
		ui.Success("Se liberaron " + ui.FormatBytes(freed))
	}
	return nil
}
