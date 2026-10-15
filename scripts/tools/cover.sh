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
# одного statement'а, — новая функция без тестов не проходит при любом проценте. Блок из списка
# непокрываемого (ниже) идёт исполненным. Отказ — с блоками и функциями (detailed — все блоки).
# Последняя строка — `OK: …` / `FAIL: …`.

set -uo pipefail
export LC_ALL=C

FORMAT=concise
ARGS=()
for arg in "$@"; do
    case "$arg" in
        --format=*) FORMAT="${arg#--format=}" ;;
        -h | --help)
            sed -n '2,13p' "$0" | sed 's/^# \{0,1\}//'
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

# Непокрываемое — блок, который тесты не исполнят по построению: строка «<файл от корня репо>:<текст из
# строк блока>», над ней — комментарий «# почему». Запись покрывает ровно один неисполненный блок этого файла —
# тот, в строках которого есть её текст; запись без текста, без такого блока или с несколькими — отказ: список
# не копит устаревшее, и новый неисполненный блок с тем же текстом за старой записью не прячется. Пополнить
# список — ослабить гейт (CLAUDE.md § Сначала спросить).
uncoverable=$(
    cat <<'LIST'
LIST
)

# Блок профиля: <файл>:<начало>,<конец> <statement'ов> <счётчик>; блок приходит от каждого тестового бинаря,
# покрыт — если счётчик хоть одного больше нуля. Блоки scope идут ещё и в $scoped — для покрытия функций;
# там блок пустого тела считается за statement: иначе -func даёт исполненной пустой функции 0.0% (0 из 0).
report=$(awk -v module="$module/" -v testkit="$module/internal/testkit/" -v scoped="$scoped" '
    FNR == 1 { part++ }
    part == 1 {
        if ($0 !~ /^[[:space:]]*(#|$)/) {
            i = index($0, ":"); efile[++ne] = substr($0, 1, i - 1); etext[ne] = substr($0, i + 1)
            if (etext[ne] ~ /^[[:space:]]*$/) etext[ne] = ""
        }
        next
    }
    part == 2 { if ($0 != "") gen[$0] = 1; next }
    FNR == 1 { head = $0; next }
    {
        file = substr($1, 1, index($1, ":") - 1)
        if (file in gen || index(file, testkit) == 1) next
        lines[++nl] = $0
        if (!($1 in stmts)) { stmts[$1] = $2; fileOf[$1] = substr(file, length(module) + 1); order[++n] = $1 }
        if ($3 > 0) hit[$1] = 1
    }
    # inblock — текст записи e есть в строках блока b.
    function inblock(b, e,   se, from, to, l, ln, found) {
        split(substr(b, index(b, ":") + 1), se, ","); from = int(se[1]); to = int(se[2])
        ln = 0; found = 0
        while ((getline l < efile[e]) > 0) if (++ln >= from && ln <= to && index(l, etext[e])) found = 1
        close(efile[e])
        return found
    }
    END {
        for (i = 1; i <= n; i++) {
            b = order[i]
            if (b in hit) continue
            for (e = 1; e <= ne; e++) if (etext[e] != "" && efile[e] == fileOf[b] && inblock(b, e)) { blocks[e]++; blockOf[e] = b }
        }
        for (e = 1; e <= ne; e++) {
            if (etext[e] == "") printf "EMPTY %s:\n", efile[e]
            else if (blocks[e] > 1) printf "WIDE %s:%s — блоков %d\n", efile[e], etext[e], blocks[e]
            else if (blocks[e] == 1 && !(blockOf[e] in hit)) { hit[blockOf[e]] = 1; excluded += stmts[blockOf[e]] }
        }
        print head > scoped
        for (i = 1; i <= nl; i++) {
            $0 = lines[i]
            if ($2 == 0) $2 = 1
            if ($1 in hit) $3 = 1
            print > scoped
        }
        for (i = 1; i <= n; i++) {
            b = order[i]; total += stmts[b]
            if (b in hit) { covered += stmts[b]; continue }
            printf "BLOCK %s%s %d\n", fileOf[b], substr(b, index(b, ":")), stmts[b]
        }
        for (e = 1; e <= ne; e++) if (etext[e] != "" && !blocks[e]) printf "STALE %s:%s\n", efile[e], etext[e]
        printf "TOTAL %d %d %d\n", covered, total, excluded
    }' <(printf '%s\n' "$uncoverable") <(printf '%s\n' "$generated") "$profile")

read -r covered total excluded < <(awk '$1 == "TOTAL" {print $2, $3, $4}' <<<"$report")
if [ "${total:-0}" -eq 0 ]; then
    echo "FAIL: в профиле $profile нет statement'ов scope (internal/, pkg/)"
    exit 1
fi
for kind in EMPTY WIDE STALE; do
    entries=$(sed -n "s/^$kind //p" <<<"$report")
    [ -n "$entries" ] || continue
    sed 's/^/  /' <<<"$entries"
    case $kind in
        EMPTY) echo "FAIL: запись непокрываемого без текста блока — сняла бы весь файл: впиши текст из строк блока" ;;
        WIDE) echo "FAIL: запись непокрываемого подходит к нескольким неисполненным блокам — текст должен быть из одного" ;;
        STALE) echo "FAIL: запись непокрываемого без неисполненного блока — убери её из scripts/tools/cover.sh" ;;
    esac
    exit 1
done
funcs=$(go tool cover -func="$scoped") || { echo "FAIL: go tool cover -func"; exit 1; }
untested=$(awk '$1 != "total:" && $NF == "0.0%" {print $1, $2}' <<<"$funcs")

pct=$(awk -v c="$covered" -v t="$total" 'BEGIN { printf "%.2f", 100 * c / t }')
summary="$covered/$total statement'ов; юнит + integration, internal/ и pkg/ без сгенерированного и internal/testkit"
[ "$excluded" -eq 0 ] || summary+="; непокрываемое списком — $excluded"
low=""
awk -v c="$covered" -v t="$total" -v f="$floor" 'BEGIN { exit !(100 * c / t < f) }' && low=1

if [ -z "$low" ] && [ -z "$untested" ]; then
    echo "OK: coverage $pct% >= $floor%, функций без тестов нет ($summary)"
    exit 0
fi
if [ -n "$untested" ]; then
    echo "Функции без тестов (не исполнен ни один statement):"
    sed 's/^/  /' <<<"$untested"
fi
if [ -n "$low" ]; then
    blocks=$(awk '$1 == "BLOCK" {printf "%s (%d)\n", $2, $3}' <<<"$report")
    echo "Непокрытые блоки (файл:строка.колонка,строка.колонка и statement'ов):"
    if [ "$FORMAT" = detailed ]; then
        sed 's/^/  /' <<<"$blocks"
    else
        head -20 <<<"$blocks" | sed 's/^/  /'
        [ "$(wc -l <<<"$blocks")" -le 20 ] || echo "  … все — FORMAT=detailed"
    fi
fi
reasons=()
[ -z "$low" ] || reasons+=("coverage $pct% < floor $floor%")
[ -z "$untested" ] || reasons+=("функций без тестов: $(grep -c . <<<"$untested")")
echo "FAIL: $(IFS=';'; echo "${reasons[*]}" | sed 's/;/; /g') ($summary)"
exit 1
