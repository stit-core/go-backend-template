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
- [x] **AC9** — RCOD-05, RCOD-06: `make new-domain NAME=api_key` → `make check` зелёный (было: lint
  против `TestHTTPTypes`); имена `repo`, `name`, `valid`, `got`, `row`, `id`, `uid` и занятая таблица
  (`password_reset`) — отказ до первой правки, `git status` пуст; сбой после первой правки — откат.
  Проверка: сценарии `R/template-code.md` § 8
  - Подтверждено: `api_key` → `make check` ✓ (S10); база — `repo` печатает «✓», а тесты среза не
    собираются, `id` и `password_reset` — сбой с четырьмя изменёнными файлами, каталогом и миграцией
    (`N/gate/s11-base-summary.txt`); ветка — `repo`, `name`, `valid`, `got`, `row`, `id`, `uid`,
    `password_reset` — отказ, `git status` пуст; сбой mockery после первой правки — откат, дерево чистое;
    `TestNewDomainRefusesOrRollsBack` — на скрипте базы 5 из 7 FAIL, на ветке PASS
- [x] **AC10** — `make mutate`: мутации спецификации — по тексту кода (без учёта пробелов, ровно одно
  вхождение), после обрыва прогон продолжается с места, эквивалентные — явным списком, итог — выжившие
  списком и код 1 при новых выживших, устаревший якорь — в отчёте, прогон не падает. Проверка: подмножество
  под flock — убитый, эквивалентный, выживший (код 1), обрыв и продолжение
  - Подтверждено: `make mutate A=--check` → якоря 360 мутаций — по одному месту; срез из 8 мутаций в
    отдельном worktree: SIGKILL посреди VS5 — повторный запуск откатил её правку и прошёл остальные 4,
    итог — убиты 3 (юнит 2, integration 1), эквивалентные U25 и VS5 выжили, новые AI4, UH7, MI1 — код 1;
    мутация с якорем вне кода — «устаревший якорь», код 0 (`N/gate/s12-run{1,2,3}.log`); песочница
    (`N/gate/s13-sandbox.sh`): обрыв посреди мутации и правка дерева после него — отказ, правка цела (раннер
    `fc521f1` стирал её — `s13-sandbox-red.log`), только след мутации — откат и продолжение (`s13-sandbox.log`)
- [x] **AC11** — итог по углу enforcement: 89 обязательных инъекций аудитора пойманы (7 непойманных на
  базе — тоже), открытых fail-open шагов нет (имена миграций, второй конфиг lint, `MOCKERY_CONFIG`).
  Проверка: перегон кейсов `R/tpl-enf/run.py` и fail-open сценариев → поймано
  - Подтверждено: ветка `5587374` (`N/gate/enf/`): 89 кейсов `run.py --heavy` пойманы все — шагами гейта 84
    (fmt 3, vet 5, lint 35, migrations 5, тесты 24, build 1, gen 6, cover-check 2, integration 3), N-b-unit и
    N-b-prod — `make check` каждого (TestBuildConstraints), три N-pl-* — pre-commit (plan-check), как на базе;
    7 непойманных на базе — gosec, TestLinterDirectives, TestTestsAssert ×2, build, TestBuildConstraints ×2.
    `failopen.sh` (41): имена миграций не по маске, второй конфиг lint, `MOCKERY_CONFIG` — FAIL (на базе OK);
    OK — заглушки в PATH (записано), пустой каталог, без cgo — с WARN, проба `DOCKER_HOST` (сокет не прячет и
    у аудитора — он брал `unshare`); `altconfig*.sh` — `.golangci.toml` и `.json` не заслоняют `.yml`;
    `gen-roundtrip.sh` — три пути после `make gen` без `git add` → OK

- [ ] **AC12** — Круг фиксов валидатора (`N/validation-gate.md`): формы B1 (`MAKEOVERRIDES`), B2 (`make mutate` в
  дереве разработчика), B3 (`.mockery.yaml`) не проходят; неблокирующие из задания дирижёра закрыты или отклонены
  в «Решениях». Проверка: воспроизведения валидатора на ветке → отказ или дерево чистое; тесты archtest → PASS
