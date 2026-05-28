#!/usr/bin/env bash
# tools/doctor.sh — fail-fast preflight здоровья репо/окружения.
#
# Использование:
#   scripts/tools/doctor.sh            # все проверки
#
# Read-only: ничего не мутирует (mocks-drift — через make check/gen-check).
# FAIL-класс (exit 2): docker-registry auth, sqlc-drift, непарные миграции.
# WARN-класс (exit 0): .env.local ключи, draft-артефакты.
# Канонический status-line в конце: `OK: ...` / `FAIL: N blocking`.

set -uo pipefail
cd "$(git rev-parse --show-toplevel 2>/dev/null || pwd)"

FAIL=0
WARN=0
ok()   { echo "OK   : $*"; }
warn() { echo "WARN : $*"; WARN=$((WARN + 1)); }
fail() { echo "FAIL : $*"; FAIL=$((FAIL + 1)); }
info() { echo "INFO : $*"; }

# 1. docker-registry auth — частая боль: make release падает на unauthorized
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

# 2. .env.local ключи (имя ключа, без значения — см. .claude/rules/env-local.md)
if [ -f .env.local ]; then
    for k in STIT_OBS_TOKEN OTEL_TENANT E2E_ADMIN_EMAIL E2E_ADMIN_PASSWORD; do
        if grep -qE "^(export +)?$k=" .env.local; then
            ok ".env.local: $k задан"
        else
            warn ".env.local: нет $k (нужен для make logs / make e2e — см. .env.local.example)"
        fi
    done
else
    info ".env.local отсутствует (нужен только для make logs / make e2e)"
fi

# 3. sqlc drift — read-only (sqlc diff не пишет файлы). mocks-drift мутирует
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

# 4. непарные up/down миграции
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

# 5. draft-артефакты DDD (не release-блокер, но видимый сигнал)
if [ -d docs/domains ]; then
    # _templates/ несёт status: draft как плейсхолдер; README/MAPPING — не артефакты.
    drafts=$(grep -rl '^status: draft' docs/domains --include='*.md' 2>/dev/null \
        | grep -vE '/_templates/|/(README|MAPPING)\.md$' \
        | sed 's#docs/domains/##' | tr '\n' ' ')
    if [ -z "$drafts" ]; then
        ok "domain-artifacts: нет status: draft"
    else
        warn "domain-artifacts: draft —$drafts (углуби через /domain-deep-dive)"
    fi
fi

# 6. draft-артефакты stack (companion README/_templates исключены)
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

echo
if [ "$FAIL" -gt 0 ]; then
    echo "FAIL: $FAIL blocking, $WARN warn"
    exit 2
fi
echo "OK: doctor clean ($WARN warn)"
exit 0
