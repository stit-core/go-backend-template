#!/usr/bin/env bash
# Валидатор migrations/: каждая NNNN — ровно одна пара .up.sql/.down.sql,
# нумерация непрерывна с 0001, посторонних файлов нет. Шаг make check.
set -euo pipefail

dir="${1:-migrations}"
fail=0

nums=$(find "$dir" -maxdepth 1 -type f -name '[0-9][0-9][0-9][0-9]_*.sql' -printf '%f\n' |
	sed -E 's/^([0-9]{4})_.*/\1/' | sort -u)
if [ -z "$nums" ]; then
	echo "OK: migrations (пусто)"
	exit 0
fi

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

extra=$(find "$dir" -maxdepth 1 -type f -printf '%f\n' | grep -vE '^[0-9]{4}_[a-z0-9_]+\.(up|down)\.sql$' || true)
if [ -n "$extra" ]; then
	echo "FAIL: migrations: файлы вне конвенции NNNN_snake_case.{up,down}.sql:"
	echo "$extra" | sed 's/^/  /'
	fail=1
fi

if [ "$fail" -eq 0 ]; then
	echo "OK: migrations ($(echo "$nums" | wc -l) пар, нумерация непрерывна)"
fi
exit "$fail"
