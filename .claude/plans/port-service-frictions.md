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
- [ ] **AC2** — отказ pre-push по сообщению коммита называет ветку шаблона (`git fetch --no-tags template main`), этап 0
  `/init-service` — ветку и теги `template/*` форка со слитой историей. Проверка: `go test ./internal/archtest -run TestPrePushGatesPushedCommit` → PASS; этап 0 SKILL.md
- [ ] **AC3** — проза о DLQ верна поведению natskit (без `NATS_JS_DLQ_SUBJECT` — Term и «jetstream message dead»), тесты
  с горутинами и временем — строкой rule `tests-pipeline`. Проверка: `git grep -n 'MAX_DELIVER.→DLQ'` → пусто
- [ ] **AC4** — подсказки домена — в references `/scaffold`, в правилах не больше строки на правило. Проверка: ручная —
  endpoint, field, migration, job, event references; `git diff template/v1.3.0 -- .claude/rules/` — по строке
- [ ] **AC5** — онбординг: гейт этапа по коду выхода, скоуп `SECURITY.md` — после своих доменов, пути strategy в
  README — `../strategy/…`. Проверка: ручная — `init-service` и `de-template` A.2
- [ ] **AC6** — без доменов идентичности мутации K10, K11, M29*, OC1 убивают тесты платформы. Проверка: `make mutate A="K10 K11 M29 OC1 …"` на копии после prune → убиты
- [ ] **AC7** — проверки небезопасных настроек — группа `config.ProvideUnsafe`, проверки шаблона — в `config.Module`.
  Проверка: `make test PKG=./internal/platform/config/...` → PASS; `make mutate A="C01 … C14"` → убиты
- [ ] **AC8** — archtest пускает таблицу-литерал из констант и тип вне провода без префикса, отказывает правке таблицы
  на месте; tzkit — в `rootLibs`. Проверка: `go test ./internal/archtest -run 'TestRuleExamples|TestDomainDeterminism|TestHTTPTypes'` → PASS
- [ ] **AC9** — зависимость кита графа (Redis у cachekit, MinIO у storagekit, NATS у natskit) поднимают boot-тест,
  smoke-стек и `make local-deps-up` сами; раннер мутаций поднимает их пулы только для китов графа. Проверка: `make check`
  → `✓ check clean`; копия с cachekit и storagekit — `make test-int PKG=./cmd/api/...` → PASS
- [ ] **AC10** — `events: none` (только при `identity: external`): валидатор, `.template/events`, `TestEventsPaths`,
  чек-лист de-template, возврат — `/scaffold` вариант event. Проверка: `go test ./internal/archtest -run 'TestEventsPaths|TestStackValidator'` → PASS
- [ ] **AC11** — `make new-domain AGG=` — срез агрегата, имена по агрегату, `-s` не удваивается; G115 исключён по
  `infra/postgres/[^/]+\.go`. Проверка: копии `NAME=order`, `NAME=objects`, `NAME=ordering AGG=order` — `make check` и
  мутации среза → убиты
- [ ] **AC12** — остатки — строками `backlog.md`. Проверка: `make plan-check` → OK
- [ ] **AC13** — гейт и строгие мутации зелёные. Проверка: `make check` → `✓ check clean`; `make mutate A=--strict` → код 0

## Шаги
### Обвязка и тексты
- [x] **S1** — T1: `make fmt` правит порядок тегов tagalign (`fmt.sh`, `Makefile`, `CLAUDE.md`, `TestFmtAppliesLintRules`)
  - Проверка: `go test ./internal/archtest -run TestFmtAppliesLintRules` → PASS · Закрывает: AC1
  - Сделано: после gofumpt — `golangci-lint run --enable-only tagalign --fix` закреплённой версии; тест — поле с тегами
    не по порядку; PASS; Review: принимает сессия корня — дифф отдан, ревью не проводилось
- [ ] **S2** — T2: подсказка pre-push — ветка шаблона; этап 0 `/init-service` — форк со слитой историей
  - Проверка: `go test ./internal/archtest -run TestPrePushGatesPushedCommit` → PASS · Закрывает: AC2
- [ ] **S3** — T3+T5: DLQ и повтор подписчика в прозе; строка о тестах с горутинами и временем
  - Проверка: `go test ./internal/archtest -run 'TestProse|TestRule'` → PASS · Закрывает: AC3
- [ ] **S4** — T4: подсказки домена в references `/scaffold`, по строке в `sqlc-and-migrations` и `domain-architecture`
  - Проверка: `go test ./internal/archtest -run TestProse` → PASS · Закрывает: AC4
- [ ] **S5** — T6: онбординг — код выхода гейта, скоуп `SECURITY.md`, пути strategy в README
  - Проверка: `go test ./internal/archtest -run TestProse` → PASS; `make check` → `✓ check clean` · Закрывает: AC5
### Код платформы и archtest
- [ ] **S6** — T7: тесты платформы без доменов идентичности (`TestBoot_Jobs`, `TestFxGraphProvidesDomainDeps`,
  `TestBoot_RateLimitKey`, задача outbox на источник, имена исходов, `TestDecision`)
  - Проверка: `make test-int PKG=./cmd/api/...` → PASS; `make mutate A=…` → убиты · Закрывает: AC6
- [ ] **S7** — T10: группа `config.ProvideUnsafe`, проверки Cookie, Log, Docs, JWT — в `config.Module`
  - Проверка: `make test PKG=./internal/platform/config/...` → PASS; `make mutate A="C01 …"` → убиты · Закрывает: AC7
- [ ] **S8** — T11: таблица-литерал, тип вне провода, tzkit в `rootLibs`
  - Проверка: `go test ./internal/archtest` → PASS; `make check` → `✓ check clean` · Закрывает: AC8
### Киты, события, срез
- [ ] **S9** — T8: зависимости китов графа — `kits.sh`, профили стека, `local-deps`, `kitDeps`, пулы раннера
  - Проверка: `go test ./internal/archtest -run TestMutateShared` → PASS; копия с китами — boot-тест PASS · Закрывает: AC9
- [ ] **S10** — T9: `events: none`, NATS — зависимость кита
  - Проверка: `go test ./internal/archtest` → PASS; `make check` → `✓ check clean` · Закрывает: AC10 · Зависит от: S9
- [ ] **S11** — T12: `make new-domain AGG=` и G115 по файлам `infra/postgres`
  - Проверка: копии `NAME=order`, `NAME=objects`, `NAME=ordering AGG=order` — `make check`, мутации среза · Закрывает: AC11
- [ ] **S12** — остатки — в бэклог
  - Проверка: `make plan-check` → OK · Закрывает: AC12
- [ ] **S13** — гейт, строгий прогон мутаций, черновик аннотации `template/v1.4.0` в «Handoff»
  - Проверка: `make check` → `✓ check clean`; `make mutate A=--strict` → код 0 · Закрывает: AC13

## Решения
—

## Открытые вопросы
—

## Handoff
—

## Где мы сейчас
S1 (T1) закрыт; следующий шаг — S2 (T2).
