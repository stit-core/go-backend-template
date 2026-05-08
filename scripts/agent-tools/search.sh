#!/usr/bin/env bash
# agent-tools/search.sh — обёртка над ripgrep с защитой от перегруза контекста.
#
# Использование:
#   scripts/agent-tools/search.sh PATTERN [path]
#   scripts/agent-tools/search.sh --lines PATTERN [path]    # с контекстом строк
#   scripts/agent-tools/search.sh --symbol PATTERN          # поиск через gopls (Go-символы)
#
# По умолчанию — только список файлов. С --lines — line:col matched-line, ≤100 матчей.
# Если матчей > 100 — отказ с просьбой сузить запрос (паттерн SWE-agent / OpenHands).

set -uo pipefail

MODE="files"
PATTERN=""
SEARCH_PATH="."
MAX_FILES=100
MAX_LINES=200

while [ $# -gt 0 ]; do
    case "$1" in
        --lines) MODE="lines"; shift;;
        --symbol) MODE="symbol"; shift;;
        --max-files=*) MAX_FILES="${1#--max-files=}"; shift;;
        --max-lines=*) MAX_LINES="${1#--max-lines=}"; shift;;
        -h|--help)
            sed -n '2,10p' "$0" | sed 's/^# \{0,1\}//'
            exit 0;;
        *)
            if [ -z "$PATTERN" ]; then
                PATTERN="$1"
            else
                SEARCH_PATH="$1"
            fi
            shift;;
    esac
done

if [ -z "$PATTERN" ]; then
    echo "FAIL: PATTERN обязателен" >&2
    echo "FAIL: missing pattern"
    exit 1
fi

cd "$(git rev-parse --show-toplevel 2>/dev/null || pwd)"

case "$MODE" in
    files)
        if ! command -v rg >/dev/null 2>&1; then
            FILES=$(grep -rlE "$PATTERN" "$SEARCH_PATH" --include='*.go' 2>/dev/null || true)
        else
            FILES=$(rg --files-with-matches --type go "$PATTERN" "$SEARCH_PATH" 2>/dev/null || true)
        fi
        COUNT=$(echo -n "$FILES" | grep -c . || true)
        if [ "$COUNT" -eq 0 ]; then
            echo "OK: 0 файлов с матчами"
            echo "OK"
            exit 0
        fi
        if [ "$COUNT" -gt "$MAX_FILES" ]; then
            echo "FAIL: $COUNT файлов с матчами (лимит $MAX_FILES). Сузь паттерн или укажи путь."
            echo "$FILES" | head -n 20
            echo "... ещё $((COUNT - 20))"
            echo "FAIL: too many matches"
            exit 1
        fi
        echo "$FILES"
        echo
        echo "OK: $COUNT файл(ов)"
        ;;
    lines)
        if ! command -v rg >/dev/null 2>&1; then
            MATCHES=$(grep -rnE "$PATTERN" "$SEARCH_PATH" --include='*.go' 2>/dev/null || true)
        else
            MATCHES=$(rg --line-number --no-heading --type go "$PATTERN" "$SEARCH_PATH" 2>/dev/null || true)
        fi
        COUNT=$(echo -n "$MATCHES" | grep -c . || true)
        if [ "$COUNT" -eq 0 ]; then
            echo "OK"
            exit 0
        fi
        if [ "$COUNT" -gt "$MAX_LINES" ]; then
            echo "FAIL: $COUNT матч(ей) — лимит $MAX_LINES. Сузь паттерн или используй mode=files."
            echo "FAIL: too many matches"
            exit 1
        fi
        echo "$MATCHES"
        echo
        echo "OK: $COUNT матч(ей)"
        ;;
    symbol)
        if ! command -v gopls >/dev/null 2>&1; then
            echo "FAIL: gopls не установлен. Запусти: go install golang.org/x/tools/gopls@latest" >&2
            echo "FAIL: gopls missing"
            exit 1
        fi
        # gopls workspace_symbol: ищет публичные символы в модуле.
        gopls workspace_symbol "$PATTERN" 2>/dev/null | head -n "$MAX_LINES"
        echo
        echo "OK"
        ;;
esac