- [ ] **AC13** — Ветка на `main` (`98e425e`, слиты lifecycle и identity): спецификация мутаций — под код `main`,
  `.template/paths` и prune держат её кодом форка, флака `TestReleaseBuildForwardsSSHAgent` нет; полный
  `make mutate` → код 0. Проверка: `make check` → ✓; полный прогон под замком → код 0
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
- [x] **S10** — Имя домена с инициализмом: скелет по правилу revive, archtest сверяет префикс без регистра
  - Проверка: `make new-domain NAME=api_key` → `make check` ✓ (в отдельном worktree) · Закрывает: AC9 · Зависит от: —
  - Сделано: TITLE и CAMEL скелета — по списку инициализмов revive v1.15 (`api_key` → `APIKey`, `apiKey`;
    `user_id` → `UserID`, `userID`); `TestHTTPTypes` сверяет префикс без регистра, `TestDomainErrorTables` —
    имя таблицы. База: срез `api_key` — 8 замечаний var-naming, после переименования — `TestHTTPTypes` FAIL
    (`N/gate/s10-base-*`); ветка: `api_key` → `make check` ✓ 824/824 (`s10-branch-check-api_key.log`),
    `user_id` — lint, vet и archtest OK; `make check` → ✓
- [x] **S11** — `make new-domain`: негодное имя и занятая таблица — отказ до первой правки; сбой после — откат
  - Проверка: сценарии RCOD-06 → отказ, `git status` пуст · Закрывает: AC9 · Зависит от: S10
  - Сделано: имена кода среза — из Go-шаблонов самого скрипта (вместо ручного списка), таблица, которую
    миграции создают и не удаляют, — отказ до первой правки; после неё — копии общих файлов и откат по
    `trap`, а в конце — `go vet -tags=integration` среза и `cmd/api`; тест на песочнице; `domain.md`;
    `make check` → ✓
- [x] **S12** — `make mutate`: раннер в `scripts/tools/`, спецификация аудитора по текущему коду, документ —
  rule `tests-pipeline`
  - Проверка: подмножество под flock (AC10) · Закрывает: AC10 · Зависит от: —
  - Сделано: `scripts/tools/mutate.py` (stdlib), `scripts/tools/mutants.txt` — 360 мутаций аудитора (60
    форм первого аудита и 300 новых) в текстовом формате, блоки по слоям кода, 9 эквивалентных — строкой
    «=»; цель `make mutate`; rule `tests-pipeline` § Мутации, CLAUDE.md § Команды; полный прогон — S13;
    `make check` → ✓
- [x] **S13** — Перегон экспериментов трека и гейт
  - Проверка: кейсы AC11 → поймано; `make check` → `✓ check clean` · Закрывает: AC11 · Зависит от: S1–S12
  - Сделано: полный `make mutate` ветки (`fc521f1`, 360; кусками под замком, после обрывов сессии и
    перезагрузки — с места): убиты 337 (юнит 273, integration 64), вердикты — как у аудитора, кроме K09 (его
    «убил» сбой Docker; эквивалентна — строка «=»), новых выживших 14 (`N/gate/s13-mutate-16.log`, итоги —
    `N/gate/mutate-branch/`); якоря 11 мутаций — под код обеих веток, на ветке убиты все (`s13z-mutate.log`);
    временное слияние с `nine/identity` `b4fa321` с патчем спецификации: 356 мутаций, убиты 343, эквивалентные
    10, новые — K12, K13, SD2, код 1 (`s13m-mutate-8.log`, `mutate-merge/`); кейсы enforcement — AC11;
    `make check` → ✓; `N/wt-gate-{x,y,m,z}` сняты
- [x] **S14** — Перенос на `main` (`98e425e`)
  - Проверка: `make check` → ✓ · Закрывает: AC13 · Зависит от: S1–S13
  - Сделано: rebase, конфликт — комментарий пакета archtest (обе стороны); новые проверки гейта видят код
    identity — `make check` → ✓, 795/795
