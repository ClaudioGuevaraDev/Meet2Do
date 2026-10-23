<div align="center">

# ◉ Meet2Do

*Graba tu reunión y conviértela en una lista de tareas*

[![Release](https://img.shields.io/github/v/release/ClaudioGuevaraDev/Meet2Do?style=flat-square)](https://github.com/ClaudioGuevaraDev/Meet2Do/releases/latest)
[![CI](https://img.shields.io/github/actions/workflow/status/ClaudioGuevaraDev/Meet2Do/ci.yml?branch=main&style=flat-square&label=CI)](https://github.com/ClaudioGuevaraDev/Meet2Do/actions/workflows/ci.yml)
[![Go](https://img.shields.io/github/go-mod/go-version/ClaudioGuevaraDev/Meet2Do?style=flat-square)](go.mod)
![Linux](https://img.shields.io/badge/Linux-x86--64%20%7C%20ARM64-3B82F6?style=flat-square&logo=linux&logoColor=white)

Versión: **0.11.3**

[Características](#características) • [Instalación](#instalación) • [Uso](#uso) • [Desinstalación](#desinstalación) • [Desarrollo](#desarrollo)

</div>

Meet2Do graba una reunión (Google Meet, Teams, Zoom…) desde tu terminal: tu micrófono y el audio
del sistema. La transcribe localmente con [whisper.cpp](https://github.com/ggml-org/whisper.cpp) y
le pide a **Claude** o **Codex** una lista de tareas accionables y un resumen. Todo queda en Markdown,
en una carpeta por reunión.

Es todo en uno: solo necesitas `claude` o `codex`. Lo demás se descarga solo la primera vez.

## Características

- **Graba micrófono y audio del sistema** a la vez, con botones para pausar, continuar y terminar.
- **Transcripción local**: el audio no sale de tu equipo; whisper.cpp corre en tu CPU.
- **Tareas y resumen con IA**: features, bugs, correcciones y pendientes en `tareas.md`; contexto,
  decisiones y próximos pasos en `resumen.md`. Eliges la IA, el modelo y el nivel de esfuerzo.
- **Una carpeta por reunión**, con fecha y hora: nunca se sobrescribe nada.
- **Sin dependencias manuales**: whisper.cpp y el modelo de voz se descargan con barra de progreso
  y se verifican con SHA-256.
- **Se actualiza solo** cuando hay una versión nueva.
- **Atajo `m2do`** para escribir menos.

## Requisitos

- Linux x86-64 o ARM64 con PulseAudio o PipeWire (los traen todos los escritorios).
- El CLI de [`claude`](https://docs.anthropic.com/en/docs/claude-code) o de
  [`codex`](https://github.com/openai/codex), instalado y con la sesión iniciada.

## Instalación

```sh
curl -fsSL https://raw.githubusercontent.com/ClaudioGuevaraDev/Meet2Do/main/install.sh | sh
```

El instalador descarga la última versión desde
[GitHub Releases](https://github.com/ClaudioGuevaraDev/Meet2Do/releases), verifica su SHA-256 e
instala `meet2do` y su atajo `m2do` en `~/.local/bin`, sin `sudo`.

> [!TIP]
> Si `~/.local/bin` no está en tu `PATH`, el instalador te muestra la línea exacta para agregarlo.

Puedes cambiar la versión o la carpeta con variables de entorno:

| Variable | Para qué | Por defecto |
|---|---|---|
| `MEET2DO_VERSION` | Instalar una versión concreta, p. ej. `v0.11.2` | la última |
| `MEET2DO_INSTALL_DIR` | Carpeta de destino | `~/.local/bin` |

```sh
curl -fsSL https://raw.githubusercontent.com/ClaudioGuevaraDev/Meet2Do/main/install.sh | MEET2DO_INSTALL_DIR=/opt/bin sh
```

### Con Go

Si tienes Go 1.24.2 o superior:

```sh
go install github.com/ClaudioGuevaraDev/Meet2Do/cmd/...@latest   # instala meet2do y m2do en ~/go/bin
```

> [!NOTE]
> Meet2Do revisa al iniciar si hay una versión nueva. Si la hay, se descarga, se verifica, se reemplaza y
> se reinicia solo. Sin conexión arranca igual. Para desactivarlo, usa `MEET2DO_NO_UPDATE=1`.

## Uso

```sh
m2do
```

1. Eliges la IA (Claude o Codex), su modelo y su nivel de esfuerzo. *Por defecto* usa lo que tengas
   configurado en ese CLI.
2. Grabas la reunión. Te mueves con `←`/`→` entre **Pausar** y **Terminar**, o usas los atajos `p` y `t`.
3. Eliges qué archivos generar y dónde guardarlos.
4. Meet2Do transcribe, genera los documentos y te dice dónde quedaron.

`m2do` y `meet2do` son el mismo programa y aceptan las mismas opciones:

| Comando | Qué hace |
|---|---|
| `m2do -model medium` | Usa otro modelo de voz (ver [Modelos de voz](#modelos-de-voz)) |
| `m2do -lang es` | Fija el idioma de la reunión (`auto` por defecto) |
| `m2do -setup` | Descarga lo que falte y termina |
| `m2do uninstall` | Desinstala Meet2Do (ver [Desinstalación](#desinstalación)) |
| `m2do -version` | Muestra la versión |
| `m2do -h` | Muestra la ayuda |

### Archivos generados

Cada reunión se guarda en su propia carpeta dentro de la que elijas (por defecto, la actual):

```text
reunion-2026-10-23_13-07/
├── tareas.md          # tareas accionables, agrupadas por tipo
├── resumen.md         # contexto, puntos principales, decisiones y próximos pasos
├── transcripcion.md   # el texto de la reunión
└── reunion.wav        # el audio grabado
```

`tareas.md` y `resumen.md` vienen marcados por defecto; `transcripcion.md` y `reunion.wav` los
marcas si los necesitas.

### Modelos de voz

| Modelo | Tamaño | |
|---|---|---|
| `tiny` | ~74 MB | el más rápido, menos preciso |
| `base` | ~141 MB | |
| `small` | ~465 MB | **por defecto**, buen equilibrio |
| `medium` | ~1.4 GB | más preciso, más lento |
| `large-v3` | ~2.9 GB | el más preciso |

> [!NOTE]
> La primera vez se descargan whisper.cpp (~10 MB) y el modelo elegido en `~/.cache/meet2do/`. Para
> adelantarlo, ejecuta `m2do -setup`. Si cambias de modelo con `-model`, el nuevo se descarga la
> próxima vez que lo uses.

## Desinstalación

```sh
m2do uninstall
```

Te muestra lo que Meet2Do instaló, todo marcado: los ejecutables `meet2do` y `m2do`, whisper.cpp
y los modelos de voz, cada uno con su tamaño. Eliges qué borrar y confirmas.

> [!NOTE]
> Tus reuniones guardadas no se tocan. Meet2Do no instala ffmpeg ni nada fuera de esas rutas.

## Desarrollo

```sh
make build             # bin/meet2do y bin/m2do
make test              # tests
make test-race         # tests con detector de carreras
make cover-check       # cobertura mínima de 80 % en internal/ (salvo ui)
make test-integration  # graba unos segundos con tu servidor de audio real
make lint              # go vet + golangci-lint
make dist              # binarios estáticos para el release (dist/)
```

Para usar un whisper.cpp propio mientras desarrollas, define `MEET2DO_WHISPER=/ruta/a/whisper-cli`.

Los releases son automáticos: al subir la versión (`internal/cli/cli.go`) y hacer push a `main`,
GitHub Actions corre los tests, compila los binarios y publica el release que descargan el
instalador y la actualización automática.
