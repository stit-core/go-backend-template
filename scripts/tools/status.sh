#!/usr/bin/env bash
# tools/status.sh — always-on cockpit feed для smartdev.
#
# Использование:
#   scripts/tools/status.sh          # быстрая сводка + критичный детект (~2s)
#   scripts/tools/status.sh --full   # + golangci-lint count (тяжёлое, ~10s)
#
# Критичный детект (build/sqlc-drift/непарные миграции/draft/infra-touch)
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
echo "== Infra-touch =="
INFRA_RE='(^|/)\.env\.example|^internal/platform/config/|^migrations/|/events\.go$|/delivery/nats/|^cmd/api/|^go\.mod$'
INFRA_HITS=$(git status --porcelain 2>/dev/null | awk '{print $2}' | grep -E "$INFRA_RE" || true)
if [ -n "$INFRA_HITS" ]; then
    echo "незакоммиченные изменения задевают инфра-поверхность:"
    echo "$INFRA_HITS" | sed 's|^|  |'
    echo "→ оформи INFRA-HANDOFF (skill infra-handoff / .claude/rules/infra-impact.md)"
    ATTN="$ATTN infra-touch"
else
    echo "none"
fi
echo

# === Plan === (единый roadmap: focus + по секции Active — статус и следующий открытый атом)
echo "== Plan =="
if [ -f .claude/plans/roadmap.md ]; then
    foc=$(grep -m1 '^\*\*Current focus:\*\*' .claude/plans/roadmap.md | sed -E 's/\*\*Current focus:\*\*[[:space:]]*//')
    [ -n "$foc" ] && echo "  focus: $foc"
    awk '
      function flush(){ if(slug!="") printf "  %s: %s | next: %s\n", slug, st, (nx==""?"—":nx) }
      /^## Active/ { inact=1; next }
      /^## /       { flush(); inact=0; slug=""; next }
      !inact       { next }
      /^### /      { flush(); slug=$0; sub(/^### /,"",slug); sub(/ .*/,"",slug); st="?"; nx=""; next }
      slug=="" { next }
      /\*\*Статус:\*\*/ { s=$0; sub(/.*\*\*Статус:\*\*[[:space:]]*/,"",s); sub(/[[:space:]].*$/,"",s); st=s }
      /^- \[ \] \*\*A[0-9]/ && nx=="" { n=$0; sub(/^- \[ \] \*\*/,"",n); sub(/:.*/,"",n); nx=n }
      END { flush() }
    ' .claude/plans/roadmap.md
    grep -q '^### ' .claude/plans/roadmap.md || echo "  (no active plans)"
else
    echo "  (no roadmap)"
fi
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
    if command -v golangci-lint >/dev/null 2>&1; then
        ISSUES=$(golangci-lint run --output.json.path=/dev/stdout ./... 2>/dev/null \
            | grep -oE '"FromLinter":' | wc -l | tr -d ' ')
        echo "issues: $ISSUES"
    else
        echo "(golangci-lint не установлен — make setup)"
    fi
    echo
fi

if [ -n "$ATTN" ]; then
    echo "ATTENTION:$ATTN"
else
    echo "OK"
fi
