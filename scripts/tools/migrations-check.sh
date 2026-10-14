#!/usr/bin/env bash
# Валидатор migrations/: каждая NNNN — ровно одна пара .up.sql/.down.sql, нумерация непрерывна с 0001,
# посторонних файлов и каталогов нет; down не пуст; up с эффектом для выкатки — только с маркером
# `-- infra-handoff: <…>`. Эффект: разрушительный оператор (DROP, TRUNCATE, ALTER … TYPE, RENAME), правка
# данных (DELETE, UPDATE … SET), код, которого машина не видит (DO, CALL, SELECT верхнего уровня), долгий lock
# (SET NOT NULL, CREATE INDEX без CONCURRENTLY по таблице не из этого файла), расширение (CREATE EXTENSION).
# Тело $$…$$ (функция, DO) — не оператор файла: CREATE FUNCTION его только объявляет. Шаг make check.
set -euo pipefail

dir="${1:-migrations}"
fail=0

if [ ! -d "$dir" ]; then
	echo "FAIL: migrations: нет каталога $dir"
	exit 1
fi
if [ -z "$(find "$dir" -mindepth 1 -maxdepth 1 -printf '%f\n')" ]; then
	echo "OK: migrations (пусто)"
	exit 0
fi

nums=$(find "$dir" -maxdepth 1 -type f -name '[0-9][0-9][0-9][0-9]_*.sql' -printf '%f\n' |
	sed -E 's/^([0-9]{4})_.*/\1/' | sort -u)

expect=1
for n in $nums; do
	printf -v want '%04d' "$expect"
	if [ "$n" != "$want" ]; then
		echo "FAIL: migrations: ожидался номер $want, найден $n (нумерация непрерывна с 0001)"
		fail=1
		expect=$((10#$n))
	fi
	ups=$(find "$dir" -maxdepth 1 -name "${n}_*.up.sql" | wc -l)
	downs=$(find "$dir" -maxdepth 1 -name "${n}_*.down.sql" | wc -l)
	if [ "$ups" -ne 1 ] || [ "$downs" -ne 1 ]; then
		echo "FAIL: migrations: $n — up=$ups down=$downs (ровно одна пара)"
		fail=1
	fi
	expect=$((expect + 1))
done

# golang-migrate читает только файлы этого каталога по маске: остальное — не миграция, а подкаталог — не
# накатится никогда.
extra=$(find "$dir" -mindepth 1 -maxdepth 1 -printf '%f\n' | grep -vE '^[0-9]{4}_[a-z0-9_]+\.(up|down)\.sql$' || true)
if [ -n "$extra" ]; then
	echo "FAIL: migrations: файлы и каталоги вне конвенции NNNN_snake_case.{up,down}.sql:"
	echo "$extra" | sed 's/^/  /'
	fail=1
fi

for f in "$dir"/[0-9][0-9][0-9][0-9]_*.down.sql; do
	[ -e "$f" ] || continue
	if ! grep -q '[^[:space:]]' "$f"; then
		echo "FAIL: migrations: ${f##*/} пустой — down возвращает схему к состоянию до up (миграция только данных — \`-- noop\` с причиной)"
		fail=1
	fi
done

# Выпущенные миграции с операторами классов данных, lock и расширения: выпущенную миграцию не правят, её
# эффект принят выкаткой — у них проверяются только разрушительные операторы.
released=" 0001_users.up.sql 0003_email_verifications_idempotency.up.sql "

# Up с эффектом нужен порядок или окно выкатки: маркер делает это видимым в файле, а INFRA-HANDOFF-блок —
# в хабе (rule infra-impact). Операторы — по строке, в нижнем регистре, без комментариев, строк в кавычках и
# тел $$…$$.
for f in "$dir"/[0-9][0-9][0-9][0-9]_*.up.sql; do
	[ -e "$f" ] || continue
	grep -qE '^[[:space:]]*--[[:space:]]*infra-handoff:[[:space:]]*[^[:space:]]' "$f" && continue
	only_destructive=""
	[[ $released == *" ${f##*/} "* ]] && only_destructive=1
	hits=$(sed -e 's/--.*//' -e "s/'[^']*'//g" "$f" | tr '\n' ' ' | tr '[:upper:]' '[:lower:]' |
		awk '{
			out = ""
			while (match($0, /\$[a-z0-9_]*\$/)) {
				tag = substr($0, RSTART, RLENGTH); out = out substr($0, 1, RSTART - 1) tag "…" tag
				$0 = substr($0, RSTART + RLENGTH); i = index($0, tag); $0 = i ? substr($0, i + length(tag)) : ""
			}
			print out $0
		}' | tr ';' '\n' |
		awk -v only_destructive="$only_destructive" '
			{
				stmt[NR] = $0
				if (match($0, /create[[:space:]]+table[[:space:]]+(if[[:space:]]+not[[:space:]]+exists[[:space:]]+)?[^[:space:](]+/)) {
					t = substr($0, RSTART, RLENGTH); sub(/.*[[:space:]]/, "", t); gsub(/"/, "", t); created[t] = 1
				}
			}
			END {
				w = "([^a-z0-9_]|$)"
				destructive = "(^|[^a-z0-9_])(drop|truncate|rename)" w "|alter[[:space:]]+(column[[:space:]]+)?[^[:space:]]+[[:space:]]+(set[[:space:]]+data[[:space:]]+)?type" w
				effect = "(^|[^a-z0-9_])delete[[:space:]]+from" w "|(^|[^a-z0-9_])update[[:space:]]+(only[[:space:]]+)?[^[:space:]]+([[:space:]]+(as[[:space:]]+)?[a-z0-9_]+)?[[:space:]]+set" w \
					"|^[[:space:]]*(do|call|select)" w "|set[[:space:]]+not[[:space:]]+null" w "|create[[:space:]]+extension" w
				create_index = "create[[:space:]]+(unique[[:space:]]+)?index[[:space:]]+(concurrently[[:space:]]+)?((if[[:space:]]+not[[:space:]]+exists[[:space:]]+)?[^[:space:]]+[[:space:]]+)?on[[:space:]]+(only[[:space:]]+)?[^[:space:](]+"
				for (i = 1; i <= NR; i++) {
					s = stmt[i]
					if (s ~ destructive) { print s; continue }
					if (only_destructive) continue
					if (s ~ effect) { print s; continue }
					if (match(s, create_index) && s !~ /index[[:space:]]+concurrently/) {
						t = substr(s, RSTART, RLENGTH); sub(/.*[[:space:]]/, "", t); gsub(/"/, "", t)
						if (!(t in created)) print s
					}
				}
			}')
	if [ -n "$hits" ]; then
		echo "FAIL: migrations: ${f##*/} — оператор с эффектом для выкатки без маркера \`-- infra-handoff: <что сделать инфре>\`:"
		echo "$hits" | sed -E 's/[[:space:]]+/ /g; s/^ //; s/^/  /'
		fail=1
	fi
done

if [ "$fail" -eq 0 ]; then
	echo "OK: migrations ($(echo "$nums" | wc -l) пар, нумерация непрерывна)"
fi
exit "$fail"
