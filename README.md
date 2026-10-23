# Meet2Do

Versión: **0.2.3**

Graba el audio de una reunión (Google Meet, Teams, Zoom…), lo transcribe localmente con
whisper.cpp y genera un `TODO.md` con features, bugs y correcciones usando Claude o Codex.

## Requisitos (Linux)

- Go 1.24.2+
- `ffmpeg` con soporte PulseAudio (funciona sobre PipeWire vía `pipewire-pulse`)
- [whisper.cpp](https://github.com/ggml-org/whisper.cpp) (`whisper-cli` en el PATH, o la ruta en `MEET2DO_WHISPER`)
- `claude` y/o `codex` CLI instalados y autenticados

## Instalación

```sh
go install github.com/ClaudioGuevaraDev/Meet2Do/cmd/meet2do@latest
# o desde el repo:
make build                # genera bin/meet2do
```

## Uso

```sh
meet2do                   # modelo whisper "small", idioma autodetectado
meet2do -model medium -lang es
meet2do -version
```

Flujo: eliges la IA → inicias la grabación (micrófono + audio del sistema) → Enter/q para
terminar → eliges qué archivos generar (`TODO.md`, `transcripcion.md`, `reunion.wav`) →
eliges la carpeta (por defecto la actual).

El modelo de whisper se descarga la primera vez en `~/.cache/meet2do/models/`.

## Desarrollo

```sh
make test         # tests
make test-race    # tests con detector de carreras
make cover-check  # cobertura (mínimo 80% en internal/, salvo ui)
make lint         # go vet + golangci-lint
```
