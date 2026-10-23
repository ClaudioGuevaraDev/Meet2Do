#!/bin/sh
# Instalador de Meet2Do: descarga el binario de GitHub Releases, verifica su SHA-256 y lo
# instala junto con el atajo m2do. Uso:
#
#   curl -fsSL https://raw.githubusercontent.com/ClaudioGuevaraDev/Meet2Do/main/install.sh | sh
#
# Variables opcionales:
#   MEET2DO_VERSION       versión a instalar, p. ej. v0.10.0 (por defecto, la última)
#   MEET2DO_INSTALL_DIR   carpeta de destino (por defecto ~/.local/bin, no requiere sudo)
#   MEET2DO_DOWNLOAD_URL  origen de los archivos (para pruebas; por defecto GitHub Releases)
#
# Volver a ejecutarlo actualiza Meet2Do a la última versión.
set -eu

REPO="ClaudioGuevaraDev/Meet2Do"
INSTALL_DIR="${MEET2DO_INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${MEET2DO_VERSION:-latest}"

if [ -t 1 ]; then
	GREEN='\033[32m' RED='\033[31m' YELLOW='\033[33m' BLUE='\033[34m' DIM='\033[2m' BOLD='\033[1m' RESET='\033[0m'
else
	GREEN='' RED='' YELLOW='' BLUE='' DIM='' BOLD='' RESET=''
fi

info() { printf '%b›%b %s\n' "$BLUE" "$RESET" "$1"; }
ok() { printf '%b✓%b %s\n' "$GREEN" "$RESET" "$1"; }
warn() { printf '%b!%b %s\n' "$YELLOW" "$RESET" "$1"; }
fail() {
	printf '%b✗%b %s\n' "$RED" "$RESET" "$1" >&2
	exit 1
}

# Plataforma: solo Linux x86-64 y ARM64.
[ "$(uname -s)" = "Linux" ] || fail "Meet2Do solo funciona en Linux (este equipo es $(uname -s))."
case "$(uname -m)" in
x86_64 | amd64) ARCH=amd64 ;;
aarch64 | arm64) ARCH=arm64 ;;
*) fail "Arquitectura no soportada: $(uname -m) (se necesita x86-64 o ARM64)." ;;
esac

# Herramientas: vienen con cualquier Linux; no se instala nada más.
if command -v curl >/dev/null 2>&1; then
	download() { curl -fsSL "$1" -o "$2"; }
elif command -v wget >/dev/null 2>&1; then
	download() { wget -qO "$2" "$1"; }
else
	fail "Se necesita curl o wget para descargar Meet2Do."
fi
for tool in tar sha256sum; do
	command -v "$tool" >/dev/null 2>&1 || fail "Falta la herramienta $tool."
done

if [ -n "${MEET2DO_DOWNLOAD_URL:-}" ]; then
	BASE_URL="$MEET2DO_DOWNLOAD_URL"
elif [ "$VERSION" = "latest" ]; then
	BASE_URL="https://github.com/$REPO/releases/latest/download"
else
	BASE_URL="https://github.com/$REPO/releases/download/$VERSION"
fi
ASSET="meet2do-linux-$ARCH.tar.gz"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT INT TERM

printf '\n%b◉ Meet2Do%b %binstalador%b\n\n' "$BOLD$BLUE" "$RESET" "$DIM" "$RESET"
info "Descargando $ASSET ($VERSION)…"
download "$BASE_URL/$ASSET" "$TMP/$ASSET" || fail "No se pudo descargar $BASE_URL/$ASSET"
download "$BASE_URL/checksums.txt" "$TMP/checksums.txt" || fail "No se pudo descargar checksums.txt"

# Verificación de integridad: el tar.gz debe coincidir con el SHA-256 publicado.
expected="$(awk -v f="$ASSET" '$2 == f { print $1 }' "$TMP/checksums.txt")"
[ -n "$expected" ] || fail "checksums.txt no incluye $ASSET."
actual="$(sha256sum "$TMP/$ASSET" | awk '{ print $1 }')"
[ "$expected" = "$actual" ] || fail "El SHA-256 de $ASSET no coincide: la descarga está dañada o fue alterada."
ok "Integridad verificada (SHA-256)"

tar -xzf "$TMP/$ASSET" -C "$TMP" meet2do || fail "No se pudo extraer $ASSET."
mkdir -p "$INSTALL_DIR" || fail "No se pudo crear $INSTALL_DIR."
# install deja el archivo listo de una vez: no queda un binario a medio copiar si se corta.
install -m 755 "$TMP/meet2do" "$INSTALL_DIR/meet2do" || fail "No se pudo copiar meet2do a $INSTALL_DIR."
ln -sf meet2do "$INSTALL_DIR/m2do" || fail "No se pudo crear el atajo m2do en $INSTALL_DIR."

installed="$("$INSTALL_DIR/meet2do" -version 2>/dev/null | awk '{ print $2 }')"
ok "Meet2Do v${installed:-?} instalado en $INSTALL_DIR (meet2do y su atajo m2do)"

# Ayudas finales: PATH y el CLI de IA que Meet2Do necesita.
case ":$PATH:" in
*":$INSTALL_DIR:"*) ;;
*)
	warn "$INSTALL_DIR no está en tu PATH. Agrégalo con:"
	shell_rc="$HOME/.bashrc"
	case "${SHELL:-}" in */zsh) shell_rc="$HOME/.zshrc" ;; esac
	printf '\n    echo '\''export PATH="%s:$PATH"'\'' >> %s && . %s\n\n' "$INSTALL_DIR" "$shell_rc" "$shell_rc"
	;;
esac
if ! command -v claude >/dev/null 2>&1 && ! command -v codex >/dev/null 2>&1; then
	warn "Meet2Do necesita el CLI de claude o de codex instalado y autenticado."
fi
printf '%bSiguiente paso:%b ejecuta %bm2do%b para grabar tu primera reunión.\n' "$DIM" "$RESET" "$BOLD" "$RESET"
