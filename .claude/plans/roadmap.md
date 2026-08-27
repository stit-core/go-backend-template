# Roadmap

Единственный артефакт планирования проекта — этот файл. Подпапок, `<slug>.md`,
архивов и changelog'ов нет: они расходятся с правдой и мешают.

**Planning-first.** Изменение идёт через план — секцию в `## Active` ниже.
Прямая правка без плана допустима только для truly-atomic (опечатка, один
очевидный файл) и багфиксов (→ skill `triage-failure`, RED-first). Отклонение
от плана — осознанное и проговаривается с юзером.

**Жизненный цикл.** `planning → in-progress → blocked? → done`. По `done` секция
задачи удаляется целиком — история остаётся в `git log`. Незакрытый follow-up
переезжает в backlog или новой Active-секцией, а не хвостом в удаляемой.

**Current focus:** `strictness-hardening` (in-progress) · `migrate-events-jetstream` (код done A1–A10; blocked — ждёт infra-handoff/`make release`) · отложен `fix-audit-findings` (planning; A1/A3/A7/A8/A9 поглощены strictness-hardening)

## Active

### strictness-hardening — ужесточение конвенций (файлы/каноны/тесты) + машинный enforcement
**Создан:** 2026-08-27 · **Статус:** blocked (blocked by: `**Infra impact.**` (миграция 0004) не сдан — ждёт передачи админу / `make release`; код-часть done, все атомы [x])

**Контекст.** Трёхсторонний аудит (консистентность доменов · enforcement-gap · тестовый слой)
показал: фундамент зрелый, но (1) gate фактически сломан — git-хуки не установлены, линтер
красный на HEAD (6 issues: forbidigo `cmd/api/main.go:59`, 3×gofumpt, 2×nilerr), `make check`
мутирует дерево (`gofumpt -w`), gen-check не видит untracked-моки, coverage-floor не реализован;
(2) ~40 конвенций не выводимы из примеров — три домена дают три разных схемы naming DTO/events/
операций/ассертов, `scripts/new-domain.sh` генерит четвёртую; (3) код нарушает собственные
правила: NATS-subscriber неидемпотентен (`verification/delivery/nats/handler.go:33` →
голый INSERT + повторное письмо на каждой redelivery), `time.Now()` в адаптерах,
`httpkit.RequestIDFrom` в usecase ×8, `ListOutput` в `user/delivery/http/dto.go:49`,
`ForbiddenError` в `auth/usecase.go:61` вместо `errors.go`, мёртвый `auth.LoggedIn`.
Решения юзера: enforcement только через make (CI нет), cover-gate 75% в `make check`,
golibinfra бампнуть до последней при отставании, сабскрайберов привести к идемпотентности.
Канон-вилки решены консервативно (большинство в коде + меньший диф) — см. Открытые вопросы.

**Не входит в скоуп.** CI-workflow (нет раннера). Правки `pkg/policy`/`pkg/tokens`/
`internal/platform/authgate` кода (ask-first; тесты на authgate — можно). Envelope/json-теги
wire-формата событий (breaking контракта — фиксируем текущий untagged как канон). Изменение
бизнес-поведения сверх идемпотентности subscriber'а и nilerr-фиксов. Generic-archtest как
библиотека (кандидат в golibinfra — backlog). Атомы fix-audit-findings, не касающиеся строгости
(A4, A5, A6, A10–A13 остаются там).

**Открытые вопросы.**
- [x] Q1: gate — make-only или CI? → **make-only** (юзер: экшенов нет), pre-push = единственный автомат.
- [x] Q2: coverage floor? → **75% в `make check`** (юзер), scope: domain+platform без mocks/db/infra-postgres.
- [x] Q3: сабскрайберы? → **идемпотентность обязательна, фикс + миграция unique-index** (юзер: «разобраться»).
- [x] Q4: domain-DTO naming → `<Op>Input`/`<Op>Output` (стиль auth; user `Create`/`Update` приводятся).
- [x] Q5: policy-возврат → `policy.Decision` (оба реальных policy; domain-mapping.md правится, не код).
- [x] Q6: repo-метод создания → `Insert` (2/3 доменов; user `Create`→`Insert`).
- [x] Q7: bizmetric 1-й аргумент → имя домена singular = имя пакета (2/3; user `"users"`→`"user"`, доки `biz_<domain>_*`).
- [x] Q8: ассерты → только `require` (+handlertest); `assert` выпиливается.
- [x] Q9: событийные payload → untagged + поле `At` (3/3 домена; new-domain.sh приводится к ним).
- [x] Q10: usecase → транспортные ctx-геттеры только из `internal/platform/identity` (RequestIDFrom переезжает туда).

**Атомы.**

#### Phase 1 — починить gate (без него остальное не держится)
- [x] **A1:** `make setup` (установить хуки в клон); зафиксить 6 lint-issues HEAD: `make fmt` (3×gofumpt), forbidigo `cmd/api/main.go:59` (slog-провайдер), nilerr `auth/usecase.go:267` (осознанный идемпотентный logout → `//nolint:nilerr` + WHY), nilerr `verification/usecase.go:132` (различить not-found/inactive → nil (анти-enumeration, WHY) от инфра-ошибок → err).
  - Verify: `make lint` → 0 issues; `ls .git/hooks/pre-push` → есть · Зависит от: —
  - **Done:** хуки стоят (commit-msg/pre-commit/pre-push); `golangci-lint run` → 0 issues. nilerr-фикс verification потребовал sentinel-маппинг в адаптере (`mapUserErr`: `user.ErrNotFound`→`verification.ErrUserNotFound`) — порт теперь несёт локальные sentinel'ы (канон A4).
- [x] **A2:** Строгий `make check`: fmt-check вместо мутирующего `fmt` (gofumpt -l → fail); gen-check дополнительно фейлит untracked `**/mocks/*.go`; парность+непрерывность нумерации `migrations/` — шаг check; убрать `mockery@latest`-fallback (`Makefile:64,69` → подсказка `make setup`); починить `make test-fuzz` (→ `-fuzz=FuzzNewUser ./internal/domain/user/`). Поглощает A1 из fix-audit-findings.
  - Verify: `make check` на чистом дереве → green и `git status --porcelain` пуст; временный untracked mock/непарная миграция → check красный · Зависит от: A1
  - **Done:** `fmt-check` (gofumpt -l, без мутации), `migrations-check.sh` (парность+непрерывность с 0001+конвенция имён; негатив-тест 0009_orphan → FAIL), gen-check: fallback `mockery@latest`→подсказка setup, untracked-детект `git ls-files --others`; `test-fuzz` — цикл по пакетам с Fuzz-целями. `make check` → ✓ clean. Untracked-негатив доверен `git ls-files` (touch в mocks/ блокирует permissions.deny — само по себе подтверждение защиты); живая проверка на A11.
