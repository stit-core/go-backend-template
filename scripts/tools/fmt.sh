#!/usr/bin/env bash
# tools/fmt.sh — gofumpt с extra-правилами, как у шага lint (.golangci.yml: extra-rules), по каталогам репо с
# Go-файлами (отслеживаемыми и новыми): worktree раннеров и хуков в игнорируемом .cache/ — не код репо.
# Сгенерированное gofumpt в каталогах пропускает сам. Порядок и выравнивание тегов полей (tagalign) gofumpt не правит —
# их правит golangci-lint --fix закреплённой версии, тот же линтер, что судит шаг lint.
#
# Использование:
#   scripts/tools/fmt.sh            форматировать (make fmt)
#   scripts/tools/fmt.sh --check    только проверить, дерево не меняется (make fmt-check — шаг гейта):
#                                   неотформатированные файлы и FAIL последней строкой

set -uo pipefail
export LC_ALL=C

check=""
case "${1:-}" in
    --check) check=1 ;;
    "") ;;
    *)
        echo "FAIL: usage: scripts/tools/fmt.sh [--check]"
        exit 1
        ;;
esac

cd "$(git rev-parse --show-toplevel)" || exit 1
bash scripts/tools/versions.sh gofumpt || exit 1

# Каталог верхнего уровня с Go-файлом репо или сам файл в корне — из тех, что есть на диске: удалённого без git rm
# в дереве нет.
mapfile -t paths < <(git ls-files -co --exclude-standard -- '*.go' | sed 's|/.*||' | sort -u |
    while IFS= read -r p; do [ -e "$p" ] && printf '%s\n' "$p"; done)
[ ${#paths[@]} -gt 0 ] || exit 0

if [ -z "$check" ]; then
    gofumpt -extra -w "${paths[@]}" || exit 1
    bash scripts/tools/versions.sh golangci-lint || exit 1
    # Кеш и окружение — как у scripts/tools/lint.sh: общий кеш подмешал бы чужое дерево.
    export GOLANGCI_LINT_CACHE="$PWD/.cache/golangci-lint"
    unset GOLANGCI_DIFF_PROCESSOR_PATCH GOLANGCI_LINT_CACHEPROG
    # Код 1 — замечания, которые --fix не снял (их называет make lint); выше — линтер не дошёл до правки.
    out=$(golangci-lint run --config .golangci.yml --enable-only tagalign --fix ./... 2>&1)
    if [ $? -gt 1 ]; then
        tail -20 <<<"$out"
        echo "FAIL: golangci-lint --fix (tagalign) — вывод выше"
        exit 1
    fi
    exit 0
fi
out=$(gofumpt -extra -l "${paths[@]}") || {
    echo "FAIL: gofumpt не разобрал дерево (ошибка выше)"
    exit 1
}
if [ -n "$out" ]; then
    sed 's/^/  /' <<<"$out"
    echo "FAIL: fmt — файлы не отформатированы, запусти make fmt"
    exit 1
fi
