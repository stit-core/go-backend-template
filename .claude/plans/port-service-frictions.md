---
status: in-progress
parent: —
blocked-by: —
---

# port-service-frictions — общие трения шаблона проекта на шаблонах stit — в шаблон

## Цель
Перенести в шаблон трения, которые нашли сервисы проекта на шаблонах stit (T1–T12 спецификации сессии корня), без
привязки к проекту: форк получает их обвязкой и кодом форка выпуска `template/v1.4.0` (выпуск — сессия корня).

## Контекст
- Источник — шаблон проекта на `template/v1.2.0` (только читать): коммиты be344d7, 2fd75cd, 52b4e5e, 6554a3f, 354cf1b,
  80a10ed, 81732ae, f892d27, 29e4d2f, 6ae8145, defac94, bce78e5, 4209590, 1139399, 9a22f59; отчёты его сервисов —
  вне репо. Спецификация сессии корня — вне репо (`frictions-spec.md` её сессии).
- Форма stit остаётся: домены идентичности `auth`/`user`/`verification`, `authgate` на cookie-сессии, путь
  `/api/<ресурс>`, OperationID lowerCamel, ошибка поля — 422 с `Field`, срез `make new-domain` под
  `identitytest.Admin/Client`.
- Решения владельца (06.11): T11 и исключение G115 по `infra/postgres/[^/]+\.go` — ослабления гейта «да»;
  `events: none` — да, без варианта `consume`; сервис без доменов и без базы — строками бэклога; проверка Origin
  (CSRF) — не здесь.
- Пометки: **Ш** — путь шаблона (`.template/paths`), приходит переносом; **Ф** — код форка, в аннотации тега — раздел
  «Платформа» с `PATHS`.

## Не входит
- Проектное: вход своим китом проекта, его module path и домены, заготовка стека из паспорта, выпуски шаблона
  проекта, путь `/<контекст>/`, `no-store` всех ответов, канон 400 предметной ошибки, исключения данных в событиях,
  поток событий ≤168h, UUID v7 среза; вариант `consume` событий.
- CSRF (проверка Origin в httpkit golibinfra) — отдельным шагом сессии корня.
- Тег `template/v1.4.0`, `.template/version`, push — выпуск сессии корня.

## Критерии приёмки
- [x] **AC1** — `make fmt` правит порядок тегов tagalign, как требует шаг lint. Проверка: `go test ./internal/archtest -run TestFmtAppliesLintRules` → PASS
  - Подтверждено: `go test ./internal/archtest -run TestFmtAppliesLintRules` → PASS
- [x] **AC2** — отказ pre-push по сообщению коммита называет ветку шаблона (`git fetch --no-tags template main`), этап 0
  `/init-service` — ветку и теги `template/*` форка со слитой историей. Проверка: `go test ./internal/archtest -run TestPrePushGatesPushedCommit` → PASS; этап 0 SKILL.md
  - Подтверждено: `TestPrePushGatesPushedCommit` → PASS; этап 0 — remote, ветка и теги; этап 5 ссылается на него
- [x] **AC3** — проза о DLQ верна поведению natskit (без `NATS_JS_DLQ_SUBJECT` — Term и «jetstream message dead»), тесты
  с горутинами и временем — строкой rule `tests-pipeline`. Проверка: `git grep -n 'MAX_DELIVER.→DLQ'` → пусто
  - Подтверждено: `git grep` → пусто (rule, architecture, domain-mapping, шаблон messaging, накопитель handoff)
- [x] **AC4** — подсказки домена — в references `/scaffold`, в правилах не больше строки на правило. Проверка: ручная —
  endpoint, field, migration, job, event references; `git diff template/v1.3.0 -- .claude/rules/` — по строке
  - Подтверждено: endpoint (query explode, omitempty, UUID, предел тела), migration (jsonb, nil-срез), job (имя,
    справочник выкатки), event (обратная запись схемы); rules — по строке в `sqlc-and-migrations` и `domain-architecture`
