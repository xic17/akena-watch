#!/bin/sh
# Akena Watch — arranque en modo desarrollo con datos de prueba.
#
# Compila el binario, reinicia la instancia anterior y arranca en primer plano
# escuchando solo en loopback, con el directorio de datos de prueba (nunca los
# datos reales de producción).
#
# Uso:
#   sh scripts/dev.sh
#   AKENA_TEST_DATA_DIR=/ruta/alterna sh scripts/dev.sh
#   PORT=9000 sh scripts/dev.sh
#
# El directorio de datos se resuelve en este orden:
#   1. $AKENA_TEST_DATA_DIR
#   2. TEST_DATA_DIR de Makefile.local (config personal, gitignoreada)
#   3. ./.test-data
set -e

cd "$(dirname "$0")/.."

# --- directorio de datos de prueba ---
DATA_DIR="${AKENA_TEST_DATA_DIR:-}"
if [ -z "$DATA_DIR" ] && [ -f Makefile.local ]; then
  DATA_DIR=$(sed -n 's/^TEST_DATA_DIR[[:space:]]*:=[[:space:]]*//p' Makefile.local | head -1)
fi
if [ -z "$DATA_DIR" ]; then
  DATA_DIR="./.test-data"
fi

PORT="${PORT:-${AKENA_PORT:-8080}}"

echo "==> Compilando"
make build

echo "==> Deteniendo la instancia anterior (si la hay)"
pkill -x akena-watch 2>/dev/null || true
sleep 1

mkdir -p "$DATA_DIR"

# Si la base de datos es nueva, avisamos del asistente de primer arranque.
if [ ! -f "$DATA_DIR/akena.db" ]; then
  echo
  echo "  (directorio nuevo: el primer arranque te pedirá crear el administrador en /setup)"
fi

echo
echo "  Akena Watch — modo desarrollo"
echo "  ----------------------------------------"
echo "  URL:   http://localhost:$PORT"
echo "  Datos: $DATA_DIR"
echo "  Salir: Ctrl+C"
echo

exec env AKENA_DATA_DIR="$DATA_DIR" AKENA_PORT="$PORT" AKENA_BIND=127.0.0.1 ./bin/akena-watch
