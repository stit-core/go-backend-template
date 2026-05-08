#!/usr/bin/env bash
# agent-tools/lint.sh — обёртка над golangci-lint со сжатым выводом.
#
# Использование:
#   scripts/agent-tools/lint.sh [paths…]
#   scripts/agent-tools/lint.sh --format=detailed
#
# Топ замечаний (по умолчанию 20), сгруппированных по файлу, отсортированных
# по severity. На зелёном — одна строка. Status-line: `OK` / `FAIL: N issues`.

set -uo pipefail

FORMAT="concise"
TOP=20
PATHS=("./...")
PATHS_SET=0

for arg in "$@"; do
    case "$arg" in
        --format=*) FORMAT="${arg#--format=}";;
        --top=*) TOP="${arg#--top=}";;
        -h|--help)
            sed -n '2,9p' "$0" | sed 's/^# \{0,1\}//'
            exit 0;;
        *)
            if [ "$PATHS_SET" -eq 0 ]; then
                PATHS=()
                PATHS_SET=1
            fi
            PATHS+=("$arg");;
    esac
done

cd "$(git rev-parse --show-toplevel 2>/dev/null || pwd)"

if ! command -v golangci-lint >/dev/null 2>&1; then
    echo "FAIL: golangci-lint не установлен. Запусти: make tools" >&2
    echo "FAIL: golangci-lint not installed"
    exit 1
fi

TMP=$(mktemp)
trap 'rm -f "$TMP"' EXIT

# JSON-вывод: { "Issues": [{"FromLinter": "...", "Text": "...", "Severity": "...", "Pos": {"Filename": "...", "Line": N, "Column": N}}, ...] }
golangci-lint run --output.json.path "$TMP" "${PATHS[@]}" >/dev/null 2>&1
EXIT=$?

if [ ! -s "$TMP" ]; then
    echo "OK: lint чисто"
    echo "OK"
    exit 0
fi

# Считаем issues через простой grep (jq может быть не установлен).
TOTAL=$(grep -oE '"FromLinter":' "$TMP" | wc -l | tr -d ' ')

if [ "$TOTAL" -eq 0 ]; then
    echo "OK"
    exit 0
fi

echo "FAIL: $TOTAL issue(s)"
echo

# Простой парсер: вытаскиваем нужные поля построчно.
# Если есть jq — используем его (детерминированный порядок).
if command -v jq >/dev/null 2>&1; then
    LIMIT="$TOP"
    [ "$FORMAT" = "detailed" ] && LIMIT=10000
    SEVERITY_ORDER='{"error":0,"warning":1,"info":2}'
    jq -r --argjson n "$LIMIT" --argjson sev "$SEVERITY_ORDER" '
        .Issues
        | sort_by([($sev[.Severity // "warning"] // 1), .Pos.Filename, .Pos.Line])
        | .[:$n]
        | group_by(.Pos.Filename)
        | .[]
        | (.[0].Pos.Filename + ":") as $f
        | $f, (.[] | "  " + (.Pos.Line|tostring) + ":" + (.Pos.Column|tostring) + " [" + .FromLinter + "] " + .Text)
    ' "$TMP"
else
    # Fallback без jq: сырая выжимка через sed.
    grep -oE '"Pos":\{"Filename":"[^"]+","Offset":[0-9]+,"Line":[0-9]+,"Column":[0-9]+\},"FromLinter":"[^"]+","Text":"[^"]+"' "$TMP" \
        | head -n "$TOP" \
        | sed 's/.*"Filename":"\([^"]*\)".*"Line":\([0-9]*\),"Column":\([0-9]*\).*"FromLinter":"\([^"]*\)","Text":"\([^"]*\)".*/\1:\2:\3 [\4] \5/'
fi

if [ "$FORMAT" = "concise" ] && [ "$TOTAL" -gt "$TOP" ]; then
    echo
    echo "... ещё $((TOTAL - TOP)) issues. --format=detailed для полного списка."
fi

echo
echo "FAIL: $TOTAL issue(s)"
exit 1
