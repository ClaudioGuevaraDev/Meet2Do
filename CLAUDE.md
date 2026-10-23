# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

Meet2Do is a Linux-only Go CLI that records a meeting (mic + system audio), transcribes it locally with whisper.cpp, and asks the `claude` or `codex` CLI to turn the transcript into a `TODO.md`. User-facing text (prompts, messages, the AI prompt, output file names) is in Spanish — keep it that way.

## Commands

```sh
go build -o meet2do .            # build
go run . [-model small] [-lang auto]
go vet ./...
go test ./...
go test ./internal/transcriber -run TestTranscribeWithFakeBinary -v   # single test
```

Runtime dependencies (not Go deps): `ffmpeg` with PulseAudio input (works on PipeWire via pipewire-pulse), `whisper-cli` from whisper.cpp (or `MEET2DO_WHISPER=/path/to/binary`), and at least one of `claude` / `codex` on PATH. The interactive UI needs a real TTY, so it can't be exercised from a non-interactive shell — test the packages directly instead.

## Architecture

`main.go` is a linear orchestrator: dependency checks → choose AI → record → choose outputs and folder → (transcribe) → (generate TODO) → write files. Each step lives in its own package under `internal/`, and all of them shell out to external binaries rather than using Go libraries:

- `recorder` — runs `ffmpeg -f pulse` with two inputs (`default` and `@DEFAULT_MONITOR@`) mixed via `amix` into a 16 kHz mono PCM WAV (the format whisper expects). ffmpeg runs in its own process group so terminal Ctrl+C doesn't kill it; `Stop()` writes `q` to its stdin so the WAV header is finalized, with SIGINT/kill fallbacks.
- `transcriber` — finds the whisper.cpp binary, downloads `ggml-<model>.bin` from Hugging Face into `$XDG_CACHE_HOME/meet2do/models/` on first use, runs it with `-otxt -of <base>` and reads `<base>.txt`.
- `ai` — `Provider` interface with `Claude` (`claude -p <prompt>`, transcript on stdin, reads stdout) and `Codex` (`codex exec --skip-git-repo-check --ephemeral --sandbox read-only -C <tmpdir> --output-last-message <file> -`, prompt+transcript on stdin, reads the file to avoid log noise). The shared instructions are in `ai/prompt.go`; `clean()` strips an enclosing ```markdown fence. Adding a provider = implement `Provider` and add it to `All()`.
- `output` — fixed file names (`TODO.md`, `transcripcion.md`, `reunion.wav`) and writers.
- `ui` — charmbracelet stack: `huh` forms for prompts, a `bubbletea` stopwatch screen for recording, and `Spin()` which runs a task in a goroutine behind a spinner (the task can update the label via the `Status` callback, used for download progress). `ui.ErrAborted` (= `huh.ErrUserAborted`) is the cancellation signal; `main` maps it to exit code 130.

Transcription only runs if `TODO.md` or `transcripcion.md` was selected, and the AI only runs if `TODO.md` was selected. Everything intermediate goes in an `os.MkdirTemp` dir removed on exit.

Tests avoid real binaries where possible: the transcriber test uses a fake shell-script `whisper-cli`.

## Versioning rule (`/conventional-commit`)

Every time the user asks to run the `/conventional-commit` skill, bump the app version (`A.B.C`) **before** committing, and update it in every place it appears — currently the `version` constant in `main.go` and the `Versión:` line in `README.md` — so all occurrences stay in sync and are included in the same commit.

- `A` (major) must **never** be increased, not even for breaking changes.
- `feat` (including breaking changes) → increase `B`, reset `C` to 0.
- Any other type (`fix`, `refactor`, `perf`, `docs`, `chore`, …) → increase `C`.
