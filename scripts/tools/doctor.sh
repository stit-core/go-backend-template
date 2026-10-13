#!/usr/bin/env bash
# tools/doctor.sh — fail-fast preflight здоровья репо/окружения.
#
# Использование:
#   scripts/tools/doctor.sh            # все проверки
#   scripts/tools/doctor.sh --hooks    # только git-хуки: пусто — подключены, иначе причина (make status)
#
# Read-only: ничего не мутирует (mocks-drift — через make check/gen-check).
# FAIL-класс (exit 2): docker-демон, docker-registry auth, sqlc-drift, непарные миграции, git-хуки.
# WARN-класс (exit 0): .env.local ключи, draft-артефакты, новый тег шаблона (remote template: сеть, без fetch).
# Канонический status-line в конце: `OK: ...` / `FAIL: N blocking`.

set -uo pipefail
cd "$(git rev-parse --show-toplevel 2>/dev/null || pwd)"

# hooks_problem — пусто, когда git запускает хуки этого дерева (make setup): каждый хук scripts/git-hooks
# есть в каталоге хуков git, исполняемый и тот же; иначе — что не так.
hooks_problem() {
    local dir f h
    dir=$(git rev-parse --git-path hooks 2>/dev/null) || { echo "не git-репо"; return; }
    for f in scripts/git-hooks/*; do
        [ -f "$f" ] || continue
        h=${f##*/}
        [ -x "$dir/$h" ] || { echo "$dir: нет исполняемого $h"; return; }
        cmp -s "$dir/$h" "$f" || { echo "$dir/$h — не хук этого дерева ($f)"; return; }
    done
}
if [ "${1:-}" = "--hooks" ]; then
    hooks_problem
    exit 0
fi

FAIL=0
WARN=0
ok()   { echo "OK   : $*"; }
warn() { echo "WARN : $*"; WARN=$((WARN + 1)); }
fail() { echo "FAIL : $*"; FAIL=$((FAIL + 1)); }
info() { echo "INFO : $*"; }

# 1. docker-демон — make check гоняет integration-тесты (testcontainers).
if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
    ok "docker: демон доступен"
else
    fail "docker: демон недоступен — make check гоняет integration-тесты (testcontainers)"
fi

# 2. docker-registry auth — частая боль: make release падает на unauthorized
#    в середине пуша. Проверяем ПРИСУТСТВИЕ кред, не валидность токена
#    (валидность — только сетевым запросом; здесь fail-fast по факту логина).
HOST=$(git remote get-url origin 2>/dev/null | sed -E 's#^(git@|https?://)([^:/]+)[:/].*#\2#')
if [ -z "$HOST" ]; then
    warn "docker: не определить registry host из git remote"
elif ! command -v docker >/dev/null 2>&1; then
    warn "docker: бинарь не найден (нужен для make release)"
else
    CFG="${DOCKER_CONFIG:-$HOME/.docker}/config.json"
    if [ -f "$CFG" ] && grep -q "\"$HOST\"" "$CFG" 2>/dev/null; then
        ok "docker auth: креды для $HOST присутствуют"
    elif [ -f "$CFG" ] && grep -qE '"(credsStore|credHelpers)"' "$CFG" 2>/dev/null; then
        ok "docker auth: настроен credential helper (проверь покрытие $HOST)"
    else
        fail "docker auth: нет логина к $HOST — перед make release: docker login $HOST"
    fi
fi

# 3. .env.local ключи (имя ключа, без значения — см. .claude/rules/env-local.md)
if [ -f .env.local ]; then
    for k in E2E_ADMIN_EMAIL E2E_ADMIN_PASSWORD; do
        if grep -qE "^(export +)?$k=" .env.local; then
            ok ".env.local: $k задан"
        else
            warn ".env.local: нет $k (опц. для make e2e; иначе дефолты — см. .env.local.example)"
        fi
    done
else
    info ".env.local отсутствует (опционален для make e2e — есть дефолты seed-admin)"
fi

# 4. sqlc drift — read-only (sqlc diff не пишет файлы). mocks-drift мутирует
#    дерево регеном, поэтому здесь не проверяем — это делает make check.
if command -v sqlc >/dev/null 2>&1; then
    if sqlc diff >/dev/null 2>&1; then
        ok "sqlc: в синхроне"
    else
        fail "sqlc: DRIFT — запусти make gen и закоммить"
    fi
else
    warn "sqlc: не установлен (make setup) — drift не проверен"
fi
info "mocks-drift проверяется make check/gen-check (doctor read-only)"

# 5. непарные up/down миграции
if [ -d migrations ]; then
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
        ok "migrations: все up/down парные"
    else
        fail "migrations: непарные —$unpaired"
    fi
fi

# 6. draft-артефакты stack (companion README/_templates исключены).
# Домены живут в соседнем strategy-репо (../strategy); их draft-статус — забота того репо.
if [ -d docs/stack ]; then
    sdrafts=$(grep -rl '^status: draft' docs/stack --include='*.md' 2>/dev/null \
        | grep -vE '/_templates/|/README\.md$' \
        | sed 's#docs/stack/##' | tr '\n' ' ')
    if [ -z "$sdrafts" ]; then
        ok "stack-artifacts: нет status: draft"
    else
        warn "stack-artifacts: draft —$sdrafts (углуби через /stack-deep-dive)"
    fi
fi

# 7. git-хуки — CI нет: commit-msg, pre-commit и pre-push — единственная проверка изменения до remote.
hooks=$(hooks_problem)
if [ -z "$hooks" ]; then
    ok "git hooks: подключены (scripts/git-hooks)"
else
    fail "git hooks: не подключены — make setup ($hooks)"
fi

# 8. обвязка шаблона — новый тег у remote template (git ls-remote); сеть — не ошибка.
tpl=$(bash scripts/tools/template-update.sh --remote 2>&1)
case $? in
    0) ok "template: $tpl" ;;
    1) warn "template: $tpl" ;;
    *) info "template: $tpl" ;;
esac

echo
if [ "$FAIL" -gt 0 ]; then
    echo "FAIL: $FAIL blocking, $WARN warn"
    exit 2
fi
echo "OK: doctor clean ($WARN warn)"
exit 0
