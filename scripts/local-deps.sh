#!/usr/bin/env bash
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

POSTGRES_CONTAINER="local-deps-postgres"
NATS_CONTAINER="local-deps-nats"
# Зависимости китов графа (scripts/tools/kits.sh): контейнер — у сервиса, чей cmd/api собирает кит.
REDIS_CONTAINER="local-deps-redis"
MINIO_CONTAINER="local-deps-minio"

check_docker() {
  if ! command -v docker &>/dev/null; then
    echo "ERROR: docker is not installed or not in PATH."
    exit 1
  fi
}

# run_once NAME ARGS… — docker run -d --name NAME ARGS…, если контейнера ещё нет: повторный up не падает на занятом
# имени.
run_once() {
  local name=$1
  shift
  if docker inspect "$name" >/dev/null 2>&1; then
    echo "✓ $name уже запущен"
    return
  fi
  docker run -d --name "$name" --restart unless-stopped "$@"
}

up() {
  check_docker

  echo "Starting local dependencies..."

  run_once "$POSTGRES_CONTAINER" \
    -e POSTGRES_DB=app \
    -e POSTGRES_USER=app \
    -e POSTGRES_PASSWORD=app \
    -p 5432:5432 \
    postgres:17-alpine

  env=(
    "PG_DSN=postgres://app:app@localhost:5432/app?sslmode=disable"
  )
  # Отказ kits.sh (нет go) — отказ up: без списка китов зависимость кита молча не встала бы.
  kits=$(bash scripts/tools/kits.sh)
  for kit in $kits; do
    case "$kit" in
      natskit)
        run_once "$NATS_CONTAINER" -p 4222:4222 nats:2.10-alpine -js
        env+=("NATS_URL=nats://localhost:4222")
        ;;
      cachekit)
        run_once "$REDIS_CONTAINER" -p 6379:6379 redis:7-alpine
        env+=("REDIS_URL=redis://localhost:6379/0")
        ;;
      storagekit)
        # Бакет сервиса (STORAGE_S3_BUCKET из .env.example) заводит сам сервис на старте — STORAGE_S3_CREATE_BUCKET.
        run_once "$MINIO_CONTAINER" \
          -e MINIO_ROOT_USER=local-s3 \
          -e MINIO_ROOT_PASSWORD=local-s3-secret \
          -p 9000:9000 \
          pgsty/silo:RELEASE.2026-10-26T00-00-00Z server /data
        env+=(
          "STORAGE_S3_ENDPOINT=http://localhost:9000"
          "STORAGE_S3_ACCESS_KEY=local-s3"
          "STORAGE_S3_SECRET_KEY=local-s3-secret"
          "STORAGE_S3_CREATE_BUCKET=true"
        )
        ;;
    esac
  done

  echo ""
  echo "All containers started. Recommended environment variables:"
  echo ""
  printf '  %s\n' "${env[@]}"
  # Потоки событий — у сервиса с событиями (natskit): у сервиса без них строк NATS_* в .env.example нет.
  grep -E '^NATS_(JETSTREAM_ENABLED|JS_STREAM_NAMES)=' .env.example | sed 's/^/  /' || true
  echo ""
}

down() {
  check_docker

  echo "Stopping local dependencies..."

  for name in "$POSTGRES_CONTAINER" "$NATS_CONTAINER" "$REDIS_CONTAINER" "$MINIO_CONTAINER"; do
    docker rm -f "$name" 2>/dev/null || true
  done

  echo "All containers removed."
}

case "${1:-up}" in
  up)
    up
    ;;
  down)
    down
    ;;
  *)
    echo "Usage: $0 [up|down]"
    exit 1
    ;;
esac
