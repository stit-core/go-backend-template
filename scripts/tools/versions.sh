#!/usr/bin/env bash
# tools/versions.sh — закреплённые версии инструментов: их ставит make setup (scripts/install-tools.sh), а шаг
# гейта сверяет с ними свой инструмент — у машины с другой версией был бы свой гейт.
#
# Использование:
#   scripts/tools/versions.sh <инструмент>…       сверка: молчит, если стоят закреплённые; иначе — FAIL на
#                                                  каждый и код 1
#   scripts/tools/versions.sh --pin <инструмент>  закреплённая версия

set -uo pipefail
export LC_ALL=C

declare -A pins=(
    [gofumpt]=v0.10.0
    [golangci-lint]=v2.12.2
    [sqlc]=v1.31.1
    [mockery]=v3.7.0
    [govulncheck]=v1.8.0
)

# installed — версия установленного инструмента: команда версии у каждого своя.
installed() {
    case $1 in
        gofumpt) gofumpt --version | awk 'NR == 1 {print $1}' ;;
        golangci-lint) golangci-lint --version | awk 'NR == 1 && $3 == "version" {print "v" $4}' ;;
        sqlc) sqlc version ;;
        mockery) mockery version ;;
        govulncheck) govulncheck -version | sed -n 's/^Scanner: govulncheck@//p' ;;
    esac 2>/dev/null
}

if [ "${1:-}" = --pin ]; then
    pin=${pins[${2:-}]:-}
    [ -n "$pin" ] || { echo "FAIL: нет закреплённой версии '${2:-}'"; exit 1; }
    echo "$pin"
    exit 0
fi
if [ $# -eq 0 ]; then
    echo "FAIL: usage: scripts/tools/versions.sh [--pin] <инструмент>…"
    exit 1
fi
fail=0
for tool in "$@"; do
    pin=${pins[$tool]:-}
    if [ -z "$pin" ]; then
        echo "FAIL: нет закреплённой версии '$tool'"
    elif ! command -v "$tool" >/dev/null; then
        echo "FAIL: $tool не найден — make setup"
    elif got=$(installed "$tool") && [ "$got" = "$pin" ]; then
        continue
    else
        echo "FAIL: $tool ${got:-не назвал версию}, закреплена $pin — make setup"
    fi
    fail=1
done
exit "$fail"
