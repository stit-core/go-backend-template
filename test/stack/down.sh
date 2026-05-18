#!/usr/bin/env bash
# test/stack/down.sh — снести стек, volumes и локально собранный image.
# Скрипты/compose/Dockerfile.local остаются как артефакты.

set -uo pipefail
cd "$(git rev-parse --show-toplevel 2>/dev/null || pwd)"

STACK_DIR="test/stack"
COMPOSE="$STACK_DIR/compose.yml"

# Compose down не упадёт если стека нет — он silent на 0 контейнерах.
docker compose -f "$COMPOSE" down -v --rmi local --remove-orphans

# Подчистить бинари (соберутся заново при следующем up).
rm -rf "$STACK_DIR/bin"

echo "OK"