- [x] **AC5** — онбординг: гейт этапа по коду выхода, скоуп `SECURITY.md` — после своих доменов, пути strategy в
  README — `../strategy/…`. Проверка: ручная — `init-service` и `de-template` A.2
  - Подтверждено: `init-service` § Этапы, этапы 2–3; `de-template` A.2
- [x] **AC6** — без доменов идентичности мутации K10, K11, M29*, OC1 убивают тесты платформы. Проверка: `make mutate A="K10 K11 M29 OC1 …"` на копии после prune → убиты
  - Подтверждено: каждая мутация в отдельном worktree против одних тестов платформы (`-run` без тестов идентичности):
    OC1 — `TestResult_String`, P08 — `TestDecision`, K11 — `TestFxGraphProvidesDomainDeps`, K10 — `TestBoot_Jobs`,
    M29–M29h4 — `TestBoot_RateLimitKey` → убиты
- [x] **AC7** — проверки небезопасных настроек — группа `config.ProvideUnsafe`, проверки шаблона — в `config.Module`.
  Проверка: `make test PKG=./internal/platform/config/...` → PASS; `make mutate A="C01 … C14"` → убиты
  - Подтверждено: `go test ./internal/platform/config/` → PASS; `make mutate A="C01 … C20"` → убиты 20 из 20
- [x] **AC8** — archtest пускает таблицу-литерал из констант и тип вне провода без префикса, отказывает правке таблицы
  на месте; tzkit — в `rootLibs`. Проверка: `go test ./internal/archtest -run 'TestRuleExamples|TestDomainDeterminism|TestHTTPTypes'` → PASS
  - Подтверждено: PASS — примеры `determinism-table-{literal,changed,not-constant}`,
    `http-types-{handler-config,nested-no-prefix}`; `TestLayout` с tzkit в `rootLibs` → PASS
- [x] **AC9** — зависимость кита графа (Redis у cachekit, MinIO у storagekit) поднимают boot-тест, smoke-стек и
  `make local-deps-up` сами; раннер мутаций поднимает их пулы только для китов графа. Проверка: `make check`
  → `✓ check clean`; копия с cachekit и storagekit — `make test-int PKG=./cmd/api/...` → PASS
  - Подтверждено: `make check` → `✓ check clean`; копия с cachekit и storagekit — `make test-int PKG=./cmd/api/...`
    → PASS (`TestBoot` поднял Redis и MinIO), её smoke-стек — `/ready` с пробами cachekit и storagekit;
    `local-deps.sh` — на заглушке docker; `TestMutateShared` → PASS
- [ ] **AC10** — `events: none` (только при `identity: external`): валидатор, `.template/events`, `TestEventsPaths`,
  чек-лист de-template, возврат — `/scaffold` вариант event; NATS — зависимость кита natskit (boot-тест, smoke-стек,
  `local-deps`, пул раннера, тексты). Проверка: `go test ./internal/archtest -run 'TestEventsPaths|TestStackValidator'` → PASS
- [ ] **AC11** — `make new-domain AGG=` — срез агрегата, имена по агрегату, `-s` не удваивается; G115 исключён по
  `infra/postgres/[^/]+\.go`. Проверка: копии `NAME=order`, `NAME=objects`, `NAME=ordering AGG=order` — `make check` и
  мутации среза → убиты
- [ ] **AC12** — остатки — строками `backlog.md`. Проверка: `make plan-check` → OK
- [x] **AC14** — свежий worktree pre-push — на диске, в `.cache/` репо, а не в `${TMPDIR:-/tmp}`. Проверка:
  `go test ./internal/archtest -run TestPrePushGatesPushedCommit` → PASS
  - Подтверждено: PASS — дерево гейта в `<репо>/.cache/pre-push.*`, в TMPDIR его нет
- [x] **AC15** — мутацию, которую убила паника тестового бинаря вне теста (убийца — пакет), прогон по ключу не гоняет
  заново, пока в пакете есть тесты. Проверка: `go test ./internal/archtest -run TestMutate` → PASS
  - Подтверждено: `TestMutatePanicKiller` — паника в `TestMain` убивает, повтор берёт убийство по ключу, без тестов
    в пакете — заново и выживает; на раннере до фикса — FAIL на повторе
