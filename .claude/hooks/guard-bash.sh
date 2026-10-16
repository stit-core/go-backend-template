#!/usr/bin/env bash
# .claude/hooks/guard-bash.sh — PreToolUse(Bash): не пускает из сессии Claude удобные обходы git-хуков.
# exit 2 блокирует вызов, stderr уходит Claude.
#
# Блокирует: git commit с -n; git с --no-veri… (любая подкоманда); git send-pack и http-push (push без
# pre-push) — и бинарём подкоманды (git-commit, /usr/lib/git-core/git-send-pack; у «читателя» без подстановки,
# man или ls, — это данные, а пейджер и браузер man — код); алиасы и include конфига git в команде (-c alias.…, git config include.path …, и
# значением в кавычках: -c 'alias.c=commit -n'); снятие секции core (git config --remove-section или
# --rename-section core — с ней уходит hooksPath); SKIP_PREPUSH и ключ hooksPath в любой команде (прочитать — git
# config --list, поиск — инструментом Grep, сообщение коммита — heredoc); команду с путём scripts/git-hooks, хука
# (pre-push, pre-commit, commit-msg) или .git/config — и с //, /./ и <каталог>/.. в пути, — в которой не узнал
# чтение, и запись туда перенаправлением. Слово с кавычками ($'…' тоже) или `\` внутри — как без них ("git",
# $'git', --no-ver'i'fy).
# Команда проверяется построчно; тела heredoc и строки в кавычках с пробелом — данные (сообщение
# коммита может упоминать флаги), но у оболочки-интерпретатора (bash -e -c, sh -s, eval, | bash -x,
# bash <<EOF, sh<<<'…') и у git, который исполняет строку из значения (вложенный git в кавычках или после «=»:
# git rebase --exec=…, git submodule foreach, git -c core.editor=…, GIT_SEQUENCE_EDITOR=…), — код, как и текст echo и
# printf рядом с ней; вложенный git — своя команда.
# Это барьер от случайного обхода, а не песочница: формы, которые он держит, — TestGuardBash
# (internal/archtest/testdata/guard-cases.txt), чего не видит — docs/contributing.md § guard-bash.
# Ослабить сам гейт не даёт make check.
set -uo pipefail

block() {
    echo "guard-bash: $1 — обход гейта. Почини причину падения; обойти гейт может только человек." >&2
    exit 2
}

# Значение "command" из JSON без jq: \" — вручную, остальное экранирование — printf %b.
raw=$(sed -nE 's/.*"command"[[:space:]]*:[[:space:]]*"(([^"\\]|\\.)*)".*/\1/p' | head -n 1)
cmd=$(printf '%b' "$(printf '%s' "$raw" | sed 's/\\"/"/g')")
# Путь сверяется нормализованным: .git//config, scripts/./git-hooks и .git/hooks/../config — те же файлы.
cmd=$(printf '%s' "$cmd" | sed -E -e ':a' -e 's#/\./#/#g' -e 'ta' -e 's#/{2,}#/#g' \
    -e ':b' -e 's#/[^/[:space:]]*[^./[:space:]][^/[:space:]]*/\.\./#/#' -e 'tb')

