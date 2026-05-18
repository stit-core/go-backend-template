#!/usr/bin/env bash
# Claude Code hook: PreToolUse на Skill.
# Блокирует запуск scaffolding-скиллов (add-domain, add-endpoint, add-event), если в
# docs/domains/ есть незакрытые драфты или валидатор красный.
#
# Логика:
#  1. Если вызываемый skill — один из gated, читаем frontmatter всех артефактов в
#     docs/domains/**.md.
#  2. Если хотя бы один stable отсутствует и валидатор фейлит — блок.
#  3. Иначе — пропускаем.
#
# Цель — не дать Claude скаффолдить код по неполным артефактам.

set -uo pipefail

INPUT=$(cat)

if command -v jq >/dev/null 2>&1; then
    SKILL=$(echo "$INPUT" | jq -r '.tool_input.skill // empty')
else
    SKILL=$(echo "$INPUT" | grep -oE '"skill":"[^"]+"' | head -1 | sed 's/"skill":"\(.*\)"/\1/')
fi

[ -z "$SKILL" ] && exit 0

case "$SKILL" in
    add-domain|add-endpoint|add-event) ;;
    *) exit 0 ;;
esac

ROOT=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
VALIDATOR="$ROOT/scripts/tools/validate-domain-doc.sh"

# Если docs/domains/ пуст или нет валидатора — не мешаем (greenfield).
if [ ! -d "$ROOT/docs/domains" ] || [ ! -x "$VALIDATOR" ]; then
    exit 0
fi

# Найди артефакты со status: draft
DRAFTS=$(grep -lE '^status:[[:space:]]*draft' "$ROOT"/docs/domains/*/*.md "$ROOT"/docs/domains/*/*/*.md 2>/dev/null | grep -v '/_templates/' || true)

VALIDATOR_OUT=$("$VALIDATOR" "$ROOT/docs/domains" 2>&1)
VALIDATOR_RC=$?

if [ -z "$DRAFTS" ] && [ "$VALIDATOR_RC" -eq 0 ]; then
    exit 0
fi

cat >&2 <<EOF
БЛОК: skill '$SKILL' требует, чтобы все DDD-артефакты в docs/domains/ были stable и валидны.

EOF

if [ -n "$DRAFTS" ]; then
    echo "Артефакты в status: draft (нужно углубить через /domain-deepen):" >&2
    echo "$DRAFTS" | sed "s|$ROOT/||" | sed 's/^/  - /' >&2
    echo >&2
fi

if [ "$VALIDATOR_RC" -ne 0 ]; then
    echo "Валидатор красный:" >&2
    echo "$VALIDATOR_OUT" | sed 's/^/  /' >&2
fi

cat >&2 <<EOF

Что делать:
  1. /domain-deepen <context>  — добить драфты до stable
  2. scripts/tools/validate-domain-doc.sh docs/domains  — проверить
  3. Повторить вызов '$SKILL'

Если действительно нужен bypass (например, репо в greenfield-состоянии и docs/domains/
ещё пустая) — явное одобрение пользователя.
EOF

exit 2
