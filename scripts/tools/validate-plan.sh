#!/usr/bin/env bash
# tools/validate-plan.sh — детерминированный валидатор структуры планов .claude/plans/.
#
# Использование:
#   scripts/tools/validate-plan.sh <file|dir>   # один план или директория
#   scripts/tools/validate-plan.sh              # все .claude/plans/*.md (кроме roadmap)
#
# Проверяет:
#   1. Header: **Slug:**, **Создан:**, **Статус:** ∈ {planning, in-progress}
#      (done → план удаляется, в репо его быть не должно).
#   2. Обязательные секции: Контекст, Не входит в скоуп, Атомы, Чекпоинты.
#   3. Каждый атом (`- [ ]/[x] **A<N>:**`) имеет `Verify:` и `Зависит от:`.
#   4. Ссылки `Зависит от:` вида A<M> указывают на существующий атом ЭТОГО плана
#      (Q-*, слаги, «—» — внешние/допустимые, не валидируются как атомы).
#
# НЕ сканирует код на инфра-изменения: детект инфра — rule+skill (infra-impact /
# infra-handoff), не скрипт. Здесь только структура плана.
#
# Выход: 0 — валидно; 1 — ошибки (список + канонический status-line).
# Зависимости: bash + awk + grep (без внешних).

set -uo pipefail
ROOT=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
cd "$ROOT"

TARGET="${1:-.claude/plans}"
[ -e "$TARGET" ] || {
    echo "FAIL: target not found: $TARGET"
    exit 1
}

if [ -f "$TARGET" ]; then
    FILES=("$TARGET")
else
    mapfile -t FILES < <(find "$TARGET" -maxdepth 1 -type f -name '*.md' ! -name 'roadmap.md' 2>/dev/null | sort)
fi

ERRORS=()
N=0
REQ_SECTIONS=("## Контекст" "## Не входит в скоуп" "## Атомы" "## Чекпоинты")

for f in "${FILES[@]}"; do
    [ -e "$f" ] || continue
    case "$f" in */roadmap.md) continue ;; esac
    N=$((N + 1))

    grep -q '^\*\*Slug:\*\*' "$f" || ERRORS+=("$f: нет **Slug:**")
    grep -q '^\*\*Создан:\*\*' "$f" || ERRORS+=("$f: нет **Создан:**")
    st=$(grep -m1 '^\*\*Статус:\*\*' "$f" | sed -E 's/.*\*\*Статус:\*\*[[:space:]]*//; s/[[:space:]]*$//')
    if [ -z "$st" ]; then
        ERRORS+=("$f: нет **Статус:**")
    elif ! printf '%s' "$st" | grep -qE '^(planning|in-progress)$'; then
        ERRORS+=("$f: Статус='$st' не ∈ {planning,in-progress}")
    fi

    for s in "${REQ_SECTIONS[@]}"; do
        grep -qF "$s" "$f" || ERRORS+=("$f: нет секции '$s'")
    done

    # Атомы + ссылки — single-pass awk, валидация в END (forward-ref ок).
    while IFS= read -r line; do
        [ -n "$line" ] && ERRORS+=("$line")
    done < <(awk -v F="$f" '
        /^- \[[ x]\] \*\*A[0-9]+:/ {
            id=$0; sub(/^- \[[ x]\] \*\*/,"",id); sub(/:.*$/,"",id)
            cur=id; ids[id]=1; order[++oc]=id; vf[id]=0; df[id]=0; next
        }
        /^#{1,6} / { cur="" ; next }
        /^[[:space:]]*-[[:space:]]*Verify:/        { if(cur!="") vf[cur]=1 }
        /^[[:space:]]*-[[:space:]]*Зависит от:/ {
            if(cur!=""){ df[cur]=1
                ln=$0; sub(/.*Зависит от:[[:space:]]*/,"",ln)
                n=split(ln, p, /[ ,]+/)
                for(i=1;i<=n;i++){ if(p[i] ~ /^A[0-9]+$/) deps[cur"\t"p[i]]=1 }
            }
        }
        END{
            if(oc==0){ print F ": в плане нет атомов (- [ ] **A<N>:**)"; exit }
            for(i=1;i<=oc;i++){ a=order[i]
                if(vf[a]!=1) print F ": атом " a " без Verify:"
                if(df[a]!=1) print F ": атом " a " без \"Зависит от:\""
            }
            for(k in deps){ split(k,kk,"\t")
                if(ids[kk[2]]!=1) print F ": атом " kk[1] " зависит от несуществующего " kk[2]
            }
        }
    ' "$f")
done

if [ "${#ERRORS[@]}" -gt 0 ]; then
    for e in "${ERRORS[@]}"; do echo "$e"; done
    echo "FAIL: ${#ERRORS[@]} error(s) across $N plan(s)"
    exit 1
fi

echo "OK ($N plan(s) checked)"