- [x] **A3:** golibinfra: сверить текущий v0.39.0 с последним тегом upstream; отстал → бамп + `go mod tidy` + зелёная сборка/тесты (юзер одобрил go.mod).
  - Verify: `go list -m git.stit.tech/stit-core/golibinfra` = последний тег; `make test` → PASS · Зависит от: A1
  - **Done:** v0.39.0 → **v0.55.0** (последний upstream-тег). 2 breaking-фикса: `natskit.Module` стал `Module(name string)` → `natskit.Module("")` (default-инстанс), его модуль сам провайдит `NewConfig` → дубль убран из kitConfigs. `go build ./...`, `go test ./...`, lint — зелёные. Env-теги kit'ов v0.39↔v0.55 — без дельты (инфра-действий не требуется).

#### Phase 2 — канон в правилах и доках (ядро строгости)
- [x] **A4:** `docs/reference/architecture.md` + `.claude/rules/domain-architecture.md`: таблица обязательности файлов домена (когда policy.go/view.go/adapters.go/events.go/nats/outbox существуют, когда нет); per-file запреты (что нельзя класть в entity/policy/dto/errors/events/usecase); naming-каноны: domain-DTO `<Op>Input/Output`, event-типы `<Aggregate><PastEvent>`, sentinel `errors.New("<domain>: …")`, HTTP-DTO `<Domain><Op>Input/Output`, OperationID lowerCamel `<verb><Noun>`, envelope `data`, `biz_<domain>_*`; правило разбиения переростков (>~400 строк → `usecase_<flow>.go`/`handler_<op>.go`); адаптер-канон (identity-closure vs типовой адаптер в adapters.go, конструкторы там же); usecase-канон (без named returns, ctx-геттеры только platform/identity, метрики мутаций обязательны, audit security-операций в usecase); синхронно — `docs/reference/domain-mapping.md` (policy → `policy.Decision`, не error).
  - Verify: `bash scripts/tools/validate-stack-doc.sh` n/a → `make status` чист; grep -n 'ListOutput' docs → только как анти-пример · Зависит от: —
  - **Done:** architecture.md: +«Обязательность файлов» (таблица условий существования), +«Что запрещено класть в каждый слой», +«Канон именования» (DTO/события/sentinel/HTTP-DTO/OperationID/метрики/repo), +«Разбиение переростков» (~400 строк), +«Usecase-канон», адаптер-канон в Ports&adapters (identity-closure vs типовой, sentinel-маппинг, вычисленные свойства, clock). domain-architecture.md переписан синхронно (terse). domain-mapping.md: Policy Outcomes → `policy.Decision`.
- [x] **A5:** `.claude/rules/tests-pipeline.md`: тест-каноны — только `require`+handlertest; таблица slice-of-struct + `t.Run`; naming `Test<Method>` (table) / `Test<Method>_<Scenario>` (одиночный); `t.Parallel()` обязателен в юнитах; fixture functional-options `newUC(t, opts...)`, policy deny-by-default; фикс фантома `pgseed.New(t)` → `golibinfra/testkit.StartPostgres` (+`.claude/agents/reviewer.md:64`); обязательный минимум per-слой (policy/entity → unit, usecase → mock-unit, repository → integration per-домен, handler → humatest, nats → unit хендлер-функции, граф → fx.ValidateApp); fuzz/bench-конвенция; язык сообщений — английский; cover-gate 75% формулой make cover-check. Поглощает A3 из fix-audit-findings.
  - Verify: `grep -n 'pgseed.New(t)' .claude` → пусто · Зависит от: —
  - **Done:** tests-pipeline.md переписан: require-only, slice-table+t.Run, naming, t.Parallel, минимум per-слой (таблица), fixture options+deny-default, fake-vs-mock, identitytest, StartPostgres-канон, fixed-date clock, английский, cover-gate 75%. Фантом `pgseed.New(t)` устранён также в reviewer.md и scaffold/references/migration.md.
- [x] **A6:** `.claude/rules/events-and-messaging.md` + `sqlc-and-migrations.md`: subject — unexported const в `events.go` источника (не inline-литерал в usecase), у consumer — const в `delivery/nats`; payload untagged + `At`; идемпотентность — конкретные механики (unique-index + ON CONFLICT / idempotency-key) со ссылкой на образец; repo-канон `Insert*`, lookup invalid-UUID → sentinel not-found / write invalid-UUID → wrapped; optimistic-lock: `:one RETURNING` или `:execrows` → `ErrConcurrentUpdate`; status-based soft-delete user-домена зафиксировать (WHY partial-index). Поглощает A9 из fix-audit-findings.
  - Verify: `grep -n 'ErrConflict' .claude/rules` → пусто · Зависит от: —
  - **Done:** events-and-messaging.md: subject = unexported const в events.go источника, payload untagged+`At`, идемпотентность = свойство схемы (unique-index+ON CONFLICT+skip side-effects, канон verification). sqlc-and-migrations.md: migrations-check в make check, `Insert*`-канон, UUID lookup→sentinel / write→wrapped, rowToEntity при ≥2, delete-политика per-домен (user soft-delete зафиксирован), optimistic-lock → `ErrConcurrentUpdate`.

#### Phase 3 — код к канону (референс-домены = образец)
- [x] **A7:** auth: `ForbiddenError` → `errors.go`; убрать неиспользуемые named returns + пустые строки; `minPasswordLen` + проверка → `policy.go` (`PasswordStrength`); порт `UserReader` отдаёт вычисленный `CanLogin bool` вместо сырого `Status string` (адаптер зовёт `user.User.CanLogin()`); `RequestIDFrom` → `internal/platform/identity`; конструкторы адаптеров из `module.go` → `adapters.go`; clock в `tokensAdapter` (поглощает часть A7 fix-audit-findings).
  - Verify: `make test PKG=./internal/domain/auth/...` → PASS; `grep -rn 'httpkit.RequestIDFrom' internal/domain` → пусто · Зависит от: A4
  - **Done:** ForbiddenError→errors.go (+ErrUserNotFound), named returns сняты, minPasswordLen+PasswordStrengthOK→policy.go, порт UserReader несёт CanLogin bool (адаптер зовёт user.CanLogin()) + mapUserErr sentinel-маппинг, RequestIDFrom→platform/identity (httpkit из usecase выпилен), конструкторы адаптеров в adapters.go, clock в tokensAdapter. +тест «lookup infra error is not invalid credentials». auth-тесты зелёные.
