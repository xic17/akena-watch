#!/bin/sh
# Smoke test de Akena Watch: ejercita el flujo completo contra un
# servidor real en el puerto 8099. Requiere: binario compilado
# (make build) y curl. Uso: sh scripts/smoke.sh
set -e

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
ROOT=$(dirname "$SCRIPT_DIR")
BASE=/tmp/aw-smoke
rm -rf "$BASE"
mkdir -p "$BASE/data"

cd "$ROOT"
AKENA_DATA_DIR="$BASE/data" AKENA_PORT=8099 ./bin/akena-watch > "$BASE/server.log" 2>&1 &
SRV=$!
sleep 1
trap 'kill $SRV 2>/dev/null || true' EXIT

echo "== 1. raiz redirige a /setup =="
curl -s -o /dev/null -w "%{http_code} -> %{redirect_url}\n" http://127.0.0.1:8099/

echo "== 2. /setup responde 200 =="
curl -s -o /dev/null -w "%{http_code}\n" http://127.0.0.1:8099/setup

echo "== 3. crear admin (setup) =="
curl -s -c "$BASE/cookies.txt" -X POST http://127.0.0.1:8099/api/setup \
  -H "Content-Type: application/json" -d '{"username":"akena","password":"guardiana2026"}'
echo

echo "== 4. /setup bloqueado tras instalar =="
curl -s -X POST http://127.0.0.1:8099/api/setup \
  -H "Content-Type: application/json" -d '{"username":"x","password":"12345678"}'
echo

echo "== 5. login =="
curl -s -c "$BASE/cookies.txt" -X POST http://127.0.0.1:8099/api/login \
  -H "Content-Type: application/json" -d '{"username":"akena","password":"guardiana2026"}'
echo

echo "== 6. crear monitor http y tcp =="
curl -s -b "$BASE/cookies.txt" -X POST http://127.0.0.1:8099/api/monitors \
  -H "Content-Type: application/json" -d '{"name":"Web de prueba","type":"http","url":"http://example.com","interval_s":10,"timeout_s":5}' > /dev/null
curl -s -b "$BASE/cookies.txt" -X POST http://127.0.0.1:8099/api/monitors \
  -H "Content-Type: application/json" -d '{"name":"Puerto 443","type":"tcp","url":"example.com:443","interval_s":10,"timeout_s":5}' > /dev/null
echo ok

echo "== 7. crear colaboradora y compartir monitor 1 =="
curl -s -b "$BASE/cookies.txt" -X POST http://127.0.0.1:8099/api/users \
  -H "Content-Type: application/json" -d '{"username":"luna","password":"colaboradora2026","role":"collaborator"}' > /dev/null
curl -s -b "$BASE/cookies.txt" -X PUT http://127.0.0.1:8099/api/monitors/1/share/2 \
  -H "Content-Type: application/json" -d '{"can_edit":true}'
echo

echo "== 8. csrf: origin ajeno debe fallar =="
curl -s -b "$BASE/cookies.txt" -X POST http://127.0.0.1:8099/api/logout \
  -H "Origin: https://evil.example" -H "Content-Type: application/json" -d '{}'
echo

echo "== 9. heartbeats tras 12s =="
sleep 12
curl -s -b "$BASE/cookies.txt" "http://127.0.0.1:8099/api/monitors/1/heartbeats?hours=1"
echo

echo "== 10. codigos de paginas =="
curl -s -b "$BASE/cookies.txt" -o /dev/null -w "dashboard=%{http_code} " http://127.0.0.1:8099/dashboard
curl -s -o /dev/null -w "status=%{http_code} " http://127.0.0.1:8099/status/akena
curl -s -o /dev/null -w "about=%{http_code}\n" http://127.0.0.1:8099/about

echo "== FIN (ok si no hubo errores arriba) =="
