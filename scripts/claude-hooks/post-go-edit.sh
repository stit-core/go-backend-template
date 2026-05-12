#!/usr/bin/env bash
# Claude Code hook: PostToolUse на Edit/Write/MultiEdit.
# Если редактировался .go файл — gofumpt + go vet, изменения возвращаются модели
# через stderr (exit 2 кормит фидбек обратно в loop).
#
# Tolerant: если go/gofumpt недоступны — skip без ошибки (агент увидит проблему
# в make lint/CI). Иначе хук был бы шумным на каждый edit в средах без Go.
#
# Anthropic Best Practices: "CLAUDE.md instructions are advisory; hooks are
# deterministic and guarantee the action happens."

set -uo pipefail

# Расширяем PATH стандартными местами установки Go-инструментов.
export PATH="$PATH:/usr/local/go/bin:$HOME/go/bin:/root/go/bin"

# Hook получает JSON через stdin.
INPUT=$(cat)

# Извлекаем file_path. Для MultiEdit может быть несколько — берём все file_path в JSON.
if command -v jq >/dev/null 2>&1; then
    FILE=$(echo "$INPUT" | jq -r '.tool_input.file_path // empty')
else
    FILE=$(echo "$INPUT" | grep -oE '"file_path":"[^"]+"' | head -1 | sed 's/"file_path":"\(.*\)"/\1/')
fi

[ -z "$FILE" ] && exit 0
[[ "$FILE" != *.go ]] && exit 0
[ ! -f "$FILE" ] && exit 0

# Сгенерированные файлы: gofumpt не запускаем.
case "$FILE" in
    */infra/postgres/db/*) exit 0;;
    *mock_*_test.go) exit 0;;
esac

cd "$(git rev-parse --show-toplevel 2>/dev/null || pwd)" || exit 0

CHANGED=0
MESSAGES=""

# 1. gofumpt -w (skip если недоступен).
if command -v gofumpt >/dev/null 2>&1; then
    BEFORE=$(md5sum "$FILE" 2>/dev/null | awk '{print $1}' || echo "")
    gofumpt -w "$FILE" 2>/dev/null
    AFTER=$(md5sum "$FILE" 2>/dev/null | awk '{print $1}' || echo "")
    if [ "$BEFORE" != "$AFTER" ]; then
        CHANGED=1
        MESSAGES+="gofumpt: переформатировал $FILE\n"
    fi
fi

# 2. go vet на пакет файла (skip если go недоступен).
# cd в директорию файла и vet '.' — устойчиво к абсолютным путям вне CWD
# (например при правке golibinfra через `replace` directive из другого репо).
if command -v go >/dev/null 2>&1; then
    PKG_DIR=$(dirname "$FILE")
    VET_OUT=$(cd "$PKG_DIR" && go vet . 2>&1 || true)
    if [ -n "$VET_OUT" ] && ! echo "$VET_OUT" | grep -q "no Go files"; then
        MESSAGES+="go vet: $(echo "$VET_OUT" | head -3)\n"
    fi
fi

if [ -z "$MESSAGES" ]; then
    exit 0
fi

# Возврат через stderr с exit 2 — фидбек в модель.
printf '%b' "$MESSAGES" >&2
[ "$CHANGED" -eq 1 ] && echo "(файл переформатирован — изменения уже на диске)" >&2
exit 2
