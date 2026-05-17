#!/usr/bin/env bash
# Claude Code hook: PreToolUse на Edit/Write/MultiEdit.
# Защищает пути от прямой правки: секреты (credentials/ключи), сгенерированные файлы.

set -uo pipefail

INPUT=$(cat)

if command -v jq >/dev/null 2>&1; then
    FILE=$(echo "$INPUT" | jq -r '.tool_input.file_path // empty')
else
    FILE=$(echo "$INPUT" | grep -oE '"file_path":"[^"]+"' | head -1 | sed 's/"file_path":"\(.*\)"/\1/')
fi

[ -z "$FILE" ] && exit 0

# Нормализуем путь относительно корня репо для матчинга.
ROOT=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
REL="${FILE#"$ROOT/"}"

block() {
    local reason="$1"
    cat <<EOF >&2
БЛОК записи: $FILE
Причина: $reason
EOF
    exit 2
}

# Секреты.
case "$REL" in
    *.pem|*.key|*credentials*|*secrets*)
        block "путь похож на секреты. Если это не так и реально нужно записать — пользователь должен явно одобрить."
        ;;
esac

# Сгенерированные файлы — регенерация через make-таргеты.
case "$REL" in
    */infra/postgres/db/*.go)
        block "файл сгенерирован sqlc. Правь .sql в queries/ и запусти 'make sqlc'."
        ;;
    */mocks/*.go)
        block "файл сгенерирован mockery. Правь интерфейс и запусти 'make mocks'."
        ;;
    coverage.out|coverage.*)
        block "артефакт coverage. Не должен лежать в репо."
        ;;
    bin/*)
        block "артефакт сборки. Не правь руками."
        ;;
esac

# go.sum — только через `go mod tidy`.
if [ "$REL" = "go.sum" ]; then
    block "go.sum управляется 'go mod tidy', не правь руками."
fi

exit 0
