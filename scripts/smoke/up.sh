#!/usr/bin/env bash
# scripts/smoke/up.sh — собрать бинари локально и поднять smoke-стек.
#
# Резолв секретов (НЕ логируем, НЕ возвращаем наружу):
#   STIT_OBS_TOKEN: env > ./credentials.md > ./credential.md
#   OTEL_TENANT:    env > ./credentials.md > ./credential.md
#
# Шаги:
#   1. Build api/worker/migrate бинарей локально в scripts/smoke/bin/
#      (использует прогретый GOMODCACHE; не нужен ssh/.netrc в docker).
#   2. docker compose up -d --build с подстановкой токена через окружение.
#   3. Дождаться healthy / migrate completed.
#
# Канонический status-line: `OK` / `FAIL: <reason>`.

set -uo pipefail

cd "$(git rev-parse --show-toplevel 2>/dev/null || pwd)"

SMOKE_DIR="scripts/smoke"
BIN_DIR="$SMOKE_DIR/bin"
COMPOSE="$SMOKE_DIR/compose.yml"

# === Token resolution ===
resolve_kv() {
    local key="$1"
    local env_val="${2:-}"
    if [ -n "$env_val" ]; then
        printf '%s' "$env_val"
        return 0
    fi
    for f in credentials.md credential.md; do
        if [ -f "$f" ]; then
            local v
            v=$(grep -m1 -oE "${key}=[^[:space:]\`\"']+" "$f" 2>/dev/null | head -1 | cut -d= -f2-)
            if [ -n "$v" ]; then
                printf '%s' "$v"
                return 0
            fi
        fi
    done
    return 1
}

TOKEN=$(resolve_kv STIT_OBS_TOKEN "${STIT_OBS_TOKEN:-}") || true
TENANT=$(resolve_kv OTEL_TENANT "${OTEL_TENANT:-}") || true

if [ -z "${TOKEN:-}" ]; then
    echo "FAIL: no STIT_OBS_TOKEN. Add line 'STIT_OBS_TOKEN=<token>' to ./credentials.md (or set env)." >&2
    exit 1
fi
if [ -z "${TENANT:-}" ]; then
    echo "FAIL: no OTEL_TENANT. Add line 'OTEL_TENANT=<tenant>' to ./credentials.md (or set env)." >&2
    exit 1
fi

export STIT_OBS_TOKEN="$TOKEN"
export OTEL_TENANT="$TENANT"

# === Build binaries locally ===
mkdir -p "$BIN_DIR"
echo "== Build (CGO_ENABLED=0) =="
for target in api migrate; do
    out="$BIN_DIR/$target"
    echo "  go build -> $out"
    if ! CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o "$out" "./cmd/$target" 2>&1; then
        echo "FAIL: go build $target" >&2
        exit 1
    fi
done

# === Compose up (build context = repo root) ===
echo
echo "== Compose up =="
if ! docker compose -f "$COMPOSE" up -d --build 2>&1; then
    echo "FAIL: docker compose up" >&2
    exit 1
fi

# === Wait for api healthy via openapi ===
echo
echo "== Wait for api (max 60s) =="
for i in $(seq 1 30); do
    if curl -fsS -o /dev/null --max-time 2 http://127.0.0.1:8080/openapi.yaml 2>/dev/null \
       || curl -fsS -o /dev/null --max-time 2 http://127.0.0.1:8080/healthz 2>/dev/null; then
        echo "  api up after ${i}s × 2s = $((i*2))s"
        echo
        docker compose -f "$COMPOSE" ps
        echo
        echo "OK"
        exit 0
    fi
    sleep 2
done

echo "FAIL: api did not become healthy in 60s. Logs:" >&2
docker compose -f "$COMPOSE" logs --tail=80 api migrate >&2
exit 1
