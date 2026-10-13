---
status: in-progress
parent: —
blocked-by: —
---

# close-gate-gaps — гейт без выключателей, порог покрытия по факту, мутационный прогон

## Цель
Закрыть пункты трека gate программы `reaudit-to-nine`: самозащита гейта не выключается окружением
вызывающего, ни один Go-файл и ни одна директива не выводят код из гейта, порог покрытия равен факту,
шаги гейта не пропускают без проверки, `make new-domain` не оставляет полусостояния, а силу тестов
держит мутационный прогон `make mutate`.

## Контекст
- Программа — `.claude/plans/reaudit-to-nine.md` (трек gate, волна 1): владение файлами, решения
  владельца, метод (коммит на шаг, валидатор после трека). Ветка `nine/gate`, worktree
  `N/wt-gate`, тяжёлые прогоны — под `flock N/gate.lock` (`N` = `/home/mm/.cache/stit-audit-program/nine`).
- Находки — разделы `### <ID>` отчётов `R/template-{enforcement,tests,code,security}.md`
  (`R` = `/home/mm/.cache/stit-audit-program/reaudit`), эксперименты — `R/tpl-enf/`, `R/tpl-tests/`;
  формулировки пунктов бэклога — `git show ab46f45:.claude/plans/backlog.md`.
- Образец самозащиты от окружения — golibinfra v1.5.0: `fce672b` (MAKECMDGOALS), `9244d64` (`bash -p`,
  `unexport` переменных оболочки, scrub окружения в pre-push).
- Опыты с `MAKECMDGOALS` на настоящем гейте запускают его рекурсивно (инцидент аудитора) — только тестом
  с поддельным `go` в PATH или в отдельном worktree. Логи и заметки трека — `N/gate/`.

## Не входит
- RENF-03 (хуки не подключены — трек lifecycle), RENF-10…16, кроме строки «нет каталога» migrations-check
  и `MOCKERY_*` из RENF-16 (fail-open того же шага); RTSTT-02…10 (identity и бэклог).
- Шаг `make mutate` в `/release` (трек harness), `scripts/git-hooks/commit-msg` сверх строки интерпретатора.
- Существующие миграции, `go.mod`/`go.sum`, `pkg/policy`, `internal/platform/authgate/`.

## Критерии приёмки
- [x] **AC1** — RENF-01, переменные make: `make -i check MAKECMDGOALS=x`, `MAKECMDGOALS=x make -n check`,
  `MAKECMDGOALS=x MAKEFLAGS=i make check` на красном дереве печатали `✓ check clean` — теперь отказ make
  до первого шага; переменные `MAKE*` пушащего (`MAKECMDGOALS=x GOFLAGS=… git push`) до гейта pre-push не
  доходят. Проверка: `go test ./internal/archtest -run 'TestMakeCheckCannotBeForged|TestPrePushGatesPushedCommit'`
  → PASS; те же тесты на Makefile и pre-push базы → FAIL
  - Подтверждено: на базе FAIL 10 форм (`-i`/`-n` с `MAKECMDGOALS` из командной строки и окружения,
    `MAKEFLAGS=i` с ней, `GATE`, `SHELL`, `GOFLAGS` с ней, `MAKEFILE_LIST`, `-i --eval 'x: check'`) и
    pre-push (`goals=x make=true`); на ветке оба теста PASS (`N/gate/s1-red-list.txt`, `s1-green.log`)
- [x] **AC2** — RENF-01, окружение оболочки: с `BASH_ENV=<файл с exit 0>`, `SHELLOPTS=noexec` и
  экспортированной функцией `set` шаги-скрипты гейта на красном дереве остаются красными, pre-push не
  пропускает красный коммит, commit-msg — сообщение не по форме. Проверка: тесты окружения оболочки в
  `internal/archtest` → PASS; на базе → FAIL
  - Подтверждено: на базе `BASH_ENV`, функция `set` и `SHELLOPTS=noexec` пропускают commit-msg, pre-commit и
    `make lint` (код 0, `N/gate/s2-base-forgeries.txt`), функции `read`/`env` — pre-push; на ветке
    `TestGateStepsIgnoreShellEnvironment`, `TestHooksIgnoreCallerEnvironment`, `TestPrePushGatesPushedCommit` → PASS
