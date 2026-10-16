---
status: in-progress
parent: —
blocked-by: —
---

# polish-gate-tests — гейт и тесты без хвостов (S7)

## Цель
Закрыть находки переаудита углов «Принуждение» и «Тесты» (NENF-01, NTST-02–04), остатки RTSTT-03 и RTSTT-05,
minor бэклога `minor-enforcement` (кроме RENF-12), `minor-tests`, RADP-10, RLIF-13 (`make setup` и версии
инструментов) и трения догфудинга T7, T9, T12 — чтобы гейт шаблона `template/v1.2.0` держал машиной то, что
обещает проза, а спецификация мутаций росла вместе с кодом.

## Контекст
- Программа `reaudit-to-nine`, шаг S7, трек gate+tests: ветка `nine/s7-gate`, worktree `N/wt-s7g` от `main`
  `a4020ee`; `N` = `/home/mm/.cache/stit-audit-program/nine`, `R` = `/home/mm/.cache/stit-audit-program/reaudit`.
  Песочница и логи — `N/s7g/`, база для опытов — worktree `N/wt-s7g-base` (`a4020ee`).
- Находки: `N/reaudit/template-enforcement.md` (NENF-01, § 4 — бэклог угла), `N/reaudit/template-tests.md`
  (NTST-01–04, § 3 — RTSTT-03, 05, 07–10), `R/template-enforcement.md` (RENF-10, 11, 13–16), `R/template-tests.md`
  (RTSTT-07–10), `R/template-adapters.md` (RADP-10), `N/validation-gate.md` (остаток gate, пп. 1–10),
  `N/dogfood.md` (T7, T9, T12), `R/template-lifecycle.md` и `N/reaudit/template-lifecycle.md` (RLIF-13 —
  дополнение дирижёра: `install-tools.sh` трека); мутации аудитора — `N/reaudit/tpl-tests/new-mutants.txt`,
  вердикты — `ra-verdicts.tsv`, мутации скелета — `tpl-tests/scaf/run.py`.
- Решения владельца программы не пересматриваются: порог покрытия — факт 100 с явным списком непокрываемого,
  `make mutate` — вне гейта, шаг `/release`.
- Параллельно — треки harness+lifecycle (`N/wt-s7h`: `commit-msg`, `docs/contributing.md`, скиллы) и
  code+security+adapters (`N/wt-s7c`: `cmd/**`, домены, NTST-01 — тест на `MarkPasswordResetSent … OR TRUE`).
- Замеры базы: `cmd/api.test` под `-race` — пик 2207 МиБ, с `ARGON2_MEMORY_KIB=8192` — 390 МиБ
  (`N/s7g/api-rss.log`); в `cmd/` вне профиля — `main` трёх бинарей и ветки сбоев `run` (`N/s7g/base-cover-cmd.out`);
  все 461 subject истории, кроме `ea1de93 clean` (в `template/v1.0.0`), — Conventional Commits.

## Не входит
- RENF-12 и `status.sh`, часть RENF-11 про `commit-msg` и cherry-pick — трек harness+lifecycle; тест NTST-01 и
  остальное `minor-adapters` — трек code+security+adapters.
- `go.mod`/`go.sum`, `pkg/policy`, `internal/platform/authgate/`, выпущенные миграции; новые шаги `make check`.
- Мутация на каждую функцию: машина держит файл с кодом без мутаций, функцию — правило и ревью.
- Проверки качества SQL (`sqlc vet` и т. п.) — решение владельца: в S7 не добавляются.
- Полный `make mutate` ветки — S8 программы (дирижёр).

## Критерии приёмки
- [ ] **AC1** — Шаги гейта не зависят от локали, версии инструментов и мусора в `.cache/` (NENF-01, RENF-15,
  RENF-16, RLIF-13, `GOPACKAGESDRIVER`, `fmt-check` в worktree раннера). Проверка: опыты на базе и ветке —
  `LC_ALL=tr_TR.UTF-8` у migrations-check (репо и `CREATE INDEX` по существующей таблице), `gofumpt` и
  `golangci-lint` чужой версии в `PATH`, `make setup` при закреплённых версиях без сети, неотформатированный файл в
  `.cache/mutate/wt`, драйвер `GOPACKAGESDRIVER` с меткой: база — ложный отказ, пропуск или переустановка, ветка —
  верный вердикт
