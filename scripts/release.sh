#!/usr/bin/env bash
# scripts/release.sh — релиз одной командой: confirm → release-preflight env (ветка/clean/доступ сборки к
# приватным модулям/buildx + doctor) → чистое дерево + make check + e2e-smoke → govulncheck → annotated
# tag → release-preflight tag (annotated semver на HEAD) → buildx api+migrate push lockstep →
# агрегированный INFRA-HANDOFF → STOP.
#
#   make release VERSION=vX.Y.Z
#   make release VERSION=vX.Y.Z DRY_RUN=1   только handoff: без проверок, тега и образов
#   release.sh --image                      путь образа из origin для make doctor; пусто — не выводится
#
# Handoff — накопитель .claude/plans/handoff.md и разделы `## Handoff` активных планов; после
# выкатки образов накопитель очищается коммитом (отданное остаётся в дереве тега).
#
# Зона ответственности этого репо: код → image в registry. На STOP — всё.
# Деплой (make bump SVC=<репо> TAG=… — <РЕПО>_TAG в .env VPS) — в <client-org>/infra, НЕ здесь.
#
# Неинтерактивный запуск (нет TTY / EOF на prompt) → отмена (exit 1):
# accidental release невозможен. Неинтерактивно, с явным согласием: RELEASE_YES=1.
set -euo pipefail
cd "$(dirname "$0")/.."

# Образ — <host>/<org>/<репо> из origin: у сервисов одной орги свои пути. Userinfo origin (токен https) в
# вывод не идёт. Порт https-origin — порт registry Gitea, порт ssh к registry не относится.
ORIGIN=$(git remote get-url origin 2>/dev/null | sed -E 's#^([A-Za-z][A-Za-z0-9+.-]*://)[^@/]*@#\1#; s#/$##' || true)
IMAGE=$(printf '%s\n' "${ORIGIN%.git}" | tr '[:upper:]' '[:lower:]' |
    sed -nE -e 's#^https?://([^:/]+(:[0-9]+)?)/([^/]+)/([^/]+)$#\1/\3/\4#p' -e t \
        -e 's#^([a-z]+://)?([^@/]+@)?([^:/]+)(:[0-9]*)?[:/]([^/]+)/([^/]+)$#\3/\5/\6#p')
[ "${1:-}" != --image ] || { echo "$IMAGE"; exit 0; }

