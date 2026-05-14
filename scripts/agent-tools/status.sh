#!/usr/bin/env bash
# agent-tools/status.sh — короткая сводка о состоянии репо для smartdev-skill'а.
#
# Использование:
#   scripts/agent-tools/status.sh             # быстрая сводка (≤2k токенов, ~1s)
#   scripts/agent-tools/status.sh --full      # + sqlc-diff + mocks-check + lint count (~10s)
#
# Оптимизирован под parsing агентом. Каждая секция начинается с `== Name ==`.

set -uo pipefail

FULL=0
for arg in "$@"; do
    case "$arg" in
        --full) FULL=1;;
        -h|--help)
            sed -n '2,9p' "$0" | sed 's/^# \{0,1\}//'
            exit 0;;
    esac
done

cd "$(git rev-parse --show-toplevel 2>/dev/null || pwd)"

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

# === Plans ===
echo "== Plans =="
if [ -f ".claude/plans/roadmap.md" ]; then
    echo "roadmap.md (first lines):"
    head -10 .claude/plans/roadmap.md | sed 's|^|  |'
else
    echo "roadmap: (none)"
fi
PLAN_FILES=$(find .claude/plans -maxdepth 1 -name '*.md' ! -name 'roadmap.md' 2>/dev/null | sort)
if [ -n "$PLAN_FILES" ]; then
    echo "active task plans:"
    echo "$PLAN_FILES" | sed 's|^|  |'
else
    echo "active task plans: (none)"
fi
echo

# === Domains ===
echo "== Domains =="
if [ -d "internal/domain" ]; then
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

# === Migrations ===
echo "== Migrations =="
LAST_MIG=$(ls migrations 2>/dev/null | grep -oE '^[0-9]+' | sort -un | tail -1)
TOTAL_UP=$(find migrations -maxdepth 1 -name '*.up.sql' 2>/dev/null | wc -l | tr -d ' ')
TOTAL_DOWN=$(find migrations -maxdepth 1 -name '*.down.sql' 2>/dev/null | wc -l | tr -d ' ')
echo "last: ${LAST_MIG:-none}, up-files: $TOTAL_UP, down-files: $TOTAL_DOWN"
if [ "$TOTAL_UP" != "$TOTAL_DOWN" ]; then
    echo "  WARN: up/down count mismatch — каждая миграция должна иметь пару"
fi
echo

# === Heavy checks (opt-in) ===
if [ "$FULL" -eq 1 ]; then
    echo "== Generated code drift =="
    if command -v sqlc >/dev/null 2>&1; then
        if sqlc diff >/dev/null 2>&1; then
            echo "sqlc-diff: OK"
        else
            echo "sqlc-diff: DRIFT — run 'make sqlc'"
        fi
    else
        echo "sqlc-diff: (sqlc not installed)"
    fi

    if command -v mockery >/dev/null 2>&1; then
        TMP_BEFORE=$(git diff --stat -- '**/mocks/*.go' 2>/dev/null)
        mockery >/dev/null 2>&1 || true
        TMP_AFTER=$(git diff --stat -- '**/mocks/*.go' 2>/dev/null)
        if [ "$TMP_BEFORE" = "$TMP_AFTER" ]; then
            echo "mocks-check: OK"
        else
            echo "mocks-check: DRIFT — run 'make mocks'"
        fi
    else
        echo "mocks-check: (mockery not installed)"
    fi
    echo

    echo "== Lint =="
    if command -v golangci-lint >/dev/null 2>&1; then
        ISSUES=$(golangci-lint run --output.json.path=/dev/stdout ./... 2>/dev/null \
            | grep -oE '"FromLinter":' | wc -l | tr -d ' ')
        echo "issues: $ISSUES"
    else
        echo "(golangci-lint not installed — run: make tools)"
    fi
    echo
fi

# === TODOs in working-tree ===
echo "== TODOs in changed files =="
CHANGED_GO=$(git diff --name-only HEAD 2>/dev/null | grep '\.go$' || true)
if [ -n "$CHANGED_GO" ]; then
    # shellcheck disable=SC2086
    grep -nE '(TODO|FIXME|XXX|HACK):' $CHANGED_GO 2>/dev/null | head -10 | sed 's|^|  |' || echo "  (none)"
else
    echo "  (no changed .go files)"
fi
echo

echo "OK"
