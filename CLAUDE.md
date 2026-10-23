# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

Meet2Do is a Linux-only Go CLI that records a meeting (mic + system audio), transcribes it locally with whisper.cpp, and asks the `claude` or `codex` CLI to turn the transcript into a `TODO.md`. User-facing text (prompts, messages, the AI prompt, output file names) is in Spanish — keep it that way.

## Commands

```sh
make build                        # -> bin/meet2do (entry point: ./cmd/meet2do)
go run ./cmd/meet2do [-model small] [-lang auto] [-version]
make test | make test-race
make cover-check                  # fails if internal/{ai,output,recorder,transcriber} < 80%
make lint                         # go vet + golangci-lint (v2 config in .golangci.yml)
go test ./internal/transcriber -run 'TestTranscribe/binary_fails' -v   # single (sub)test
go test ./internal/output -run TestWriteTranscriptGolden -update      # regenerate golden file
go test ./internal/output -run '^$' -fuzz=FuzzWriteTranscript -fuzztime=30s
```

golangci-lint isn't installed locally; run it with `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run ./...`. CI (`.github/workflows/ci.yml`) runs vet, `go test -race`, the coverage gate and golangci-lint.

Runtime dependencies (not Go deps): `ffmpeg` with PulseAudio input (works on PipeWire via pipewire-pulse), `whisper-cli` from whisper.cpp (or `MEET2DO_WHISPER=/path/to/binary`), and at least one of `claude` / `codex` on PATH. The interactive UI needs a real TTY, so it can't be exercised from a non-interactive shell — test the packages directly instead.

## Architecture

`cmd/meet2do/main.go` is a linear orchestrator: a `session` struct carries state through small step methods (dependency checks → choose AI → record → choose outputs and folder → (transcribe) → (generate TODO) → write files). The root context comes from `signal.NotifyContext`. Each step lives in its own package under `internal/`, and all of them shell out to external binaries rather than using Go libraries:

- `recorder` — runs `ffmpeg -f pulse` with two inputs (`default` and `@DEFAULT_MONITOR@`) mixed via `amix` into a 16 kHz mono PCM WAV (the format whisper expects). ffmpeg runs in its own process group so terminal Ctrl+C doesn't kill it; `Stop()` writes `q` to its stdin so the WAV header is finalized, with SIGINT/kill fallbacks.
- `transcriber` — finds the whisper.cpp binary, downloads `ggml-<model>.bin` from Hugging Face into `$XDG_CACHE_HOME/meet2do/models/` on first use, runs it with `-otxt -of <base>` and reads `<base>.txt`.
- `ai` — `Provider` interface with `Claude` (`claude -p <prompt>`, transcript on stdin, reads stdout) and `Codex` (`codex exec --skip-git-repo-check --ephemeral --sandbox read-only -C <tmpdir> --output-last-message <file> -`, prompt+transcript on stdin, reads the file to avoid log noise). The shared instructions are in `ai/prompt.go`; `clean()` strips an enclosing ```markdown fence. Adding a provider = implement `Provider` and add it to `All()`.
- `output` — fixed file names (`TODO.md`, `transcripcion.md`, `reunion.wav`) and writers.
- `ui` — charmbracelet stack: `huh` forms for prompts, a `bubbletea` stopwatch screen for recording, and `Spin()` which runs a task in a goroutine behind a spinner (the task can update the label via the `Status` callback, used for download progress). `Spin(ctx, …)` passes a derived context to the task and cancels it on Ctrl+C so child processes die. `ui.ErrAborted` (= `huh.ErrUserAborted`) is the cancellation signal; `main` maps it (and `context.Canceled`) to exit code 130.

Transcription only runs if `TODO.md` or `transcripcion.md` was selected, and the AI only runs if `TODO.md` was selected. Everything intermediate goes in an `os.MkdirTemp` dir removed on exit.

Errors are wrapped with `%w` and context; packages expose sentinel errors (`recorder.ErrNotStarted`/`ErrEmptyRecording`, `transcriber.ErrNotInstalled`, `ai.ErrNoProvider`) for `errors.Is`. `transcriber.Whisper` has a useful zero value (empty `Model`/`Lang`/`BaseURL` → defaults), and `BaseURL` lets tests point the model download at `httptest`.

Tests follow the golang-testing skill: external `_test` packages (public API only), table-driven with `t.Run`, and external binaries replaced by shell scripts via `internal/testutil` (`FakeBin` + `IsolatedPath`, which uses `t.Setenv`, so those tests can't be `t.Parallel`). `ui` and `cmd/meet2do` are untested (need a TTY).

## Versioning rule (`/conventional-commit`)

Every time the user asks to run the `/conventional-commit` skill, bump the app version (`A.B.C`) **before** committing, and update it in every place it appears — currently the `version` constant in `cmd/meet2do/main.go` and the `Versión:` line in `README.md` — so all occurrences stay in sync and are included in the same commit.

- `A` (major) must **never** be increased, not even for breaking changes.
- `feat` (including breaking changes) → increase `B`, reset `C` to 0.
- Any other type (`fix`, `refactor`, `perf`, `docs`, `chore`, …) → increase `C`.
