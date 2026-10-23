# Meet2Do

Graba el audio de una reunión (Google Meet, Teams, Zoom…), lo transcribe localmente con
whisper.cpp y genera un `TODO.md` con features, bugs y correcciones usando Claude o Codex.

## Requisitos (Linux)

- Go 1.24+
- `ffmpeg` con soporte PulseAudio (funciona sobre PipeWire vía `pipewire-pulse`)
- [whisper.cpp](https://github.com/ggml-org/whisper.cpp) (`whisper-cli` en el PATH, o la ruta en `MEET2DO_WHISPER`)
- `claude` y/o `codex` CLI instalados y autenticados

## Uso

```sh
go build -o meet2do .
./meet2do                 # modelo whisper "small", idioma autodetectado
./meet2do -model medium -lang es
```

Flujo: eliges la IA → inicias la grabación (micrófono + audio del sistema) → Enter/q para
terminar → eliges qué archivos generar (`TODO.md`, `transcripcion.md`, `reunion.wav`) →
eliges la carpeta (por defecto la actual).

El modelo de whisper se descarga la primera vez en `~/.cache/meet2do/models/`.