- [x] **S15** — B1: `MAKEOVERRIDES` не от make — отказ make; `coverage.out` снят в начале `check::`
  - Проверка: три формы `MAKEOVERRIDES` и `TestCheckCoversItsOwnRun` — RED на Makefile до правки, GREEN после ·
    Закрывает: AC12 · Зависит от: S14
  - Сделано: `gate_forged` — и `MAKEOVERRIDES` (законное происхождение — `default`); `rm -f coverage.out` первой
    строкой шагов; `TestMakeCheckCannotBeForged` — в песочнице с копией Makefile (гейт снимает `coverage.out`,
    а test-int вокруг теста пишет его в дерево репо); `docs/contributing.md`; `make check` → ✓
- [x] **S16** — B2: `make mutate` правит свой worktree `.cache/mutate/wt`, а не дерево разработчика
  - Проверка: SIGTERM, SIGHUP, SIGKILL посреди мутации → дерево разработчика чистое, повтор — с места ·
    Закрывает: AC12 · Зависит от: S14
  - Сделано: worktree раннера — к HEAD до и после каждой мутации; SIGTERM и SIGHUP выходят через откат, как
    Ctrl-C; откат следа в дереве разработчика (`00c4f5d`) снят. Песочница (`N/gate/r2-sandbox.log`): TERM, HUP —
    дерево разработчика и worktree чистые; KILL — правка только в worktree раннера; правка README после обрыва
    цела, повтор — с места; убитая, эквивалентная, новая (код 1), устаревший якорь — как прежде; `make check` → ✓
- [x] **S17** — B3: mockery — только `--config .mockery.yml` (gen и gen-check)
  - Проверка: `.mockery.yaml` рядом — gen-check FAIL, случай теста · Закрывает: AC12 · Зависит от: S14
  - Сделано: `mockery --config .mockery.yml` в gen и gen-check; заглушка `TestGenCheckComparesMocksWithTree` падает
    без него (на Makefile до правки — 4 из 4 FAIL); форма валидатора (`.mockery.yaml` с выводом в `mocks2`, метод
    `Ping` без make gen) — gen-check FAIL с файлом мока; `make check` → ✓
- [x] **S18** — Список непокрываемого `cover.sh`: запись — ровно один свой блок, пустой текст — отказ
  - Проверка: случаи `TestCoverCheck` · Закрывает: AC12 · Зависит от: S14
  - Сделано: запись покрывает блок, только если её текст есть ровно в одном неисполненном блоке файла; без
    текста (и из одних пробелов) — отказ; `TestCoverCheck` + 2 случая (на `cover.sh` до правки оба — OK);
    rule `tests-pipeline` § Coverage; `make check` → ✓
- [x] **S19** — `TestTestsAssert` вне доменов: засчитывается хелпер, который утверждает, а не любой вызов с `t`
  - Проверка: `fmt.Println(t)` → отказ · Закрывает: AC12 · Зависит от: S14
  - Сделано: вне доменов и платформы `t` засчитывается, только отданный проверке тестового кита — функции его
    пакета (golibinfra, `internal/testkit`: `archkit.Check`, `testkit.MigrationLadder`) или методу значения из
    него (`envAllowlist.Check`); утверждающие хелперы пакета — как прежде. Подброшенные `fmt.Println(t)` в
    `pkg/paging` и неутверждающая фикстура в `cmd/seed-admin` — на коде до правки PASS, после — FAIL; `make check` → ✓
- [x] **S20** — `make mutate`: таймаут и обрыв снимают группу процессов стадии с тестовыми бинарями
  - Проверка: таймаут стадии → процессов стадии нет · Закрывает: AC12 · Зависит от: S16
  - Сделано: команда стадии — своя группа процессов (`start_new_session`), таймаут и любое исключение прогона
    (Ctrl-C, SIGTERM, SIGHUP) — `killpg`; песочница `N/gate/r2-s20-sandbox.sh` (тест мутанта спит 60 с, таймаут
    5 с): раннер HEAD оставляет тестовый бинарь после таймаута и SIGTERM, новый — нет; `make check` → ✓