- [x] **AC3** — RENF-02: инъекции N-b-unit, N-b-prod, N-b-migrate (на базе `✓ check clean`) — гейт
  красный: ограничение сборки, кроме `//go:build integration` в `*_test.go`, — отказ archtest; шаг build
  собирает `./...` без тегов, как образ. Проверка: инъекция → `go test ./internal/archtest` FAIL с файлом,
  `make build` на N-b-migrate → FAIL
  - Подтверждено: база с N-b-unit, N-b-prod, N-b-migrate — `make check` → `✓ check clean`
    (`N/gate/s3-base-nb.log`); ветка — FAIL на test-int, `TestBuildConstraints` называет три файла
    (`s3-branch-nb.log`); `make build` на N-b-migrate → `undefined: lockHint`, FAIL
- [x] **AC4** — RTSTT-01: эксперименты CV01, CV04, CV05, CV15 (на базе `✓ check clean`) — `make check`
  красный на cover-check; порог — 100, непокрываемое — явный список `cover.sh`, запись без непокрытого
  блока — отказ. Проверка: эксперименты под flock → `FAIL: coverage … < floor 100%`; тест списка → PASS
  - Подтверждено: база — CV01 95,39 %, CV04 99,74 %, CV05 98,58 %, CV15 96,32 % → `✓ check clean`
    (`N/gate/cov-base/`); ветка — те же доли → `FAIL: … < floor 100%` с непокрытыми блоками
    (`N/gate/cov-branch/`); `TestCoverCheck` → PASS
- [x] **AC5** — RENF-04, RSECT-07: migrations-check — каталог с файлами не по маске → FAIL (было
  `OK (пусто)`); `DELETE`, `UPDATE … SET`, `SET NOT NULL`, `CREATE INDEX` без `CONCURRENTLY` по таблице не
  из этого файла, `CREATE EXTENSION`, `DO` без маркера → FAIL; новая таблица с индексом и миграции
  0001–0005 → OK. Проверка: `go test ./internal/archtest -run TestMigrationsCheck` → PASS; `make check` →
  migrations OK
  - Подтверждено: на скрипте базы `TestMigrationsCheck` — 10 из 17 случаев FAIL (нет каталога, три
    формы имён, DELETE, UPDATE, SET NOT NULL, индекс по существующей таблице, расширение, DO;
    `N/gate/s5-red.txt`), на ветке — 17 из 17 PASS; `make check` → `OK: migrations (5 пар…)`
- [x] **AC6** — RENF-05, RENF-06: неотслеживаемый слабый `.golangci.toml` больше не заслоняет
  `.golangci.yml`; `// #nosec` и `//revive:disable` ничего не глушат (N-n-nosec, N-n-revive на базе —
  `✓`). Проверка: инъекции → `make lint` FAIL и archtest FAIL
  - Подтверждено: база — `make lint` OK со слабым `.golangci.toml` (`gate/altconfig2.sh`), с `// #nosec` у
    md5 и с `//revive:disable:var-naming` (`N/gate/s6-base-*.txt`); ветка — те же инъекции: `make lint` FAIL
    (errcheck, forbidigo, gosec), `TestLinterDirectives` называет обе директивы; `TestLintUsesItsConfig` → PASS
- [x] **AC7** — RENF-07: после `make gen` без `git add` `make gen-check` → OK (было FAIL «запусти make
  gen»); ручная правка мока в дереве (M04b-unstaged) → FAIL с файлом (было `✓`, правка стёрта молча);
  `MOCKERY_*` из окружения шаг не меняют. Проверка: сценарии `R/tpl-enf/gate/gen-roundtrip.sh` и M04b
  - Подтверждено: база — после `make gen` без `git add` FAIL «запусти make gen», правка мока в дереве —
    OK и стёрта, `MOCKERY_CONFIG` с выводом в `mocks2` при устаревшем моке — OK (`N/gate/s7-base-*.txt`);
    ветка — три пути gen-roundtrip OK, обе подделки FAIL с файлом (`s7-branch-*.txt`);
    `TestGenCheckComparesMocksWithTree` — на Makefile базы 4 из 4 FAIL, на ветке PASS
