#!/usr/bin/env bash
# agent-tools/build.sh — обёртка над `go build ./...` для агента.
#
# Использование:
#   scripts/agent-tools/build.sh [paths…]
#
# На зелёном — одна строка. На красном — первые 5 ошибок c file:line:col,
# в конце — сколько ещё осталось. Status-line: `OK` / `FAIL: build failed`.

set -uo pipefail

PATHS=("./...")
PATHS_SET=0
TOP=5

for arg in "$@"; do
    case "$arg" in
        --top=*) TOP="${arg#--top=}";;
        -h|--help)
            sed -n '2,9p' "$0" | sed 's/^# \{0,1\}//'
            exit 0;;
        *)
            if [ "$PATHS_SET" -eq 0 ]; then
                PATHS=()
                PATHS_SET=1
            fi
            PATHS+=("$arg");;
    esac
done

cd "$(git rev-parse --show-toplevel 2>/dev/null || pwd)"

TMP=$(mktemp)
trap 'rm -f "$TMP"' EXIT

go build "${PATHS[@]}" 2>"$TMP"
EXIT=$?

if [ "$EXIT" -eq 0 ]; then
    echo "OK: build чистый"
    echo "OK"
    exit 0
fi

# Ошибки go build: формат `path/file.go:line:col: message` или `package <pkg>: ...`.
ERRORS=$(grep -E '\.go:[0-9]+:[0-9]+:' "$TMP" || true)
TOTAL=$(echo "$ERRORS" | grep -c . || true)
PKG_ERRORS=$(grep -E '^# ' "$TMP" || true)

if [ -n "$PKG_ERRORS" ]; then
    echo "FAIL: build error(s)"
    echo
    echo "=== Пакеты с ошибками ==="
    echo "$PKG_ERRORS"
    echo
fi

if [ "$TOTAL" -gt 0 ]; then
    echo "=== Первые $TOP ошибок (всего $TOTAL) ==="
    echo "$ERRORS" | head -n "$TOP"
    if [ "$TOTAL" -gt "$TOP" ]; then
        echo
        echo "... ещё $((TOTAL - TOP)) ошибок. Полный вывод: go build ${PATHS[*]}"
    fi
else
    # Не смогли распарсить — выводим сырой stderr (обрезанный).
    echo "=== Сырой stderr (первые 1000 байт) ==="
    head -c 1000 "$TMP"
fi

echo
echo "FAIL: build failed"
exit 1
