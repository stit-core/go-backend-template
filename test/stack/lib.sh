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
