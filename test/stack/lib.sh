#!/usr/bin/env bash
# test/stack/lib.sh — общая инициализация e2e-стека, sourced из up.sh/down.sh.
#
# Project name детерминирован из пути репо: разные форки шаблона на одной
# машине не схлопывают контейнеры/volumes/сети, а down.sh бьёт ровно тот
# стек, что поднял up.sh, без передачи имени между процессами.

cd "$(git rev-parse --show-toplevel 2>/dev/null || pwd)"

STACK_DIR="test/stack"
BIN_DIR="$STACK_DIR/bin"
COMPOSE="$STACK_DIR/compose.yml"
ENDPOINTS_FILE="$STACK_DIR/.e2e-endpoints.env"

_e2e_hash() {
    if command -v sha1sum >/dev/null 2>&1; then
        sha1sum
    else
        shasum -a 1
    fi
}

E2E_PROJECT="e2e-$(printf '%s' "$PWD" | _e2e_hash | cut -c1-12)"
export COMPOSE_PROJECT_NAME="$E2E_PROJECT"

# Сервисы-профили стека: admin для e2e заводит seed-admin — он есть у проекта со своей идентичностью; кит графа с
# внешней зависимостью (scripts/tools/kits.sh) поднимает сервис с именем кита: cachekit — Redis, natskit — NATS,
# storagekit — MinIO.
profiles=()
if [ -d cmd/seed-admin ]; then
    profiles+=(seed-admin)
fi
# Отказ kits.sh (нет go) — отказ стека: без профиля кита сервис не стартовал бы без своей зависимости. Снос стека
# (down.sh, E2E_ALL_PROFILES=1) киты графа не спрашивает: он берёт все профили стека и без go.
if [ -z "${E2E_ALL_PROFILES:-}" ]; then
    if ! kits=$(bash scripts/tools/kits.sh); then
        echo "FAIL: scripts/tools/kits.sh" >&2
        exit 1
    fi
    for kit in $kits; do
        profiles+=("$kit")
    done
fi
COMPOSE_PROFILES=$(IFS=,; printf '%s' "${profiles[*]}")
export COMPOSE_PROFILES

# Своя зависимость сервиса, которой нет у китов (поисковый движок, заглушка внешнего API), — второй compose-файл
# сервиса compose.service.yml рядом: его сервисы, env и depends_on api compose дописывает слиянием файлов, путь шаблона
# compose.yml сервис не правит.
COMPOSE_FILES=(-f "$COMPOSE")
if [ -f "$STACK_DIR/compose.service.yml" ]; then
    COMPOSE_FILES+=(-f "$STACK_DIR/compose.service.yml")
fi
