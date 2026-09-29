#!/usr/bin/env bash
# tools/test.sh — `go test` со сжатым выводом.
#
# Использование:
#   scripts/tools/test.sh [--integration] [--format=detailed] [pkg…]   (по умолчанию ./...)
#
# --integration — ещё и тесты с build-тегом integration (testcontainers, нужен Docker),
# всегда без кеша результатов.
# Зелёный — одна строка сводки. Красный — вывод `go test` без строк прошедших пакетов
# (падающие тесты, ошибки сборки и загрузки пакетов), не больше 25000 байт; detailed —
# весь вывод. Последняя строка — `OK` / `FAIL: <причина>`.

set -uo pipefail

FORMAT=concise
INTEGRATION=""
PKGS=()
for arg in "$@"; do
    case "$arg" in
        --format=*) FORMAT="${arg#--format=}" ;;
        # Исход integration зависит от Docker, а не только от кода: из кеша go test
        # вернул бы «ok» прогона, где testkit пропустил тесты без Docker.
        --integration) INTEGRATION="-tags=integration -count=1" ;;
        -h | --help)
            sed -n '2,11p' "$0" | sed 's/^# \{0,1\}//'
            exit 0
            ;;
        *) PKGS+=("$arg") ;;
    esac
done
[ ${#PKGS[@]} -gt 0 ] || PKGS=(./...)

cd "$(git rev-parse --show-toplevel)" || exit 1

# race detector требует cgo: без него прогон идёт без -race, и это видно в выводе.
RACE="-race"
if [ "$(go env CGO_ENABLED)" != "1" ]; then
    RACE=""
    echo "WARN: без -race — CGO_ENABLED=$(go env CGO_ENABLED) (нужны cgo и C-компилятор)"
fi
out=$(go test $INTEGRATION $RACE "${PKGS[@]}" 2>&1)
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
    [ ${#fails} -le 25000 ] || printf '\n… вывод обрезан, весь — FORMAT=detailed\n'
fi
echo "FAIL: go test exit $code"
exit 1
