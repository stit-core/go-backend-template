#!/usr/bin/env bash
# test/stack/down.sh — снести стек, volumes и локально собранный image.
# Скрипты/compose/Dockerfile.local остаются как артефакты.

set -uo pipefail

E2E_ALL_PROFILES=1
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

# Все профили стека: контейнер кита, которого в графе уже нет, снимается тоже, и go для этого не нужен.
COMPOSE_PROFILES=$(docker compose -p "$E2E_PROJECT" "${COMPOSE_FILES[@]}" config --profiles | paste -sd, -)
export COMPOSE_PROFILES

# Compose down не упадёт если стека нет — он silent на 0 контейнерах.
docker compose -p "$E2E_PROJECT" "${COMPOSE_FILES[@]}" down -v --rmi local --remove-orphans

# Подчистить бинари + endpoints-файл (пересоздаются при следующем up).
rm -rf "$BIN_DIR"
rm -f "$ENDPOINTS_FILE"

echo "OK"
