#!/usr/bin/env bash
# scripts/release.sh — релиз одной командой: confirm → чистое дерево + make check →
# annotated tag → release-preflight (ветка/clean/semver/buildx + doctor) → buildx
# api+migrate push lockstep → агрегированный INFRA-HANDOFF → STOP.
#
#   make release VERSION=vX.Y.Z
#
# Зона ответственности этого репо: код → image в registry. На STOP — всё.
# Деплой (make bump SVC=backend TAG=… — BACKEND_TAG в .env VPS) — в <client-org>/infra, НЕ здесь.
#
# Неинтерактивный запуск (нет TTY / EOF на prompt) → отмена (exit 1):
# accidental release невозможен. Для CI-автоматизации: RELEASE_YES=1.
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION="${1:-}"
[ -n "$VERSION" ] || { echo "release: нужен VERSION. Usage: make release VERSION=vX.Y.Z" >&2; exit 2; }
[[ "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] \
    || { echo "release: VERSION=$VERSION не semver vX.Y.Z (lightweight/:latest/:edge запрещены)" >&2; exit 2; }

BR=$(git rev-parse --abbrev-ref HEAD)
SHA=$(git rev-parse --short HEAD)
HOST_ORG=$(git remote get-url origin | sed -E 's#^(git@|https?://)([^:/]+)[:/]([^/]+)/.*#\2/\3#')
IMAGE_BASE="$HOST_ORG/backend"

# --- confirm (видимое вне репо действие — explicit OK обязателен) ---
if [ "${RELEASE_YES:-}" = "1" ]; then
    echo "release: RELEASE_YES=1 — пропускаю prompt"
else
    printf "⚠ confirm release %s from %s@%s? [y/N] " "$VERSION" "$BR" "$SHA"
    read -r ans 2>/dev/null || ans=""
    case "$ans" in
        y | Y) ;;
        *) echo "release: отменено (нет подтверждения)"; exit 1 ;;
    esac
fi

# --- гейт до тега: тег и образы — только у чистого HEAD, прошедшего make check ---
[ -z "$(git status --porcelain)" ] || { echo "release: working tree dirty — закоммить или stash" >&2; exit 2; }
make --no-print-directory check

# --- annotated tag, если на HEAD ещё нет ---
existing=$(git describe --tags --exact-match HEAD 2>/dev/null || true)
if [ -z "$existing" ]; then
    git tag -a "$VERSION" -m "release $VERSION"
    echo "release: создан annotated tag $VERSION (push: git push origin $VERSION)"
elif [ "$existing" != "$VERSION" ]; then
    echo "release: на HEAD уже тег $existing ≠ запрошенному $VERSION" >&2
    exit 2
fi

# --- preflight (ветка=main / clean / annotated semver / buildx / doctor) ---
bash scripts/release-preflight.sh "$VERSION"

# --- build + push lockstep (api + migrate одним тегом) ---
docker buildx build --platform linux/amd64 --push \
    --build-arg TARGET=api \
    --build-arg VERSION="$VERSION" \
    --build-arg GIT_SHA="$SHA" \
    -t "$IMAGE_BASE:$VERSION" .

docker buildx build --platform linux/amd64 --push -f Dockerfile.migrate \
    --build-arg VERSION="$VERSION" \
    --build-arg GIT_SHA="$SHA" \
    -t "$IMAGE_BASE-migrate:$VERSION" .

echo "✓ released $IMAGE_BASE{,-migrate}:$VERSION"

# --- агрегированный INFRA-HANDOFF (BACKEND_TAG + накопленные infra-изменения) ---
echo
echo "=== INFRA HANDOFF · передай инфра-админу out-of-band ==="
echo "BACKEND_TAG → $VERSION   (api + migrate, lockstep)"

# Handoff активных планов: разделы `## Handoff` в .claude/plans/*.md (validate-plan.sh --handoff).
impacts=$(bash scripts/tools/validate-plan.sh --handoff)
if [ -n "$impacts" ]; then
    echo
    echo "Накопленный handoff (раздел ## Handoff активных планов .claude/plans/):"
    echo "$impacts"
else
    echo
    echo "(в активных планах нет раздела ## Handoff — убедись, что всё инфра-затрагивающее"
    echo " зафиксировано: .claude/rules/infra-impact.md / skill infra-handoff)"
fi

cat <<EOF

Действия инфра-стороны:
  проект:  в <client-org>/infra — make bump SVC=backend TAG=$VERSION
           (правит BACKEND_TAG в .env на VPS + force-recreate)

=== STOP. Зона этого репо закрыта (image в registry). Деплой — не здесь. ===
EOF
