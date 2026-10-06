#!/usr/bin/env bash
# tools/cover.sh — покрытие по профилю `make test-int` (юнит + integration одним прогоном).
#
# Использование:
#   scripts/tools/cover.sh [--format=detailed] <profile> <floor>
#
# Scope — пакеты internal/ и pkg/ (профиль пишет test-int с -coverpkg, так что fx-провайдеры, которые
# исполняет boot-тест cmd/api, засчитываются), кроме сгенерированного (заголовок «Code generated … DO
# NOT EDIT.»: sqlc, mockery) и тестовой обвязки internal/testkit/. Два условия: доля statement'ов всех
# блоков (и замыканий вне функций) не ниже floor и ни одной функции, у которой тесты не исполнили ни
# одного statement'а, — новая функция без тестов не проходит при любом проценте. Отказ — с файлами и
# функциями, detailed — ещё и каждый непокрытый блок. Последняя строка — `OK: …` / `FAIL: …`.

set -uo pipefail

FORMAT=concise
ARGS=()
for arg in "$@"; do
    case "$arg" in
        --format=*) FORMAT="${arg#--format=}" ;;
        -h | --help)
            sed -n '2,12p' "$0" | sed 's/^# \{0,1\}//'
            exit 0
            ;;
        *) ARGS+=("$arg") ;;
    esac
done
if [ ${#ARGS[@]} -ne 2 ]; then
    echo "FAIL: usage: scripts/tools/cover.sh [--format=detailed] <profile> <floor>"
    exit 1
fi
profile=${ARGS[0]}
floor=${ARGS[1]}

cd "$(git rev-parse --show-toplevel)" || exit 1
if [ ! -s "$profile" ]; then
    echo "FAIL: нет профиля $profile — его пишет make test-int по всему дереву"
    exit 1
fi

module=$(go list -m) || { echo "FAIL: go list -m"; exit 1; }
generated=$(grep -rlE --include='*.go' '^// Code generated .* DO NOT EDIT\.$' internal pkg | sed "s|^|$module/|")
scoped=$(mktemp) || exit 1
trap 'rm -f "$scoped"' EXIT

# Блок профиля: <файл>:<начало>,<конец> <statement'ов> <счётчик>; блок приходит от каждого тестового бинаря,
# покрыт — если счётчик хоть одного больше нуля. Блоки scope идут ещё и в $scoped — для покрытия функций;
# там блок пустого тела считается за statement: иначе -func даёт исполненной пустой функции 0.0% (0 из 0).
report=$(LC_ALL=C awk -v testkit="$module/internal/testkit/" -v scoped="$scoped" '
    FNR == NR { if ($0 != "") gen[$0] = 1; next }
    FNR == 1 { print > scoped; next }
    {
        file = substr($1, 1, index($1, ":") - 1)
        if (file in gen || index(file, testkit) == 1) next
        line = $0; if ($2 == 0) { $2 = 1 }; print > scoped; $0 = line
        if (!($1 in stmts)) { stmts[$1] = $2; fileOf[$1] = file; order[++n] = $1 }
        if ($3 > 0) hit[$1] = 1
    }
    END {
        for (i = 1; i <= n; i++) {
            b = order[i]; total += stmts[b]
            if (b in hit) { covered += stmts[b]; continue }
            miss[fileOf[b]] += stmts[b]
            printf "BLOCK %s %d\n", b, stmts[b]
        }
        for (f in miss) printf "MISS %d %s\n", miss[f], f
        printf "TOTAL %d %d\n", covered, total
    }' <(printf '%s\n' "$generated") "$profile")

read -r covered total < <(awk '$1 == "TOTAL" {print $2, $3}' <<<"$report")
if [ "${total:-0}" -eq 0 ]; then
    echo "FAIL: в профиле $profile нет statement'ов scope (internal/, pkg/)"
    exit 1
fi
funcs=$(go tool cover -func="$scoped") || { echo "FAIL: go tool cover -func"; exit 1; }
untested=$(awk '$1 != "total:" && $NF == "0.0%" {print $1, $2}' <<<"$funcs")

pct=$(LC_ALL=C awk -v c="$covered" -v t="$total" 'BEGIN { printf "%.2f", 100 * c / t }')
summary="$covered/$total statement'ов; юнит + integration, internal/ и pkg/ без сгенерированного и internal/testkit"
low=""
LC_ALL=C awk -v c="$covered" -v t="$total" -v f="$floor" 'BEGIN { exit !(100 * c / t < f) }' && low=1

if [ "$FORMAT" = detailed ]; then
    awk '$1 == "BLOCK" {printf "  %s (%d)\n", $2, $3}' <<<"$report"
fi
if [ -z "$low" ] && [ -z "$untested" ]; then
    echo "OK: coverage $pct% >= $floor%, функций без тестов нет ($summary)"
    exit 0
fi
if [ -n "$untested" ]; then
    echo "Функции без тестов (не исполнен ни один statement):"
    sed 's/^/  /' <<<"$untested"
fi
if [ -n "$low" ]; then
    echo "Непокрытые statement'ы по файлам (FORMAT=detailed — блоки):"
    awk '$1 == "MISS" {print $2, $3}' <<<"$report" | sort -k1,1nr | head -20 | sed 's/^/  /'
fi
reasons=()
[ -z "$low" ] || reasons+=("coverage $pct% < floor $floor%")
[ -z "$untested" ] || reasons+=("функций без тестов: $(grep -c . <<<"$untested")")
echo "FAIL: $(IFS=';'; echo "${reasons[*]}" | sed 's/;/; /g') ($summary)"
exit 1
