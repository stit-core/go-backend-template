#!/usr/bin/env bash
# tools/fmt.sh — gofumpt с extra-правилами, как у шага lint (.golangci.yml: extra-rules), по каталогам репо с
# Go-файлами (отслеживаемыми и новыми): worktree раннеров и хуков в игнорируемом .cache/ — не код репо.
# Сгенерированное gofumpt в каталогах пропускает сам.
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
    gofumpt -extra -w "${paths[@]}"
    exit
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
