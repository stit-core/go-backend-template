#!/usr/bin/env bash
# Claude Code hook: Stop.
# Перед тем как агент остановится — гонит `go build ./...` и `go vet ./...`.
# Если падает — блокирует Stop через exit 2 + stderr; агент должен починить.
#
# stop_hook_active: если true — мы уже в stop-цикле, второй блок приведёт
# к infinite loop, поэтому пропускаем.
#
# Tolerant: если go недоступен — skip без ошибки.

set -uo pipefail

# Расширяем PATH стандартными местами установки Go-инструментов.
export PATH="$PATH:/usr/local/go/bin:$HOME/go/bin:/root/go/bin"

INPUT=$(cat)

if command -v jq >/dev/null 2>&1; then
    ACTIVE=$(echo "$INPUT" | jq -r '.stop_hook_active // false')
else
    ACTIVE=$(echo "$INPUT" | grep -oE '"stop_hook_active":(true|false)' | head -1 | grep -oE '(true|false)')
fi

if [ "$ACTIVE" = "true" ]; then
    exit 0
fi

cd "$(git rev-parse --show-toplevel 2>/dev/null || pwd)" || exit 0

# Только если есть .go файлы и go доступен.
[ ! -f "go.mod" ] && exit 0
command -v go >/dev/null 2>&1 || exit 0

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
    echo "Запусти 'scripts/agent-tools/build.sh' для сжатого диагноза."
} >&2

exit 2
