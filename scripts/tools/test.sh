#!/usr/bin/env bash
# tools/test.sh — `go test` со сжатым выводом.
#
# Использование:
#   scripts/tools/test.sh [--format=detailed] [pkg…]      (по умолчанию ./...)
#
# Зелёный — одна строка сводки. Красный — вывод `go test` без строк прошедших пакетов
# (падающие тесты, ошибки сборки и загрузки пакетов), не больше 25000 байт; detailed —
# весь вывод. Последняя строка — `OK` / `FAIL: <причина>`.

set -uo pipefail

FORMAT=concise
PKGS=()
for arg in "$@"; do
    case "$arg" in
        --format=*) FORMAT="${arg#--format=}" ;;
        -h | --help)
            sed -n '2,9p' "$0" | sed 's/^# \{0,1\}//'
            exit 0
            ;;
        *) PKGS+=("$arg") ;;
    esac
done
[ ${#PKGS[@]} -gt 0 ] || PKGS=(./...)

cd "$(git rev-parse --show-toplevel)" || exit 1

# race detector требует cgo.
RACE=""
[ "$(go env CGO_ENABLED)" = "1" ] && RACE="-race"
out=$(go test -short $RACE "${PKGS[@]}" 2>&1)
code=$?

if [ "$code" -eq 0 ]; then
    echo "PASS: $(grep -c '^ok' <<<"$out") пакет(ов) с тестами"
    echo "OK"
    exit 0
fi

if [ "$FORMAT" = detailed ]; then
    echo "$out"
else
    fails=$(grep -vE '^(ok|\?) ' <<<"$out")
    head -c 25000 <<<"$fails"
    [ ${#fails} -le 25000 ] || printf '\n… вывод обрезан: make test FORMAT=detailed\n'
fi
echo "FAIL: go test exit $code"
exit 1
