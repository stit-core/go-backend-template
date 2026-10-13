#!/usr/bin/env bash
# tools/validate-stack-doc.sh — детерминированный валидатор stack-артефактов docs/stack/.
#
# Модель зеркалит доменные артефакты (живут в соседнем strategy-репо): 8 singleton-
# артефактов (по одному на kind),
# frontmatter + обязательные H2-секции; при status: stable секции непусты.
# stack-артефакты — вход для skill `de-template` (секция `Generates` каждого
# описывает, что из него родится в коде).
#
# Использование:
#   scripts/tools/validate-stack-doc.sh [--stable] <path>   # файл или директория
#   scripts/tools/validate-stack-doc.sh [--stable]          # вся docs/stack/
#
# Проверяет:
#   1. Frontmatter: id, kind, name, status (обязательные); kind ∈ 8 stack-kinds.
#   2. basename == frontmatter.name; id == "stack.<name>".
#   3. Все обязательные H2 для kind присутствуют; при status: stable — в каждой решение. Не решение:
#      плейсхолдер шаблона (блок от строки, которая начинается с `<`, до строки, которая кончается `>`, и
#      пункт `- <…>`), заглушка — строка, пункт, чекбокс или цитата из TBD, TODO или FIXME словом (с
#      хвостом или без), прочерка, многоточия или вопросов, — подзаголовок и таблица без строк или из
#      строк одних заглушек.
#   4. kind auth при status: stable — `identity: own | external` во frontmatter (домены идентичности
#      остаются или prune их удаляет — skill init-service).
#   --stable — ещё и все 8 kinds на месте и stable: вход de-template.
# _templates/ и README.md пропускаются.
#
# Выход: 0 — валидно; 1 — ошибки. bash + awk + grep, без внешних зависимостей.

set -uo pipefail
ROOT=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
cd "$ROOT"

STABLE=""
if [ "${1:-}" = "--stable" ]; then STABLE=1; shift; fi
TARGET="${1:-docs/stack}"
[ -e "$TARGET" ] || { echo "FAIL: target not found: $TARGET"; exit 1; }

ERRORS=()
FILES_CHECKED=0

VALID_KINDS=(service runtime config persistence messaging auth observability dependencies)

required_sections() {
    case "$1" in
        service)       echo "Identity Versioning Binaries Generates" ;;
        runtime)       echo "Entrypoints Lifecycle Shutdown Generates" ;;
        config)        echo "'Required env' 'Optional env' Secrets Validation Generates" ;;
        persistence)   echo "Engine 'Migration strategy' 'Kit-owned tables' 'Connection pool' Generates" ;;
        messaging)     echo "'Published subjects' 'Consumed subjects' 'Delivery semantics' Generates" ;;
        auth)          echo "Authn Authgate Roles 'Seed-admin' 'Brute-force' Generates" ;;
        observability) echo "'Log sink' 'Log levels & content' Generates" ;;
        dependencies)  echo "'golibinfra kits' 'Что НЕ подключено' 'External runtime deps' Capacity Generates" ;;
        *)             echo "" ;;
    esac
}

is_valid_kind() {
    local k="$1" v
    for v in "${VALID_KINDS[@]}"; do [ "$k" = "$v" ] && return 0; done
    return 1
}

read_field() {
    awk -v field="$2" '
        /^---$/ { fm = !fm; if (!fm) exit; next }
        fm && $0 ~ "^"field":" {
            sub("^"field":[ \t]*", ""); gsub(/^[ \t]+|[ \t]+$/, ""); print; exit
        }
    ' "$1"
}

has_section() {
    grep -qE "^## ${2}([ \t]*$|[ \t]+)" "$1"
}

