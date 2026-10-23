# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

Meet2Do is a Linux-only Go CLI that records a meeting (mic + system audio), transcribes it locally with whisper.cpp, and asks the `claude` or `codex` CLI to turn the transcript into a `TODO.md`. User-facing text (prompts, messages, the AI prompt, output file names) is in Spanish — keep it that way.

## Commands

```sh
make build                        # -> bin/meet2do (entry point: ./cmd/meet2do)
go run ./cmd/meet2do [-model small] [-lang auto] [-setup] [-version]
make test | make test-race
make test-integration             # records ~3 s with the real PulseAudio/PipeWire server (not in CI)
make cover-check                  # fails if internal/{ai,output,recorder,transcriber} < 80% (recorder/pulse.go excluded)
make lint                         # go vet + golangci-lint (v2 config in .golangci.yml)
go test ./internal/transcriber -run 'TestTranscribe/binary_fails' -v   # single (sub)test
go test ./internal/output ./internal/recorder -update               # regenerate golden files
go test ./internal/output -run '^$' -fuzz=FuzzWriteTranscript -fuzztime=30s
```

golangci-lint isn't installed locally; run it with `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run ./...`. CI (`.github/workflows/ci.yml`) runs vet, `go test -race`, the coverage gate and golangci-lint.

The app must stay **all-in-one**: users only need `claude` or `codex` on PATH. Never add a "please install X" requirement — anything else is pure Go or downloaded on first run (checksum-pinned, into `$XDG_CACHE_HOME/meet2do/`) through the install screen. Set `MEET2DO_WHISPER=/path/to/whisper-cli` to use a local whisper.cpp during development. The interactive UI needs a real TTY; to drive it from a non-interactive shell use a pty (e.g. `script -qfec "bin/meet2do -setup -model tiny" log`) with an isolated `XDG_CACHE_HOME`.

## Architecture

`cmd/meet2do/main.go` is a linear orchestrator: a `session` struct carries state through small step methods (check `claude`/`codex` → `setup` (install screen for whatever is missing) → choose AI → record → choose outputs and folder → (transcribe) → (generate TODO) → write files). `-setup` runs only the install step. The root context comes from `signal.NotifyContext`. Each step lives in its own package under `internal/`:

- `recorder` — pure Go, no ffmpeg. `pulse.go` opens two `jfreymuth/pulse` record streams (default source = mic, monitor of default sink = system audio) at 16 kHz mono int16 (the server resamples). Each `Source` appends to its buffer; a goroutine mixes every 100 ms what both have (`Mix`, saturating) and streams it to disk via `wav.go` (header sizes patched on close). If one source lags more than 2 s (e.g. the monitor sends nothing while nothing plays) it is padded with silence, so a meeting is never held in RAM. `NewWithSources` injects fake sources for tests.
- `transcriber` — `bundle.go` downloads the official whisper.cpp Linux build (`BundleTag`, per-arch asset + pinned SHA-256 in `bundles`), verifies it, extracts only `whisper-cli` + `lib*.so*` (rejecting unsafe tar paths/symlinks) into `$XDG_CACHE_HOME/meet2do/whisper/<tag>/` via an atomic rename; `whisper-cli` has `RUNPATH=$ORIGIN`, so no system install is needed. To upgrade whisper.cpp, change `BundleTag` and the hashes (GitHub shows each asset's `sha256` digest). `download.go` is the shared `.part`+rename downloader also used for `ggml-<model>.bin` from Hugging Face. `Transcribe` runs `whisper-cli -otxt -of <base>` and reads `<base>.txt`.
- `ai` — `Provider` interface with `Claude` (`claude -p <prompt>`, transcript on stdin, reads stdout) and `Codex` (`codex exec --skip-git-repo-check --ephemeral --sandbox read-only -C <tmpdir> --output-last-message <file> -`, prompt+transcript on stdin, reads the file to avoid log noise). The shared instructions are in `ai/prompt.go`; `clean()` strips an enclosing ```markdown fence. Adding a provider = implement `Provider` and add it to `All()`.
- `output` — fixed file names (`TODO.md`, `transcripcion.md`, `reunion.wav`) and writers.
- `ui` — charmbracelet stack: `huh` forms for prompts, a `bubbletea` stopwatch screen for recording, `Install()` (install screen: one row per `Download` with a `bubbles/progress` bar, %, MB and speed, run sequentially; progress messages are throttled) and `Spin()` which runs a task in a goroutine behind a spinner. `Spin(ctx, …)` passes a derived context to the task and cancels it on Ctrl+C so child processes die. `ui.ErrAborted` (= `huh.ErrUserAborted`) is the cancellation signal; `main` maps it (and `context.Canceled`) to exit code 130.

Transcription only runs if `TODO.md` or `transcripcion.md` was selected, and the AI only runs if `TODO.md` was selected. Everything intermediate goes in an `os.MkdirTemp` dir removed on exit.

Errors are wrapped with `%w` and context; packages expose sentinel errors (`recorder.ErrNotStarted`/`ErrEmptyRecording`/`ErrNoAudioServer`, `transcriber.ErrChecksum`/`ErrUnsafeArchive`/`ErrUnsupportedPlatform`, `ai.ErrNoProvider`) for `errors.Is`. `transcriber.Whisper` has a useful zero value (empty `Model`/`Lang`/`BaseURL`/`BundleBaseURL` → defaults); the URLs let tests point downloads at `httptest`.

Tests follow the golang-testing skill: external `_test` packages (public API only), table-driven with `t.Run`, and external binaries replaced by shell scripts via `internal/testutil` (`FakeBin` + `IsolatedPath`, which uses `t.Setenv`, so those tests can't be `t.Parallel`). `transcriber/export_test.go` provides `SetTestBundle` to accept a test tarball's checksum (global state, not parallel). The install screen's `Update`/`View` are tested in `ui/install_test.go` without a TTY; the rest of `ui` and `cmd/meet2do` are untested.

## Versioning rule (`/conventional-commit`)

Every time the user asks to run the `/conventional-commit` skill, bump the app version (`A.B.C`) **before** committing, and update it in every place it appears — currently the `version` constant in `cmd/meet2do/main.go` and the `Versión:` line in `README.md` — so all occurrences stay in sync and are included in the same commit.

- `A` (major) must **never** be increased, not even for breaking changes.
- `feat` (including breaking changes) → increase `B`, reset `C` to 0.
- Any other type (`fix`, `refactor`, `perf`, `docs`, `chore`, …) → increase `C`.
