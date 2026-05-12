#!/usr/bin/env bash
# agent-tools/validate-domain-doc.sh — детерминированный валидатор DDD-артефактов в docs/domains/.
#
# Использование:
#   scripts/agent-tools/validate-domain-doc.sh <path>            # один файл или директория контекста
#   scripts/agent-tools/validate-domain-doc.sh                   # вся docs/domains/
#
# Проверяет:
#   1. Frontmatter валиден: id, kind, context, name, status (обязательные); kind ∈ enum.
#   2. Все обязательные H2-секции для данного kind присутствуют и непусты.
#   3. Если status: stable — все обязательные секции непусты И целостность ссылок (relations/emits/consumes
#      указывают на существующие id).
#
# Выход:
#   0 — всё валидно.
#   1 — есть ошибки. Вывод сначала перечисляет ошибки, в конце канонический status-line.
#
# Не зависит от внешних инструментов: bash + awk + grep.

set -uo pipefail

ROOT=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
cd "$ROOT"

TARGET="${1:-docs/domains}"

if [ ! -e "$TARGET" ]; then
    echo "FAIL: target not found: $TARGET"
    exit 1
fi

ERRORS=()
FILES_CHECKED=0
STABLE_FILES=()

# === required sections per kind ===
# Lists are space-separated H2 headings (without "## ").

required_sections() {
    case "$1" in
        aggregate)
            echo "Identity Properties Methods Invariants Lifecycle Boundary 'Contained Entities' 'Spanning Invariants' 'Value Objects' Events Relations 'Use Cases'"
            ;;
        entity)
            echo "Identity Properties Methods Invariants Lifecycle Relations"
            ;;
        event)
            echo "Trigger Payload Consumers 'Delivery semantics'"
            ;;
        use-case)
            echo "Actor Inputs Outputs Dependencies Steps 'Called aggregate methods' 'Events emitted' Errors"
            ;;
        policy)
            echo "'Kind of decision' Inputs Rules Outcomes 'Used by'"
            ;;
        vo)
            echo "Properties Invariants Constructor 'Consumed by'"
            ;;
        *)
            echo ""
            ;;
    esac
}

VALID_KINDS=(aggregate entity event use-case policy vo)

is_valid_kind() {
    local k="$1"
    for v in "${VALID_KINDS[@]}"; do
        [ "$k" = "$v" ] && return 0
    done
    return 1
}

# Extract frontmatter scalar field value: read_field <file> <field>
read_field() {
    awk -v field="$2" '
        /^---$/ { fm = !fm; if (!fm) exit; next }
        fm && $0 ~ "^"field":" {
            sub("^"field":[ \t]*", "")
            gsub(/^[ \t]+|[ \t]+$/, "")
            print
            exit
        }
    ' "$1"
}

# Extract frontmatter list field (relations/emits/consumes).
# Supports both inline form `field: [a, b, c]` and YAML block form:
#   field:
#     - a
#     - b
# Prints one value per line.
read_list_field() {
    local file="$1" field="$2"
    awk -v field="$field" '
        BEGIN { fm = 0; inline_done = 0; in_block = 0 }
        /^---$/ {
            fm = !fm
            if (!fm) exit
            next
        }
        !fm { next }
        in_block {
            # Block items: "  - value"
            if (match($0, /^[ \t]+-[ \t]+/)) {
                v = substr($0, RLENGTH + 1)
                gsub(/[ \t]+$/, "", v)
                if (v != "") print v
                next
            }
            # Any non-indented line ends the block
            if (match($0, /^[^ \t]/)) {
                in_block = 0
            } else {
                next
            }
        }
        $0 ~ "^"field":[ \t]*\\[" {
            # Inline form: extract between [ and ]
            v = $0
            sub(".*\\[", "", v)
            sub("\\].*", "", v)
            n = split(v, a, ",")
            for (i = 1; i <= n; i++) {
                gsub(/^[ \t]+|[ \t]+$/, "", a[i])
                if (a[i] != "") print a[i]
            }
            inline_done = 1
            next
        }
        $0 ~ "^"field":[ \t]*$" {
            in_block = 1
            next
        }
    ' "$file"
}