- [x] **S21** — `make mutate A=--strict`: устаревший якорь и несобираемая правка — код 1 (шаг `/release`)
  - Проверка: песочница → код 1 · Закрывает: AC12 · Зависит от: S16
  - Сделано: `--strict` (и с id мутаций): устаревшие якоря и несобравшиеся правки — код 1 с их числом; подробность
    вердикта — одной строкой; rule `tests-pipeline` § Мутации (и таймаут — код 1), `make help`. Песочница
    (`N/gate/r2-s21-sandbox.log`): без `--strict` — код 0, с ним — 1; `make check` → ✓
- [x] **S22** — migrations-check: `CALL` и `SELECT` — код без маркера, тело `$$…$$` — не эффект
  - Проверка: случаи `TestMigrationsCheck` · Закрывает: AC12 · Зависит от: S14
  - Сделано: `CALL` и `SELECT` верхнего уровня — класс «код», тела `$tag$…$tag$` вырезаются до разбора операторов;
    `TestMigrationsCheck` + 3 случая (на скрипте HEAD: `CALL` проходит, `SELECT f()` ловится лишь `UPDATE` тела,
    тело функции с `UPDATE` — ложный отказ); rule `sqlc-and-migrations`, `migration.md`; `make check` → ✓
- [x] **S23** — Forward-only: `Makefile`, `migrations-check.sh`
  - Проверка: текст — условие, а не история · Закрывает: AC12 · Зависит от: S14
  - Сделано: порог покрытия — «запас ниже факта пропустил бы…», список выпущенных миграций — по их операторам,
    без «до классов»; `make check` → ✓
- [x] **S24** — Спецификация мутаций под код `main`: V12, V17, VN3, VN6 сняты, V14 — по коду `main`
  - Проверка: `make mutate A=--check` → OK · Закрывает: AC13 · Зависит от: S14
  - Сделано: патч `N/gate/mutants-after-identity.patch`; V14 — под `376d2bb` (лимит считает адрес): мутация
    отдаёт лимиту учётную запись; `make mutate A=--check` → якоря 356 — по одному месту; `make check` → ✓
- [x] **S25** — `.template/paths` и prune: спецификация мутаций — код форка
  - Проверка: `TestTemplatePaths` → PASS · Закрывает: AC13 · Зависит от: S14
  - Сделано: `:(exclude)scripts/tools/mutants.txt` в `.template/paths`; prune — блоки удалённых путей и строк
    подключения из спецификации, итог — `make mutate A=--check` → OK; rule `tests-pipeline` § Мутации;
    `make check` → ✓
- [x] **S26** — Флак `TestReleaseBuildForwardsSSHAgent`: заглушка пишется без чужого fork
  - Проверка: тест ×N под нагрузкой → PASS · Закрывает: AC13 · Зависит от: S14
  - Сделано: `writeExecutable` — заглушка-программа пишется под `syscall.ForkLock`; так — все заглушки archtest
    (`make_test.go`, `steps_test.go`, а также `hooks_test.go`, `release_test.go` трека lifecycle — тот же класс).
    Механизм — `N/gate/etxtbsy/`: запись и запуск при соседних fork — ETXTBSY 416 из 2000, под `ForkLock` — 0;
    тесты с заглушками ×120 под нагрузкой — PASS (флак на прогоне мутаций — 2 из 356); `make check` → ✓
- [ ] **S27** — Полный `make mutate` на итоге ветки и гейт
  - Проверка: код 0; `make check` → ✓ · Закрывает: AC13 · Зависит от: S15–S26

## Решения
- Шаги-скрипты гейта зовутся `bash scripts/…` из рецептов, их защищает `unexport` Makefile; `bash -p` —
  у хуков, которые git запускает сам. Хуки перезапускают себя без переменных make и оболочки (без маркера:
  условие — сами переменные), а не чистят окружение каждой проверки.
