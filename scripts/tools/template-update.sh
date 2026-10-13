#!/usr/bin/env bash
# tools/template-update.sh — обвязка шаблона в форк (skill /upgrade): изменения путей шаблона
# (.template/paths) от версии форка (.template/version) до тега шаблона — трёхсторонним слиянием git.
#
#   make template-update                      версия форка, новые теги шаблона и их аннотации
#   make template-update TO=template/vX.Y.Z   перенести изменения до тега TO
#   make template-update TO=… FROM=<коммит>   база переноса — этот коммит шаблона
#   make template-update TO=… PATHS='<пути>'  тем же переносом — и код форка по этим путям (каталог, файл
#                                             или :(exclude)<путь>): разделы «Безопасность» и «Платформа»
#   template-update.sh --status               строка версии для make status (без сети)
#   template-update.sh --remote               теги remote template для make doctor (git ls-remote): exit 0 —
#                                             новее версии форка нет, 1 — есть или remote нет, 2 — не ответил
#
# Первый перенос форка идёт от коммита шаблона, из которого он создан (дерево первого коммита форка),
# следующие — от .template/version. Теги шаблона — из remote `template` (git remote add --no-tags
# template <url шаблона>). Module path
# шаблона в Go-файлах сравнивается как module path форка. Конфликты остаются в индексе (git status —
# UU, AA); файл, которого в форке нет, и удалённый шаблоном файл, который форк менял, не трогаются
# и называются.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

fail() { echo "FAIL: $*" >&2; exit 1; }
version() { cat .template/version 2>/dev/null || true; }
latest() { git tag -l 'template/v*' --sort=-v:refname | head -1; }
# newer A B — тег B новее тега A.
newer() { [ "$1" != "$2" ] && [ "$(printf '%s\n%s\n' "$1" "$2" | sort -V | tail -1)" = "$2" ]; }
module() { awk '$1 == "module" { print $2; exit }'; }
golibinfra() { awk '{ for (i = 1; i < NF; i++) if ($i ~ /\/golibinfra$/ && $(i + 1) ~ /^v/) { print $(i + 1); exit } }'; }
# canon V — это репо шаблона: тег его версии V в истории и module path шаблона.
canon() {
    git rev-parse -q --verify "refs/tags/$1" >/dev/null && git merge-base --is-ancestor "$1" HEAD &&
        [ "$(git show "$1:go.mod" | module)" = "$(module <go.mod)" ]
}

if [ "${1:-}" = "--status" ]; then
    v=$(version) l=$(latest)
    if [ -z "$v" ]; then
        echo "версии обвязки нет (.template/version) — skill /upgrade"
    elif canon "$v"; then
        echo "$v +$(git rev-list --count "$v..HEAD") — канон шаблона"
    elif [ -n "$l" ] && newer "$v" "$l"; then
        echo "$v · в шаблоне новее: $l → skill /upgrade"
    elif [ -n "$l" ]; then
        echo "$v · новее известных тегов шаблона нет (теги remote — make doctor)"
    else
        echo "$v · тегов шаблона в репо нет — теги remote называет make doctor"
    fi
    exit 0
fi

# Сеть — не ошибка: ни запроса пароля, ни ожидания дольше timeout.
if [ "${1:-}" = "--remote" ]; then
    v=$(version)
    if [ -n "$v" ] && canon "$v"; then
        echo "$v — канон шаблона"
        exit 0
    fi
    git remote get-url template >/dev/null 2>&1 || {
        echo "remote template не заведён — о новых тегах шаблона не узнать: git remote add --no-tags template <url шаблона> (skill /upgrade)"
        exit 1
    }
    deadline=()
    ! command -v timeout >/dev/null || deadline=(timeout 30)
    ssh=${GIT_SSH_COMMAND:-$(git config core.sshCommand || echo ssh)}
    tags=$(GIT_TERMINAL_PROMPT=0 GIT_SSH_COMMAND="$ssh -o BatchMode=yes -o ConnectTimeout=10" \
        "${deadline[@]}" git ls-remote --tags --refs template 'refs/tags/template/v*' 2>/dev/null) || {
        echo "remote template не ответил — теги шаблона не проверены"
        exit 2
    }
    l=$(sed -n 's#.*refs/tags/##p' <<<"$tags" | sort -V | tail -1)
    if [ -n "$l" ] && { [ -z "$v" ] || newer "$v" "$l"; }; then
        echo "${v:-версии обвязки нет} · в шаблоне новее: $l → skill /upgrade"
        exit 1
    fi
    echo "${v:-—} · новее тегов шаблона нет"
    exit 0
