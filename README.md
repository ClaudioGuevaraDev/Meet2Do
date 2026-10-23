# Meet2Do

Versión: **0.4.1**

Graba el audio de una reunión (Google Meet, Teams, Zoom…), lo transcribe localmente con
whisper.cpp y genera una lista de tareas (`tareas.md`) con features, bugs y correcciones usando
Claude o Codex.

Es todo en uno: no necesitas instalar ffmpeg, whisper.cpp ni modelos. Lo que falta se
descarga solo la primera vez, con una barra de progreso que indica qué se está descargando.

## Requisitos

- Linux (x86-64 o ARM64) con PulseAudio o PipeWire (lo traen todos los escritorios)
- `claude` y/o `codex` CLI instalados y autenticados
- Go 1.24.2+ solo para compilar desde el código fuente

## Instalación

```sh
go install github.com/ClaudioGuevaraDev/Meet2Do/cmd/meet2do@latest
meet2do -setup            # opcional: deja todo descargado ahora (~476 MB con el modelo small)
```

Si te saltas `-setup`, la primera ejecución muestra la misma pantalla de instalación antes
de empezar a grabar. Se descargan, en `~/.cache/meet2do/`:

| Componente | Para qué | Tamaño |
|---|---|---|
| whisper.cpp `b5454` (build oficial, verificado con SHA-256) | motor de transcripción | ~10 MB |
| Modelo de voz (`small` por defecto) | reconocimiento de voz | ~465 MB |

## Uso

```sh
meet2do                   # modelo whisper "small", idioma autodetectado
meet2do -model medium -lang es
meet2do -setup            # descarga lo que falte y termina
meet2do -version
```

Flujo: eliges la IA → inicias la grabación (micrófono + audio del sistema) → Enter/q para
terminar → eliges qué archivos generar (`tareas.md`, `transcripcion.md`, `reunion.wav`) →
eliges la carpeta (por defecto la actual).

Si cambias de modelo con `-model`, el nuevo se descarga la próxima vez que lo uses.

## Desarrollo

```sh
make test         # tests
make test-race    # tests con detector de carreras
make cover-check  # cobertura (mínimo 80% en internal/, salvo ui)
make test-integration  # graba unos segundos con tu servidor de audio real
make lint         # go vet + golangci-lint
```
