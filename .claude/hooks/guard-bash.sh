#!/usr/bin/env bash
# .claude/hooks/guard-bash.sh — PreToolUse(Bash): не пускает из сессии Claude обходы git-хуков
# и гейта. exit 2 блокирует вызов, stderr уходит Claude.
#
# Блокирует: git commit с -n или --no-veri…; git push с --no-veri…; SKIP_PREPUSH= и
# core.hooksPath в команде с git (прочитать — git config --list); GOFLAGS= вместе с make, git
# или go env -w. Тела heredoc — данные (сообщение коммита может упоминать флаги); флаги в
# строках в кавычках не ищутся. Это страховка от «удобного» обхода, а не от обфускации.
set -uo pipefail

block() {
    echo "guard-bash: $1 — обход гейта. Почини причину падения; обойти гейт может только человек." >&2
    exit 2
}

# Значение "command" из JSON без jq: \" — вручную, остальное экранирование — printf %b.
raw=$(sed -nE 's/.*"command"[[:space:]]*:[[:space:]]*"(([^"\\]|\\.)*)".*/\1/p' | head -n 1)
cmd=$(printf '%b' "$(printf '%s' "$raw" | sed 's/\\"/"/g')")

# Без тел heredoc: от строки с `<<WORD` до строки-терминатора WORD.
text=$(printf '%s\n' "$cmd" | awk '
    body != "" { line = $0; sub(/^\t+/, "", line); if (line == body) body = ""; next }
    { print }
    match($0, /(^|[^<])<<-?[ \t]*["'\'']?[A-Za-z_][A-Za-z0-9_]*/) {
        body = substr($0, RSTART, RLENGTH); sub(/^[^<]*<<-?[ \t]*["'\'']?/, "", body)
    }')

# Переменные и ключи конфига опасны только рядом с git или make: grep по ним не блокируется.
re_git='(^|[^[:alnum:]_.-])git([[:space:]]|$)'
re_gate='(^|[^[:alnum:]_.-])(git|make)([[:space:]]|$)'
[[ $text == *SKIP_PREPUSH=* && $text =~ $re_git ]] && block "SKIP_PREPUSH"
[[ $(printf '%s' "$text" | tr '[:upper:]' '[:lower:]') == *core.hookspath* && $text =~ $re_git ]] &&
    block "core.hooksPath"
[[ $text == *GOFLAGS=* && ($text =~ $re_gate || $text == *"go env -w"*) ]] && block "GOFLAGS="

# Флаги git ищем вне кавычек, по сегментам между ; & | ( ) и обратными кавычками.
segments=$(printf '%s' "$text" | tr '\n' ' ' |
    sed -E -e 's/"([^"\\]|\\.)*"//g' -e "s/'[^']*'//g" | tr ';&|()`' '\n\n\n\n\n\n')
while read -r -a words; do
    git=0 sub="" skip=0
    for w in ${words[@]+"${words[@]}"}; do
        if [ "$git" = 0 ]; then
            [[ $w == git || $w == */git ]] && git=1
        elif [ -z "$sub" ]; then # подкоманда — первое слово не-флаг, кроме значения -C/-c
            case "$w" in
                -C | -c) skip=1 ;;
                -*) ;;
                *) if [ "$skip" = 1 ]; then skip=0; else sub=$w; fi ;;
            esac
        else
            case "$w" in
                --no-veri*) [[ $sub == commit || $sub == push ]] && block "git $sub $w" ;;
                # -n в кластере коротких флагов до первого флага со значением: -an, -nm.
                -[a-zA-Z]*) [[ $sub == commit && ${w%%[mFcCtS]*} == *n* ]] && block "git commit $w" ;;
            esac
        fi
    done
done <<<"$segments"
exit 0