- [x] **AC8** — RENF-08, RENF-09: тест без утверждений в `pkg/` и `cmd/` (N-t-pkg, N-t-cmd, на базе `✓`)
  ловит `TestTestsAssert`; нарушение extra-правил gofumpt (N-fmt2) ловит `make fmt-check` и чинит
  `make fmt`. Проверка: инъекции
  - Подтверждено: N-t-pkg, N-t-cmd — база `✓ check clean`, ветка FAIL (S8); N-fmt2 на базе — `fmt-check`
    OK, `make fmt` не чинит, lint FAIL; на ветке `fmt-check` FAIL, `make fmt` чинит (`a, b string`), lint OK;
    `TestFmtAppliesLintRules` — на Makefile базы FAIL, на ветке PASS
- [ ] **AC9** — RCOD-05, RCOD-06: `make new-domain NAME=api_key` → `make check` зелёный (было: lint
  против `TestHTTPTypes`); имена `repo`, `name`, `valid`, `got`, `row`, `id`, `uid` и занятая таблица
  (`password_reset`) — отказ до первой правки, `git status` пуст; сбой после первой правки — откат.
  Проверка: сценарии `R/template-code.md` § 8
- [ ] **AC10** — `make mutate`: мутации спецификации — по тексту кода (без учёта пробелов, ровно одно
  вхождение), после обрыва прогон продолжается с места, эквивалентные — явным списком, итог — выжившие
  списком и код 1 при новых выживших, устаревший якорь — в отчёте, прогон не падает. Проверка: подмножество
  под flock — убитый, эквивалентный, выживший (код 1), обрыв и продолжение
- [ ] **AC11** — итог по углу enforcement: 89 обязательных инъекций аудитора пойманы (7 непойманных на
  базе — тоже), открытых fail-open шагов нет (имена миграций, второй конфиг lint, `MOCKERY_CONFIG`).
  Проверка: перегон кейсов `R/tpl-enf/run.py` и fail-open сценариев → поймано

## Шаги
- [x] **S1** — Самозащита `make check` не зависит от `MAKECMDGOALS`: `MAKECMDGOALS` и `MAKEFILE_LIST`
  вызывающего — отказ make, отказы гейта — при разборе и первой строкой рецепта; pre-push снимает `MAKE*`
  - Проверка: RED — новые формы на базе; GREEN — тесты PASS; `make check` → ✓ · Закрывает: AC1 · Зависит от: —
  - Сделано: `gate_forged` — отказ make при `MAKECMDGOALS`/`MAKEFILE_LIST` вызывающего, `gate_refuse` — при
    разборе по цели и первой строкой `check::`; pre-push снимает `MAKE*`, `MFLAGS`, `GNUMAKEFLAGS`; 44 формы
    `TestMakeCheckCannotBeForged` (go-заглушка в PATH — подделка краснеет за секунды, без рекурсии) и
    `TestPrePushGatesPushedCommit` → PASS; `make check` → ✓
- [x] **S2** — Окружение оболочки не подделывает гейт: Makefile снимает `BASH_ENV` и соседей с рецептов,
  хуки стартуют `bash -p` и снимают их с проверок, которые запускают
  - Проверка: RED → GREEN тестов окружения оболочки; `make check` → ✓ · Закрывает: AC2 · Зависит от: S1
  - Сделано: Makefile — `unexport` `BASH_ENV`, `ENV`, `SHELLOPTS`, `BASHOPTS`, `CDPATH`, `GLOBIGNORE` и
    `BASH_FUNC_*`; хуки — `#!/bin/bash -p`, pre-push и pre-commit перезапускаются без переменных make и
    оболочки; тесты на шаги lint и test (и с `SHELL=/bin/bash`), хуки и pre-push → PASS; `make check` → ✓
- [x] **S3** — Ограничение сборки — только `integration` в `*_test.go` (archtest); build — `go build ./...`
  - Проверка: N-b-* на базе `✓`, на ветке — красный; `make check` → ✓ · Закрывает: AC3 · Зависит от: —
  - Сделано: `TestBuildConstraints` (`generated_test.go`, рядом с заголовком сгенерированного): строка
    ограничения — только `//go:build integration` у `*_test.go`, файл не исключён из сборки гейта
    (`build.MatchFile`: суффиксы ОС и архитектуры); build — `go build ./...`; `make check` → ✓
