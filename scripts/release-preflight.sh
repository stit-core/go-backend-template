#!/usr/bin/env bash
# Pre-flight gate для `make release`. Проверяет:
#   - ветка == main
#   - working tree clean
#   - на HEAD стоит annotated tag формата vX.Y.Z
#   - tag (если передан как $1) совпадает с тем, что на HEAD
#   - docker buildx доступен
#   - make doctor (docker-auth/sqlc-drift/непарные миграции) — fail-fast
# Output: внятное сообщение на каждый fail, exit 2 (отличаем от "сбоя shell"=1).
set -euo pipefail
cd "$(dirname "$0")/.."

fail() { echo "release-preflight: $*" >&2; exit 2; }

branch=$(git rev-parse --abbrev-ref HEAD 2>/dev/null) || fail "не git-репо"
[ "$branch" = "main" ] || fail "branch=$branch, релиз только с main"

[ -z "$(git status --porcelain)" ] || fail "working tree dirty — закоммить или stash"

tag_arg="${1:-}"
detected=$(git describe --tags --exact-match HEAD 2>/dev/null || true)
[ -n "$detected" ] || fail "на HEAD нет тега — поставь annotated tag: git tag -a vX.Y.Z -m '<changelog>'"

if [ -n "$tag_arg" ] && [ "$tag_arg" != "$detected" ]; then
  fail "переданный TAG=$tag_arg ≠ тег на HEAD ($detected)"
fi
tag="$detected"

[[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] \
  || fail "tag=$tag не соответствует формату vX.Y.Z (semver)"

kind=$(git cat-file -t "$tag" 2>/dev/null || echo "?")
[ "$kind" = "tag" ] || fail "tag=$tag не annotated (kind=$kind) — пересоздай через git tag -a"

docker buildx version >/dev/null 2>&1 || fail "docker buildx не доступен"

echo "release-preflight: → doctor"
bash "$(dirname "$0")/tools/doctor.sh" || fail "doctor preflight не прошёл (см. вывод выше)"

echo "release-preflight: OK (tag=$tag)"
