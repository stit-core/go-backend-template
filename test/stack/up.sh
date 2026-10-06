#!/usr/bin/env bash
# test/stack/up.sh — собрать бинари локально и поднять стек.
#
# Шаги:
#   1. Build бинарей cmd/* локально в test/stack/bin/
#      (использует прогретый GOMODCACHE; не нужен ssh/.netrc в docker).
#   2. docker compose up -d --build.
#   3. Дождаться healthy / migrate completed.
#
# Канонический status-line: `OK` / `FAIL: <reason>`.

set -uo pipefail

source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

# === Value resolution (env > ./.env.local) ===
resolve_kv() {
    local key="$1"
    local env_val="${2:-}"
    if [ -n "$env_val" ]; then
        printf '%s' "$env_val"
        return 0
    fi
    local file=".env.local"
    [ -f "$file" ] || return 1
    local line v
    line=$(grep -E "^[[:space:]]*(export[[:space:]]+)?${key}=" "$file" 2>/dev/null | tail -1)
    [ -n "$line" ] || return 1
    v="${line#*=}"
    v="${v%\"}"; v="${v#\"}"
    v="${v%\'}"; v="${v#\'}"
    printf '%s' "$v"
}

# Креды seed-admin (one-off bootstrap первого admin). Не прод-секрет —
# admin эфемерного e2e-стека; дефолты если .env.local не задаёт.
export E2E_ADMIN_EMAIL="$(resolve_kv E2E_ADMIN_EMAIL "${E2E_ADMIN_EMAIL:-}" || true)"
export E2E_ADMIN_PASSWORD="$(resolve_kv E2E_ADMIN_PASSWORD "${E2E_ADMIN_PASSWORD:-}" || true)"
: "${E2E_ADMIN_EMAIL:=e2e-admin@example.com}"
: "${E2E_ADMIN_PASSWORD:=e2e-admin-pass-1234}"
export E2E_ADMIN_EMAIL E2E_ADMIN_PASSWORD

# === Build binaries locally ===
mkdir -p "$BIN_DIR"
echo "== Build (CGO_ENABLED=0) =="
for dir in cmd/*/; do
    target=$(basename "$dir")
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
if ! docker compose -p "$E2E_PROJECT" -f "$COMPOSE" up -d --build 2>&1; then
    echo "FAIL: docker compose up" >&2
    exit 1
fi

# === Resolve ephemeral host-ports → endpoints file ===
# Порты эфемерны (127.0.0.1::<port>) для изоляции параллельных форков —
# фактический host-порт известен только после `compose up`. pytest читает
# ENDPOINTS_FILE (conftest), экспорт env обратно из subprocess не доходит.
resolve_port() {
    docker compose -p "$E2E_PROJECT" -f "$COMPOSE" port "$1" "$2" 2>/dev/null \
        | awk -F: 'NF{print $NF; exit}'
}
API_PORT=$(resolve_port api 8080)
PG_PORT=$(resolve_port postgres 5432)
NATS_PORT=$(resolve_port nats 4222)
for _p in API_PORT PG_PORT NATS_PORT; do
    if [ -z "${!_p:-}" ]; then
        echo "FAIL: could not resolve host port ($_p)" >&2
        docker compose -p "$E2E_PROJECT" -f "$COMPOSE" ps >&2
        exit 1
    fi
done
API_BASE="http://127.0.0.1:${API_PORT}"
cat > "$ENDPOINTS_FILE" <<EOF
E2E_API_BASE_URL=${API_BASE}
E2E_PG_DSN=postgresql://postgres:postgres@127.0.0.1:${PG_PORT}/app
E2E_NATS_URL=nats://127.0.0.1:${NATS_PORT}
EOF

# === Wait for api healthy: проверка здоровья образа (`api healthcheck`), та же, что у деплоя ===
echo
echo "== Wait for api healthy (max 60s) =="
API_CONTAINER=$(docker compose -p "$E2E_PROJECT" -f "$COMPOSE" ps -q api)
for i in $(seq 1 30); do
    if [ "$(docker inspect -f '{{.State.Health.Status}}' "$API_CONTAINER" 2>/dev/null)" = healthy ]; then
        echo "  api up after ${i}s × 2s = $((i*2))s"
        echo
        docker compose -p "$E2E_PROJECT" -f "$COMPOSE" ps
        echo
        echo "OK"
        exit 0
    fi
    sleep 2
done

echo "FAIL: api did not become healthy in 60s. Health checks and logs:" >&2
docker inspect -f '{{range .State.Health.Log}}{{.ExitCode}} {{.Output}}{{end}}' "$API_CONTAINER" >&2
docker compose -p "$E2E_PROJECT" -f "$COMPOSE" logs --tail=80 api migrate >&2
exit 1
