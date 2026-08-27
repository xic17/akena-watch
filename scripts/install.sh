#!/bin/sh
# Instalador de Akena Watch — descarga el binario de la última release
# publicada en GitHub y lo instala en /usr/local/bin.
#
# Uso:
#   AKENA_REPO=usuario/akena-watch sh scripts/install.sh
#   AKENA_REPO=usuario/akena-watch VERSION=v1.0.0 sh scripts/install.sh
#   sh scripts/install.sh          # detecta el repo desde el git remote
#
# Variables opcionales:
#   AKENA_REPO        repo GitHub "usuario/repo" (o se detecta de git remote)
#   VERSION           versión a instalar (default: latest)
#   AKENA_INSTALL_DIR directorio destino (default: /usr/local/bin)
set -e

# --- detectar repositorio ---
REPO="${AKENA_REPO:-}"
if [ -z "$REPO" ]; then
  REMOTE=$(git config --get remote.origin.url 2>/dev/null || true)
  case "$REMOTE" in
    *github.com*)
      REPO=$(echo "$REMOTE" | sed -E 's#.*github.com[:/]([^/]+/[^/.]+)(\.git)?$#\1#')
      ;;
  esac
fi
if [ -z "$REPO" ]; then
  echo "No se pudo detectar el repositorio." >&2
  echo "Pásalo explícitamente: AKENA_REPO=usuario/akena-watch sh scripts/install.sh" >&2
  exit 1
fi

# --- detectar plataforma ---
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64 | amd64) ARCH="amd64" ;;
  aarch64 | arm64) ARCH="arm64" ;;
  *)
    echo "Arquitectura no soportada: $ARCH (solo amd64 y arm64)" >&2
    exit 1
    ;;
esac
case "$OS" in
  linux | darwin) ;;
  *)
    echo "Sistema operativo no soportado: $OS (solo linux y darwin)" >&2
    exit 1
    ;;
esac

VERSION="${VERSION:-latest}"
BASE="https://github.com/${REPO}/releases/${VERSION}"
BIN_NAME="akena-watch-${OS}-${ARCH}"
DEST="${AKENA_INSTALL_DIR:-/usr/local/bin}"
DEST_BIN="${DEST}/akena-watch"

echo "==> Descargando ${BIN_NAME} (${VERSION}) desde ${REPO}"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

curl -sSL -o "$TMP/$BIN_NAME" "$BASE/download/$BIN_NAME"
chmod +x "$TMP/$BIN_NAME"

# verificación de checksum (si la release lo incluye)
if curl -sSL -o "$TMP/SHA256SUMS" "$BASE/download/SHA256SUMS"; then
  (cd "$TMP" && grep "$BIN_NAME" SHA256SUMS | sha256sum -c -) || {
    echo "Checksum no válido. Abortando." >&2
    exit 1
  }
fi

mkdir -p "$DEST"
cp "$TMP/$BIN_NAME" "$DEST_BIN"
chmod +x "$DEST_BIN"

# ICMP sin fricción: si tenemos root (típico al instalar en /usr/local/bin),
# concedemos CAP_NET_RAW al binario para que el ping ICMP funcione sin que
# el usuario tenga que configurar nada. Se reaplica en cada actualización.
if [ "$(id -u)" = "0" ] && command -v setcap >/dev/null 2>&1; then
  if setcap cap_net_raw+ep "$DEST_BIN" 2>/dev/null; then
    echo "✔ Ping ICMP habilitado (cap_net_raw concedido al binario)"
  fi
fi

echo
echo "✔ Akena Watch ${VERSION} instalado en ${DEST_BIN}"
echo
echo "Siguiente paso (Linux):"
echo "  1. Crea el usuario de sistema y el directorio de datos:"
echo "     sudo useradd --system --home /var/lib/akena-watch --shell /usr/sbin/nologin akena"
echo "     sudo mkdir -p /var/lib/akena-watch/data"
echo "     sudo chown -R akena:akena /var/lib/akena-watch"
echo "  2. Arranca en primer plano:"
echo "     sudo -u akena AKENA_DATA_DIR=/var/lib/akena-watch/data AKENA_BIND=127.0.0.1 ${DEST_BIN}"
echo "     (o instala el servicio systemd: cp deploy/akena-watch.service /etc/systemd/system/)"
echo "  3. Abre http://localhost:8080 y crea el administrador."
