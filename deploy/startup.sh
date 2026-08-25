#!/bin/sh
# Punto de entrada del contenedor.
#
# - Con credenciales R2 (Cloudflare Containers): monta el bucket en /data
#   vía FUSE para que la base de datos sobreviva a reinicios e instancias.
# - Sin credenciales R2: /data es un directorio local efímero.
set -e

if [ -n "${R2_ACCOUNT_ID}" ] && [ -n "${R2_BUCKET_NAME}" ]; then
  echo "[akena-watch] Montando bucket R2 ${R2_BUCKET_NAME} en /data ..."
  R2_ENDPOINT="https://${R2_ACCOUNT_ID}.r2.cloudflarestorage.com"
  mkdir -p /data
  /usr/local/bin/tigrisfs --endpoint "${R2_ENDPOINT}" "${R2_BUCKET_NAME}" /data &
  sleep 3
  echo "[akena-watch] Bucket montado."
fi

exec /usr/local/bin/akena-watch