- [x] **AC16** — шаблон на golibinfra v1.8.0 (OpenTelemetry v1.47.0, x/net, x/crypto), тексты `Detail` таблиц ошибок
  и среза `make new-domain` — по-русски, правило — в `domain-architecture`, `/scaffold` endpoint и `architecture.md`.
  Проверка: `make mutate A=--anchors` → OK; `make check` → `✓ check clean`
  - Подтверждено: якоря 437 мутаций — по одному месту (AH8 — под новый текст); `make gen` без правок
    `openapi.json`; `make check` → `✓ check clean`
- [x] **AC17** — проверка Origin включена: `httpkit.SessionCookies{access_token, refresh_token}` — в графе сервиса и
  спецификации, `403 ORIGIN_NOT_ALLOWED` — в `openapi.json` изменяющих операций, `HTTP_CORS_ORIGINS` — в
  `.env.example`, клиенты тестов и смока шлют Origin, INFRA-HANDOFF в накопителе. Проверка: `make test-int
  PKG=./cmd/api/...` → PASS (`TestBoot_SessionOrigin`); мутации G15, G16 → убиты; `make check` → `✓ check clean`
  - Подтверждено: `make test-int PKG=./cmd/api/...` → PASS; G15 убил `TestBoot_SessionOrigin`, G16 — `TestBoot`;
    `openapi.json` — 403 `ORIGIN_NOT_ALLOWED` у изменяющих операций; `make check` → `✓ check clean`
- [ ] **AC13** — гейт и строгие мутации зелёные. Проверка: `make check` → `✓ check clean`; `make mutate A=--strict` → код 0

## Шаги
### Обвязка и тексты
- [x] **S1** — T1: `make fmt` правит порядок тегов tagalign (`fmt.sh`, `Makefile`, `CLAUDE.md`, `TestFmtAppliesLintRules`)
  - Проверка: `go test ./internal/archtest -run TestFmtAppliesLintRules` → PASS · Закрывает: AC1
  - Сделано: после gofumpt — `golangci-lint run --enable-only tagalign --fix` закреплённой версии; тест — поле с тегами
    не по порядку; PASS; Review: принимает сессия корня — дифф отдан, ревью не проводилось
- [x] **S2** — T2: подсказка pre-push — ветка шаблона; этап 0 `/init-service` — форк со слитой историей
  - Проверка: `go test ./internal/archtest -run TestPrePushGatesPushedCommit` → PASS · Закрывает: AC2
  - Сделано: подсказка — `git fetch --no-tags template main`; суд тегов при push в пустой remote не тронут; PASS;
    Review: принимает сессия корня — дифф отдан, ревью не проводилось
- [x] **S3** — T3+T5: DLQ и повтор подписчика в прозе; строка о тестах с горутинами и временем
  - Проверка: `go test ./internal/archtest -run 'TestProse|TestRule'` → PASS · Закрывает: AC3
  - Сделано: natskit сверен (`NATS_JS_DLQ_SUBJECT` по умолчанию пуст, Term после всех доставок); строка rule
    `tests-pipeline` § Структура и стиль; PASS
- [x] **S4** — T4: подсказки домена в references `/scaffold`, по строке в `sqlc-and-migrations` и `domain-architecture`
  - Проверка: `go test ./internal/archtest -run TestProse` → PASS · Закрывает: AC4
  - Сделано: UUID — `idkit.Validate` в usecase, отказ 422 с `Field`; предел тела — хук `HumaConfigs` (httpkit v1.7.0
    сверен: хук кита ставит `HTTP_MAX_BODY_BYTES` каждой операции); образец справочника — нейтральный; PASS
- [x] **S5** — T6: онбординг — код выхода гейта, скоуп `SECURITY.md`, пути strategy в README
  - Проверка: `go test ./internal/archtest -run TestProse` → PASS; `make check` → `✓ check clean` · Закрывает: AC5
  - Сделано: гейт этапа — `make check > .cache/check.log 2>&1; echo rc=$?`; скоуп `SECURITY.md` — этап 3; пути
    strategy — `../strategy/…` (de-template A.2); `TestProseReferencesExist` → PASS
