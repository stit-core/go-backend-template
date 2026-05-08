#!/usr/bin/env bash
# agent-tools/repomap.sh — карта пакетов проекта с публичными символами.
#
# Использование:
#   scripts/agent-tools/repomap.sh                # все пакеты модуля
#   scripts/agent-tools/repomap.sh ./internal/... # подмножество
#
# Идея — Aider repo-map для Go: вместо tree-sitter используем `go list -json`
# (импорты, deps) + `go doc` (публичные символы). Бюджет по умолчанию ~1000 строк.

set -uo pipefail

PATHS=("./...")
PATHS_SET=0
MAX_LINES=1000
DEPTH="public"

for arg in "$@"; do
    case "$arg" in
        --max-lines=*) MAX_LINES="${arg#--max-lines=}";;
        --all) DEPTH="all";;
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

# Список пакетов модуля.
PKGS=$(go list "${PATHS[@]}" 2>/dev/null | grep -v "^$(go list -m)\$" | sort)

if [ -z "$PKGS" ]; then
    echo "FAIL: пакеты не найдены"
    echo "FAIL"
    exit 1
fi

OUTPUT=""
LINES=0

while IFS= read -r pkg; do
    [ -z "$pkg" ] && continue

    # Краткое описание пакета: первая строка doc-комментария + публичные символы.
    DOC=$(go doc -short "$pkg" 2>/dev/null | head -c 4000 || true)
    [ -z "$DOC" ] && continue

    OUTPUT+="=== $pkg ===\n"
    OUTPUT+="$DOC"$'\n\n'

    LINES=$(printf '%s' "$OUTPUT" | wc -l | tr -d ' ')
    if [ "$LINES" -gt "$MAX_LINES" ]; then
        OUTPUT+="...\n"
        OUTPUT+="(обрезано на лимите $MAX_LINES строк; --max-lines=N для увеличения, --all для приватных символов)\n"
        break
    fi
done <<<"$PKGS"

printf '%b' "$OUTPUT"
echo
echo "OK: $LINES строк"