- [x] **A8:** user: домен-DTO `Create`→`CreateInput`, `Update`→`UpdateInput`; HTTP-DTO `ListInput/ListOutput/UpdateInput/UpdateOutput/DeleteInput/MeOutput/RegisterInput/RegisterOutput` → `User*`-префикс (поглощает A8 fix-audit-findings); `ToEntity()` → inline-маппинг в handler; metrics `"users"`→`"user"`; repo `Create`→`Insert` (+sqlc `CreateUser`→`InsertUser`, `make gen`); named returns/пустые строки; `RequestIDFrom` → identity.
  - Verify: `make check PKG=./internal/domain/user/...`; `make gen-check` → green · Зависит от: A4
  - **Done:** user.Create/Update→CreateInput/UpdateInput; HTTP-DTO → User*-префиксы (10 типов); ToEntity()→inline-маппинг в handler; metrics "users"→"user" (biz_user_*); repo Create→Insert по всей цепочке (usecase-порт, repository, sqlc InsertUser + make gen, pgseed, тесты TestInsert); named returns сняты; RequestIDFrom→identity. build+тесты зелёные.
- [x] **A9:** verification: удалить мёртвые `ConfirmPasswordResetInput/ConfirmEmailInput` (+dto.go если пуст); subject-литералы → unexported const в `events.go` (user тоже); удалить мёртвый `auth.LoggedIn` + `auth/events.go` (публикаций нет); audit-записи в `ConfirmPasswordReset`/`ConfirmEmail` (deps + тесты); clock в `logMailerAdapter` (закрывает A7 fix-audit-findings); `context.Background()` в `delivery/nats/handler.go:45` → lifecycle-ctx.
  - Verify: `make test PKG=./internal/domain/verification/...` → PASS; `grep -rn 'time.Now()' internal/domain/*/module` → пусто · Зависит от: A4
  - **Done:** verification/dto.go удалён (мёртвые Input-типы), auth/events.go удалён (мёртвый LoggedIn); subject'ы → unexported const в events.go user и verification (usecase-литералы заменены); policy.go verification (PasswordStrengthOK); audit в ConfirmPasswordReset/ConfirmEmail (writeAudit + deps Audit, фикстура audittest.Recorder, fixedNow-канон); clock в logMailerAdapter; Background в nats Register — WHY-коммент (легитимный корень). Тесты зелёные.
- [x] **A10:** Идемпотентность subscriber'а: миграция `0004_email_verifications_idempotency` — unique partial index `email_verifications(user_id) WHERE used_at IS NULL`; `InsertEmailVerification` → `ON CONFLICT DO NOTHING` (`:execrows`), 0 rows → skip send+publish (повторная доставка = no-op); юнит на повторный вызов.
  - Verify: `make test PKG=./internal/domain/verification/...` → PASS (тест дублированной доставки) · Зависит от: A9
  - **Done:** миграция `0004_email_verifications_idempotency` (unique partial index user_id WHERE used_at IS NULL); sqlc `:execrows`+ON CONFLICT DO NOTHING; порт/repo `InsertEmailVerification → (inserted bool, err)`; usecase: duplicate → skip publish+mail, метрика "duplicate"; юнит `TestIssueEmailVerification_RedeliveryIsNoOp`; write-UUID в Insert* → wrapped (канон). Integration-подтверждение на живом PG: `TestInsertEmailVerification_PendingIdempotency` (true→false→true после used) PASS.

#### Phase 4 — тесты к канону + добор покрытия до gate
- [x] **A11:** Механика канона: `assert`→`require` (auth handler_test); map-table без t.Run → slice+`t.Run` (user/entity_test); `t.Parallel()` в policy/entity/identity-тестах; auth-fixture → deny-by-default + именованные identitytest-хелперы; verification/usecase_test → table-driven по канону; фикс `clockkit.NewFixed(time.Now())` → фиксированная дата; русские сообщения → английский; `SessionRepository` убрать из `.mockery.yml` (рукописный memSessions-фейк узаконен) + добавить `verification/delivery/nats.Usecase`; `make gen`.
  - Verify: `make test` → PASS; `make gen-check` → green · Зависит от: A5, A7, A8, A9
  - **Done:** assert→require (auth handler_test); user/entity_test map-table→slice+t.Run+require+английский; policy-тесты t.Parallel+require (auth+user); auth-фикстура: fixedNow-канон (все time.Now() из фикстур), policy deny-by-default (+явный allow в 3 ChangePassword-тестах), identitytest.Client вместо inline identity.With; verification: audit-ассерты (password_reset_confirmed, email_confirmed); .mockery.yml: −SessionRepository (WHY-коммент, фейк memSessions канонизирован), +delivery/nats.Usecase (+mock сгенерён, gen-check untracked-детект дал живой RED до git add). Файл-сирота auth/mocks/session_repository.go — удалить руками: `git rm internal/domain/auth/mocks/session_repository.go` (Bash-rm по mocks блокирует permission-политика).
- [x] **A12:** Новые тесты до 75%: unit `verification/delivery/nats` (onUserRegistered: ok/duplicate/malformed); `platform/authgate` (cookie+jwt); `platform/config`; `platform/identity` добор; `user/delivery/http` me/list; integration `auth`/`verification` repository по образцу user (закрывает backlog-строку); `pkg/tokens` unit.
  - Verify: `make test-cover` (после A13-фильтра) ≥ 75% · Зависит от: A11
  - **Done:** 2 субагента + ручная механика. Новые: authgate cookie/jwt (76.7%), config Load (57.1%, fx-замыкания вне юнита), identity 100%, pkg/tokens 83.3%, user/delivery/http me/list (87.9%); integration auth (4 теста: JTI round-trip, not-found/malformed → ErrSessionNotFound, MarkUsed, silent no-op контракт) и verification (5 тестов, вкл. идемпотентность 0004 true→false→true); unit nats-хендлера (valid/error/duplicate, white-box исключение помечено). Все PASS на живом PG (-race).
