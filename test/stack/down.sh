#!/usr/bin/env bash
# test/stack/down.sh — снести стек, volumes и локально собранный image.
# Скрипты/compose/Dockerfile.local остаются как артефакты.

set -uo pipefail

source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

# Compose down не упадёт если стека нет — он silent на 0 контейнерах.
docker compose -p "$E2E_PROJECT" -f "$COMPOSE" down -v --rmi local --remove-orphans

# Подчистить бинари + endpoints-файл (пересоздаются при следующем up).
rm -rf "$BIN_DIR"
rm -f "$ENDPOINTS_FILE"

echo "OK"
