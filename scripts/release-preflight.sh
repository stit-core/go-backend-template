#!/usr/bin/env bash
# Pre-flight gate для `make release`, две фазы:
#   release-preflight.sh env         до гейта и тега — окружение релиза: ветка main, чистое дерево, ключи
#                                    ssh-agent читают приватные модули go.mod (ими их берёт сборка образа),
#                                    docker buildx, make doctor (docker-auth/sqlc-drift/непарные миграции)
#   release-preflight.sh tag vX.Y.Z  после тега — на HEAD annotated тег формата vX.Y.Z, и он — переданный
# Output: внятное сообщение на каждый fail, exit 2 (отличаем от "сбоя shell"=1).
set -euo pipefail
cd "$(dirname "$0")/.."

fail() { echo "release-preflight: $*" >&2; exit 2; }

# Сборка образа берёт приватные модули go.mod (хосты GOPRIVATE из Dockerfile) по SSH ключом ssh-agent:
# docker buildx build --ssh default. Проверка — как увидит сборка: только агент, без ~/.ssh/config и
# файлов ключей.
check_private_modules() {
    local hosts host repos repo out
    ssh-add -l >/dev/null 2>&1 ||
        fail "в ssh-agent нет ключей (SSH_AUTH_SOCK) — сборка образа берёт приватные модули по SSH ключом агента: ssh-add <ключ с доступом к git-хосту модулей>"
    hosts=$(sed -nE 's/^ENV GOPRIVATE=//p' Dockerfile | tr ',' '\n' | sed -nE 's#^([^/*]+)/\*$#\1#p')
    for host in $hosts; do
        repos=$(grep -oE "${host//./\\.}/[^[:space:]]+[[:space:]]+v[0-9]" go.mod |
            sed -E "s#^${host//./\\.}/([^/[:space:]]+/[^/[:space:]]+).*#\1#" | sort -u)
        for repo in $repos; do
            out=$(GIT_SSH_COMMAND='ssh -F /dev/null -o BatchMode=yes -o IdentityFile=none -o ConnectTimeout=15' \
                git ls-remote "git@$host:$repo.git" HEAD 2>&1) ||
                fail "git@$host:$repo.git не читается ключами ssh-agent — сборка образа не скачает модуль: $(printf '%s' "$out" | head -n 1)"
        done
    done
}

case "${1:-}" in
    env)
        branch=$(git rev-parse --abbrev-ref HEAD 2>/dev/null) || fail "не git-репо"
        [ "$branch" = "main" ] || fail "branch=$branch, релиз только с main"
        [ -z "$(git status --porcelain)" ] || fail "working tree dirty — закоммить или stash"
        check_private_modules
        docker buildx version >/dev/null 2>&1 || fail "docker buildx не доступен"
        echo "release-preflight: → doctor"
        bash "$(dirname "$0")/tools/doctor.sh" || fail "doctor preflight не прошёл (см. вывод выше)"
        echo "release-preflight: OK (окружение)"
        ;;
    tag)
        tag_arg="${2:-}"
        [ -n "$tag_arg" ] || fail "usage: release-preflight.sh tag vX.Y.Z"
        tag=$(git describe --tags --exact-match --exclude 'template/*' HEAD 2>/dev/null || true)
        [ -n "$tag" ] || fail "на HEAD нет тега — поставь annotated tag: git tag -a vX.Y.Z -m '<changelog>'"
        [ "$tag_arg" = "$tag" ] || fail "переданный TAG=$tag_arg ≠ тег на HEAD ($tag)"
        [[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] ||
            fail "tag=$tag не соответствует формату vX.Y.Z (semver)"
        kind=$(git cat-file -t "$tag" 2>/dev/null || echo "?")
        [ "$kind" = "tag" ] || fail "tag=$tag не annotated (kind=$kind) — пересоздай через git tag -a"
        echo "release-preflight: OK (tag=$tag)"
        ;;
    *)
        fail "usage: release-preflight.sh env | tag vX.Y.Z"
        ;;
esac
