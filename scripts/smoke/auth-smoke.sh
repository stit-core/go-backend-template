#!/usr/bin/env bash
# scripts/smoke/auth-smoke.sh — e2e-проверка cookie-based auth-флоу на
# поднятом smoke-стеке.
#
# Предусловия:
#   1. `scripts/smoke/up.sh` отработал зелёным (api/migrate/postgres healthy).
#
# Сценарий:
#   1. POST /api/users  — регистрируем тестового юзера (через публичный register).
#   2. POST /api/auth/login — ожидаем 200 + Set-Cookie access_token/refresh_token.
#   3. GET  /api/users/me с cookie — 200 + email совпадает.
#   4. POST /api/auth/refresh с cookie — 204 + новые cookies, refresh_token rotated.
#   5. POST /api/auth/refresh со старым refresh_token cookie — 401 (reuse-detect).
#   6. GET  /api/users/me с новой cookie — 200.
#   7. POST /api/auth/logout — 204 + expired cookies.
#   8. GET  /api/users/me с очищенным jar — 401.
#
# Каноничный status-line: `OK` / `FAIL: <reason>`.

set -uo pipefail

cd "$(git rev-parse --show-toplevel 2>/dev/null || pwd)"

API="http://127.0.0.1:8080"
EMAIL="smoke-$(date +%s)@example.com"
PASSWORD="smoke-pass-1234"
NAME="Smoke Tester"
JAR="$(mktemp -t auth-smoke-jar.XXXXXX)"
JAR_REFRESH="$(mktemp -t auth-smoke-jar-refresh.XXXXXX)"
JAR_AFTER="$(mktemp -t auth-smoke-jar-after.XXXXXX)"
trap 'rm -f "$JAR" "$JAR_REFRESH" "$JAR_AFTER"' EXIT

fail() { echo "FAIL: $*" >&2; exit 1; }

# === 1. Register ===
echo "== POST /api/users (register) =="
STATUS=$(curl -sS -o /dev/null -w '%{http_code}' \
    -X POST "$API/api/users" \
    -H 'Content-Type: application/json' \
    -d "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\",\"name\":\"$NAME\"}")
if [[ "$STATUS" != "201" && "$STATUS" != "200" ]]; then
    fail "register expected 201/200, got $STATUS"
fi
echo "  user registered ($EMAIL)"

# === 2. Login ===
echo "== POST /api/auth/login =="
STATUS=$(curl -sS -o /dev/null -w '%{http_code}' \
    -X POST "$API/api/auth/login" \
    -H 'Content-Type: application/json' \
    -d "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}" \
    -c "$JAR")
if [[ "$STATUS" != "200" ]]; then
    fail "login expected 200, got $STATUS"
fi
grep -E '(access_token|refresh_token)' "$JAR" >/dev/null || fail "login did not set access_token/refresh_token cookies"
echo "  login ok (200 + cookies)"

# === 3. GET /api/users/me ===
echo "== GET /api/users/me =="
ME_RESP=$(curl -sS -w '\n%{http_code}' "$API/api/users/me" -b "$JAR")
ME_BODY="${ME_RESP%$'\n'*}"
ME_STATUS="${ME_RESP##*$'\n'}"
if [[ "$ME_STATUS" != "200" ]]; then
    fail "/api/users/me expected 200, got $ME_STATUS, body: $ME_BODY"
fi
echo "$ME_BODY" | grep -q "\"email\":\"$EMAIL\"" || fail "/api/users/me body missing email=$EMAIL: $ME_BODY"
echo "  me ok"

# === 4. Refresh (rotation) ===
echo "== POST /api/auth/refresh =="
OLD_REFRESH=$(grep -oE "refresh_token[[:space:]]+[^[:space:]]+" "$JAR" | awk '{print $2}')
[[ -n "$OLD_REFRESH" ]] || fail "no refresh_token in jar after login (cookie file: $JAR)"
STATUS=$(curl -sS -o /dev/null -w '%{http_code}' \
    -X POST "$API/api/auth/refresh" \
    -b "$JAR" -c "$JAR_REFRESH")
if [[ "$STATUS" != "204" ]]; then
    fail "refresh expected 204, got $STATUS"
fi
grep -E '(access_token|refresh_token)' "$JAR_REFRESH" >/dev/null || fail "refresh did not set new cookies"
NEW_REFRESH=$(grep -oE "refresh_token[[:space:]]+[^[:space:]]+" "$JAR_REFRESH" | awk '{print $2}')
[[ "$NEW_REFRESH" != "$OLD_REFRESH" ]] || fail "refresh did not rotate refresh_token value"
echo "  refresh ok (204 + rotated cookies)"

# === 5. Reuse-of-old-refresh должен дать 401 ===
echo "== POST /api/auth/refresh with OLD refresh cookie (replay) =="
STATUS=$(curl -sS -o /dev/null -w '%{http_code}' \
    -X POST "$API/api/auth/refresh" \
    -b "$JAR")
if [[ "$STATUS" != "401" ]]; then
    fail "refresh replay with revoked cookie expected 401, got $STATUS"
fi
echo "  replay rejected ok (401)"

# === 6. /api/users/me with refreshed jar ===
echo "== GET /api/users/me with refreshed jar =="
STATUS=$(curl -sS -o /dev/null -w '%{http_code}' "$API/api/users/me" -b "$JAR_REFRESH")
if [[ "$STATUS" != "200" ]]; then
    fail "/api/users/me after refresh expected 200, got $STATUS"
fi
echo "  me-after-refresh ok"

# === 7. Logout ===
echo "== POST /api/auth/logout =="
STATUS=$(curl -sS -o /dev/null -w '%{http_code}' \
    -X POST "$API/api/auth/logout" \
    -b "$JAR_REFRESH" -c "$JAR_AFTER")
if [[ "$STATUS" != "204" ]]; then
    fail "logout expected 204, got $STATUS"
fi
echo "  logout ok (204)"

# === 8. /api/users/me with cleared jar ===
echo "== GET /api/users/me with cleared jar =="
STATUS=$(curl -sS -o /dev/null -w '%{http_code}' "$API/api/users/me" -b "$JAR_AFTER")
if [[ "$STATUS" != "401" ]]; then
    fail "/api/users/me after logout expected 401, got $STATUS"
fi
echo "  expired-session ok ($STATUS)"

echo
echo "OK"
