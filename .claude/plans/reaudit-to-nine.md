---
status: in-progress
parent: —
blocked-by: —
---

# reaudit-to-nine — шаблон до 9 по семи углам повторного аудита

## Цель
Закрыть всё, что по отчётам повторного аудита (`cc77e67`) мешает девятке: 21 medium-пункт и minor,
без которых эксперименты не чисты, — и подтвердить итог переаудитом свежими агентами той же рубрикой
и догфудингом новой фичи. Этот план — точка входа дирижёра: треки, владение файлами, порядок слияния,
решения владельца; содержание работы — в планах треков.

## Контекст
- **Отчёты аудита** — `/home/mm/.cache/stit-audit-program/reaudit/template-<угол>.md` (дальше `R/`), углы:
  harness, enforcement, tests, code, security, adapters, lifecycle. Находка — раздел `### <ID>`: где,
  воспроизведение, что сделать; «Что мешает девятке» — условие оценки; «Как повторить» и `R/tpl-<угол>/` —
  эксперименты для приёмки и переаудита. Формулировки пунктов бэклога, взятых в программу, —
  `git show ab46f45:.claude/plans/backlog.md`.
- **Правило оценки:** 9 — выше minor ничего нет и эксперименты чисты; 8 — одна medium; 7 — несколько.
- **Материалы программы** — `/home/mm/.cache/stit-audit-program/nine/` (дальше `N/`): worktree треков
  `N/wt-<трек>`, отчёты валидаторов `N/validation-<трек>.md`, переаудит `N/reaudit/`, замок `N/gate.lock`.
- **Метод.** Дирижёр (главная сессия) код не пишет: заводит треки, запускает агентов, читает их короткие
  отчёты, сливает ветки. Трек — свой план `.claude/plans/<slug>.md`, ветка `nine/<трек>` и worktree
  `N/wt-<трек>` от `main`. Исполнитель (свежий агент) первым коммитом пишет план трека, затем шаги — по
  коммиту на шаг, план тем же коммитом. Затем независимый валидатор (свежий агент, не автор)
  воспроизводит эксперименты аудитора на базе (дефект есть) и на ветке (дефекта нет), ищет обходы и ложные
  срабатывания, ревьюит дифф (у identity — и security), гейт — в свежем worktree; FAIL → фиксы `fix(...)` →
  перепроверка. Затем дирижёр читает рискованные места диффа сам, переносит ветку на `main` (rebase,
  `make check`) и сливает fast-forward; план трека удаляется тем же коммитом плана, след ревью — в
  «Сделано» шага этого плана.
- **Аккуратная параллельность.** Треки одной волны владеют непересекающимися файлами (таблица ниже); общий
  файл правят только в своём разделе. `.claude/plans/handoff.md` дописывают в конец, конфликты сводит
  дирижёр; `backlog.md` и этот план правит только дирижёр. Тяжёлые прогоны (`make check`, `make e2e*`,
  мутационный) — только под `flock N/gate.lock`: одновременно один. Одновременно — не больше трёх агентов
  всего; скиллы, которые сами запускают агентов (`/code-review` уровня high и выше), не используются.
  Машина: 20 ядер, 15 ГБ RAM, Docker.
- **Агенты владельца не спрашивают:** решают сами и пишут в «Решения» плана трека; то, что может решить
  только владелец, — остановка этого шага с вопросом в отчёте, остальное продолжается. `/code-review`,
  `/security-review`, Workflow и вложенных агентов изнутри не запускают: вызов повиснет.

### Треки и владение файлами