fi

TO=${1:-} FROM=${2:-} PATHS=${3:-} v=$(version)
git remote get-url template >/dev/null 2>&1 ||
    fail "нет remote template — git remote add --no-tags template <url шаблона> (skill /upgrade)"
git fetch -q --no-tags template 'refs/tags/template/*:refs/tags/template/*'
[ -z "$TO" ] || { [[ $TO == template/v* ]] && git rev-parse -q --verify "refs/tags/$TO" >/dev/null; } ||
    fail "TO=$TO — не тег шаблона template/vX.Y.Z (список — make template-update)"
# Перенос до своей же версии её не сдвинул бы: следующий перенос повторил бы те же правки.
[ -z "$TO" ] || [ -z "$v" ] || newer "$v" "$TO" ||
    fail "TO=$TO не новее версии форка $v — перенос до тега новее (список — make template-update)"

# До первого переноса (версия та же, что в первом коммите форка) база — коммит шаблона с деревом
# первого коммита: «Use this template» копирует кончик main, а он обычно новее тега версии.
root=$(git rev-list --max-parents=0 HEAD | tail -1) tag=${TO:-$(latest)}
if [ -z "$FROM" ] && [ -n "$tag" ] && [ "$(git show "$root:.template/version" 2>/dev/null || true)" = "$v" ]; then
    FROM=$(git log --format='%H %T' "$tag" | awk -v t="$(git rev-parse "$root^{tree}")" '$2 == t { print $1; exit }')
    # Форк с коммита тега своей версии: база — сам тег.
    [ -z "$FROM" ] || [ "$FROM" != "$(git rev-parse -q --verify "$v^{commit}" || true)" ] || FROM=$v
fi
FROM=${FROM:-$v}
[ -n "$FROM" ] || fail "базы нет: ни .template/version, ни коммита шаблона с деревом первого коммита форка — FROM=<коммит шаблона> (skill /upgrade)"
git rev-parse -q --verify "$FROM^{commit}" >/dev/null || fail "FROM=$FROM: такой ревизии нет"

if [ -z "$TO" ]; then
    echo "обвязка форка: ${v:-—}"
    [ "$FROM" = "$v" ] || echo "база переноса: $FROM — коммит шаблона, из которого создан форк"
    last=""
    for t in $(git tag -l 'template/v*' --sort=v:refname --no-merged "$FROM"); do
        [ -z "$v" ] || newer "$v" "$t" || continue
        echo
        git tag -l --format='%(contents)' "$t" | sed 's/^/  /'
        last=$t
    done
    [ -n "$last" ] || { echo "новее тегов шаблона нет"; exit 0; }
    echo "golibinfra: форк $(golibinfra <go.mod), шаблон на $last — $(git show "$last:go.mod" | golibinfra)"
    echo "перенос: make template-update TO=<тег> — процедура в skill /upgrade"
    exit 0
fi