### Код платформы и archtest
- [x] **S6** — T7: тесты платформы без доменов идентичности (`TestBoot_Jobs`, `TestFxGraphProvidesDomainDeps`,
  `TestBoot_RateLimitKey`, задача outbox на источник, имена исходов, `TestDecision`)
  - Проверка: `make test-int PKG=./cmd/api/...` → PASS; `make mutate A=…` → убиты · Закрывает: AC6
  - Сделано: `forwardedChain` — из `identity_test.go` в `boot_test.go` (prune удаляет первый); описания K10 и M29 —
    по новым убийцам; integration `cmd/api` → PASS; Review: принимает сессия корня — дифф отдан, ревью не проводилось
- [x] **S7** — T10: группа `config.ProvideUnsafe`, проверки Cookie, Log, Docs, JWT — в `config.Module`
  - Проверка: `make test PKG=./internal/platform/config/...` → PASS; `make mutate A="C01 …"` → убиты · Закрывает: AC7
  - Сделано: `Guard(app, []Unsafe)` — отказ сортирован; блоки C01–C12 переписаны под проверки, C13–C20 — новые
    (модуль без проверки, группа, порядок, пусто); убиты 20 из 20; Review: принимает сессия корня — дифф отдан,
    ревью не проводилось
- [x] **S8** — T11: таблица-литерал, тип вне провода, tzkit в `rootLibs`
  - Проверка: `go test ./internal/archtest` → PASS; `make check` → `✓ check clean` · Закрывает: AC8
  - Сделано: таблица-литерал из констант законна, правка на месте (`delete`, `clear`, `copy`, `append` в срез,
    `slices`/`sort`/`maps`) — отказ; префикс — у типа провода (`Body`, теги провода, тип в их полях); tzkit — в
    `rootLibs` (Ф); проза — rule `domain-architecture`, `architecture.md`; `make check` → `✓ check clean`;
    Review: принимает сессия корня — дифф отдан, ревью не проводилось
### Киты, события, срез
- [x] **S9** — T8: зависимости китов графа — `kits.sh`, профили стека, `local-deps`, `kitDeps`, пулы раннера только
  китов графа (бэклог форка `mutate-servers-by-graph`)
  - Проверка: `go test ./internal/archtest -run TestMutateShared` → PASS; копия с китами — boot-тест PASS · Закрывает: AC9
  - Сделано: `kits.sh` — по `go list -deps ./cmd/api`; профили `cachekit`/`storagekit` стека и второй compose-файл
    сервиса; `local-deps` и `kitDeps` — по тем же китам; раннер: Start* зависимости кита в boot-тесте — пул только у
    кита графа; копия с китами — PASS; `make check` → `✓ check clean`; Review: принимает сессия корня — дифф отдан,
    ревью не проводилось
- [ ] **S10** — T9: `events: none`, NATS — зависимость кита `natskit` и в текстах (бэклог форка `kit-deps-nats-docs`)
  - Проверка: `go test ./internal/archtest` → PASS; `make check` → `✓ check clean` · Закрывает: AC10 · Зависит от: S9
- [ ] **S11** — T12: `make new-domain AGG=` и G115 по файлам `infra/postgres`
  - Проверка: копии `NAME=order`, `NAME=objects`, `NAME=ordering AGG=order` — `make check`, мутации среза · Закрывает: AC11
### golibinfra v1.8.0
- [x] **S16** — golibinfra v1.8.0 и тексты ответов по-русски
  - Проверка: `make mutate A=--anchors` → OK; `make check` → `✓ check clean` · Закрывает: AC16
  - Сделано: `go get …golibinfra@v1.8.0`, `go mod tidy` (otel v1.47.0, contrib v0.72.0, x/net v0.63.0, x/crypto
    v0.59.0); `Detail` 33 правил доменов идентичности, `authgate`, `authlimit` и двух правил среза — по-русски; якорь
    AH8; строка правила — `domain-architecture`, `/scaffold` endpoint, `architecture.md`; тестов, которые сверяли бы
    английский текст ответа, нет; `make check` → `✓ check clean`; Review: принимает сессия корня