| Трек | План | Пункты | Владеет |
|---|---|---|---|
| gate, волна 1 | `close-gate-gaps` | RENF-01, 02, 04–09; RTSTT-01; RSECT-07; RCOD-05, 06; мутационный прогон | `Makefile` (кроме целей `setup`, `template-update`, `release`), `scripts/git-hooks/{pre-push,pre-commit}`, `scripts/tools/{cover,lint,test,migrations-check,e2e}.sh`, `scripts/install-tools.sh`, `scripts/new-domain.sh`, `.golangci.yml`, `internal/archtest/{make,generated}_test.go` и в `archtest_test.go` — `TestTestsAssert` и правило тегов сборки; `CLAUDE.md § Команды`, раздел гейта `docs/contributing.md`, rules `tests-pipeline`, `sqlc-and-migrations` |
| identity, волна 1 | `fix-identity-edges` | RCOD-01–04, 13; RSECT-01–04; RTSTT-02–06; окно access-JWT | `internal/domain/{user,auth,verification}/**`, `cmd/**`, новые файлы `migrations/`, `openapi.json`, `SECURITY.md`, `docs/stack/_templates/auth.md`, `.env.example`, rule `events-and-messaging`, `scaffold/references/event.md`, `docs/reference/domain-mapping.md` |
| lifecycle, волна 1 | `fix-fork-lifecycle` | RLIF-01–05; RENF-03; RADP-01 | `scripts/release.sh`, `scripts/release-preflight.sh`, `scripts/tools/{template-update,doctor,status,validate-stack-doc}.sh`, цели `setup`, `template-update`, `release` в `Makefile`, `.template/`, skills `init-service`, `upgrade`, `de-template`, `release`, `smartdev`, `stack-deep-dive`, `docs/reference/{ecosystem,release}.md`, `docs/stack/`, кроме `_templates/auth.md`, `internal/archtest/{template,onboarding}_test.go`, `scaffold/references/port.md` |
| boundaries, волна 2 | `enforce-domain-boundaries` | RADP-02, 03, 04, 09 | `internal/archtest/{archtest,crossdomain,layout,layout_settings}_test.go`, правки доменов под новые правила, rule `domain-architecture`, `scaffold/references/{port,endpoint,event}.md`, `docs/reference/architecture.md` |
| harness, волна 2 | `close-harness-gaps` | RHAR-01, 02, 03, 08; трение догфудинга T6; шаг `make mutate` в `/release` | `CLAUDE.md` (кроме § Команды), `internal/archtest/{harness,commitmsg,guard}_test.go`, `scripts/git-hooks/commit-msg`, skills `smartdev`, `triage-failure`, `stack-deep-dive`, `release`, `scaffold/SKILL.md`, `scaffold/references/domain.md`, `docs/README.md`, `CONTRIBUTING.md` |

Волна 2 стартует от `main` после слияния волны 1: boundaries — после identity (новые правила проверят и
его код), harness — после gate и lifecycle (шаг `make mutate`, проза по итогам).

## Не входит
- Minor, которые не закрыть в этом репо: к киту golibinfra, к strategy-репо, к зонам «сначала спросить» без OK
  владельца — строкой в бэклоге с причиной.
- Регистрация через подтверждение адреса (RSECT-03) и отказ при переполнении очереди хэша в ките
  (RSECT-01) — см. «Решения».
- golibinfra и bump его версии; правки `go.mod`/`go.sum`, `pkg/policy`, `internal/platform/authgate/`;
  выкатка (`make bump` в хабе); ветка `feat-feedback` догфудера прошлой программы.

## Критерии приёмки
- [ ] **AC1** — Каждый трек принят: валидатор воспроизвёл эксперименты аудитора своих пунктов (на базе —
  дефект, на ветке — нет), обходов и ложных срабатываний не нашёл, гейт в свежем worktree зелёный.
  Проверка: `N/validation-<трек>.md` → PASS у пяти треков
- [ ] **AC2** — Переаудит свежими агентами той же рубрикой и экспериментами: каждый из семи углов ≥ 9.
  Проверка: `N/reaudit/template-<угол>.md` → оценка ≥ 9, выше minor ничего нет
- [ ] **AC3** — Догфудинг новой фичи на итоговом `main`: трения записаны и исправлены, в том числе minor.
  Проверка: `N/dogfood.md` и коммиты фиксов
- [ ] **AC4** — Программа закрыта: планы треков и этот удалены, `make check` → `✓ check clean` на `main`,
  в `backlog.md` — только то, что не закрыть в этом репо, handoff — в накопителе. Проверка: `make status` → нет
  планов программы
