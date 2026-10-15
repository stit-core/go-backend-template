#!/usr/bin/env bash
# tools/status.sh — always-on cockpit feed для smartdev.
#
# Использование:
#   scripts/tools/status.sh          # быстрая сводка + критичный детект (~2s)
#   scripts/tools/status.sh --full   # + итог make lint (тяжёлое, ~10s)
#
# Критичный детект (хуки/build/sqlc-drift/непарные миграции/draft/infra-touch)
# гонится ВСЕГДА — не за --full. Каждая секция `== Name ==` для парсинга
# агентом. Финальная строка: `OK` / `ATTENTION: <что>` — сигнал cockpit'у.

set -uo pipefail

FULL=0
for arg in "$@"; do
    case "$arg" in
        --full) FULL=1 ;;
        -h | --help)
            sed -n '2,12p' "$0" | sed 's/^# \{0,1\}//'
            exit 0
            ;;
    esac
done

cd "$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
ATTN=""

# === Branch ===
BRANCH=$(git symbolic-ref --short HEAD 2>/dev/null || echo "DETACHED")
LAST_COMMIT=$(git log -1 --format='%h %s (%ar)' 2>/dev/null || echo "—")
UPSTREAM=$(git rev-parse --abbrev-ref --symbolic-full-name "@{u}" 2>/dev/null || echo "")
AHEAD_BEHIND=""
if [ -n "$UPSTREAM" ]; then
    AB=$(git rev-list --left-right --count "HEAD...$UPSTREAM" 2>/dev/null | awk '{print $1" ahead, "$2" behind"}')
    AHEAD_BEHIND=" ($AB $UPSTREAM)"
fi
echo "== Branch =="
echo "$BRANCH$AHEAD_BEHIND"
echo "last: $LAST_COMMIT"
echo

# === Working tree ===
MODIFIED=$(git diff --name-only 2>/dev/null | wc -l | tr -d ' ')
STAGED=$(git diff --cached --name-only 2>/dev/null | wc -l | tr -d ' ')
UNTRACKED=$(git ls-files --others --exclude-standard 2>/dev/null | wc -l | tr -d ' ')
echo "== Working tree =="
echo "modified: $MODIFIED, staged: $STAGED, untracked: $UNTRACKED"
if [ "$MODIFIED" -gt 0 ] || [ "$STAGED" -gt 0 ]; then
    git diff --name-only HEAD 2>/dev/null | head -20 | sed 's|^|  |'
fi
echo

# === Critical === (всегда, быстро)
echo "== Critical =="

# git-хуки — единственная проверка изменения до remote (CI нет); подключает make setup.
hooks=$(bash scripts/tools/doctor.sh --hooks)
if [ -z "$hooks" ]; then
    echo "HOOKS: OK"
else
    echo "HOOKS: не подключены — make setup ($hooks)"
    ATTN="$ATTN hooks"
fi

# build — компилируется ли дерево
if command -v go >/dev/null 2>&1 && [ -f go.mod ]; then
    if go build ./... >/dev/null 2>&1; then
        echo "BUILD: OK"
    else
        echo "BUILD: BROKEN — make build для диагноза"
        ATTN="$ATTN build"
    fi
else
    echo "BUILD: (go недоступен)"
fi

# sqlc drift — read-only (mocks-drift мутирует дерево → make check, не здесь)
if command -v sqlc >/dev/null 2>&1; then
    if sqlc diff >/dev/null 2>&1; then
        echo "SQLC: OK"
    else
        echo "SQLC: DRIFT — make gen и закоммить"
        ATTN="$ATTN sqlc-drift"
    fi
else
    echo "SQLC: (sqlc не установлен — make setup)"
fi
echo "MOCKS: drift проверяется make check/gen-check (status read-only)"

# непарные up/down миграции
if [ -d migrations ]; then
    LAST_MIG=$(ls migrations 2>/dev/null | grep -oE '^[0-9]+' | sort -un | tail -1)
    unpaired=""
    for up in migrations/*.up.sql; do
        [ -e "$up" ] || continue
        [ -f "${up%.up.sql}.down.sql" ] || unpaired="$unpaired ${up##*/}"
    done
    for dn in migrations/*.down.sql; do
        [ -e "$dn" ] || continue
        [ -f "${dn%.down.sql}.up.sql" ] || unpaired="$unpaired ${dn##*/}"
    done
    if [ -z "$unpaired" ]; then
        echo "MIGRATIONS: last=${LAST_MIG:-none}, все пары OK"
    else
        echo "MIGRATIONS: непарные —$unpaired"
        ATTN="$ATTN migrations"
    fi
fi

# stack draft-артефакты (companion README/_templates исключены).
# Домены — в соседнем strategy-репо (../strategy); их статус смотрят там.
if [ -d docs/stack ]; then
    sdrafts=$(grep -rl '^status: draft' docs/stack --include='*.md' 2>/dev/null \
        | grep -vE '/_templates/|/README\.md$' \
        | sed 's#docs/stack/##' | tr '\n' ' ')
    if [ -z "$sdrafts" ]; then
        echo "STACK-DRAFTS: none"
    else
        echo "STACK-DRAFTS:$sdrafts (/stack-deep-dive)"
    fi
