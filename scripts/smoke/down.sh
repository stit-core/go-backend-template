#!/usr/bin/env bash
# scripts/smoke/down.sh — снести smoke-стек, volumes и локально собранный image.
# Скрипты/compose/Dockerfile.local остаются как артефакты.

set -uo pipefail
cd "$(git rev-parse --show-toplevel 2>/dev/null || pwd)"

SMOKE_DIR="scripts/smoke"
COMPOSE="$SMOKE_DIR/compose.yml"

# Compose down не упадёт если стека нет — он silent на 0 контейнерах.
docker compose -f "$COMPOSE" down -v --rmi local --remove-orphans

# Подчистить бинари (соберутся заново при следующем up).
rm -rf "$SMOKE_DIR/bin"

echo "OK"