- [ ] **AC5** — Тег `template/v1.2.0` с аннотацией и push `main` — с OK владельца. Проверка:
  `git ls-remote --tags origin template/v1.2.0` → есть

## Шаги
### Волна 1
- [x] **S1** — Трек gate (`close-gate-gaps`) принят и слит в `main`
  - Проверка: валидатор с ревью → PASS; `make check` на `main` → `✓ check clean` · Закрывает: AC1 · Зависит от: —
  - Сделано: `98e425e..02f7845` fast-forward (перенос на `main` — конфликт в комментарии пакета archtest);
    валидатор — FAIL (B1 `MAKEOVERRIDES`, B2 мутация в дереве разработчика после SIGTERM, B3 `.mockery.yaml`) →
    фиксы, неблокирующие, хвосты (спецификация мутаций под код identity, она же — код форка, флак ETXTBSY) → PASS:
    гейт в свежем worktree на `02f7845` → `✓ check clean`, 89 кейсов аудитора пойманы, `make mutate` — 356
    мутаций, 346 убиты, 10 эквивалентных, код 0, `A=--strict` — для шага `/release` (`N/validation-gate.md`);
    ревью — валидатор, дирижёр — самозащита `Makefile` и хуки; остаток minor — в бэклоге; план трека удалён
- [x] **S2** — Трек identity (`fix-identity-edges`) принят и слит в `main`
  - Проверка: валидатор с ревью и security → PASS; `make check` → `✓ check clean` · Закрывает: AC1 · Зависит от: —
  - Сделано: `1acaf61..d5fa709` fast-forward; валидатор с security — FAIL (B1–B3, N1–N7) → фиксы → FAIL (B4:
    `make e2e` — 429 строгого лимита) → фиксы, тесты на выживших мутантов трека gate (K12, K13, SD2), N8 дирижёра
    (ключ лимита сброса — адрес, а не id учётной записи) → PASS: гейт в свежем worktree на `0ab22c4` →
    `✓ check clean`, `make e2e` → 16 passed (`N/validation-identity.md`); ревью — валидатор (корректность,
    security), дирижёр — ключ лимита запроса сброса (B1, N8); тело последнего коммита переписано (escape-запись
    вместо невидимых символов), дерево то же; план трека удалён
- [x] **S3** — Трек lifecycle (`fix-fork-lifecycle`) принят и слит в `main`
  - Проверка: валидатор с ревью → PASS; `make check` → `✓ check clean` · Закрывает: AC1 · Зависит от: —
  - Сделано: `e8ea150..a94979b` fast-forward; валидатор — FAIL (B1, N1–N9) → 8 фиксов → PASS, гейт в свежем
    worktree на `a94979b` → `✓ check clean` (`N/validation-lifecycle.md`); ревью — валидатор, дирижёр —
    `release.sh` (userinfo origin вне вывода, имя образа, имя переменной тега); план трека удалён

### Волна 2
- [x] **S4** — Трек boundaries (`enforce-domain-boundaries`) принят и слит в `main`
  - Проверка: валидатор с ревью → PASS; `make check` → `✓ check clean` · Закрывает: AC1 · Зависит от: S2
  - Сделано: `f825fac..da82ffa` fast-forward (перенос на `main` с gate — без конфликтов); валидатор — FAIL (B1: ключ
    вызывающего в любом вызове) → фиксы, неблокирующие и добор gate (тест `new-domain` у форка с доменом `order` и
    после prune, контроль U25) → FAIL (B2: фикс ложных отказов открыл три обхода правила policy) → фиксы
    продолжателем → PASS: гейт в свежем worktree на `da82ffa` → `✓ check clean` при пороге 100 %, пробное слияние
    с harness — ✓ (`N/validation-boundaries.md`); ревью — валидатор, дирижёр — состав диффа (продакшн-код не
    тронут); остаток minor — в бэклоге; план трека удалён