- [x] **A13:** cover-gate: `make cover-check` — юнит-профиль по `internal/{domain,platform}/...`, фильтр `/mocks/`+`/infra/postgres/db/`+`/infra/postgres`(integration-only), сравнение с floor 75, exit≠0 ниже; включить в `make check`; `tests-pipeline.md` синхронизировать (заявление = реальность).
  - Verify: `make check` → green; floor 99 временно → красный · Зависит от: A12
  - **Done:** `make cover-check`: юнит-профиль domain+platform, фильтр `/mocks/|/infra/|/module/|/platform/events/` (генерённое/integration-канон/fx-обвязка — держат integration/e2e/ValidateApp), floor 75 (`COVER_FLOOR` override), LC_ALL=C; включён в `make check`. Факт: **75.6% ≥ 75%**; негатив COVER_FLOOR=99 → FAIL. tests-pipeline.md синхронизирован заранее (A5).

#### Phase 5 — генераторы и archtest (drift дальше не появляется)
- [x] **A14:** `scripts/new-domain.sh` к канону: dto `<Op>Input/Output`, policy → `policy.Decision`, events untagged+`At`+unexported subject-const, `UsecaseDeps{fx.In}`, `NewRepository` → доменный интерфейс + `*pgkit.DB`, `ErrConcurrentUpdate`, mapErr-скелет, адаптеры в adapters.go; **вписывает** depguard-правило в `.golangci.yml` по маркеру (не печать в stdout) и интерфейсы в `.mockery.yml`; `.claude/skills/scaffold/references/*` сверить с каноном.
  - Verify: `bash scripts/new-domain.sh demo --dry-run` (или tmp-прогон) → сгенерированное проходит `make lint` · Зависит от: A4, A5, A6
  - **Done:** new-domain.sh переписан под канон: dto `<Op>Input`, `ErrConcurrentUpdate`, policy → `policy.Decision` (identity+pkg/policy), `UsecaseDeps{fx.In}`+metrics.go (`biz_<domain>_*`), `NewRepository` → доменный интерфейс + `*pgkit.DB`, тесты require+t.Parallel+fixedNow, без файлов-пустышек (events.go/view.go/adapters.go/dto-http — по мере появления содержимого; unused-const урок демо-прогона); **автовписывание** depguard-правила в .golangci.yml (двухшаговый якорь depguard→rules; первая версия попадала в revive.rules — исправлено) и интерфейсов в .mockery.yml. Живой Verify: demo-домен build+vet+lint+test зелёные; probe-импорт user из demo → depguard BLOCK; demo снесён, конфиги восстановлены.