# Check section exists and has non-empty body before next H2/EOF.
section_body_nonempty() {
    local file="$1" section="$2"
    awk -v sec="$section" '
        BEGIN { in_sec = 0; has_content = 0 }
        /^## / {
            if (in_sec) exit
            heading = $0; sub(/^## /, "", heading)
            if (heading == sec) { in_sec = 1; next }
        }
        in_sec {
            line = $0
            gsub(/^[ \t]+|[ \t]+$/, "", line)
            if (line == "") next
            if (line ~ /^<!--/) next
            if (line ~ /^\|[ \t-|]+\|$/) next        # divider row of empty table
            if (line ~ /^\|[ \t|]*\|$/ && !has_content) next  # empty table row before any header
            has_content = 1
        }
        END { exit (has_content ? 0 : 1) }
    ' "$file"
}

# Check section exists at all.
has_section() {
    local file="$1" section="$2"
    grep -qE "^## ${section}([ \t]*$|[ \t]+)" "$file"
}

# C1: ensure every aggregate file in <ctx>/ is referenced from <ctx>/README.md
# and every README link to <ctx>/<X>.md points to an existing file.
check_readme_aggregates() {
    local readme="$1"
    local ctx_dir
    ctx_dir=$(dirname "$readme")

    # Skip _shared/ and _templates/ READMEs (no aggregates by design).
    case "$ctx_dir" in
        */docs/domains/_shared|docs/domains/_shared) return 0 ;;
        */docs/domains/_templates|docs/domains/_templates) return 0 ;;
        */docs/domains|docs/domains) return 0 ;;
    esac

    # Collect aggregate files in ctx_dir (top-level *.md, kind=aggregate).
    local agg_files=()
    local f bn k
    for f in "$ctx_dir"/*.md; do
        [ -f "$f" ] || continue
        bn=$(basename "$f" .md)
        [ "$bn" = "README" ] && continue
        k=$(read_field "$f" kind)
        [ "$k" = "aggregate" ] && agg_files+=("$bn")
    done

    # Collect refs from README body: links of form `[X](X.md)` or `[`X`](X.md)` to local files.
    local refs
    refs=$(grep -oE '\]\([A-Za-z][A-Za-z0-9_-]*\.md\)' "$readme" \
        | sed -E 's/^\]\(([A-Za-z0-9_-]+)\.md\)$/\1/' \
        | sort -u)

    # Every aggregate file must appear in README links.
    local a found
    for a in "${agg_files[@]}"; do
        found=0
        while IFS= read -r ref; do
            [ "$ref" = "$a" ] && { found=1; break; }
        done <<< "$refs"
        if [ "$found" -eq 0 ]; then
            ERRORS+=("$readme: aggregate file '$a.md' exists but is not linked from README")
        fi
    done

    # Every README link to a local <X>.md (no subdir) must resolve.
    while IFS= read -r ref; do
        [ -z "$ref" ] && continue
        [ -f "$ctx_dir/$ref.md" ] || ERRORS+=("$readme: link to '$ref.md' is dangling (no such file in $ctx_dir)")
    done <<< "$refs"
}

# Validate single file.
validate_file() {
    local file="$1"
    FILES_CHECKED=$((FILES_CHECKED + 1))

    # Skip template files.
    case "$file" in
        */docs/domains/_templates/*) return 0 ;;
    esac

    # Skip top-level docs (README.md, MAPPING.md).
    case "$file" in
        */docs/domains/README.md|*/docs/domains/MAPPING.md|docs/domains/README.md|docs/domains/MAPPING.md) return 0 ;;
    esac

    # README.md of context: required sections + aggregates ↔ files sync.
    if [[ "$(basename "$file")" == "README.md" ]]; then
        if ! grep -qE "^## Purpose" "$file"; then
            ERRORS+=("$file: missing required section 'Purpose' in context README")
        fi
        if ! grep -qE "^## Ubiquitous Language" "$file"; then
            ERRORS+=("$file: missing required section 'Ubiquitous Language' in context README")
        fi
        check_readme_aggregates "$file"
        return 0
    fi

    # Frontmatter must exist.
    if ! head -1 "$file" | grep -qE '^---$'; then
        ERRORS+=("$file: missing frontmatter (must start with ---)")
        return
    fi

    local id kind ctx name status
    id=$(read_field "$file" id)
    kind=$(read_field "$file" kind)
    ctx=$(read_field "$file" context)
    name=$(read_field "$file" name)
    status=$(read_field "$file" status)

    [ -z "$id" ]     && ERRORS+=("$file: frontmatter.id is empty")
    [ -z "$kind" ]   && ERRORS+=("$file: frontmatter.kind is empty")
    [ -z "$ctx" ]    && ERRORS+=("$file: frontmatter.context is empty")
    [ -z "$name" ]   && ERRORS+=("$file: frontmatter.name is empty")
    [ -z "$status" ] && ERRORS+=("$file: frontmatter.status is empty")

    if [ -n "$kind" ] && ! is_valid_kind "$kind"; then
        ERRORS+=("$file: frontmatter.kind '$kind' not in {${VALID_KINDS[*]}}")
        return
    fi

    if [ "$status" != "draft" ] && [ "$status" != "stable" ]; then
        ERRORS+=("$file: frontmatter.status must be 'draft' or 'stable' (got '$status')")
    fi

    # id must equal context.<Name> with exactly one dot (sanity + C4 strict).
    if [ -n "$id" ] && [ -n "$ctx" ]; then
        if [[ "$id" != "$ctx".* ]]; then
            ERRORS+=("$file: id '$id' must start with '$ctx.'")
        fi
        # Strict: id has exactly one dot — no sub-namespaces.
        local dots="${id//[^.]/}"
        if [ "${#dots}" -ne 1 ]; then
            ERRORS+=("$file: id '$id' must have exactly one dot (form: <context>.<Name>)")
        fi
    fi

    # C2: basename of file must match frontmatter.name.
    if [ -n "$name" ]; then
        local bn
        bn=$(basename "$file" .md)
        if [ "$bn" != "$name" ]; then
            ERRORS+=("$file: basename '$bn.md' does not match frontmatter.name '$name'")
        fi
    fi

    # Required sections per kind.
    if [ -n "$kind" ]; then
        local secs
        secs=$(required_sections "$kind")
        # eval to handle quoted multi-word section names
        eval "set -- $secs"
        for s in "$@"; do
            if ! has_section "$file" "$s"; then
                ERRORS+=("$file: missing required H2 section '$s' (kind=$kind)")
                continue
            fi
            if [ "$status" = "stable" ]; then
                if ! section_body_nonempty "$file" "$s"; then
                    ERRORS+=("$file: section '$s' is empty (status=stable requires content)")
                fi
            fi
        done
    fi

    if [ "$status" = "stable" ]; then
        STABLE_FILES+=("$file")
    fi
}