- `make mutate`: якорь — текст кода без учёта пробелов (а не номера строк), устаревший якорь — в отчёте с
  кодом 0 (иначе любая правка кода валила бы прогон), код 1 — новые выжившие и таймауты; без `-race` и с
  кешем go test (заново идут пакеты, задетые мутацией); базовый прогон красный — отказ; спецификация —
  текст, не код (исполнять её не нужно); 13 выживших пробелов на базе (AI4, SD*, K*, MI1, A36, UH7) —
  тесты трека identity (RTSTT-02…06).
- `make mutate` правит свой worktree `.cache/mutate/wt` по HEAD (B2 валидатора): дерево разработчика не трогают ни
  обрыв любым сигналом, ни повтор; worktree раннера каждая мутация начинает и кончает на HEAD. Тест раннера в гейт
  не входит — гейту понадобился бы python3; сценарии — песочница `N/gate/r2-sandbox.sh`.
- K09 (httpModule первым в графе) — эквивалентная, строка «=»: Invoke модуля fx выполняет раньше корневого
  хука сервера httpkit, где бы модуль ни стоял (`cmd/api/http.go`), HTTP стартует последним — TestBoot; у
  аудитора её «убил» сбой Docker (`R/tpl-tests/new/logs-results/K09.gate.log`).
- K12 (без `fx.RecoverFromPanics`), K13 (meter доменов без имени scope), SD2 (`seed-admin` без `FOR UPDATE`) —
  неэквивалентные выжившие в файлах трека identity (`cmd/**`): тест — его, в отчёт дирижёру; до тех пор
  `make mutate` после слияния — код 1 с ними. Остальные 10 из 14 выживших ветки убивают тесты identity.
- Якоря 11 мутаций, чей код трек identity переписал, — общие для его кода и прежнего (E01 — константа предела
  адреса, V33 — префикс subject и т. п.); V12, V17, VN3, VN6 сняты — их кода в `main` нет (запрос сброса делает
  одну работу для любого адреса, токена в событии запроса нет, подписки прежнего subject нет); V14 — лимит
  считает адрес, мутация — учётную запись.
- Спецификация мутаций — рядом с раннером, в `scripts/tools/`: строка «=» и снятый блок ослабляют проверку,
  и правку `scripts/`, после которой проверяется меньше, CLAUDE.md уже отдаёт «сначала спросить» — как
  список непокрываемого `cover.sh`. Описывает она код сервиса — форк получает её затравкой:
  `:(exclude)scripts/tools/mutants.txt` в `.template/paths` (как у `layout_settings_test.go`) и шаг prune
  (`init-service/references/prune.md`) — файлы трека lifecycle (слит), правки минимальные (S25).
- RCOD-06: имена из шаблонов — надмножество (и поля таблиц тестов: `status`, `code`), лишний отказ дешевле
  полусостояния; занятая таблица — по последнему `CREATE`/`DROP TABLE` в миграциях (созданная и
  удалённая, как `refresh_sessions`, свободна); любой иной сбой после первой правки — откат.
- RCOD-05: archtest без регистра (`TestHTTPTypes`, `TestDomainErrorTables` — файл трека boundaries), а не
  перевод имени со списком инициализмов в archtest: список — один, в `new-domain.sh` (revive v1.15).
- `.claude/skills/scaffold/references/domain.md` (файл трека harness): строки о `make new-domain` — под
  инициализмы, отказ и откат (S10, S11).
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
- `hooks_test.go`, `release_test.go` (трек lifecycle, слит): заглушки — через `writeExecutable` (S26), иначе тот
  же флак ETXTBSY остаётся в соседних тестах.
- `.claude/skills/scaffold/references/migration.md` (вне таблицы владения): рецепт индекса и список классов
  migrations-check (S5, S22) — иначе reference врёт о машине.
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
S1–S13 приняты по существу, валидатор — FAIL на `829481c` (`N/validation-gate.md`): круг фиксов S14–S27. Ветка
перенесена на `main` `98e425e` (S14); B1–B3 и неблокирующие закрыты (S15–S23); спецификация — под код `main`, она
— код форка (S24, S25), флак ETXTBSY снят (S26). Следующее: S27 — полный `make mutate` на итоге.