- [x] **S5** — Трек harness (`close-harness-gaps`) принят и слит в `main`
  - Проверка: валидатор с ревью → PASS; `make check` → `✓ check clean` · Закрывает: AC1 · Зависит от: S1, S3
  - Сделано: `5d88ad9..59b5dad` fast-forward (перенос на `main` с boundaries; тесты harness — в перечне комментария
    пакета archtest); валидатор — FAIL (B1 ложные отказы `paths:` и `name:`, B2 guard обезврежен полем обработчика)
    → фиксы и неблокирующие → FAIL (B3: `paths:` без кавычек — регрессия B1) → фиксы → PASS: гейт в свежем
    worktree на `59b5dad` → `✓ check clean`, 59 тестов archtest из 59 в перечне (`N/validation-harness.md`); ревью
    — валидатор, дирижёр — решение о `timeout` у guard (от 1 с); остаток minor — в бэклоге; план трека удалён

### Проверка и выпуск
- [x] **S6** — Переаудит семи углов свежими агентами (не больше трёх одновременно) и догфудинг новой фичи
  от `main` после S1–S5
  - Проверка: семь отчётов и `N/dogfood.md` · Закрывает: AC2, AC3 · Зависит от: S4, S5
  - Сделано: четыре аудитора парами углов и догфудер (`N/reaudit/template-<угол>.md`, `N/dogfood.md`): код 9,
    security 9, обвязка 9, адаптеры 9, принуждение 9, тесты 9 (было 7–8), жизненный цикл 8 (NLIF-01 medium — первый
    перенос обвязки без `PATHS`); новых minor 24; догфудинг домена `note` на ветке `dog/note` — 12 трений (T4
    medium — флак `TestIdentity`). Всё — в S7; аудитор code+security начат от `5d88ad9` (см. «Решения»)
- [ ] **S7** — Добор: находки переаудита, трения догфудинга и minor бэклога — треками с валидаторами по углам;
  в бэклоге остаётся только то, что не закрыть в этом репо, с причиной
  - Проверка: повтор экспериментов находок → чисто; `make check` → `✓ check clean` · Закрывает: AC2, AC3 · Зависит от: S6
- [ ] **S8** — Закрытие: бэклог прополот, handoff в накопителе, тег `template/v1.2.0` и push — с OK владельца,
  план удалён
  - Проверка: `make status` → нет планов программы; тег на remote · Закрывает: AC4, AC5 · Зависит от: S7

## Решения
- Решения владельца (13.10, «давай сделаем» на рекомендации дирижёра):
  - RHAR-01: ревью проводит принимающая сторона (хаб, юзер, дирижёр); след — строка `Review:` в теле
    коммита (у `feat` и `fix` её требует хук `commit-msg`) или в «Сделано» шага плана; агент без
    собеседника пишет, кто принимает работу.
  - RSECT-03: 409 на занятый адрес остаётся — уникальность адреса решает продукт; компромисс записан в
    `SECURITY.md` и шаблоне `auth.md`.
  - RSECT-01: строгий лимит на `/api/auth/*` в шаблоне и требование лимита в `docs/stack`; отказ при
    переполнении очереди хэша — пунктом бэклога (раздел других репозиториев).
  - RTSTT-01: порог покрытия равен факту (100) с явным списком непокрываемого; мутационный прогон —
    `make mutate` и шаг `/release` (прогону нужны Docker и локальное окружение — расписание в облаке не годится).
  - Цель — девятка по всем углам; режим — параллельные треки с валидаторами; догфудинг — новая фича.
  - Миграция для `reset-timing` согласована программой: новый файл, существующие миграции не правим.
- Решение владельца (15.10, «миноры тоже думаю можно закрыть, чтобы у нас чисто все было уже в шаблоне»): S7 закрывает
  и minor — новые находки переаудита, трения догфудинга и пункты бэклога; не закрываемое в этом репо — в бэклог.
- Не больше трёх агентов одновременно, ревью — в задании валидатора, без скиллов, которые сами запускают
  агентов, — решение владельца (13.10): `/code-review` уровня high запустил десять ревьюеров.