- [x] **S17** — проверка Origin у изменяющего запроса с cookie сессии (Handoff golibinfra v1.8.0)
  - Проверка: `make test-int PKG=./cmd/api/...` → PASS; `make mutate A="G15 G16"` → убиты; `make check` →
    `✓ check clean` · Закрывает: AC17 · Зависит от: S16
  - Сделано: `sessionCookies` (`httpkit.SessionCookies{access_token, refresh_token}`) — в `authgate.Module` и
    `SecurityScheme`; `HTTP_CORS_ORIGINS` — в `.env.example` (dev-сервер SPA) и стеке смока; клиенты тестов
    `cmd/api` и харнеса шлют Origin; `TestBoot_SessionOrigin`; мутации G15, G16; `architecture.md`, `SECURITY.md`;
    INFRA-HANDOFF в накопителе; Review: принимает сессия корня
### Бэклог форка шаблона
- [x] **S14** — `prepush-worktree-disk`: свежий worktree pre-push — в `.cache/` репо, с тестом хука
  - Проверка: `go test ./internal/archtest -run TestPrePushGatesPushedCommit` → PASS · Закрывает: AC14
  - Сделано: `mktemp -d "$root/.cache/pre-push.XXXXXX"` вместо `${TMPDIR:-/tmp}`; тест хука — `dir=$(CURDIR)`
    игрушечного гейта под `.cache/pre-push.` репо и пустой TMPDIR; Review: принимает сессия корня
- [x] **S15** — `mutate-panic-killer`: убийца-пакет засчитывается, пока в пакете есть тесты, с тестом раннера
  - Проверка: `go test ./internal/archtest -run TestMutate` → PASS · Закрывает: AC15
  - Сделано: `test_names` — и пакеты с тестами (путь модуля из `go.mod`), `killer_present` — пакет как есть, тест —
    без подтеста; строка правила `tests-pipeline`; Review: принимает сессия корня
- [ ] **S12** — остатки — в бэклог
  - Проверка: `make plan-check` → OK · Закрывает: AC12
- [ ] **S13** — гейт, строгий прогон мутаций, черновик аннотации `template/v1.4.0` в «Handoff»
  - Проверка: `make check` → `✓ check clean`; `make mutate A=--strict` → код 0 · Закрывает: AC13

## Решения
- Go 1.26.11 (GO-2026-6859 в net/http) — коммитом до S8 по задаче сессии корня: toolchain `go.mod`, образ сборки
  `Dockerfile` и `Dockerfile.migrate`; дайджест — индекс тега в реестре сейчас (в нём добавился riscv64, манифест
  linux/amd64 тот же, что у индекса форка).
- Пункты бэклога форка шаблона — по задаче сессии корня: `mutate-servers-by-graph` — в S9, `kit-deps-nats-docs` — в
  S10, `prepush-worktree-disk` — S14 (правка хука pre-push: каталог worktree гейта, проверок не меньше),
  `mutate-panic-killer` — S15; `archtest-golibinfra-version` — строкой бэклога (S12): дока с версией golibinfra в
  шаблоне нет, `docs/stack/dependencies.md` заводит форк.
- golibinfra v1.8.0, тексты по-русски и проверка Origin (S16, S17) — в этом плане, до S10: решение владельца 24.11 —
  тексты ответов API по-русски (golibinfra v1.8.0, breaking minor), проверку Origin шаблон включает с её выпуском;
  тексты среза `make new-domain` правит S16, T12 (S11) их сохраняет.

## Открытые вопросы
—

## Handoff
—

## Где мы сейчас
S1–S9 (T1–T8, T10, T11), Go 1.26.11, S16 (golibinfra v1.8.0, тексты по-русски) и S17 (проверка Origin) закрыты,
`make check` на S17 — `✓ check clean`; следующий шаг — S10 (T9).