# Без тел heredoc: от строки с `<<WORD` до строки-терминатора WORD.
text=$(printf '%s\n' "$cmd" | awk '
    body != "" { line = $0; sub(/^\t+/, "", line); if (line == body) body = ""; next }
    { print }
    match($0, /(^|[^<])<<-?[ \t]*["'\'']?[A-Za-z_][A-Za-z0-9_]*/) {
        body = substr($0, RSTART, RLENGTH); sub(/^[^<]*<<-?[ \t]*["'\'']?/, "", body)
    }')
# Кавычки без пробела внутри ($'…', $"…" тоже) и `\` перед буквой слово не меняют: $'git' и \git — git.
unquote() { sed -E -e "s/\\\$?'([^' ]*)'/\\1/g" -e 's/\$?"([^" \\]*)"/\1/g' -e 's/\\([[:alnum:]_.-])/\1/g'; }
# Строка с `\` в конце и следующая — одна команда.
join_lines() { sed -e ':a' -e '/\\$/{N;s/\\\n//;ba' -e '}'; }

# Интерпретатор вне тел heredoc исполняет строку в кавычках или тело heredoc — это код, а не данные.
# Оболочку узнаём по имени (и с каталогом, и в кавычках): с кластером -c или -s среди слов своей команды
# (bash -e -c, env bash -euc), справа от `|` (| bash -x, | sudo bash) и с `<<` (bash -e <<EOF, sh<<<'…').
nl=$'\n'
shell='(ba|da|z|k|a|mk|c|tc|fi)?sh'
start='(^|[;&|(`[:space:]])'
before='(^|[;&|(`[:space:]/])'
rest="([[:space:]]+[^;&|${nl}]*)?"
re_interp="${before}(${shell})${rest}[[:space:]]+-[[:alnum:]]*[cs][[:alnum:]]*([[:space:]]|\$)"
re_interp+="|${start}eval([[:space:]]|\$)"
re_interp+="|\\|&?([^;&|)${nl}]*[[:space:]/(])?(${shell})([[:space:];&|)<>]|\$)"
re_interp+="|${before}(${shell})${rest}[[:space:]]*<<"
# man исполняет пейджер (-P, --pager, MANPAGER, PAGER) и браузер (-H, --html): их значение — код.
re_interp+="|${before}man${rest}[[:space:]](-[[:alpha:]]*[PH]|--(pager|html))|${start}(MAN)?PAGER="
# Git исполняет и строку из значения — опции (git rebase -x и --exec=…, submodule foreach, bisect run, filter-branch,
# difftool --extcmd), конфига (-c core.editor=…) или переменной (GIT_SEQUENCE_EDITOR=…): вложенный git в кавычках или
# после «=» команды с git делает её кодом, как у интерпретатора; текст сообщения (-m, --message) — данные.
re_git='(^|[^[:alnum:]_.-])git([[:space:]]|$)'
re_nested='(^|[[:space:];&|(=])([^[:space:]=]*/)?git(-[[:alnum:]-]+)?([[:space:];&|)\\]|$)'
quoted='"([^"\\]|\\.)*"|'\''[^'\'']*'\'''
message='(^|[[:space:]])(-[[:alnum:]]*m|--message)([[:space:]]+|=)('"$quoted"'|[^[:space:]"'\'']+)'
git_runs() {
    local body
    body=$(printf '%s' "$1" | sed -E "s/$message/\\1/g")
    [[ $(printf '%s' "$body" | sed -E "s/$quoted//g") =~ $re_git ]] &&
        printf '%s' "$body" | grep -oE "$quoted|=[^[:space:]\"']+" | tr -d "\"'" | grep -qE "$re_nested"
}
norm=$(printf '%s' "$text" | join_lines | unquote)
interp=0
if [[ $norm =~ $re_interp ]] || git_runs "$norm"; then
    interp=1
    text=$(printf '%s' "$cmd" | tr -d "\"'")
    norm=$(printf '%s' "$text" | join_lines | unquote)
fi

lower=$(printf '%s' "$norm" | tr '[:upper:]' '[:lower:]')
# Ключевые слова ищем без кавычек вовсе: кавычка с пробелом внутри слово не прячет
# (core.hooksP'ath=/tmp/a b', -c 'alias.c=commit -n').
keys=$(printf '%s' "$lower" | sed -E "s/\\\$?[\"']//g")
[[ $keys == *skip_prepush* ]] && block "SKIP_PREPUSH"
[[ $keys == *hookspath* ]] && block "hooksPath"
# Алиас и подключаемый конфиг прячут флаг или hooksPath от проверок ниже: git -c alias.c='commit -n' c.
re_cfg='(-c[[:space:]]*|config[^;&|]*)(alias|include(if)?)\.'
[[ $norm =~ $re_git && $keys =~ $re_cfg ]] && block "алиас или include конфига git"

# Слова команды с учётом кавычек. Вне кавычек `\x` — буква, пробел — граница слова (код 037); в
# кавычках и после `\` разделители команд прячутся в коды 001–006 и возвращаются, когда строка уже
# разрезана на команды: слово в кавычках остаётся одним словом, и его текст виден целиком. Перед
# тем, что оболочка раскроет сама (`$…`, обратная кавычка, вне кавычек — и `{`, `*`, `?`, `[`),
# встаёт код 007: что получится из такого слова, guard не знает. Перенаправление, приклеенное к
# слову (`хук>/dev/null`), — отдельное слово; номер дескриптора остаётся при нём (`2>…`). Кавычка,
# не закрытая до конца строки, кончается вместе со строкой: следующая строка — снова код.
split_words() {
    awk '
        function hide(c,    k) { k = index(";&|()`", c); return k ? substr("\001\002\003\004\005\006", k, 1) : c }
        {
            out = ""; q = ""; n = length($0); word = ""
            for (i = 1; i <= n; i++) {
                c = substr($0, i, 1)
                if (q == "") {
                    if (c == "\\") { out = out c; word = word c; i++; c = hide(substr($0, i, 1)) }
                    else if (c == "\"") q = c
                    else if (c == "\047") q = (i > 1 && substr($0, i - 1, 1) == "$") ? "$" : c
                    else if (c == " " || c == "\t") { c = "\037"; word = "" }
                    else if (index("$`{*?[", c)) out = out "\007"
                    else if ((c == "<" || c == ">") && word ~ /[^0-9<>]/) { out = out "\037"; word = "" }
                    if (c != "\037") word = word c
                } else if (q == "\047") {
                    if (c == q) q = ""; else c = hide(c)
                } else if (c == "\\") { out = out c; i++; c = hide(substr($0, i, 1)) }
                else if (c == (q == "$" ? "\047" : q)) q = ""
                else { if (q == "\"" && index("$`", c)) out = out "\007"; c = hide(c) }
                out = out c
            }
            print out
        }'
}

# Изменённый, неисполняемый или удалённый хук git молча пропускает, а .git/config задаёт, откуда хуки
# берутся. Команда с таким путём проходит, только если в ней узнано чтение: readers, git без правки
# дерева, bash -n, запуск самого хука (и bash <хук>) и «читатели», которые умеют писать и запускать, —
# без таких опций и программ; остальное — отказ, как и запись туда перенаправлением. cd туда — отказ:
# дальше путь в команде не виден.
hook='((^|[[:space:]/"'\''=])(git-hooks|pre-push|pre-commit|commit-msg)|\.git/config)([[:space:]/"'\''},]|$)'
run_hook='^([.[:alnum:]_~-]*/)*(pre-push|pre-commit|commit-msg)$'
readers='cat|head|tail|less|more|grep|egrep|fgrep|ls|stat|file|wc|diff|cmp|md5sum|sha256sum|shellcheck|test|\[|echo|printf|which|whereis|type|readlink|realpath|strings'
git_readers='add|blame|cat-file|commit|diff|grep|log|ls-files|ls-tree|push|rev-parse|show|status|tag'
bel=$'\007'
re_redirect='^[0-9]*[<>]'
re_no_target='[<>]$'
re_path='^[[:alnum:]_./~-]+$'
re_awk_opts='^(-F.*|-v.*|--field-separator(=.*)?|--assign(=.*)?|--)$'
re_awk_io='[>|@]|system'
re_cp_opts='^(-[adfHLnPpRrsuvx]+|--(archive|recursive|force|verbose|parents|dereference|no-dereference|update|no-clobber|preserve(=.*)?)|--)$'

# parse <regex опций с отдельным аргументом> — слова команды: opts — опции, optargs — их аргументы,
# ops — операнды; все — без кавычек и `\` ("-delete" и --out""put — те же опции). Перенаправления с
# целями и хвост-комментарий — не слова команды.
parse() {
    opts=() optargs=() ops=()
    local j skip=0 arg=0 w b
    for ((j = i + 1; j < ${#words[@]}; j++)); do
        w=${words[j]} b=${bare[j]}
        if ((skip)); then
            skip=0
            continue
        fi
        [[ $w == \#* ]] && break
        if [[ $w =~ $re_redirect ]]; then
            [[ $w =~ $re_no_target ]] && skip=1
            continue
        fi
        if ((arg)); then
            arg=0
            optargs+=("$b")
        elif [[ $b == -?* ]]; then
            opts+=("$b")
            [[ -n $1 && $b =~ $1 ]] && arg=1
        else
            ops+=("$b")
        fi
    done
}
has() { # has <regex> — среди опций команды есть такая
    local o
    for o in ${opts[@]+"${opts[@]}"}; do [[ $o =~ $1 ]] && return 0; done
    return 1
}
only() { # only <regex> — все опции команды такие
    local o
    for o in ${opts[@]+"${opts[@]}"}; do [[ $o =~ $1 ]] || return 1; done
    return 0
}

while IFS= read -r seg; do
    plain=${seg//$'\037'/ }
    plain=${plain//$bel/}
    flat=${plain//[\"\'\\]/} # кавычки и `\` внутри пути его не прячут: git-ho""oks
    [[ $plain =~ $hook || $flat =~ $hook ]] || continue
    while IFS= read -r target; do
        [[ $target =~ $hook ]] && block "запись в scripts/git-hooks или .git/config"
    done <<<"$(printf '%s\n%s' "$plain" "$flat" | grep -oE '>>?[[:space:]]*[^[:space:]>]+' | sed -E 's/^>>?[[:space:]]*//')"
    IFS=$'\037' read -r -a raw <<<"$seg"
    words=() bare=()
    for w in ${raw[@]+"${raw[@]}"}; do
        [ -n "$w" ] || continue
        words+=("$w")
        w=${w//$bel/}
        bare+=("${w//[\"\'\\\$]/}")
    done
    # cp, sort, find и awk проходят только в виде, который guard разобрал целиком: без подстановок.
    expands=0
    [[ $seg == *$bel* ]] && expands=1
    i=0
    while ((i < ${#words[@]})) && [[ ${words[i]} == *=* || ${bare[i]} =~ ^(sudo|env|command)$ ]]; do
        # Значение переменной команда может исполнить (GIT_PAGER, GIT_SSH_COMMAND): путь хука в нём — не чтение.
        [[ ${words[i]} == *=* && ${bare[i]#*=} =~ $hook ]] && block "переменная с путём scripts/git-hooks или .git/config"
        i=$((i + 1))
    done
    cmd=${bare[i]-}
    name=${cmd##*/}
    case "$name" in
        git | git-*)
            # Бинарь подкоманды (git-commit, и по пути /usr/lib/git-core/…) — та же подкоманда git.
            sub="" skip=0 cfg=""
            [[ $name == git-* ]] && sub=${name#git-}
            [ -n "$sub" ] || for w in "${bare[@]:i+1}"; do # подкоманда — первое слово не-флаг, кроме значений -C/-c…
                case "$w" in
                    -c) skip=2 ;;
                    -C | --git-dir | --work-tree | --namespace | --config-env | --attr-source) skip=1 ;;
                    -*) ;;
                    *)
                        [[ $skip == 2 && $w =~ $hook ]] && cfg=$w
                        if ((skip)); then skip=0; else sub=$w && break; fi
                        ;;
                esac
            done
            parse ''
            # --output пишет вывод в файл, add --chmod=-x снимает право на запуск, grep -O запускает команду,
            # значение -c git исполняет (core.pager, core.sshCommand) — путь хука в нём не чтение.
            [[ -z $cfg && $sub =~ ^($git_readers)$ ]] && ! has '^(--ou|--chmod=-)' &&
                ! { [ "$sub" = grep ] && has '^(-O|--open-files-in-pager)'; } && continue
            ;;
        sed)
            # -i правит файл на месте; команды w и e скрипта пишут в файл и запускают команду — скрипт
            # (-e или первый операнд) не называет каталог хуков и .git/config.
            parse '^(-[a-zA-Z]*[efl]|--(e[a-z]*|fi[a-z]*|l[a-z-]*))$'
            scripts=("${ops[0]-}")
            has '^(-[a-zA-Z]*e|--e)' && scripts=(${opts[@]+"${opts[@]}"} ${optargs[@]+"${optargs[@]}"})
            has '^(-[a-zA-Z]*i|--i)' || [[ "${scripts[*]}" == *git-hooks* || "${scripts[*]}" == *.git/config* ]] || continue
            ;;
        awk | gawk)
            # Программа awk пишет и запускает через `>`, `|` и system, а файл-операнд знает как FILENAME;
            # опции — только разделитель и переменные: остальные читают программу из файла или пишут.
            parse '^(-F|-v|--field-separator|--assign)$'
            prog=${ops[0]-}
            ((!expands)) && only "$re_awk_opts" && [[ -n $prog && ! $prog =~ $re_awk_io ]] && continue
            ;;
        sort)
            parse ''
            ((expands)) || has '^(-[a-zA-Z]*o|--o|--co)' || continue # вывод в файл и --compress-program
            ;;
        find)
            parse ''
            ((expands)) || has '^-(exec|execdir|ok|okdir|delete|fprint|fprint0|fprintf|fls)$' || continue
            ;;
        rg)
            parse ''
            has '^--pre(=|$)' || continue # --pre запускает команду на каждом файле (--pretty — нет)
            ;;
        man)
            parse ''
            has '^(-[a-zA-Z]*[PH]|--(pager|html))' || continue # -P, --pager — пейджер, -H, --html — браузер
            ;;
        cp)
            # Копия из хука — чтение. Назначение — последний операнд; опции — только без аргумента:
            # иначе последним словом встал бы аргумент опции (-S x, -t каталог).
            parse ''
            ((!expands && ${#ops[@]} > 1)) && only "$re_cp_opts" && ! [[ ${ops[${#ops[@]} - 1]} =~ $hook ]] && continue
            ;;
        bash | sh)
            # bash -n только разбирает скрипт; с другими опциями (+n, -c) он исполняет.
            parse ''
            if [[ ${bare[i + 1]-} == -n ]]; then
                plain_ops=1
                for o in ${ops[@]+"${ops[@]}"}; do [[ $o =~ $re_path ]] || plain_ops=0; done
                ((${#opts[@]} == 1 && plain_ops)) && continue
            elif [[ ${bare[i + 1]-} =~ $run_hook ]]; then
                continue
            fi
            ;;
        *)
            [[ $cmd =~ $run_hook ]] && continue
            # Текст echo и printf рядом с интерпретатором — его код (echo '…' | bash), а не чтение.
            [[ $name =~ ^($readers)$ ]] && ! { ((interp)) && [[ $name =~ ^(echo|printf)$ ]]; } && continue
            ;;
    esac
    block "${name:-команда} по scripts/git-hooks или .git/config"
done <<<"$(printf '%s' "$text" | tr -d '\001-\007\037' | join_lines | split_words |
    sed -e 's/>|/>/g' -e 's/>&/>/g' | tr ';&|()`' '\n\n\n\n\n\n' | tr '\001\002\003\004\005\006' ';&|()`')"

# Флаги git ищем вне кавычек, по сегментам: строки (с учётом `\` в конце) и части между
# ; & | ( ) и обратными кавычками. Перенаправление с целью — не слово команды и не её граница
# (git commit 2>&1 -n, git 2>/dev/null send-pack); слово после `<<<` — текст для команды (bash<<<git).
# Подстановка $(…), <(…) или `…` среди слов команды её тоже не режет: команда проверяется и как есть —
# это видит команды внутри подстановки, — и с подстановками, свёрнутыми в слово (git commit -m $(…) -n).
flags=$(printf '%s' "$norm" | tr '\n' ';' | sed -E -e 's/"([^"\\]|\\.)*"//g' -e "s/'[^']*'//g")
segments=$({
    printf '%s\n' "$flags"
    printf '%s\n' "$flags" | sed -E -e ':a' -e 's/[$<>]\([^()]*\)/ _ /' -e 'ta' -e 's/`[^`]*`/ _ /g'
} | sed -E -e 's/<<</ /g' -e 's/[0-9]*(&>>?|>>?[&|]?|<<?&?)[[:space:]]*[^[:space:];&|()`<>]*/ /g' |
    tr ';&|()`' '\n\n\n\n\n\n')
# «Читатель» (кроме echo и printf) слов не исполняет, если в команде нет подстановки: git-<подкоманда> среди его
# слов — данные (man git-send-pack, ls /usr/lib/git-core/git-send-pack).
substituted=0
[[ $cmd == *'$('* || $cmd == *'<('* || $cmd == *'`'* ]] && substituted=1
while read -r -a words; do
    git=0 sub="" skip=0 prev=""
    j=0
    while ((j < ${#words[@]})) && [[ ${words[j]} == *=* || ${words[j]} =~ ^(sudo|env|command)$ ]]; do j=$((j + 1)); done
    name=${words[j]-}
    name=${name##*/}
    ((!interp && !substituted)) && [[ $name =~ ^($readers|man)$ && ! $name =~ ^(echo|printf)$ ]] && continue
    for w in ${words[@]+"${words[@]}"}; do
        w=${w%\\}
        # git, по пути или бинарём подкоманды (git-commit), — и вложенный в строку git bisect run, git
        # submodule foreach, git rebase -x и приклеенный к «=» или короткой опции (--exec=git, -c core.editor=git,
        # GIT_EDITOR=git, -xgit): каждый — своя команда со своей подкомандой. git-core — каталог бинарей git.
        v=${w##*=}
        [[ $v =~ ^-[[:alpha:]](.+)$ ]] && v=${BASH_REMATCH[1]}
        if [[ $v == git || $v == */git ]]; then
            git=1 sub="" skip=0
            continue
        elif [[ $v != -* && ${v##*/} == git-?* && ${v##*/} != git-core ]]; then
            w=$v
            git=1 sub=${w##*/} skip=0
            sub=${sub#git-}
            [[ $sub == send-pack || $sub == http-push ]] && block "git $sub"
            continue
        fi
        if [ "$git" = 0 ]; then
            :
        elif [ -z "$sub" ]; then # подкоманда — первое слово не-флаг, кроме значений -C/-c/--git-dir…
            case "$w" in
                -C | -c | --git-dir | --work-tree | --namespace | --config-env | --attr-source) skip=1 ;;
                -*) ;;
                *)
                    if [ "$skip" = 1 ]; then skip=0; else sub=$w; fi
                    [[ $sub == send-pack || $sub == http-push ]] && block "git $sub"
                    ;;
            esac
        else
            case "$w" in
                --no-veri*) block "git $sub $w" ;;
                # -n в кластере коротких флагов до первого флага со значением: -an, -nm; -uno — значение -u.
                -[a-zA-Z]*) [[ $sub == commit && ${w%%[mFcCtSu]*} == *n* ]] && block "git commit $w" ;;
            esac
            # Имя секции git не различает по регистру.
            [[ $sub == config && $prev =~ ^(--)?(remove|rename)-section$ && $w == [Cc][Oo][Rr][Ee] ]] &&
                block "git config $prev $w"
        fi
        prev=$w
    done
done <<<"$segments"
exit 0