- [x] **S4** — Порог покрытия 100 и явный список непокрываемого в `cover.sh`
  - Проверка: CV01, CV04, CV05, CV15 под flock — красные; `make check` → ✓ · Закрывает: AC4 · Зависит от: —
  - Сделано: `COVER_FLOOR := 100`; в `cover.sh` — список непокрываемого (пуст: «<файл>:<текст блока>» с
    причиной, запись без неисполненного блока — отказ), отказ называет блоки; `TestCoverCheck` (новый
    `steps_test.go` — тесты шагов гейта); правило `tests-pipeline`, CLAUDE.md § Команды; `make check` → ✓
- [x] **S5** — migrations-check: имена не по маске, каталог, классы эффекта RSECT-07; правило и reference
  - Проверка: `TestMigrationsCheck` RED → GREEN; `make check` → ✓ · Закрывает: AC5 · Зависит от: —
  - Сделано: «пусто» — только пустой каталог, нет каталога — FAIL со строкой, вне маски — и каталоги;
    классы эффекта RSECT-07 и `DO`; выпущенные 0001 и 0003 — списком (только разрушительное); rule
    `sqlc-and-migrations`, `infra-impact`, `scaffold/references/migration.md` (пример индекса —
    `CONCURRENTLY` своей миграцией); `make check` → ✓
- [x] **S6** — lint: `--config .golangci.yml`, gosec без `#nosec`, директивы revive — отказ archtest
  - Проверка: N-n-nosec, N-n-revive, слабый `.golangci.toml` — красные; `make check` → ✓ · Закрывает: AC6 · Зависит от: —
  - Сделано: `lint.sh` и `golangciEnabled` (harness_test.go) — `--config .golangci.yml`, `lint.sh` снимает и
    `GOLANGCI_LINT_CACHEPROG`; gosec `nosec: true`; `TestLinterDirectives` (директивы revive и gosec) и
    `TestLintUsesItsConfig`; `make check` → ✓
- [x] **S7** — gen-check сверяет моки с деревом до прогона mockery; `MOCKERY_*` окружения — не для шага
  - Проверка: gen-roundtrip и M04b-unstaged; `make check` → ✓ · Закрывает: AC7 · Зависит от: —
  - Сделано: снимок `cksum` моков дерева до и после mockery вместо `git diff` с индексом; новый мок вне
    индекса — отказ с `git add`; `unexport MOCKERY_*`; тест на заглушках; troubleshooting
    `docs/contributing.md`; `make check` → ✓
- [x] **S8** — `TestTestsAssert` — все тесты модуля
  - Проверка: N-t-pkg, N-t-cmd на базе `✓`, на ветке — красный; `make check` → ✓ · Закрывает: AC8 · Зависит от: —
  - Сделано: все `*_test.go` модуля; домены и платформа — прежние утверждения (require, кит), остальным —
    ещё `t.Error*`, `t.Fatal*`, `t.Fail*` и `t`, отданный хелперу-проверке (`archkit.Check`,
    `testkit.MigrationLadder`); база с N-t-pkg и N-t-cmd — `✓ check clean` (`N/gate/s8-base-nt.log`), ветка —
    FAIL с обоими файлами (`s8-branch-nt.log`); тест домена на одном `t.Fatalf` — по-прежнему отказ;
    правило `tests-pipeline`; `make check` → ✓
- [x] **S9** — `make fmt` и `fmt-check` — `gofumpt -extra`
  - Проверка: N-fmt2 → fmt-check FAIL, `make fmt` чинит; `make check` → ✓ · Закрывает: AC8 · Зависит от: —
  - Сделано: `gofumpt -extra` в `fmt` и `fmt-check` (как extra-rules lint); `TestFmtAppliesLintRules`;
    `make check` → ✓
- [ ] **S10** — Имя домена с инициализмом: скелет по правилу revive, archtest сверяет префикс без регистра
  - Проверка: `make new-domain NAME=api_key` → `make check` ✓ (в отдельном worktree) · Закрывает: AC9 · Зависит от: —
