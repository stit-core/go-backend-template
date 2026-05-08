#!/usr/bin/env bash
# Claude Code hook: PreToolUse на Bash.
# Блокирует команды с --no-verify (или коротким -n у git commit) — это запрещено
# проектом (см. AGENTS.md §6 Boundaries).

set -uo pipefail

INPUT=$(cat)

if command -v jq >/dev/null 2>&1; then
    CMD=$(echo "$INPUT" | jq -r '.tool_input.command // empty')
else
    CMD=$(echo "$INPUT" | grep -oE '"command":"[^"]+"' | head -1 | sed 's/"command":"\(.*\)"/\1/')
fi

[ -z "$CMD" ] && exit 0

# Ловим --no-verify в любой git-команде.
if echo "$CMD" | grep -qE '(^|[[:space:]])(--no-verify|--no-gpg-sign|--no-edit)([[:space:]]|$)'; then
    cat <<EOF >&2
БЛОК: команда содержит --no-verify (или --no-gpg-sign / --no-edit).

Проект запрещает обходить git-хуки. Если хук падает — разбирайся с причиной.
Если действительно нужен bypass — пользователь должен явно одобрить.

Команда: $CMD
EOF
    exit 2
fi

# Ловим git commit -n (короткая форма --no-verify).
if echo "$CMD" | grep -qE 'git[[:space:]]+(.*[[:space:]])?commit[[:space:]].*[[:space:]]-n([[:space:]]|$)'; then
    cat <<EOF >&2
БЛОК: 'git commit -n' эквивалентно --no-verify, проект это запрещает.

Команда: $CMD
EOF
    exit 2
fi

exit 0
