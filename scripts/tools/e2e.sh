#!/usr/bin/env bash
# tools/e2e.sh — обёртка над pytest-харнесом (test/) со сжатым выводом.
#
# Использование:
#   scripts/tools/e2e.sh                 # весь набор
#   scripts/tools/e2e.sh --smoke         # только -m smoke
#   scripts/tools/e2e.sh --format=detailed
#
# Сначала линтеры харнеса (ruff, mypy по test/pyproject.toml), затем pytest; стек поднимает/сносит pytest
# session-фикстура (test/conftest.py shell-out'ит в test/stack/{up,down}.sh — нужен Docker). venv в test/.venv
# создаётся из test/requirements.txt, а когда файл сменился, pip ставит его закреплённые версии заново.
#
# Канонический status-line в конце: `OK` / `FAIL: <reason>`.

set -uo pipefail

FORMAT="concise"
MARK=()
MAX_BYTES=25000

for arg in "$@"; do
    case "$arg" in
        --smoke) MARK=(-m smoke);;
        --format=*) FORMAT="${arg#--format=}";;
        --max-bytes=*) MAX_BYTES="${arg#--max-bytes=}";;
        -h|--help) sed -n '2,14p' "$0" | sed 's/^# \{0,1\}//'; exit 0;;
        *) echo "FAIL: unknown arg: $arg" >&2; exit 1;;
    esac
done

cd "$(git rev-parse --show-toplevel 2>/dev/null || pwd)"

VENV="test/.venv"
want=$(sha256sum test/requirements.txt | cut -d' ' -f1)
if [ ! -x "$VENV/bin/pytest" ] || [ "$(cat "$VENV/.requirements.sha256" 2>/dev/null)" != "$want" ]; then
    echo "== bootstrap $VENV (test/requirements.txt) =="
    if ! python3 -m venv "$VENV" 2>/dev/null; then
        echo "FAIL: python3 -m venv не сработал (нужен пакет python3-venv)." >&2
        exit 1
    fi
    if ! "$VENV/bin/pip" -q install -r test/requirements.txt; then
        echo "FAIL: pip install -r test/requirements.txt" >&2
        exit 1
    fi
    echo "$want" >"$VENV/.requirements.sha256"
fi

# Харнес — типизированный код: ruff и mypy (test/pyproject.toml) — до стека.
for lint in "ruff check ." "mypy"; do
    # shellcheck disable=SC2086 # команда и её аргументы — отдельными словами
    if ! out=$(cd test && ".venv/bin/"$lint 2>&1); then
        echo "$out" | tail -30
        echo "FAIL: ${lint%% *} — харнес test/ (вывод выше)"
        exit 1
    fi
done
echo "OK: ruff и mypy харнеса"

TMP=$(mktemp)
trap 'rm -f "$TMP"' EXIT

( cd test && ".venv/bin/pytest" -q "${MARK[@]}" ) >"$TMP" 2>&1
EXIT=$?

if [ "$FORMAT" = "detailed" ]; then
    cat "$TMP"
fi

if [ "$EXIT" -eq 0 ]; then
    grep -E '[0-9]+ passed' "$TMP" | tail -1 || echo "passed"
    echo "OK"
    exit 0
fi

# Красный: падающие тесты + хвост вывода.
echo "== Failures =="
grep -E '^(FAILED|ERROR) ' "$TMP" | sort -u | head -30
echo
echo "== Tail =="
tail -c "$MAX_BYTES" "$TMP"
echo
REASON=$(grep -E '[0-9]+ (failed|error)' "$TMP" | tail -1)
echo "FAIL: ${REASON:-pytest exit $EXIT}"
exit 1