- [ ] **S11** — `make new-domain`: негодное имя и занятая таблица — отказ до первой правки; сбой после — откат
  - Проверка: сценарии RCOD-06 → отказ, `git status` пуст · Закрывает: AC9 · Зависит от: S10
- [ ] **S12** — `make mutate`: раннер в `scripts/tools/`, спецификация аудитора по текущему коду, документ —
  rule `tests-pipeline`
  - Проверка: подмножество под flock (AC10) · Закрывает: AC10 · Зависит от: —
- [ ] **S13** — Перегон экспериментов трека и гейт
  - Проверка: кейсы AC11 → поймано; `make check` → `✓ check clean` · Закрывает: AC11 · Зависит от: S1–S12

## Решения
- Шаги-скрипты гейта зовутся `bash scripts/…` из рецептов, их защищает `unexport` Makefile; `bash -p` —
  у хуков, которые git запускает сам. Хуки перезапускают себя без переменных make и оболочки (без маркера:
  условие — сами переменные), а не чистят окружение каждой проверки.
- `TestTestsAssert` вне доменов и платформы засчитывает `t.Error*`/`t.Fatal*` и `t`, отданный хелперу:
  так утверждают archtest (все нарушения разом), `archkit.Check` и `testkit.MigrationLadder`; в доменах и
  платформе — прежняя строгость (require и кит, правило `tests-pipeline`), гейт не ослаблен.
- gen-check: моки сверяются с деревом до mockery (как остальной гейт — дерево), новый мок вне индекса —
  отказ по-прежнему: без `git add` он не уйдёт в коммит, а pre-push увидит его отсутствие поздно.
- `internal/archtest/harness_test.go` (файл трека harness): `golangciEnabled` — `--config .golangci.yml`,
  иначе `TestNolintNamesEnabledLinters` судит по чужому конфигу (третий вызов RENF-05).
- migrations-check: индекс по таблице, созданной в том же файле, — не эффект (иначе маркер у каждой новой
  таблицы); `CONCURRENTLY` — не эффект (рецепт `migration.md`); `DO` — эффект: его код машина не видит.
  Выпущенные 0001 (`CREATE EXTENSION`) и 0003 (индекс по существующей таблице) — списком имён в скрипте:
  существующие миграции не правим; у них — только разрушительные операторы.
- `.claude/rules/infra-impact.md` (вне таблицы владения): строка «машина держит одно…» — под новые классы,
  иначе правило врёт о машине.
- Непокрываемое — запись «<файл>:<текст из строк блока>» в `cover.sh`, а не позиция блока: правка выше в
  файле её не ломает; блок из списка идёт исполненным (функция из одних таких блоков — не «без тестов»).
- Правило ограничений сборки — в `generated_test.go` рядом с `TestGeneratedHeaderOnlyInGeneratorOutput`
  (так советует аудит, файл трека), а не в `archtest_test.go`; там — строка в комментарии пакета.
- commit-msg (файл трека harness): только строка интерпретатора `#!/bin/bash -p` — без неё `BASH_ENV`
  пропускал сообщение не по форме, RENF-01 не закрыть.
- pre-push снимает переменные по классам (`MAKE*`, `MFLAGS`, `GNUMAKEFLAGS`, переменные оболочки), а не
  `env -i` с белым списком — как golibinfra v1.5.0: белый список срезал бы `GOPRIVATE`, прокси и
  настройки Docker, без которых гейт не идёт.

## Handoff
—

## Где мы сейчас
S1–S9 закрыты: самозащита гейта не зависит от окружения вызывающего, ограничения сборки и директивы
линтеров не выводят код из гейта, порог покрытия — 100, migrations-check и gen-check без fail-open. Эксперименты: база —
worktree `N/wt-gate-x` (e8ea150, снять в S13), срез ветки с незакоммиченным шагом — временный `N/wt-gate-y`
(раннеры аудитора откатывают дерево к HEAD). Следующее: S10 — инициализм в имени домена: скелет по
правилу revive, archtest без регистра (RED — `make new-domain NAME=api_key` в отдельном worktree).
