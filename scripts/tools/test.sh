#!/usr/bin/env bash
# tools/test.sh — обёртка над `go test` со сжатым выводом.
#
# Использование:
#   scripts/tools/test.sh [pkg…]
#   scripts/tools/test.sh --format=detailed [pkg…]
#
# По умолчанию: пакеты, которые поменялись (через `git diff`). Если ничего не изменилось,
# гонит `./...`. На зелёном — одна строка PASS-сводки. На красном — только падающие
# тесты + первые 25k символов их вывода.
#
# Канонический status-line в конце: `OK` / `FAIL: <reason>`.

set -uo pipefail

FORMAT="concise"
PKGS=()
MAX_BYTES=25000

for arg in "$@"; do
    case "$arg" in
        --format=*) FORMAT="${arg#--format=}";;
        --max-bytes=*) MAX_BYTES="${arg#--max-bytes=}";;
        -h|--help)
            sed -n '2,11p' "$0" | sed 's/^# \{0,1\}//'
            exit 0;;
        *) PKGS+=("$arg");;
    esac
done

cd "$(git rev-parse --show-toplevel 2>/dev/null || pwd)"

if [ ${#PKGS[@]} -eq 0 ]; then
    # Если pkg не указан — пакеты из git diff. Иначе ./...
    CHANGED=$(git diff --name-only HEAD 2>/dev/null | grep '\.go$' | xargs -r dirname | sort -u | sed 's|^|./|' || true)
    if [ -n "$CHANGED" ]; then
        # shellcheck disable=SC2206
        PKGS=($CHANGED)
    else
        PKGS=("./...")
    fi
fi

TMP=$(mktemp)
trap 'rm -f "$TMP"' EXIT

# -json формат: один JSON-объект на событие. Парсим в bash через grep+sed.
# -race только при CGO_ENABLED=1 (иначе race detector недоступен — CI/pre-push fallback).
RACE=""
[ "$(go env CGO_ENABLED)" = "1" ] && RACE="-race"
go test -short $RACE -json "${PKGS[@]}" >"$TMP" 2>&1
EXIT=$?

# go test -json не гарантирует порядок ключей (Action может идти до Package/Test).
# Фильтруем построчно: событие теста = строка c обоими полями.
PASS=$(grep '"Action":"pass"' "$TMP" | grep -c '"Test":"' || true)
FAIL_TESTS=$(grep '"Action":"fail"' "$TMP" | grep '"Test":"' | grep -oE '"Test":"[^"]*"' | sed 's/"Test":"\(.*\)"/\1/' | sort -u || true)
FAIL_COUNT=$(echo -n "$FAIL_TESTS" | grep -c . || true)
PKGCNT=$(grep -oE '"Package":"[^"]*"' "$TMP" | sort -u | grep -c . || true)
BUILD_FAIL=$(grep -E '"Action":"(fail|output)"' "$TMP" | grep -oE 'FAIL\s+[a-zA-Z0-9_./]+\s+\[build failed\]' | sort -u || true)

if [ "$EXIT" -eq 0 ] && [ "$FAIL_COUNT" -eq 0 ]; then
    echo "PASS: $PASS тестов, $PKGCNT пакет(ов)"
    echo "OK"
    exit 0
fi

echo "FAIL: $FAIL_COUNT падающих тест(ов), $PASS прошло"
echo

if [ -n "$BUILD_FAIL" ]; then
    echo "=== Build failures ==="
    echo "$BUILD_FAIL"
    echo
fi

if [ "$FORMAT" = "concise" ]; then
    # Только заголовки падающих тестов.
    echo "=== Падающие тесты ==="
    echo "$FAIL_TESTS"
    echo
    echo "Полный вывод: scripts/tools/test.sh --format=detailed [pkg]"
else
    # detailed: вывод падающих тестов с ограничением по байтам.
    echo "=== Вывод падающих тестов ==="
    OUTPUT=$(grep '"Action":"output"' "$TMP" | grep -E "$(echo "$FAIL_TESTS" | paste -sd '|' -)" | grep -oE '"Output":"[^"]*"' | sed 's/"Output":"\(.*\)"/\1/' | sed 's/\\n/\n/g; s/\\t/\t/g' | head -c "$MAX_BYTES" || true)
    echo "$OUTPUT"
    BYTES=$(echo -n "$OUTPUT" | wc -c)
    if [ "$BYTES" -ge "$MAX_BYTES" ]; then
        echo
        echo "... (вывод обрезан до ${MAX_BYTES} байт; --max-bytes=N для увеличения)"
    fi
fi

echo
echo "FAIL: тесты не прошли"
exit 1