- [ ] **AC2** — migrations-check: `INSERT` в существующую таблицу — эффект, `.gitkeep` — пустой каталог,
  выпущенные миграции — по содержимому, а не по имени. Проверка: `TestMigrationsCheck`; опыт — своя
  `0001_users.up.sql` форка с `CREATE INDEX` по существующей таблице: база — OK, ветка — FAIL
- [ ] **AC3** — Хуки судят то, что уходит: pre-commit — снимок индекса (T9), pre-push — subject пушимых коммитов
  (RENF-11); переменные окружения вызывающего снимает одно место на механизм. Проверка: `TestPreCommitChecksIndex`,
  `TestPrePushGatesPushedCommit`; опыт T9 (индекс чист, в дереве WIP с замечанием lint) и push коммита
  `update stuff` из cherry-pick: база — отказ и пропуск, ветка — наоборот
- [ ] **AC4** — Формы «код без тестов» красные (NTST-02, NTST-04, RTSTT-03, RTSTT-07, RTSTT-08, RADP-10).
  Проверка: CV07, CV09, CV10, CV14, CV20 аудитора, `.Maybe()` в `TestMe`, запись непокрываемого на тело функции:
  база — `✓ check clean`, ветка — красный своим шагом
- [ ] **AC5** — Спецификация мутаций растёт с кодом (NTST-03, T12, RTSTT-05, остаток `make mutate`): мутации
  аудитора — в `mutants.txt`, `A=--check` называет файлы с кодом без мутаций, `make new-domain` дописывает блоки
  скелета, у раннера — тест в гейте. Проверка: `make mutate A=--check` → OK; `make new-domain NAME=order` →
  `make mutate A="order-…"` — убиты все, и `created_at` (SC25); `TestMutate`
- [ ] **AC6** — Guard, allow и секреты (RENF-10, 13, 14). Проверка: `TestGuardBash` с формами `git-<подкоманда>`,
  `.git//config`, вложенного git и `rg --pretty`; N-s-env, N-s-key, N-s-envprod, N-d-latest — база
  `✓ check clean`, ветка — красный; в allow нет `tidy`, `find *`, `git branch*`
- [ ] **AC7** — Остаток трений и minor: имена `new-domain` (`status`, `code`, `body`, `api`…) собираются и
  проходят, `make gen` при несобранном коде называет причину (T7), `make e2e` гоняет ruff и mypy (RTSTT-09),
  тесты `cmd/api` легче (RTSTT-10). Проверка: опыты на базе и ветке; замер `N/s7g/api-rss.log`
- [ ] **AC8** — Гейт ветки чист, законная работа не получает отказа (B1–B3 валидатора). Проверка: пробы
  валидатора B1–B3 на ветке — верный вердикт; `make check` → `✓ check clean`

## Шаги
- [x] **S1** — План трека
  - Проверка: `make plan-check` → OK · Закрывает: — · Зависит от: —
  - Сделано: план; `make plan-check` → OK, гейт — `✓ check clean` (`N/s7g/check-s1.log`)
- [x] **S2** — Шаги гейта: `fmt.sh` по файлам репо, версии инструментов одним местом (`versions.sh`: сверка шагом
  и `install-tools.sh` — ставит только то, чья версия не та), `LC_ALL=C` у bash-шагов, вывод `test.sh` по строкам,
  `make gen` — сборка до mockery, `GOPACKAGESDRIVER` вне шагов
  - Проверка: опыты AC1 и T7; `make check` → `✓ check clean` · Закрывает: AC1 · Зависит от: S1
  - Сделано: опыты `N/s7g/bin/exp-s2.sh` → `N/s7g/exp-s2-{base,branch}.log`: база — migrations-check под tr_TR
    отвергает миграции репо и пропускает `CREATE INDEX` по существующей таблице, gofumpt и golangci-lint чужой версии
    проходят, `make setup` переставляет sqlc и mockery, fmt-check судит `.cache/…/wt`, драйвер `GOPACKAGESDRIVER`
    зовётся, `make gen` — FTL mockery без причины, `test.sh` рвёт UTF-8; ветка — верный вердикт в каждом; тесты
    `TestToolVersions`, `TestGenBuildsFirst`, расширены `TestLintUsesItsConfig`, `TestFmtAppliesLintRules`,
    `TestMigrationsCheck`; `make check` → `✓ check clean` (`N/s7g/check-s2.log`)