section_body_nonempty() {
    awk -v sec="$2" '
        # Заглушка вместо решения: пусто, TBD, TODO или FIXME словом (с хвостом или без), прочерк, многоточие,
        # вопросы. Регистр, _*` и скобки не в счёт; Todoist и TODO-лист — слова, а не заглушки.
        function stub(s) {
            s = toupper(s); gsub(/[_*`()]/, "", s); gsub(/^[ \t]+|[ \t]+$/, "", s)
            return s ~ /^(TBD|TODO|FIXME)([^-[:alnum:]].*)?$/ || s ~ /^(—|–|-|\.\.\.|…|\?+)?[.:;!]?$/
        }
        BEGIN { in_sec = 0; has = 0 }
        /^## / {
            if (in_sec) exit
            h = $0; sub(/^## /, "", h)
            if (h == sec) { in_sec = 1; next }
        }
        in_sec {
            line = $0; gsub(/^[ \t]+|[ \t]+$/, "", line)
            if (placeholder) { if (line ~ />$/) placeholder = 0; next }
            if (line !~ /^\|/) rows = 0
            if (line == "" || line ~ /^#/) next                             # подзаголовок — не решение
            if (line ~ /^</) { if (line !~ />$/) placeholder = 1; next }    # плейсхолдер <...>, и многострочный
            if (line ~ /^\|[-:| \t]*$/) { rows = 1; next }                  # строки таблицы — под разделителем
            if (line ~ /^\|/) {                                             # шапка; строка из одних заглушек
                for (n = split(line, cell, "|"); rows && n > 0; n--) if (!stub(cell[n])) has = 1
                next
            }
            item = line; sub(/^([-*+]|[0-9]+[.)])[ \t]+/, "", item)         # пункт списка
            sub(/^(\[[ xX]\]|>)[ \t]*/, "", item)                           # чекбокс, цитата
            if (item ~ /^<.*>$/ || stub(item)) next
            has = 1
        }
        END { exit (has ? 0 : 1) }
    ' "$1"
}

validate_file() {
    local file="$1"
    case "$file" in */docs/stack/_templates/*) return 0 ;; esac
    case "$(basename "$file")" in README.md|MAPPING.md) return 0 ;; esac
    FILES_CHECKED=$((FILES_CHECKED + 1))

    if ! head -1 "$file" | grep -qE '^---$'; then
        ERRORS+=("$file: нет frontmatter (должен начинаться с ---)"); return
    fi

    local id kind name status
    id=$(read_field "$file" id); kind=$(read_field "$file" kind)
    name=$(read_field "$file" name); status=$(read_field "$file" status)

    [ -z "$id" ]     && ERRORS+=("$file: frontmatter.id пуст")
    [ -z "$kind" ]   && ERRORS+=("$file: frontmatter.kind пуст")
    [ -z "$name" ]   && ERRORS+=("$file: frontmatter.name пуст")
    [ -z "$status" ] && ERRORS+=("$file: frontmatter.status пуст")

    if [ -n "$kind" ] && ! is_valid_kind "$kind"; then
        ERRORS+=("$file: kind '$kind' не из {${VALID_KINDS[*]}}"); return
    fi
    if [ -n "$status" ] && [ "$status" != "draft" ] && [ "$status" != "stable" ]; then
        ERRORS+=("$file: status должен быть draft|stable (got '$status')")
    fi
    if [ -n "$name" ]; then
        local bn; bn=$(basename "$file" .md)
        [ "$bn" != "$name" ] && ERRORS+=("$file: basename '$bn' != frontmatter.name '$name'")
    fi
    if [ -n "$id" ] && [ -n "$name" ] && [ "$id" != "stack.$name" ]; then
        ERRORS+=("$file: id '$id' должен быть 'stack.$name'")
    fi

    if [ "$kind" = auth ] && [ "$status" = stable ]; then
        case "$(read_field "$file" identity)" in
            own | external) ;;
            *) ERRORS+=("$file: frontmatter.identity должен быть own|external (status=stable)") ;;
        esac
    fi
    [ "$status" = stable ] || [ -z "$STABLE" ] || ERRORS+=("$file: status=$status — de-template ждёт stable")
    [ -z "$kind" ] || KINDS_SEEN+=" $kind "

    if [ -n "$kind" ]; then
        local secs; secs=$(required_sections "$kind")
        eval "set -- $secs"
        for s in "$@"; do
            if ! has_section "$file" "$s"; then
                ERRORS+=("$file: нет H2-секции '$s' (kind=$kind)"); continue
            fi
            if [ "$status" = "stable" ] && ! section_body_nonempty "$file" "$s"; then
                ERRORS+=("$file: секция '$s' пуста (status=stable требует решение; плейсхолдер <…>, TBD, TODO, FIXME, прочерк, многоточие и таблица без решений — не решение)")
            fi
        done
    fi
}

KINDS_SEEN=" "
FILES=()
if [ -f "$TARGET" ]; then
    FILES=("$TARGET")
else
    while IFS= read -r f; do FILES+=("$f"); done \
        < <(find "$TARGET" -type f -name '*.md' ! -path '*/_templates/*' | sort)
fi

if [ "${#FILES[@]}" -eq 0 ] && [ -z "$STABLE" ]; then
    echo "OK (no stack .md files under $TARGET)"; exit 0
fi

for f in ${FILES[@]+"${FILES[@]}"}; do validate_file "$f"; done
if [ -n "$STABLE" ]; then
    for k in "${VALID_KINDS[@]}"; do
        [[ $KINDS_SEEN == *" $k "* ]] || ERRORS+=("$TARGET: нет артефакта kind=$k — стек не спроектирован (/stack-deep-dive)")
    done
fi

if [ "${#ERRORS[@]}" -gt 0 ]; then
    for e in "${ERRORS[@]}"; do echo "$e"; done
    echo "FAIL: ${#ERRORS[@]} error(s) across $FILES_CHECKED file(s)"
    exit 1
fi
echo "OK ($FILES_CHECKED file(s) checked)"
