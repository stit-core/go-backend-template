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
- [ ] **AC8** — Гейт и полный мутационный прогон ветки чисты. Проверка: `make check` → `✓ check clean`;
  `make mutate` → новых выживших нет (RA25 — «=» до слияния трека code+security+adapters)

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
- [ ] **S3** — migrations-check: `INSERT`, `.gitkeep`, выпущенные по содержимому; rule `sqlc-and-migrations`
  - Проверка: `TestMigrationsCheck`, опыт AC2; `make check` → `✓ check clean` · Закрывает: AC2 · Зависит от: S2
- [ ] **S4** — Хуки: pre-commit — снимок индекса, pre-push — subject пушимых коммитов и `-f Makefile`;
  переменные make — в хуках, оболочки — в `Makefile`
  - Проверка: опыты AC3; `make check` → `✓ check clean` · Закрывает: AC3 · Зависит от: S2
- [ ] **S5** — Покрытие и сила тестов: функции без тестов — по профилю до списка и в `cmd/`, fuzz и мягкие моки в
  `TestTestsAssert`, заголовок сгенерированного — только у вывода генераторов, `gochecknoinits`
  - Проверка: опыты AC4; `make check` → `✓ check clean` · Закрывает: AC4 · Зависит от: S2
- [ ] **S6** — Мутации: мутации аудитора, файлы без мутаций, блоки скелета `new-domain`, `created_at` скелета,
  раннер (временные каталоги, `killpg`), тест раннера; rule `tests-pipeline`, `CLAUDE.md § Команды`
  - Проверка: опыты AC5; `make check` → `✓ check clean` · Закрывает: AC5 · Зависит от: S5
- [ ] **S7** — Guard, allow, секреты и образы
  - Проверка: опыты AC6; `make check` → `✓ check clean` · Закрывает: AC6 · Зависит от: S4
- [ ] **S8** — Имена `new-domain`, e2e-харнес, лёгкий Argon2 тестов `cmd/api`, мелочи RENF-16
  - Проверка: опыты AC7; `make check` → `✓ check clean` · Закрывает: AC7 · Зависит от: S6
- [ ] **S9** — Приёмка: полный `make mutate` кусками под замком, повтор опытов AC1–AC7
  - Проверка: `make mutate` → код 0; `make check` → `✓ check clean` · Закрывает: AC8 · Зависит от: S8

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

## Handoff
—

## Где мы сейчас
S2 закрыт: шаги гейта не зависят от локали, версий инструментов и `.cache/`, `make gen` называет несобранный код.
Дальше — S3 (migrations-check).
