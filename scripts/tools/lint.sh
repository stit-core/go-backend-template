#!/usr/bin/env bash
# tools/lint.sh — golangci-lint для гейта: fail-closed, сжатый вывод.
#
# Использование:
#   scripts/tools/lint.sh [--format=detailed] [pkg…]      (по умолчанию ./...)
#
# Конфиг — только .golangci.yml (без --config golangci-lint взял бы первым .golangci.toml или .json
# рядом). Сначала `golangci-lint config verify`, затем `run`. OK — только exit 0 вместе со строкой
# отчёта «0 issues.». Замечания — по строке на каждое, до 40 строк (detailed — все, со
# строками кода); любой другой исход (битый конфиг, неизвестный линтер, таймаут,
# несуществующий пакет) — хвост вывода линтера и его код выхода. Последняя строка — `OK` /
# `FAIL: <причина>`.

set -uo pipefail

FORMAT=concise
PKGS=()
for arg in "$@"; do
    case "$arg" in
        --format=*) FORMAT="${arg#--format=}" ;;
        -h | --help)
            sed -n '2,12p' "$0" | sed 's/^# \{0,1\}//'
            exit 0
            ;;
        *) PKGS+=("$arg") ;;
    esac
done
[ ${#PKGS[@]} -gt 0 ] || PKGS=(./...)

cd "$(git rev-parse --show-toplevel)" || exit 1

if ! command -v golangci-lint >/dev/null 2>&1; then
    echo "FAIL: golangci-lint не установлен — make setup"
    exit 1
fi

# Кеш golangci-lint ключуется путём пакета, а у каждого worktree тот же module path:
# общий кеш подмешивает в прогон замечания чужого дерева.
export GOLANGCI_LINT_CACHE="$PWD/.cache/golangci-lint"
# Патч из окружения оставил бы в отчёте только замечания по нему, cache-программа — чужой кеш.
unset GOLANGCI_DIFF_PROCESSOR_PATCH GOLANGCI_LINT_CACHEPROG

# run молча пропускает узлы конфига не на своём месте, а без .golangci.yml берёт
# дефолтные линтеры — схема ловит и то и другое.
if ! out=$(golangci-lint config verify --config .golangci.yml 2>&1); then
    echo "$out"
    echo "FAIL: .golangci.yml не прошёл golangci-lint config verify"
    exit 1
fi

if [ "$FORMAT" = detailed ]; then
    ARGS=(--max-issues-per-linter=0 --max-same-issues=0)
else
    ARGS=(--output.text.print-issued-lines=false)
fi
out=$(golangci-lint run --config .golangci.yml "${ARGS[@]}" "${PKGS[@]}" 2>&1)
code=$?

case "$code" in
    0)
        # exit 0 без строки отчёта — прогон не дошёл до отчёта или код выхода занижен
        # конфигом (run.issues-exit-code).
        if grep -qx '0 issues\.' <<<"$out"; then
            echo "OK"
            exit 0
        fi
        echo "$out"
        echo "FAIL: golangci-lint вышел с 0 без строки «0 issues.»"
        ;;
    1)
        if [ "$FORMAT" = detailed ]; then
            echo "$out"
        else
            head -n 40 <<<"$out"
            [ "$(wc -l <<<"$out")" -le 40 ] || echo "… полный список: make lint FORMAT=detailed"
        fi
        echo "FAIL: lint — замечания выше"
        ;;
    *)
        tail -n 20 <<<"$out"
        echo "FAIL: golangci-lint exit $code — линтер не отработал, причина выше"
        ;;
esac
exit 1
