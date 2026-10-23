# Meet2Do

Versión: **0.10.0**

Graba el audio de una reunión (Google Meet, Teams, Zoom…), lo transcribe localmente con
whisper.cpp y genera con Claude o Codex una lista de tareas (`tareas.md`) con features, bugs y
correcciones, y un resumen de la reunión (`resumen.md`).

Es todo en uno: no necesitas instalar ffmpeg, whisper.cpp ni modelos. Lo que falta se
descarga solo la primera vez, con una barra de progreso que indica qué se está descargando.

## Requisitos

- Linux (x86-64 o ARM64) con PulseAudio o PipeWire (lo traen todos los escritorios)
- `claude` y/o `codex` CLI instalados y autenticados

## Instalación

```sh
curl -fsSL https://raw.githubusercontent.com/ClaudioGuevaraDev/Meet2Do/main/install.sh | sh
```

El instalador descarga el binario de la última versión desde
[GitHub Releases](https://github.com/ClaudioGuevaraDev/Meet2Do/releases), verifica su SHA-256 y
lo instala en `~/.local/bin` (sin sudo) junto con el atajo `m2do`. Si esa carpeta no está en tu
`PATH`, te muestra la línea para agregarla. Para **actualizar**, vuelve a ejecutarlo.

Opciones (variables de entorno, antes de `sh`):

```sh
curl -fsSL https://raw.githubusercontent.com/ClaudioGuevaraDev/Meet2Do/main/install.sh | MEET2DO_VERSION=v0.10.0 sh
curl -fsSL https://raw.githubusercontent.com/ClaudioGuevaraDev/Meet2Do/main/install.sh | MEET2DO_INSTALL_DIR=/opt/bin sh
```

### Alternativa: con Go

Si tienes Go 1.24.2+:

```sh
go install github.com/ClaudioGuevaraDev/Meet2Do/cmd/...@latest   # instala meet2do y m2do en ~/go/bin
```

### Primera ejecución

```sh
m2do -setup               # opcional: deja todo descargado ahora (~476 MB con el modelo small)
```

Si te saltas `-setup`, la primera ejecución muestra la misma pantalla de instalación antes
de empezar a grabar. Se descargan, en `~/.cache/meet2do/`:

| Componente | Para qué | Tamaño |
|---|---|---|
| whisper.cpp `b5454` (build oficial, verificado con SHA-256) | motor de transcripción | ~10 MB |
| Modelo de voz (`small` por defecto) | reconocimiento de voz | ~465 MB |

## Uso

`m2do` es un atajo de `meet2do`: es el mismo programa y acepta lo mismo (`m2do -setup`,
`m2do uninstall`…).

```sh
meet2do                   # modelo whisper "small", idioma autodetectado (o simplemente: m2do)
meet2do -model medium -lang es
meet2do -setup            # descarga lo que falte y termina
meet2do uninstall         # desinstala Meet2Do y lo que descargó
meet2do -version
```

Flujo: eliges la IA, su modelo y su esfuerzo → inicias la grabación (micrófono + audio del
sistema), que puedes pausar y continuar → eliges qué archivos generar (`tareas.md` y
`resumen.md` vienen marcados; también `transcripcion.md` y `reunion.wav`) → eliges dónde
guardarlos (por defecto la carpeta actual). Cada reunión se guarda en su propia carpeta
`reunion-AAAA-MM-DD_HH-MM/`, así que nunca se sobrescribe nada.

Si cambias de modelo con `-model`, el nuevo se descarga la próxima vez que lo uses.

## Desinstalación

```sh
meet2do uninstall
```

Muestra lo que Meet2Do instaló, todo marcado, para que elijas qué borrar: los ejecutables
`meet2do` y `m2do`, whisper.cpp y los modelos de voz (en `~/.cache/meet2do/`). Pide confirmación antes de
borrar. Tus reuniones guardadas no se tocan. Meet2Do no instala ffmpeg ni nada fuera de esas
rutas.

## Desarrollo

```sh
make test         # tests
make test-race    # tests con detector de carreras
make cover-check  # cobertura (mínimo 80% en internal/, salvo ui)
make test-integration  # graba unos segundos con tu servidor de audio real
make lint         # go vet + golangci-lint
```