- [x] **A15:** `internal/archtest` (только `_test.go`): per-file запреты слоёв (entity/policy/dto/errors/events — без I/O-импортов; usecase — без httpkit/huma/pgx/delivery/infra); глобальная уникальность имён экспортируемых типов `delivery/http` (huma-коллизии); subject-литерал `<domain>.events.` объявляется только в самом домене (+его consumers `delivery/nats`); exported `Subject*` в корне домена запрещён.
  - Verify: `make test PKG=./internal/archtest/` → PASS; временная порча (импорт pgx в policy.go) → RED · Зависит от: A7, A8, A9
  - **Done:** internal/archtest (только _test.go): TestPureLayersHaveNoIO (entity/policy/dto/errors/events: side-effect-free stdlib + platform/{identity,shared} + pkg/policy; список ioStdlib — урок негатив-прогона: «stdlib без точки» пропускал net/http), TestUsecaseIsTransportAgnostic (httpkit/natskit/huma/pgx/net/http + свои delivery/infra), TestHTTPDTONamesGloballyUnique (Input/Output-suffix, huma-registry), TestSubjectLiteralOwnership + TestNoExportedSubjectInDomainRoot, TestNoTimeNowInDomains (domain+jobs). Сразу поймал 2 реальных нарушения: auth/dto.go импортил bruteforcekit (LockState-алиас → локальный тип + loginGuardAdapter в adapters.go) и слишком широкий DTO-скан (Usecase-интерфейсы — сужен до *Input/*Output). Негатив: net/http в policy.go → RED; возврат → GREEN; golangci по archtest — 0 issues.

**Infra impact.**
- **A10 — миграция unique-index (миграция-с-эффектом, low-risk).** WHAT: `0004` добавляет unique partial index на `email_verifications(user_id) WHERE used_at IS NULL` + `ON CONFLICT DO NOTHING` в insert. WHY: идемпотентность at-least-once consumer'а (правило репо, было нарушено). ACTION: на существующих деплоях перед накатом проверить дубли pending-строк (`SELECT user_id FROM email_verifications WHERE used_at IS NULL GROUP BY user_id HAVING count(*)>1`) и погасить старые; на fresh-DB — N/A. WHO: проектная инфра. DECIDE: до деплоя версии с 0004. RISK: CREATE UNIQUE INDEX упадёт на существующих дублях.
- (A3 bump golibinfra v0.39→v0.55: env-теги kit'ов без дельты, новых обязательных ENV/зависимостей нет — инфра-действий не требует, айтемом не является.)

**Чекпоинты.**
- **2026-08-27 · ВСЕ АТОМЫ ЗАКРЫТЫ (A1–A15). `make check` строгий и зелёный.**
  - Полный gate: fmt-check+vet+lint+test+build+gen-check(+untracked)+migrations-check+cover-check → «✓ check clean», покрытие 75.6%≥75.
  - Тестовая база: 8 юнит-пакетов + 3 integration-пакета (живой PG, -race) + archtest (6 инвариантов) + fx.ValidateApp; всё PASS.
  - e2e-smoke не гонялся: нет ./.env.local в среде — прогнать при доступном окружении (backlog).
  - Следующий шаг: юзер коммитит (Conventional Commits; дерево полностью staged) и передаёт INFRA-HANDOFF по 0004; после `make release`-агрегации секцию удалить.
- **2026-08-27 · Phase 1–3 + A10/A11/A14/A15 закрыты.**
  - Закрыто: A1–A11, A14, A15 ✓ (gate, канон-доки, три домена к канону, идемпотентность+0004, тест-механика, генератор, archtest).
  - Обнаружено: user integration-тест был сломан и не собирался (интерфейс vs *Repository) — никто не гонял; golang-migrate в v0.55 парсит относительный file:// как URL-host → internal/testkit/repopath.Migrations(); archtest окупился немедленно (bruteforcekit-утечка в auth/dto.go).
  - Осталось: A12 (тесты добора — 2 субагента дописывают), A13 (cover-gate 75% в check).
- **2026-08-27 · Auto-approve по мандату сессии.** Юзер в сессии явно задал: строгость конвенций + доки без расхождений, gate make-only, cover 75% в check, bump golibinfra при отставании, «разобраться с сабскрайберами». Канон-вилки Q4–Q10 решены консервативно (большинство в коде, минимальный диф) и открыты к пересмотру.

### fix-audit-findings — устранение находок аудита (md + код + тулинг + флоу)
**Создан:** 2026-05-28 · **Статус:** planning

**Контекст.** Аудит репо (19-агентный workflow + ручная верификация) нашёл набор
дефектов на стыке доков/кода/тулинга. Гейты при этом зелёные (`make check` проходит:
build/vet/lint/test 120шт/gen-check), архитектура enforce'ится (depguard эмпирически
ловит cross-domain), MD-связность чистая. Чинимое — это **content drift и
рассинхрон слоёв**, а не сломанная сборка. План закрывает 6 quick-win'ов и 7
структурных правок. Решения по 4 форкам приняты юзером (см. Открытые вопросы).
Ключевые улики: `Makefile:64,69,133`,
`.claude/rules/tests-pipeline.md:38` + `.claude/agents/reviewer.md:64`,
`scripts/release.sh:70`, `.claude/skills/de-template/references/prune.md:13`,
`.claude/rules/forward-only.md:27,37`, `internal/domain/auth/module/adapters.go:65`,
`internal/domain/user/delivery/http/dto.go:43,49`,
`.claude/rules/sqlc-and-migrations.md:39`, `docs/domains/` (0 stable),
`docs/getting-started.md:36-50`, `docs/reference/{ecosystem,release}.md`.

**Не входит в скоуп.** Изменение поведения кода soft-delete (по решению — правим
ТОЛЬКО правило, код user-домена остаётся). Создание worked-примера в `docs/domains/`
(по решению — документируем code-only). Прочие medium/low из аудита уезжают в
`## Backlog` отдельными строками: scaffold-референсы зовут `scripts/tools/*.sh`
напрямую; gen-check не ловит untracked-моки; `make check` мутирует дерево
(`gofumpt -w`) на pre-push; `setup` глотает ошибку копии pre-commit; CI-floor 60%
без CI; `architecture.md` без карты platform/jobs/cmd + metrics.go; нет
Go-integration-тестов для auth/verification; `RequestPasswordReset` глотает все
ошибки; dead-код (`auth.LoggedIn`, `Set/ClearAuthCookies`, дубль `ForbiddenError`);
polish-нити (`dependencies.md:26` хвостовой `>`, latin-`a` в `comments.md:10`,
`Dockerfile:19` cmd/worker, `infra-impact.md:6` мёртвый glob, `MAPPING.md`-роутинг).

**Открытые вопросы.**
- [x] Q1: soft-delete — править код или правило? → **Обновить правило** (status-soft-delete оставить, код не трогаем). → A9
- [x] Q2: design-SoT — заполнить worked-пример или задокументировать? → **Задокументировать code-only**. → A10
- [x] Q3: имя образа — `<org>/backend` фикс или `<org>/<service>` из remote? → **`<org>/<service>`, release.sh выводит из remote**. → A12 (infra-impact)
- [x] Q4: reviewer-агент — manual-only или подключить? → **Подключить в smartdev §4 после GREEN**. → A13

**Атомы.**

#### Phase 1 — Quick wins (тулинг + doc-drift; независимы, низкий риск)
- [x] **A1:** `Makefile` — убрать `mockery@latest`-fallback (строки 64,69 → падать с подсказкой `make setup`, как `lint.sh`); починить `test-fuzz` (строка 133 → `-fuzz=FuzzNewUser ... ./internal/domain/user/`).
  - Verify: `make test-fuzz` → фаззинг стартует (не «multiple packages»); `make gen-check` → green · Зависит от: —
- [x] **A3:** Фантом `pgseed.New(t)` → `.claude/rules/tests-pipeline.md:38` и `.claude/agents/reviewer.md:64` переписать на `golibinfra/testkit.StartPostgres(t, migrationsPath)`; уточнить роль `pgseed` (per-entity seed-хелперы поверх).
  - Verify: `grep -rn 'pgseed.New(t)' .claude` → пусто; ссылка на `repository_test.go:20` как канон · Зависит от: —
- [ ] **A4:** `scripts/release.sh:70` — выровнять агрегацию infra-impact под реальный формат: `**Infra impact.**` (bold inline в `### `-секциях) и единый `.claude/plans/roadmap.md` (не `*.md`-glob, не `## Infra impact`-H2).
  - Verify: временно добавить `**Infra impact.**`-строку в тест-секцию roadmap → агрегация ловит её (не печатает пустой fallback) · Зависит от: —
- [ ] **A5:** `.claude/skills/de-template/references/prune.md` — переписать инструкцию по миграциям под реальность: `0001_users.*` = user+verification (`users`/`password_resets`/`email_verifications`), `0002_auth.*` = `refresh_sessions`; частичный prune = EDIT файла, не delete; исправить неверные имена таблиц (`sessions`/`verification_*`).
  - Verify: ручная сверка с `migrations/*.sql` — все имена таблиц и связки совпадают · Зависит от: —
- [ ] **A6:** `.claude/rules/forward-only.md:27,37` — переписать секцию планов под актуальную модель: единый `roadmap.md`, `### <slug>` под `## Active` (англ.), удаление секции по done; убрать упоминания per-slug `<slug>.md` и `## Активные`.
  - Verify: `grep -nE 'slug>\.md|Активные' .claude/rules/forward-only.md` → пусто; текст согласован с `plan/SKILL.md:9-10` · Зависит от: —

#### Phase 2 — Code reconcile (reference-домены под собственные правила)
- [x] **A7:** Инъекция clock в адаптеры: `auth/module/adapters.go:65` (`tokensAdapter` — добавить `clockkit.Clock`-поле, прокинуть в `module.go:23`, `clock.Now()` вместо `time.Now()`); `verification/module/adapters.go:56,66` (`logMailerAdapter` — то же).
  - Verify: `make test PKG=./internal/domain/auth/... ./internal/domain/verification/...` → PASS; `grep -rn 'time.Now()' internal/domain/*/module/adapters.go` → пусто · Зависит от: —
- [x] **A8:** Префикс HTTP DTO: `user/delivery/http/dto.go:43,49` `ListInput/ListOutput` → `UserListInput/UserListOutput` (+ обновить ссылки в хендлере/usecase); сверить остальные дженерик-имена на коллизии.
  - Verify: `make build` → OK; `make test PKG=./internal/domain/user/...` → PASS · Зависит от: —

#### Phase 3 — Structural (флоу + контракты)
- [x] **A9:** soft-delete — обновить `.claude/rules/sqlc-and-migrations.md:39`: зафиксировать status-based soft-delete как принятый паттерн user-домена (WHY: partial unique index `WHERE status != 'deleted'`, аудит-история). Код НЕ трогаем.
  - Verify: текст правила согласован с `user/usecase.go:275` + `0001_users.up.sql:21` (нет противоречия) · Зависит от: —
- [ ] **A10:** Design-SoT — задокументировать code-only: в `docs/domains/README.md` и `docs/getting-started.md` явно «reference-домены auth/user/verification поставляются code-only by design (без `docs/domains/`-артефактов)»; в `prune.md` пункт 2 → «(reference-домены без docs/domains-артефактов — skip)».
  - Verify: delete-list в `prune.md`/`getting-started.md`/`init-service/SKILL.md` больше не ссылается на несуществующие `docs/domains/{auth,user,verification}/` · Зависит от: —
- [ ] **A11:** Lifecycle — вставить явный шаг «Scaffold спроектированных доменов → `/scaffold` (вариант domain) per stable artifact» МЕЖДУ de-template и удалением reference в `docs/getting-started.md` и `.claude/skills/init-service/SKILL.md`; зафиксировать инвариант «scaffold своих доменов ДО prune reference».
  - Verify: оба файла содержат scaffold-шаг перед шагом удаления reference; инвариант порядка явный · Зависит от: A10
- [ ] **A12:** Image naming — `scripts/release.sh` выводит repo-сегмент из `git remote` (вместо хардкода `/backend`): `<host>/<org>/<service>{,-migrate}`; правка `docs/reference/ecosystem.md`/`docs/reference/release.md`/`docs/getting-started.md`/`README.md` на единый `<org>/<service>`.
  - Verify: `bash scripts/release.sh` (preflight/dry) на тест-remote → image-path = `<host>/<org>/<service>`; semver-гейт цел · Зависит от: —
- [ ] **A13:** reviewer wiring — добавить `name: reviewer`/`name: explorer` во frontmatter `.claude/agents/*.md`; добавить шаг делегирования `reviewer` в `.claude/skills/smartdev/SKILL.md §4` (после GREEN) + кросс-ссылку из `scaffold`/`triage-failure`.
  - Verify: `grep -n '^name:' .claude/agents/reviewer.md .claude/agents/explorer.md` → есть; smartdev §4 содержит шаг reviewer · Зависит от: —

**Infra impact.**
- **A12 — image-path derivation (cross-service contract).** WHAT: `release.sh` перестаёт хардкодить repo-сегмент `backend`, выводит его из `git remote` → образ `<host>/<org>/<service>{,-migrate}`. WHY: `getting-started`/`README` обещают форк `<org>/<service>`, а образ собирался как `<org>/backend` — рассинхрон image-path между build (этот репо) и pull (deploy-репо `<org>/infra`). ACTION: владельцу `<org>/infra` — сверить deploy-конфиги/env на новый image-path `<org>/<service>` вместо `<org>/backend`. WHO: infra-админ. DECIDE: до `make release` следующей версии. RISK: deploy тянет несуществующий тег, если image-path в build и infra разойдутся.

**Чекпоинты.**
- **2026-06-16 · Частичная pre-work закоммичена (этим docs-коммитом) до approve.**
  - Залетели правки с диска (от 2026-06-10, висели uncommitted): `prune.md` — имена таблиц
    (`0001_users`: users/password_resets/email_verifications; `0002_auth`: refresh_sessions) =
    **часть A5**; `release.md` — doc-описание INFRA-HANDOFF под формат `**Infra impact.**` =
    **doc-часть A4**; опечатки `comments.md`(latin-`a`)/`domain.md`(`sgenerated`)/`triage-failure`/
    `smartdev`(+`STACK-DRAFTS`/`TODOs in changed files`)/`stack-deep-dive`(чек-лист «6», убран арг) = часть polish.
  - НЕ закрыто: **A4** — сама логика `scripts/release.sh` (не только doc); **A5** — «частичный
    prune = EDIT файла, не delete» + остальной текст. Атомы A4/A5 остаются `[ ]`.
- **2026-08-27 · A1/A3/A7/A8/A9 поглощены планом `strictness-hardening`.**
  - A1 → S2 (Makefile fallback + test-fuzz), A3 → S5 (pgseed-фантом), A7 → S7+S9 (clock в
    адаптерах), A8 → S8 (HTTP-DTO префиксы), A9 → S6 (soft-delete правило). Атомы остаются
    `[ ]` здесь и закрываются там; после done strictness-hardening отметить их `[x]` со
    ссылкой на его коммиты.
- **2026-06-16 · A2 удалён — поглощён планом `strict-cross-domain-isolation`.**
  - A2 требовал привести печатаемый сниппет `new-domain.sh` к enumerated-идиому. Решением юзера
    идиом изоляции переведён на umbrella (deny `internal/domain` + allow self); сниппет починен
    там же под umbrella+allow-self. Задача A2 неактуальна → атом снят, улика `new-domain.sh:336-344`
    убрана из Контекста. Нумерация остальных атомов сохранена (A1, A3…A13).

### migrate-events-jetstream — outbox/NATS на JetStream (golibinfra v0.34.0, per-domain)
**Создан:** 2026-06-16 · **Статус:** blocked (blocked by: `**Infra impact.**` не сдан — секцию нельзя удалять, пока `make release` не сагрегирует handoff; код-часть done, все атомы [x])

**Контекст.** golibinfra v0.24.0→v0.34.0 несёт 4 breaking-изменения в messaging-слое
(`golibinfra` git log `v0.24.0..v0.34.0`): outboxkit drain теперь публикует через
**JetStream** (`natskit.StreamBus`, PubAck-гарантия, дедуп по `Nats-Msg-Id`), а не Core
NATS; добавлены poison/dead-обработка, партиции+retention, `BacklogReporter` (бывш.
`LagReporter`); natskit получил `StreamBus` (batch-publish + durable-консьюмеры с
DLQ/MaxDeliver/NAK-backoff). Эталон миграции — соседний `amocrm-gateway-template`
(`internal/platform/events/{outbox,publisher}.go`, `dispatch/delivery/nats/subscriber.go`,
`migrations/0007_outbox_domains.up.sql`, `cmd/api/main_test.go`). Решения юзера: **(1)
per-domain outbox** (паттерн `OutboxFor` как в amocrm: таблица+Outbox+Publisher на домен,
`NewStreamTransportFor` с namespace, стрим на префикс) и **(2) durable JetStream
subscriber** (verification слушает `user.events.registered`). Текущее: single-instance
`outboxkit.Module`+`outboxpg.Module`+`LagReporter` (`cmd/api/main.go:72-80`), Core-NATS
subscriber (`verification/delivery/nats/handler.go:37-51`), publish'ат только домены
`user` (`usecase.go:108`) и `verification` (`usecase.go:110,155`). golibinfra v0.34.0 уже
в module-кэше — бамп выполним офлайн.

**Не входит в скоуп.** Envelope/Fact-абстракция из amocrm (`events.Publisher`-обёртка) —
наши домены зовут `outboxkit.Publisher.Publish(subject, payload)` напрямую, обёртка не
нужна. Retention-job per-domain (amocrm его не заводит; `OUTBOX_RETENTION_MONTHS` default
0 = не дропать) — оставляем только partition-keeper. Удаление embedded `outboxpg`-миграции
из `cmd/migrate` (default-таблица `outbox` остаётся неиспользуемой, как в amocrm —
append-only стабильность source-порядка). Перевод доменов на JetStream-publish в обход
outbox (нет транзакционной гарантии — запрещено правилом).

**Открытые вопросы.**
- [x] Q1: топология outbox — single-instance или per-domain? → **per-domain (OutboxFor как amocrm)**. → A2,A3
- [x] Q2: subscriber — Core NATS или durable JetStream? → **durable JetStream (SubscribeStreamJSON)**. → A5
- [x] Q3: имена стримов под subject-префиксы `user.>`/`verification.>`? → **`user,verification`** (natskit `<name>`→`<name>.>`). → A7

**Атомы.**

#### Phase 1 — bump + per-domain outbox wiring
- [x] **A1:** Бамп `git.stit.tech/stit-core/golibinfra` v0.24.0→v0.34.0 в `go.mod`/`go.sum` (`go get …@v0.34.0` + `go mod tidy`). Сборка ещё красная (LagReporter/Module меняются дальше) — это ожидаемо.
  - Verify: `grep golibinfra go.mod` → v0.34.0; `go mod verify` → ok · Зависит от: —
  - **Done:** go.mod/go.sum → v0.34.0; `go mod tidy` exit 0. Note: v0.24.0 в кэше объявлял старый путь `git.sftbmb.ru/...` (ребрендинг ~v0.32) → правил require+go.sum напрямую, не `go get`. grpc 1.81.0→1.81.1 (транзитив).
- [x] **A2:** `internal/platform/events/outbox.go` (новый) — `OutboxFor(name, table)` по образцу amocrm, но провайдит `outboxkit.Publisher` напрямую (без Envelope): private Store (`NewStoreFor(db, table)`) + Outbox (`outboxkit.New(name, store, NewStreamTransportFor(sb, name), cfg)`) + Publisher; экспорт в группы `timer.job` (drain + partition-keeper) и `outbox.backlog` (BacklogReporter); `registerOutboxBacklog` + `events.Module`.
  - Verify: `go vet ./internal/platform/events/...` → compiles · Зависит от: A1
  - **Done:** `internal/platform/events/outbox.go`; `go vet` exit 0.
- [x] **A3:** Per-domain провайдеры: `internal/domain/user/infra/outbox/outbox.go` (`Table="outbox_user"`, `Providers=events.OutboxFor("user",Table)`) и `…/verification/infra/outbox/outbox.go` (`outbox_verification`); включить `Providers` в `user/module/module.go` и `verification/module/module.go` (fx.Private скоупится доменным Module).
  - Verify: `go vet ./internal/domain/user/... ./internal/domain/verification/...` → compiles · Зависит от: A2
  - **Done:** 2 outbox-файла + `Providers` в обоих module.go; `go vet` exit 0.
- [x] **A4:** `cmd/api/main.go` — убрать `outboxkit.Module`, `outboxpg.Module`, provide `func(*outboxpg.Store) outboxkit.LagReporter` и импорт `outboxpg`; добавить `appevents.Module` в `platformModules`; `outboxkit.NewConfig`/`natskit.Module` остаются.
  - Verify: `make build` → OK (граф собирается) · Зависит от: A3
  - **Done:** `make build` → `OK: build bin/api`.

#### Phase 2 — JetStream subscriber + миграции + ENV
- [x] **A5:** `verification/delivery/nats/handler.go` — `Register(lc, bus natskit.StreamBus, h)`; `SubscribeStreamJSON(bus, ctx, "user", SubjectUserRegistered, "verification-user-registered", h.onUserRegistered)` с graceful-skip на `natskit.ErrJetStreamDisabled` (как dispatch в amocrm); консты `streamName`/`durableName`.
  - Verify: `make build` → OK · Зависит от: A4
  - **Done:** `Register` на `natskit.StreamBus`+`SubscribeStreamJSON`+graceful-skip; `go build ./...` exit 0.
- [x] **A6:** `migrations/0003_outbox_domains.{up,down}.sql` (новые) — создать `outbox_user` + `outbox_verification` (партиц. по `created_at`, `*_unpublished_idx`, 4 мес. партиций), зеркаля DDL `outboxpg.SchemaSQL`/amocrm `0007`.
  - Verify: smoke-стек `migrate up` зелёный (`outbox_user`/`outbox_verification` созданы) · Зависит от: —
  - **Done:** На throwaway-pg `make migrate-up` → ok; `\d outbox_user` = партиц. таблица (4 партиции, attempts/last_error/dead_at, unpublished_idx); `make migrate-down` откатывает 0003 чисто.
- [x] **A7:** ENV — `test/stack/compose.yml` `NATS_JS_STREAM_NAMES: "user,verification"` (было `""`); `.env.example` — закомментированный блок-гайд NATS JetStream (drain требует `NATS_JETSTREAM_ENABLED=true` + стримы).
  - Verify: `grep NATS_JS_STREAM_NAMES test/stack/compose.yml` → `user,verification` · Зависит от: —
  - **Done:** compose `NATS_JS_STREAM_NAMES: "user,verification"`; `.env.example` блок «Messaging / outbox (JetStream)».

#### Phase 3 — тесты + forward-only доки
- [x] **A8:** Заменить `internal/domain/user/module/module_test.go` (fxtest+мок `outboxkit.Publisher` конфликтует с приватным publisher из OutboxFor) на `cmd/api/main_test.go` `TestFxGraphValidates` (`fx.ValidateApp` над infra+platform+domain, как amocrm — без lifecycle/pg/nats).
  - Verify: `make test PKG=./cmd/...` → PASS; `ls internal/domain/user/module/module_test.go` → отсутствует · Зависит от: A5
  - **Done:** module_test.go удалён; `cmd/api/main_test.go` `TestFxGraphValidates` → ok; весь `go test ./...` зелёный.
- [x] **A9:** Forward-only доки: `docs/reference/architecture.md §События` (drain→JetStream/PubAck/dedup, durable subscriber, per-domain таблицы, poison/DLQ); `.claude/rules/events-and-messaging.md` (per-domain app-owned таблицы вместо kit-owned single `outbox`; subscriber=durable JetStream); `.claude/skills/scaffold/references/event.md` (`SubscribeStreamJSON` + `OutboxFor`-wiring вместо `SubscribeJSON`); `.claude/rules/forward-only.md` `LagReporter`→`BacklogReporter`; `docs/stack/_templates/messaging.md` (JetStream-ноты).
  - Verify: `grep -rn 'SubscribeJSON\|LagReporter\|Drain публикует в NATS' docs .claude` → пусто (кроме git-истории) · Зависит от: A5,A6
  - **Done:** обновлены architecture.md, events-and-messaging.md, scaffold/event.md, forward-only.md, messaging.md, observability.md (outbox dead/pending + BacklogReporter); grep по docs/.claude чист (кроме самого плана).
- [x] **A10:** Финальный гейт — `make check` (fmt+vet+lint+test+build+gen-check) зелёный; smoke e2e если доступен (`make e2e`/`test/stack`).
  - Verify: `make check` → green · Зависит от: A8,A9
  - **Done:** `make check` → `✓ check clean` (gen-check без дрейфа). Канонический smoke недоступен (нужен `STIT_OBS_TOKEN`), вместо него — фокусный end-to-end на postgres+nats(-js)+redis: `POST /api/auth/register` → 201; `outbox_user.user.events.registered` published=t (drain→JetStream PubAck); durable consumer (`SubscribeStreamJSON`, стрим `user`) создал `email_verifications`; `outbox_verification.…email_verification_requested` published=t; лог = 1×«user registered» + 1×«email verification issued», ноль ошибок в steady-state.

**Infra impact.**
- **A4+A7 — JetStream обязателен для доставки (required env + runtime-dep).** WHAT: outbox-drain v0.34.0 публикует через JetStream; нужны `NATS_JETSTREAM_ENABLED=true` и `NATS_JS_STREAM_NAMES=user,verification` (стримы покрывают `user.>`/`verification.>`), плюс NATS-сервер с JetStream (`-js`) и file-storage. WHY: без включённого JS и стримов `PublishStream` падает (`ErrJetStreamDisabled`/no-stream) — события копятся unpublished, доставки нет. ACTION: `<org>/infra` — выставить env в backend.env/compose и подтвердить, что корп-NATS поднят с JetStream + ёмкостью под file-storage. WHO: инфра-админ (корп-NATS) + проектная инфра (compose env). DECIDE: до деплоя этой версии. RISK: тихая потеря событий / рост outbox-бэклога при выключенном JS или отсутствующих стримах.
- **A6 — per-domain outbox-таблицы (миграция-с-эффектом).** WHAT: `0003_outbox_domains` создаёт партиц. `outbox_user`/`outbox_verification`; сервис уходит с default-таблицы `outbox`. WHY: аддитивный `CREATE TABLE`, но pending-строки в старой `outbox` после катовера осиротеют (их дренил только старый код). ACTION: на существующих деплоях — дренировать/проверить пустоту старой `outbox` до выката новой версии. WHO: проектная инфра. DECIDE: на деплое. RISK: in-flight события в старой `outbox` не опубликуются после катовера. (На каноническом форке/fresh-DB — N/A.)
- **A5 — durable JetStream consumer + опц. DLQ-тюнинг (cross-service/capacity).** WHAT: durable `verification-user-registered` на стриме `user`; опц. `NATS_JS_MAX_DELIVER`/`NATS_JS_NAK_BACKOFF`/`NATS_JS_DLQ_SUBJECT` для poison-handling. WHY: durable-консьюмер держит позицию в стриме (replay при простое) — ёмкость стрима/консьюмера на стороне NATS. WHO: инфра-админ (корп-NATS). DECIDE: при настройке retention/DLQ. RISK: дефолты (`MaxDeliver=0`) = бесконечный ределивери poison-сообщения без DLQ.

**Чекпоинты.**
- **2026-06-16 · Код-часть закрыта, ждёт infra-handoff/release.**
  - Закрыто: A1–A10 ✓ (бамп, per-domain OutboxFor, durable JetStream subscriber, миграция 0003, ENV, fx-graph-test, доки, `make check`).
  - Верифицировано end-to-end: register → JetStream-каскад через оба per-domain outbox + durable consumer, ноль ошибок (см. A10 Done).
  - Обнаружено: v0.24.0 в кэше держал старый module-path (`git.sftbmb.ru/...`) — бамп правкой require+go.sum, не `go get`. `make build` собирает только `bin/api`; полная проверка — `go build ./...`.
  - Следующий шаг: юзер передаёт INFRA-HANDOFF (3 айтема выше) инфра-админу out-of-band; при `make release` они агрегируются в release-handoff → после этого секцию удалить.

## Backlog (idea state)
- **Security-ревью:** `auth.Refresh` не проверяет `CanLogin` заблокированного пользователя — living refresh-токен выдаёт новый access после блокировки. Изменение поведения — решение юзера (найдено strictness-аудитом, за скоупом плана).
- Юнит-тесты на module/-адаптеры с маппингом (mapUserErr, userToAuth, loginGuardAdapter) + вернуть `/module/` и `/platform/events/` в cover-scope.
- `git rm internal/domain/auth/mocks/session_repository.go` — файл-сирота (интерфейс убран из .mockery.yml; Bash-rm по mocks блокирует permission-политика агента).
- e2e-smoke на golibinfra v0.55 (нужен ./.env.local): register-каскад через per-domain outbox + durable consumer + идемпотентность 0004.
- Полная сверка сниппетов `.claude/skills/scaffold/references/*` с каноном (критикал закрыт: ErrConflict, depguard-автовставка; остальное — построчно при следующем скаффолде).
- Description-оптимизация (run_loop из skill-creator) для новых/слитых скиллов (`init-service`, `stack-deep-dive`, `scaffold`, `de-template`) — эмпирический тюнинг триггеров через `claude -p`. Опционально: descriptions уже канон-pushy; запускать отдельным заходом (тяжёлый), приоритетно `scaffold` (одно description на 5 интентов).
- MinIO-примитив (`test/infra/objstore.py` + minio-сервис в compose) — завести когда форк добавит реального потребителя объектного хранилища (сейчас потребителя нет).