! git merge-base --is-ancestor "$TO" "$FROM" || fail "обвязка форка уже содержит $TO (база $FROM)"
[ -z "$(git status --porcelain)" ] || fail "дерево не чистое — закоммить или спрячь (git stash -u)"
# Пути — через пробел или с новой строки: read без -d читал бы одну строку и молча терял остальные.
read -r -d '' -a extra <<<"$PATHS" || true
# Опечатка в пути промолчала бы: перенос без этого пути выглядит так же, как перенос без изменений в нём.
for p in "${extra[@]}"; do
    p=${p#:(exclude)}
    git cat-file -e "$TO:$p" 2>/dev/null || git cat-file -e "$FROM:$p" 2>/dev/null ||
        fail "PATHS: $p — нет ни в $TO, ни в базе $FROM"
done

mapfile -t paths < <({ git show "$FROM:.template/paths" 2>/dev/null || true; git show "$TO:.template/paths"; printf '%s\n' "${extra[@]}"; } |
    grep -vE '^[[:space:]]*(#|$)' | sort -u)
mapfile -t gofiles < <(git diff --no-renames --name-only "$FROM" "$TO" -- "${paths[@]}" | grep '\.go$' || true)
NEW=$(module <go.mod)

# Дерево ревизии с module path форка в Go-файлах переноса: иначе переименование импорта в форке —
# его правка, и любая правка шаблона рядом с импортом стала бы конфликтом.
tree() {
    local rev=$1 old idx f mode blob
    old=$(git show "$rev:go.mod" | module)
    if [ "$old" = "$NEW" ] || [ ${#gofiles[@]} -eq 0 ]; then
        git rev-parse "$rev^{tree}"
        return
    fi
    idx=$(git rev-parse --git-path index.template-update)
    GIT_INDEX_FILE=$idx git read-tree "$rev"
    for f in "${gofiles[@]}"; do
        mode=$(git ls-tree "$rev" -- "$f" | cut -d' ' -f1)
        [ -n "$mode" ] || continue
        blob=$(git show "$rev:$f" | sed "s#$old#$NEW#g" | git hash-object -w --stdin)
        GIT_INDEX_FILE=$idx git update-index --cacheinfo "$mode,$blob,$f"
    done
    GIT_INDEX_FILE=$idx git write-tree
    rm -f "$idx"
}
base=$(tree "$FROM") head=$(tree "$TO")

# git apply атомарен: правка файла, которого в форке нет, или удаление файла, который форк менял,
# сорвали бы перенос целиком. Файл, удалённый и шаблоном, и форком, решать не нужно.
absent=() kept=() excludes=()
while IFS=$'\t' read -r st f; do
    if [ "$st" != A ] && [ ! -e "$f" ]; then
        excludes+=("--exclude=$f")
        [ "$st" = D ] || absent+=("$f")
    elif [ "$st" = D ] && [ "$(git hash-object -- "$f")" != "$(git rev-parse "$base:$f")" ]; then
        excludes+=("--exclude=$f")
        kept+=("$f")
    fi
done < <(git diff --no-renames --name-status "$base" "$head" -- "${paths[@]}")

patch=$(git rev-parse --git-path template-update.patch)
git diff --binary --no-renames "$base" "$head" -- "${paths[@]}" >"$patch"
if [ -s "$patch" ] && ! out=$(git apply -3 "${excludes[@]}" "$patch" 2>&1) &&
    [ -z "$(git diff --name-only --diff-filter=U)" ]; then
    echo "$out" >&2
    fail "git apply не перенёс изменения (вывод выше), дерево не тронуто"
fi
rm -f "$patch"

conflicts=$(git diff --name-only --diff-filter=U)
echo "обвязка: $FROM → $TO, файлов в индексе: $(git diff --cached --name-only | wc -l | tr -d ' ')"
[ ${#extra[@]} -eq 0 ] || echo "код по запросу (PATHS): ${extra[*]}"
[ -z "$conflicts" ] || { echo "конфликты — разреши и git add:"; sed 's/^/  /' <<<"$conflicts"; }
[ ${#absent[@]} -eq 0 ] || { echo "нет в форке — не тронуты:"; printf '  %s\n' "${absent[@]}"; }
[ ${#kept[@]} -eq 0 ] || { echo "шаблон удалил, форк менял — реши сам:"; printf '  %s\n' "${kept[@]}"; }
echo "дальше — skill /upgrade: шаги аннотаций тегов до $TO, make gen, make check, коммит"
[ -z "$conflicts" ] && [ ${#kept[@]} -eq 0 ]