- [x] **S3** — migrations-check: `INSERT`, `.gitkeep`, выпущенные по содержимому; rule `sqlc-and-migrations`
  - Проверка: `TestMigrationsCheck`, опыт AC2; `make check` → `✓ check clean` · Закрывает: AC2 · Зависит от: S2
  - Сделано: опыты `N/s7g/bin/exp-s3.sh` → `N/s7g/exp-s3-{base,branch}.log`: база — `.gitkeep` вне конвенции,
    `INSERT … VALUES` в `users` и своя `0001_users` форка с `CREATE EXTENSION` проходят, шаблонная `0003` под
    номером `0002` — отказ; ветка — наоборот; шесть случаев в `TestMigrationsCheck`; `make check` → `✓ check clean`
- [x] **S4** — Хуки: pre-commit — снимок индекса, pre-push — subject пушимых коммитов и `-f Makefile`;
  переменные make — в хуках, оболочки — в `Makefile`
  - Проверка: опыты AC3; `make check` → `✓ check clean` · Закрывает: AC3 · Зависит от: S2
  - Сделано: опыты `N/s7g/bin/exp-s4.sh` → `N/s7g/exp-s4-{base,branch}.log`: база — pre-commit отвергает чистый
    индекс за WIP в дереве и пропускает неотформатированный индекс при исправленном дереве, pre-push пускает
    `update stuff` на зелёный гейт; ветка — наоборот; `TestPreCommitChecksIndex`, расширены
    `TestPrePushGatesPushedCommit` (subject, коммит remote'а, `unexport` Makefile репо) и
    `TestHooksIgnoreCallerEnvironment`; `make check` → `✓ check clean`
- [x] **S5** — Покрытие и сила тестов: функции без тестов — по профилю до списка и в `cmd/`, fuzz и мягкие моки в
  `TestTestsAssert`, заголовок сгенерированного — только у вывода генераторов, `gochecknoinits`
  - Проверка: опыты AC4; `make check` → `✓ check clean` · Закрывает: AC4 · Зависит от: S2
  - Сделано: пробы `N/s7g/bin/probe-s5-{tests,gen,cmd}.sh` → `N/s7g/probe-s5-cmd.log` и вывод в журнале: база —
    `.Maybe()` и EXPECT без `.Once()` в `TestMe`, fuzz без утверждений (CV20), `init()` (CV07), рукописный файл под
    заголовком в каталоге sqlc (CV09) и в `pkg/` (CV10), функция `cmd/api` без теста (CV14) — зелёные, запись
    непокрываемого на тело функции — OK (`N/s7g/ntst04`); ветка — красный своим шагом каждый; случаи в
    `TestCoverCheck`; `make check` → `✓ check clean` (`N/s7g/check-s5.log`)
- [x] **S6** — Мутации: мутации аудитора, файлы без мутаций, блоки скелета `new-domain`, `created_at` скелета,
  раннер (временные каталоги, `killpg`), тест раннера; rule `tests-pipeline`, `CLAUDE.md § Команды`
  - Проверка: опыты AC5; `make check` → `✓ check clean` · Закрывает: AC5 · Зависит от: S5
  - Сделано: спецификация — 401 мутация (+45 аудитора, RA25 — «=»), `make mutate A=--check` → OK; опыт в
    `N/wt-s7g-br`: `make new-domain NAME=order` → блоки `order-01…24`, их якоря — OK, `make check` → `✓ check clean`
    859/859 (`N/s7g/check-s6-order.log`), `make mutate` кусками → убиты 24 из 24 (юнит 17, integration 7), и
    `created_at` (`order-24`, SC25 аудитора) — integration (`N/s7g/mutate-order.log`); `TestMutate`; `make check` →
    `✓ check clean` (`N/s7g/check-s6.log`)
- [x] **S7** — Guard, allow, секреты и образы
  - Проверка: опыты AC6; `make check` → `✓ check clean` · Закрывает: AC6 · Зависит от: S4
  - Сделано: `N/s7g/exp-s7-guard.log`: на 636 случаях `guard-cases.txt` guard базы — 17 расхождений (15 форм RENF-10
    проходят, `rg --pretty` — ложный отказ), ветки — 0; обычные (158) и формы (129) аудитора — 0 расхождений; проба
    `server.key` (`git add -f`) и `FROM golang:latest` — красные `TestNoSecretFiles`, `TestImagesPinned` (у аудитора
    на базе — `✓ check clean`); allow без `tidy`, `find *` и `git branch*`; `make check` → `✓ check clean`
- [x] **S8** — Имена `new-domain`, e2e-харнес, лёгкий Argon2 тестов `cmd/api`, мелочи RENF-16
  - Проверка: опыты AC7; `make check` → `✓ check clean` · Закрывает: AC7 · Зависит от: S6
  - Сделано: имена — база отвергает `status`, `code`, `field`, `body`, `api`, `database`, `caller`, `resp` до правки
    (`N/s7g/exp-names-base.log`), ветка — срез, lint и юнит-тесты зелёные, а `repo`, `id`, `tests`, `setup`,
    `created` — откат после сборки с ошибкой об имени, дерево чистое (`N/s7g/exp-names-branch.log`); `make e2e-smoke`
    → 9 passed, `make e2e` → 16 passed (`N/s7g/e2e-s8.log`), ruff и mypy — отказ до стека, у базы их нет
    (`N/s7g/e2e-lint-probe.log`); пик памяти `cmd/api.test` под `-race` — 2207 → 418 МиБ (`N/s7g/api-rss.log`);
    `tests-pipeline.md` — 12,6 → 11,5 тыс. символов; `make check` → `✓ check clean`
- [x] **S9** — pre-push: сообщение пушимого коммита — хуком `commit-msg` из дерева его родителя (дополнение
  дирижёра к RENF-11: `git rebase --continue` хук не зовёт)
  - Проверка: `TestPrePushGatesPushedCommit`; история `origin/main..nine/s7-gate`; `make check` → `✓ check clean`
    · Закрывает: AC3 · Зависит от: S4
  - Сделано: на пробном репо `TestPrePushGatesPushedCommit` — `feat` без `Review:` после правила — отказ до гейта,
    цепочка до правила и коммит, уже бывший в ветке remote'а, — проходят; 156 незапушенных коммитов
    `origin/main..nine/s7-gate` правилу родителя отвечают все, вся история `main` — кроме `ea1de93 clean` и
    `a0f3fa9` (оба давно в `origin/main`) (`N/s7g/bin/msg-check.sh`); `make check` → `✓ check clean`
- [ ] **S10** — Круг по валидатору (`N/validation-s7-gate.md`, FAIL на `329a459`): ложные отказы B1–B3,
  герметичный `TestCommitMsgHook`, python3, RADP-10, доки, handoff — коммитом на пункт
  - Проверка: пробы валидатора B1–B3 → верный вердикт; `make check` → `✓ check clean` · Закрывает: AC8 · Зависит от: S9
  - Сделано: B1 — `N/s7g/bin/v-gen-b1.sh` → `N/s7g/v-gen-b1-branch.log`: `user.ListFilter` → `user.ListQuery` мимо
    моков — `make gen` OK, моки переписаны, `go vet` OK; порт `GetByID` сменил сигнатуру (T7) — отказ сборкой;
    случай «устаревший мок» в `TestGenBuildsFirst`

## Решения
- `internal/archtest/steps_test.go` (тесты шагов гейта, завёл трек gate волны 1) в таблице владения не назван —
  правит этот трек: другого владельца у тестов шагов нет.
- Версии инструментов — одним местом, `scripts/tools/versions.sh`: шаг гейта сверяет свой инструмент (fmt —
  gofumpt, lint — golangci-lint, gen и gen-check — sqlc и mockery), `install-tools.sh` берёт версии оттуда и ставит
  только инструмент не той версии (RLIF-13); версия из окружения (`GOFUMPT_VERSION=…`) снята — закреплённая одна.
- `fmt.sh` отдаёт gofumpt каталоги верхнего уровня с Go-файлами репо (`git ls-files -co`), а не файлы: сгенерированное
  gofumpt пропускает только при обходе каталога, а явный файл переформатировал бы.
- `LC_ALL=C` — первой строкой каждого bash-скрипта шагов и `new-domain.sh`, а не в `Makefile`: тесты и хуки зовут
  скрипты и мимо make.
- `GOPACKAGESDRIVER` снимает `Makefile` вместе с `MOCKERY_*`: его читают golangci-lint и mockery, шаги гейта идут
  через make.
- `make gen` до mockery собирает пакеты кода без `mocks` (`go list -e`, пакеты с файлами не-тестов): мок, устаревший
  после правки интерфейса или типа, перепишет mockery, а ошибка в коде — причина отказа (T7).
- migrations-check: `INSERT` в таблицу не из этого файла — эффект (и `… VALUES`, и `… SELECT`): правка данных
  прод-таблицы, выкатке нужен порядок; в таблицу файла — сид, как индекс по ней. Выпущенные миграции шаблона — по
  sha256 содержимого (без CR): своя `0001_users` форка проверяется целиком, а шаблонная под другим номером
  (NLIF-02) остаётся выпущенной. `.gitkeep` — единственный посторонний файл: каталог форка без таблиц иначе не
  держится в git.
- `scaffold/references/migration.md` (трек code+security+adapters) — одна строка перечня классов migrations-check:
  reference называет, что гейт не пропустит.
- pre-commit судит снимок индекса (`git write-tree` → коммит вне веток) в постоянном worktree
  `.cache/pre-commit/wt` — как раннер мутаций: кеш lint тёплый между коммитами; замок не нужен — два коммита в
  одном worktree не даёт `index.lock`. Проверки — целями make с пустыми `PKG`, `FORMAT`, `A`.
- pre-push судит subject пушимых коммитов самим `commit-msg` (тело — строка `Review:` о проверке): шаблон subject
  один, его держит трек harness. Пушимые — те, которых нет в ветках remote'ов: история шаблона (`ea1de93 clean`) у
  форка там, а коммиты cherry-pick и клона без хуков — нет.
- Переменные окружения: хуки снимают только переменные make (`MAKE*`, `MFLAGS`, `GNUMAKEFLAGS`) — без них make
  не запустить честно; окружение оболочки — `bash -p` у хуков и `unexport` `Makefile` у рецептов (все проверки
  хуков идут через make); `mutate.py` снимает только `API_TEST_MAIN_ARGS`, остальное снял `make mutate`.
- `docs/contributing.md` (трек harness+lifecycle) — ячейки «git-хуки» и «Самозащита гейта»: поведение хуков сменилось.
- Покрытие `cmd/`: в профиле (`-coverpkg`) и в правиле «функция без тестов», но не в доле — ветки сбоев запуска
  `run` бинарей тесты не берут, а тесты `cmd/**` — трек code+security+adapters; `main` вне правила: его исполняют
  перезапущенный тестовый бинарь и образ, профиль родителя этого не видит.
- Запись непокрываемого снимает блок с доли, а функции считаются по счётчикам тестов (NTST-04): запись,
  накрывшая тело целиком, функцию без тестов не прячет.
- Заголовок сгенерированного — только у вывода sqlc (имена его файлов по `queries/`) и mockery; в каталоге вывода
  sqlc чужих файлов нет. Пакет `pkg/` целиком под заголовком больше не пропускается: генератора у него гейт не
  знает — сгенерированный клиент заводят генератором в `make gen` и каталогом в тесте (правка гейта).
- `init()` держит линтер (`gochecknoinits`), а не archtest: одно место для правил кода — golangci-lint.
- Мягкие ожидания моков — в `TestTestsAssert` (сила теста), отдельного теста нет; ожидание — одним выражением
  цепочки, как во всём коде шаблона.
- Мутации аудитора для нового кода — в `mutants.txt` как есть (id `RA…`, у пяти несобиравшихся — их вариант `…b`).
  RA25 (`MarkPasswordResetSent … OR TRUE`) — со строкой «=»: тест, который её убивает, пишет трек
  code+security+adapters; после его слияния `make mutate A="--strict RA25"` — отказ «эквивалентная убита», и строку
  «=» снимает тот, кто сливает второй. Трек code+security+adapters правит и `IssuePasswordReset`,
  `password_resets.sql`: устаревшие якоря `RA…` назовёт `make mutate A=--check` — блоки переписывают под код.
- Рост спецификации машина держит пофайлово: файл кода без мутаций `make mutate` и `A=--check` называют, `--strict`
  (шаг `/release`) — отказ; на каждую функцию — правило и ревью. С `--strict` отказ и за «=», которую тесты убили:
  строка «=» неверна.
- `make new-domain` дописывает блоки среза (`<домен>-01…24`, мутации аудитора SC01–SC25 без несобиравшейся SC24)
  и сверяет их якоря `mutate.py --check <id…>` — только свои: устаревшие блоки форка срез не роняют; скрипту нужен
  python3, как `make mutate` и `make e2e`.
- Раннер: временные файлы go стадии — свой `GOTMPDIR` в `.cache/mutate/tmp`, снимается после стадии; `killpg` уже
  вышедшей группы — не ошибка; `TestMutate` гоняет раннер в гейте — гейту нужен python3.
- Ветка перенесена на `main` с треком harness+lifecycle (`7c03612`) после S6: конфликт — ячейка «git-хуки»
  `docs/contributing.md`, сведён без потерь (правки `main` и pre-commit/pre-push трека).
- Guard (RENF-10): путь сверяется нормализованным (`//`, `/./`), бинарь подкоманды (`git-commit`, и по пути) — та же
  подкоманда, git, исполняющий строку (`rebase -x`, `submodule foreach`, `bisect run`), — интерпретатор, а вложенный
  git — своя команда; `rg --pretty` — чтение. Случаи — в `internal/archtest/testdata/guard-cases.txt` (данные
  `TestGuardBash` трека harness): +15 отказов и +9 пропусков; раздел «guard-bash» `docs/contributing.md` — те же формы.
- Allow (RENF-13): `make tidy` и `go mod tidy` меняют `go.mod`/`go.sum` (зона «сначала спросить»), `find` пишет
  (`-delete`, `-exec`), `git branch` с `-D`/`-f` — тоже: в allow остались только чтения `git branch`; deny не менялся.
- Секреты и образы (RENF-14) — archtest `repo_test.go` по индексу (`git ls-files`): ловит `make check` до коммита и
  pre-push на верхнем коммите; секрет промежуточного коммита, убранный следующим, гейт верхнего коммита не видит.
  `.gitignore` знает `.env.*` (кроме `*.example`) — `.env.production` не уходит обычным `git add`.
- Имена `make new-domain`: список слов Go-шаблонов снят — он отвергал и имена, на которых срез собирается (`status`,
  `body`, `api`…). Имя, занятое кодом среза (`repo`, `id`, `tests`…), ловит сборка среза с откатом правок, ошибка
  называет имя; до первой правки отказывают ключевые слова и встроенные имена Go и объявления пакета домена.
- RTSTT-10: `ARGON2_MEMORY_KIB=8192` в `serviceEnv` (`cmd/api/boot_test.go`, трек code+security+adapters — одна
  строка): память хэша потоки тестов не меняет, параметры кита по умолчанию проверяет smoke релиза на стеке;
  `-p` у `test-int` не нужен — бинарь `cmd/api` один держал 2,2 ГБ.
- RTSTT-09: `e2e.sh` гоняет ruff и mypy харнеса до стека и ставит venv заново, когда сменился `requirements.txt`;
  smoke (шаг релиза) — ещё права (`test_authz.py`) и событие между доменами (`test_event_flow.py`); полный
  `make e2e` — инструмент разработчика, его потоки держат и Go-тесты.
- RENF-16: подсказка scope в `commit-msg` и пример `triage-failure` с настоящими именами — по строке в файлах трека
  harness (уже в `main`).
- RHAR-10 (дополнение дирижёра): строки `tests-pipeline` о `TestBoot`, `TestRun` и `TestIdentity` — по инварианту
  слоя, перечень проверок — в комментариях самих тестов.
- pre-push судит всё сообщение пушимого коммита хуком `commit-msg` из дерева его родителя (дополнение дирижёра,
  вместо subject текущим хуком): `git rebase --continue` и cherry-pick без конфликта commit-msg не зовут, и `feat`
  без `Review:` иначе уходил бы молча, а правило родителя не судит старые коммиты правилом, которого тогда не было.
  Корневой коммит и родитель без хука не судятся. Push из клона без веток remote'а в пустой remote судит и историю
  шаблона: `ea1de93 clean` и `a0f3fa9` (subject длиннее 72) — отказ; такой push — руками человека.
- Полный `make mutate` (прежний S10) дирижёр перенёс в S8 программы: его гоняют по коду после слияния обоих треков
  S7 — слияние с треком code+security+adapters сдвигает якоря (`N/validation-s7-gate.md` § 6). S10 — круг по
  валидатору, AC8 — без полного прогона.

## Handoff
—

## Где мы сейчас
S10 — круг по валидатору: B1 закрыт (`make gen` собирает код без моков). Дальше — B2, B3, затем неблокирующие.