- Пункты по трекам: RENF-03 — в lifecycle (он владеет `status.sh` и `doctor.sh`), RADP-01 — в lifecycle
  (`onboarding_test.go`), шаг `make mutate` в `/release` — в harness (цель приносит gate, `release` в волне 1
  правит lifecycle).

- S6: аудитор code+security стартовал от `5d88ad9` до слияния S5 — трек harness не трогает код и security-поверхность;
  дифф S5 аудитор сверяет в конце (решение дирижёра: волна 2 закончилась бы позже на время его аудита).
- Строку `Review:` хук требует и у `chore(template)` (трек S7 harness+lifecycle, NHAR-04): перенос обвязки с `PATHS`
  несёт код входа и сессий, а он по § Готово требует `/security-review` — расширение решения владельца по RHAR-01.
- `timeout` у guard-хука засчитывается от 1 с: guard работает 15–20 мс, обезвреживает только более короткий таймаут.

## Handoff
- Другие репозитории (golibinfra RSECT-01 и RCOD-16, strategy-схема, хаб RLIF-02) — разделом бэклога «Другие
  репозитории — сшивка владельцем»: владелец (15.10) сводит их сам, программа работает только в этом репо.
- Аннотация `template/v1.2.0` (S8), из трека lifecycle. Breaking: путь образа — `<host>/<org>/<репо>` из origin,
  тег в хабе — `<РЕПО>_TAG` (блок RLIF-04 — в накопителе). Гейт: `make doctor` и `release-preflight env` — FAIL
  без хуков → `make setup`; валидатор stack не считает заглушку решением. Обвязка: `PATHS=` у
  `make template-update`, строка `template:` в doctor, раздел «Безопасность» аннотации, `Dockerfile*` в путях шаблона.
- Из трека identity (блоки S1, S2, S5, S7, S8 — в накопителе): решения продукта для владельца и хаба — учётные
  записи-дубли по адресу без пробелов в данных окружений, предел имени 200, лимит `/api/auth/*` 5 rps с запасом 10
  (`HTTP_TRUST_PROXY_HOPS` за прокси); в аннотацию `template/v1.2.0` — событие
  `verification.events.password_reset_requested.v3`, миграция `0006` до замены api, env `HTTP_AUTH_RATE_LIMIT_*`.
- Из трека gate — в аннотацию `template/v1.2.0`: порог покрытия 100 % с явным списком непокрываемого (`cover.sh`),
  `gofumpt -extra` у `make fmt` и `fmt-check`, шаг build — `go build ./...` без тегов, ограничение сборки — только
  `integration` в тестах, `make mutate` (спецификация `scripts/tools/mutants.txt` — код форка), хуки — `bash -p`.
- Из трека boundaries — в аннотацию `template/v1.2.0`, Breaking: новые правила archtest входа, ошибок, подписок и
  policy (`delivery_test.go`); перечень китов входа `deliveryKits` — в `layout_settings_test.go` форка: после
  `make template-update` его дописывают из шаблона.
- Из трека harness — в аннотацию `template/v1.2.0`, раздел «Гейт»: хук `commit-msg` требует у `feat` и `fix` строку
  `Review: <кто или чем> — <итог>` в теле (и у `--amend`/`-C` прежнего коммита); archtest сверяет с репо имена тестов
  в прозе, glob'ы `paths:` правил (и в подкаталогах), `name:` скиллов, «Шаг 0», подключение guard-bash и deny — форк,
  который переименовал названный прозой тест или не перенёс пункт обвязки, получит красный гейт: перенести код
  (`make template-update TO=… PATHS='<пути>'`) или поправить свою прозу.

## Где мы сейчас
S1–S6 закрыты. S7: трек harness+lifecycle слит (`ab445c2`, валидатор — PASS, `N/validation-s7-harness.md`), его
неблокирующие — коммитами исполнителя поверх `main`; треки code+security+adapters (`nine/s7-code`) и gate+tests
(`nine/s7-gate`) идут. Следующее: их валидация и слияние, затем S8.
