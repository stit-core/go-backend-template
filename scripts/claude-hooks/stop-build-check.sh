#!/usr/bin/env bash
# Claude Code hook: Stop.
# Перед тем как агент остановится — гонит `go build ./...` и `go vet ./...`.
# Если падает — блокирует Stop через exit 2 + stderr; агент должен починить.
#
# stop_hook_active: если true — мы уже в stop-цикле, второй блок приведёт
# к infinite loop, поэтому пропускаем.
#
# Conditional: проверка нужна только если в сессии трогали .go-файлы.
# Иначе (read-only / только docs / только настройки) — skip для скорости.
#
# Tolerant: если go недоступен — skip без ошибки.

set -uo pipefail

# Расширяем PATH стандартными местами установки Go-инструментов.
export PATH="$PATH:/usr/local/go/bin:$HOME/go/bin:/root/go/bin"

INPUT=$(cat)

if command -v jq >/dev/null 2>&1; then
    ACTIVE=$(echo "$INPUT" | jq -r '.stop_hook_active // false')
    TRANSCRIPT=$(echo "$INPUT" | jq -r '.transcript_path // empty')
else
    ACTIVE=$(echo "$INPUT" | grep -oE '"stop_hook_active":(true|false)' | head -1 | grep -oE '(true|false)')
    TRANSCRIPT=$(echo "$INPUT" | grep -oE '"transcript_path":"[^"]+"' | head -1 | sed 's/.*"transcript_path":"\(.*\)"/\1/')
fi

if [ "$ACTIVE" = "true" ]; then
    exit 0
fi

cd "$(git rev-parse --show-toplevel 2>/dev/null || pwd)" || exit 0

# Только если есть .go файлы и go доступен.
[ ! -f "go.mod" ] && exit 0
command -v go >/dev/null 2>&1 || exit 0

# Conditional skip: если транскрипт сессии доступен И в нём нет ни одного
# Edit/Write/MultiEdit на .go-файл — нет смысла гонять build. Working tree
# тоже проверяем (на случай если транскрипт обрезан, но .go реально менялись).
if [ -n "$TRANSCRIPT" ] && [ -f "$TRANSCRIPT" ]; then
    if ! grep -qE '"file_path":"[^"]+\.go"' "$TRANSCRIPT" \
       && [ -z "$(git status --porcelain -- '*.go' 2>/dev/null)" ]; then
        exit 0
    fi
fi

BUILD_OUT=$(go build ./... 2>&1)
BUILD_EXIT=$?

VET_OUT=$(go vet ./... 2>&1)
VET_EXIT=$?

if [ "$BUILD_EXIT" -eq 0 ] && [ "$VET_EXIT" -eq 0 ]; then
    exit 0
fi

# Падает — блокируем Stop, кормим фидбек агенту.
{
    echo "Stop-проверка не прошла. Прежде чем остановиться — почини:"
    echo
    if [ "$BUILD_EXIT" -ne 0 ]; then
        echo "=== go build ./... ==="
        echo "$BUILD_OUT" | head -20
        echo
    fi
    if [ "$VET_EXIT" -ne 0 ]; then
        echo "=== go vet ./... ==="
        echo "$VET_OUT" | head -20
        echo
    fi
    echo "Запусти 'make build' для сжатого диагноза."
} >&2

exit 2
