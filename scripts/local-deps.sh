#!/usr/bin/env bash
set -euo pipefail

POSTGRES_CONTAINER="local-deps-postgres"
NATS_CONTAINER="local-deps-nats"

check_docker() {
  if ! command -v docker &>/dev/null; then
    echo "ERROR: docker is not installed or not in PATH."
    exit 1
  fi
}

up() {
  check_docker

  echo "Starting local dependencies..."

  # Идемпотентность: повторный up не падает на уже существующих контейнерах
  # (docker run --name на занятое имя возвращает ошибку).
  if ! docker inspect "$POSTGRES_CONTAINER" >/dev/null 2>&1; then
    docker run -d \
      --name "$POSTGRES_CONTAINER" \
      --restart unless-stopped \
      -e POSTGRES_DB=app \
      -e POSTGRES_USER=app \
      -e POSTGRES_PASSWORD=app \
      -p 5432:5432 \
      postgres:17-alpine
  else
    echo "✓ $POSTGRES_CONTAINER уже запущен"
  fi

  if ! docker inspect "$NATS_CONTAINER" >/dev/null 2>&1; then
    docker run -d \
      --name "$NATS_CONTAINER" \
      --restart unless-stopped \
      -p 4222:4222 \
      nats:2.10-alpine -js
  else
    echo "✓ $NATS_CONTAINER уже запущен"
  fi

  echo ""
  echo "All containers started. Recommended environment variables:"
  echo ""
  echo "  PG_DSN=postgres://app:app@localhost:5432/app?sslmode=disable"
  echo "  NATS_URL=nats://localhost:4222"
  echo "  NATS_JETSTREAM_ENABLED=true"
  echo "  NATS_JS_STREAM_NAMES=user,verification"
  echo ""
}

down() {
  check_docker

  echo "Stopping local dependencies..."

  docker rm -f "$POSTGRES_CONTAINER" 2>/dev/null || true
  docker rm -f "$NATS_CONTAINER" 2>/dev/null || true

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