# === collect files ===
FILES=()
if [ -f "$TARGET" ]; then
    FILES=("$TARGET")
elif [ -d "$TARGET" ]; then
    while IFS= read -r f; do
        FILES+=("$f")
    done < <(find "$TARGET" -type f -name '*.md' ! -path '*/_templates/*' | sort)
else
    echo "FAIL: target is neither file nor dir: $TARGET"
    exit 1
fi

if [ "${#FILES[@]}" -eq 0 ]; then
    echo "OK (no .md files under $TARGET)"
    exit 0
fi

for f in "${FILES[@]}"; do
    validate_file "$f"
done

# === referential integrity for stable files ===
# Build index of all known ids across the whole tree.
if [ "${#STABLE_FILES[@]}" -gt 0 ]; then
    declare -A ALL_IDS
    while IFS= read -r f; do
        case "$f" in
            */docs/domains/_templates/*) continue ;;
        esac
        if [[ "$(basename "$f")" == "README.md" ]]; then continue; fi
        case "$f" in
            */docs/domains/README.md|*/docs/domains/MAPPING.md|docs/domains/README.md|docs/domains/MAPPING.md) continue ;;
        esac
        id=$(read_field "$f" id)
        [ -n "$id" ] && ALL_IDS["$id"]=1
    done < <(find docs/domains -type f -name '*.md' 2>/dev/null)

    check_refs() {
        local file="$1" field="$2"
        local ref
        while IFS= read -r ref; do
            [ -z "$ref" ] && continue
            if [ -z "${ALL_IDS[$ref]:-}" ]; then
                ERRORS+=("$file: $field references unknown id '$ref'")
            fi
        done < <(read_list_field "$file" "$field")
    }

    for f in "${STABLE_FILES[@]}"; do
        check_refs "$f" relations
        check_refs "$f" emits
        check_refs "$f" consumes
    done
fi

# === report ===
if [ "${#ERRORS[@]}" -gt 0 ]; then
    for e in "${ERRORS[@]}"; do
        echo "$e"
    done
    echo "FAIL: ${#ERRORS[@]} error(s) across $FILES_CHECKED file(s)"
    exit 1
fi

echo "OK ($FILES_CHECKED file(s) checked, ${#STABLE_FILES[@]} stable)"