VERSION="${1:-}"
[ -n "$VERSION" ] || { echo "release: нужен VERSION. Usage: make release VERSION=vX.Y.Z" >&2; exit 2; }
[[ "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] \
    || { echo "release: VERSION=$VERSION не semver vX.Y.Z (lightweight/:latest/:edge запрещены)" >&2; exit 2; }

BR=$(git rev-parse --abbrev-ref HEAD)
SHA=$(git rev-parse --short HEAD)
[ -n "$IMAGE" ] || {
    echo "release: путь образа берётся из origin — <host>:<org>/<репо> или https://<host>/<org>/<репо>, а origin: ${ORIGIN:-нет}" >&2
    exit 2
}
# Хаб берёт образ по имени сервиса (${PROJECT_NAME}/<svc>), тег — из <SVC>_TAG, который строит make bump:
# верхний регистр, «-» → «_». У форка <org>/backend это backend и BACKEND_TAG.
SVC=${IMAGE##*/}
TAG_VAR=$(printf '%s' "$SVC" | tr '[:lower:]-' '[:upper:]_')_TAG
[[ $TAG_VAR =~ ^[[:upper:]_][[:upper:][:digit:]_]*$ ]] || {
    echo "release: make bump ищет тег в $TAG_VAR — это не имя переменной окружения, хаб не выкатит $SVC: имя репо — с буквы, из латиницы, цифр, «-» и «_»" >&2
    exit 2
}
HANDOFF=.claude/plans/handoff.md

# Агрегированный INFRA-HANDOFF: тег сервиса и накопленные infra-изменения.
print_handoff() {
    local impacts
    impacts=$(bash scripts/tools/validate-plan.sh --handoff)
    echo
    echo "=== INFRA HANDOFF · передай инфра-админу out-of-band ==="
    echo "$TAG_VAR → $VERSION   ($IMAGE{,-migrate}: api + migrate, lockstep)"
    echo
    if [ -n "$impacts" ]; then
        echo "Накопленный handoff ($HANDOFF и раздел ## Handoff активных планов):"
        echo "$impacts"
    else
        echo "(накопитель $HANDOFF пуст — убедись, что всё инфра-затрагивающее"
        echo " зафиксировано: .claude/rules/infra-impact.md / skill infra-handoff)"
    fi
    echo
    echo "Действия инфра-стороны:"
    echo "  проект:  в <client-org>/infra — make bump SVC=$SVC TAG=$VERSION"
    echo "           (правит $TAG_VAR в .env на VPS + force-recreate)"
}

if [ "${DRY_RUN:-}" = "1" ]; then
    echo "release: DRY_RUN=1 — без проверок, тега и образов; handoff, который уйдёт с $VERSION:"
    print_handoff
    exit 0
fi

# --- confirm (видимое вне репо действие — explicit OK обязателен) ---
if [ "${RELEASE_YES:-}" = "1" ]; then
    echo "release: RELEASE_YES=1 — без вопроса: $IMAGE{,-migrate}:$VERSION"
else
    printf "⚠ confirm release %s from %s@%s → %s{,-migrate}:%s? [y/N] " "$VERSION" "$BR" "$SHA" "$IMAGE" "$VERSION"
    read -r ans 2>/dev/null || ans=""
    case "$ans" in
        y | Y) ;;
        *) echo "release: отменено (нет подтверждения)"; exit 1 ;;
    esac
fi

# --- окружение релиза до гейта и тега: без ветки main, registry-логина или ключа ssh-agent с доступом к
# приватным модулям релиз останавливается сразу и тега не оставляет ---
bash scripts/release-preflight.sh env

# --- гейт до тега: тег и образы — только у чистого HEAD, прошедшего make check, smoke и govulncheck ---
[ -z "$(git status --porcelain)" ] || { echo "release: working tree dirty — закоммить или stash" >&2; exit 2; }
# -f Makefile: свой GNUmakefile make прочёл бы первым, а игнорируемый git оставляет дерево «чистым».
make -f Makefile --no-print-directory check
# Собранный бинарь со стеком в docker (HTTP, Postgres, NATS): то, что юнит и integration не видят.
make -f Makefile --no-print-directory e2e-smoke
# Уязвимость, которую вызывает код, не уезжает в образ; базе уязвимостей нужна сеть — поэтому здесь, а не
# в make check, и до тега: отказ не оставляет тега.
command -v govulncheck >/dev/null 2>&1 || { echo "release: govulncheck не найден — make setup" >&2; exit 2; }
govulncheck ./... || { echo "release: govulncheck — код вызывает уязвимость (вывод выше): обнови зависимость или Go" >&2; exit 2; }

# --- annotated tag, если на HEAD ещё нет ---
# Теги обвязки template/vX.Y.Z — не версии образа (docs/contributing.md).
existing=$(git describe --tags --exact-match --exclude 'template/*' HEAD 2>/dev/null || true)
if [ -z "$existing" ]; then
    git tag -a "$VERSION" -m "release $VERSION"
    echo "release: создан annotated tag $VERSION"
elif [ "$existing" != "$VERSION" ]; then
    echo "release: на HEAD уже тег $existing ≠ запрошенному $VERSION" >&2
    exit 2
fi

# --- тег на HEAD — annotated semver, и он — запрошенный ---
bash scripts/release-preflight.sh tag "$VERSION"

# --- build + push lockstep (api + migrate одним тегом); приватные модули go mod download берёт по SSH
# ключом ssh-agent: в слои образа ключ не попадает ---
docker buildx build --platform linux/amd64 --push --ssh default \
    --build-arg TARGET=api \
    --build-arg VERSION="$VERSION" \
    --build-arg GIT_SHA="$SHA" \
    -t "$IMAGE:$VERSION" .

docker buildx build --platform linux/amd64 --push --ssh default -f Dockerfile.migrate \
    --build-arg VERSION="$VERSION" \
    --build-arg GIT_SHA="$SHA" \
    -t "$IMAGE-migrate:$VERSION" .

echo "✓ released $IMAGE{,-migrate}:$VERSION"

# --- агрегированный INFRA-HANDOFF; накопитель пустеет: отданное остаётся в дереве тега ---
print_handoff
if grep -qvE '^(# Handoff|_пусто_|[[:space:]]*)$' "$HANDOFF" 2>/dev/null; then
    printf '# Handoff\n\n_пусто_\n' >"$HANDOFF"
    if git commit -q -m "meta(handoff): передан с релизом $VERSION" -- "$HANDOFF"; then
        echo
        echo "release: накопитель очищен коммитом; отданное — git show $VERSION:$HANDOFF"
    else
        echo "release: закоммить очистку $HANDOFF вручную — иначе handoff уйдёт со следующим релизом повторно" >&2
    fi
fi

echo
echo "release: тег и коммит очистки накопителя — локальные: git push origin main $VERSION (pre-push гонит гейт)"
echo "=== STOP. Зона этого репо закрыта (image в registry). Деплой — не здесь. ==="