fi
echo

# === Infra-touch === (незакоммиченный diff по инфра-поверхности → infra-handoff)
# Сигнал уже правила infra-impact: он ловит то, что видно по файлам без шума, решает правило. Новый домен
# (таблица, wiring в cmd/, подписчик, операция) инфре ничего не задаёт. Всегда — конфиг, go.mod, события, образ;
# .env.example — правка строки переменной, а не комментария; миграция — новая с эффектом для выкатки (маркер
# `-- infra-handoff:`, rule sqlc-and-migrations, или расширение) и правка выпущенной: она есть в последнем теге
# релиза или обвязки шаблона, предке HEAD. Тесты инфре ничего не задают.
echo "== Infra-touch =="
INFRA_RE='^internal/platform/config/|/events\.go$|^go\.mod$|^Dockerfile'
# Против HEAD — в дереве и в индексе (правка, которую вернули в дереве после git add, едет в коммит): правка,
# удаление и оба имени переименования; новые файлы — по одному.
CHANGED=$({ git diff --name-only --no-renames HEAD 2>/dev/null; git diff --cached --name-only --no-renames 2>/dev/null
    git ls-files --others --exclude-standard 2>/dev/null; } | sort -u | grep -v '_test\.go$' || true)
INFRA_HITS=$(grep -E "$INFRA_RE" <<<"$CHANGED" || true)
if grep -qx '\.env\.example' <<<"$CHANGED" && { ! git cat-file -e HEAD:.env.example 2>/dev/null ||
    { git diff -U0 HEAD -- .env.example; git diff --cached -U0 -- .env.example; } | grep -qE '^[+-][A-Z_][A-Z0-9_]*='; }; then
    INFRA_HITS+=$'\n.env.example'
fi
released=()
for match in 'v[0-9]*' 'template/v*'; do
    tag=$(git describe --tags --abbrev=0 --match "$match" HEAD 2>/dev/null) && released+=("$tag")
done
for f in $(grep -E '^migrations/.*\.sql$' <<<"$CHANGED" || true); do
    in=""
    for tag in "${released[@]}"; do
        if git cat-file -e "$tag:$f" 2>/dev/null; then in=$tag && break; fi
    done
    if [ -n "$in" ]; then
        INFRA_HITS+=$'\n'"$f (выпущена: $in)"
    elif [ -f "$f" ] && grep -qiE '^[[:space:]]*--[[:space:]]*infra-handoff:|create[[:space:]]+extension' "$f"; then
        INFRA_HITS+=$'\n'"$f"
    fi
done
INFRA_HITS=$(sed '/^$/d' <<<"$INFRA_HITS")
if [ -n "$INFRA_HITS" ]; then
    echo "незакоммиченные изменения задевают инфра-поверхность:"
    echo "$INFRA_HITS" | sed 's|^|  |'
    echo "→ оформи INFRA-HANDOFF (skill infra-handoff / .claude/rules/infra-impact.md)"
    ATTN="$ATTN infra-touch"
else
    echo "none"
fi
echo

# === Plan === (активные планы .claude/plans/: статус, прогресс, следующий шаг; бэклог)
echo "== Plan =="
bash scripts/tools/validate-plan.sh --summary
echo

# === Template === (версия обвязки и новее известный тег шаблона; теги приносит make template-update)
echo "== Template =="
bash scripts/tools/template-update.sh --status
echo

# === Domains ===
echo "== Domains =="
if [ -d internal/domain ]; then
    for d in internal/domain/*/; do
        [ -d "$d" ] || continue
        name=$(basename "$d")
        endpoints=$(grep -h 'huma.Register' "$d"delivery/http/handler.go 2>/dev/null | wc -l | tr -d ' ')
        events=$(grep -hE '^type [A-Za-z]+ struct' "$d"events.go 2>/dev/null | wc -l | tr -d ' ')
        tests=$(find "$d" -name '*_test.go' 2>/dev/null | wc -l | tr -d ' ')
        echo "  $name — endpoints: $endpoints, events: $events, test-files: $tests"
    done
else
    echo "  (no internal/domain/)"
fi
echo

# === Recent commits ===
echo "== Recent commits =="
git log --oneline -5 2>/dev/null | sed 's|^|  |'
echo

# === TODOs in changed files ===
echo "== TODOs in changed files =="
CHANGED_GO=$(git diff --name-only HEAD 2>/dev/null | grep '\.go$' || true)
if [ -n "$CHANGED_GO" ]; then
    # shellcheck disable=SC2086
    grep -nE '(TODO|FIXME|XXX|HACK):' $CHANGED_GO 2>/dev/null | head -10 | sed 's|^|  |' || echo "  (none)"
else
    echo "  (no changed .go files)"
fi
echo

# === Lint === (тяжёлое, opt-in)
if [ "$FULL" -eq 1 ]; then
    echo "== Lint =="
    if [ "$(bash scripts/tools/lint.sh | tail -1)" = "OK" ]; then
        echo "LINT: OK"
    else
        echo "LINT: FAIL — make lint для диагноза"
        ATTN="$ATTN lint"
    fi
    echo
fi

if [ -n "$ATTN" ]; then
    echo "ATTENTION:$ATTN"
else
    echo "OK"
fi
