#!/usr/bin/env bash
# agent-tools/e2e.sh — обёртка над pytest-харнесом (test/) со сжатым выводом.
#
# Использование:
#   scripts/agent-tools/e2e.sh                 # весь набор
#   scripts/agent-tools/e2e.sh --smoke         # только -m smoke
#   scripts/agent-tools/e2e.sh --format=detailed
#
# Поднимает стек через test/stack/up.sh (нужен Docker + ./.env.local с
# STIT_OBS_TOKEN+OTEL_TENANT), гоняет pytest, сносит стек. venv в test/.venv
# создаётся/обновляется из test/requirements.txt автоматически.
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
        -h|--help) sed -n '2,15p' "$0" | sed 's/^# \{0,1\}//'; exit 0;;
        *) echo "FAIL: unknown arg: $arg" >&2; exit 1;;
    esac
done

cd "$(git rev-parse --show-toplevel 2>/dev/null || pwd)"

VENV="test/.venv"
if [ ! -x "$VENV/bin/pytest" ]; then
    echo "== bootstrap $VENV =="
    if ! python3 -m venv "$VENV" 2>/dev/null; then
        echo "FAIL: python3 -m venv не сработал (нужен пакет python3-venv)." >&2
        exit 1
    fi
    if ! "$VENV/bin/pip" -q install -r test/requirements.txt; then
        echo "FAIL: pip install -r test/requirements.txt" >&2
        exit 1
    fi
fi

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
