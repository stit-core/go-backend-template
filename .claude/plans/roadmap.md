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

**Current focus:** `migrate-events-jetstream` (код done A1–A10; blocked — ждёт infra-handoff/`make release`) · отложен `fix-audit-findings` (planning)

## Active

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
- [ ] **A1:** `Makefile` — убрать `mockery@latest`-fallback (строки 64,69 → падать с подсказкой `make setup`, как `lint.sh`); починить `test-fuzz` (строка 133 → `-fuzz=FuzzNewUser ... ./internal/domain/user/`).
  - Verify: `make test-fuzz` → фаззинг стартует (не «multiple packages»); `make gen-check` → green · Зависит от: —
- [ ] **A3:** Фантом `pgseed.New(t)` → `.claude/rules/tests-pipeline.md:38` и `.claude/agents/reviewer.md:64` переписать на `golibinfra/testkit.StartPostgres(t, migrationsPath)`; уточнить роль `pgseed` (per-entity seed-хелперы поверх).
  - Verify: `grep -rn 'pgseed.New(t)' .claude` → пусто; ссылка на `repository_test.go:20` как канон · Зависит от: —
- [ ] **A4:** `scripts/release.sh:70` — выровнять агрегацию infra-impact под реальный формат: `**Infra impact.**` (bold inline в `### `-секциях) и единый `.claude/plans/roadmap.md` (не `*.md`-glob, не `## Infra impact`-H2).
  - Verify: временно добавить `**Infra impact.**`-строку в тест-секцию roadmap → агрегация ловит её (не печатает пустой fallback) · Зависит от: —
- [ ] **A5:** `.claude/skills/de-template/references/prune.md` — переписать инструкцию по миграциям под реальность: `0001_users.*` = user+verification (`users`/`password_resets`/`email_verifications`), `0002_auth.*` = `refresh_sessions`; частичный prune = EDIT файла, не delete; исправить неверные имена таблиц (`sessions`/`verification_*`).
  - Verify: ручная сверка с `migrations/*.sql` — все имена таблиц и связки совпадают · Зависит от: —
- [ ] **A6:** `.claude/rules/forward-only.md:27,37` — переписать секцию планов под актуальную модель: единый `roadmap.md`, `### <slug>` под `## Active` (англ.), удаление секции по done; убрать упоминания per-slug `<slug>.md` и `## Активные`.
  - Verify: `grep -nE 'slug>\.md|Активные' .claude/rules/forward-only.md` → пусто; текст согласован с `plan/SKILL.md:9-10` · Зависит от: —

#### Phase 2 — Code reconcile (reference-домены под собственные правила)
- [ ] **A7:** Инъекция clock в адаптеры: `auth/module/adapters.go:65` (`tokensAdapter` — добавить `clockkit.Clock`-поле, прокинуть в `module.go:23`, `clock.Now()` вместо `time.Now()`); `verification/module/adapters.go:56,66` (`logMailerAdapter` — то же).
  - Verify: `make test PKG=./internal/domain/auth/... ./internal/domain/verification/...` → PASS; `grep -rn 'time.Now()' internal/domain/*/module/adapters.go` → пусто · Зависит от: —
- [ ] **A8:** Префикс HTTP DTO: `user/delivery/http/dto.go:43,49` `ListInput/ListOutput` → `UserListInput/UserListOutput` (+ обновить ссылки в хендлере/usecase); сверить остальные дженерик-имена на коллизии.
  - Verify: `make build` → OK; `make test PKG=./internal/domain/user/...` → PASS · Зависит от: —

#### Phase 3 — Structural (флоу + контракты)
- [ ] **A9:** soft-delete — обновить `.claude/rules/sqlc-and-migrations.md:39`: зафиксировать status-based soft-delete как принятый паттерн user-домена (WHY: partial unique index `WHERE status != 'deleted'`, аудит-история). Код НЕ трогаем.
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
- **A12 — image-path derivation (cross-service contract).** WHAT: `release.sh` перестаёт хардкодить repo-сегмент `backend`, выводит его из `git remote` → образ `<host>/<org>/<service>{,-migrate}`. WHY: `getting-started`/`README` обещают форк `<org>/<service>`, а образ собирался как `<org>/backend` — рассинхрон image-path между build (этот репо) и pull (deploy-репо `<org>/infra`). ACTION: владельцу `<org>/infra` — сверить deploy-конфиги/Vault-пути на новый image-path `<org>/<service>` вместо `<org>/backend`. WHO: infra-админ. DECIDE: до `make release` следующей версии. RISK: deploy тянет несуществующий тег, если image-path в build и infra разойдутся.

**Чекпоинты.**
- **2026-06-16 · Частичная pre-work закоммичена (этим docs-коммитом) до approve.**
  - Залетели правки с диска (от 2026-06-10, висели uncommitted): `prune.md` — имена таблиц
    (`0001_users`: users/password_resets/email_verifications; `0002_auth`: refresh_sessions) =
    **часть A5**; `release.md` — doc-описание INFRA-HANDOFF под формат `**Infra impact.**` =
    **doc-часть A4**; опечатки `comments.md`(latin-`a`)/`domain.md`(`sgenerated`)/`triage-failure`/
    `smartdev`(+`STACK-DRAFTS`/`TODOs in changed files`)/`stack-deep-dive`(чек-лист «6», убран арг) = часть polish.
  - НЕ закрыто: **A4** — сама логика `scripts/release.sh` (не только doc); **A5** — «частичный
    prune = EDIT файла, не delete» + остальной текст. Атомы A4/A5 остаются `[ ]`.
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
- Description-оптимизация (run_loop из skill-creator) для новых/слитых скиллов (`init-service`, `stack-deep-dive`, `scaffold`, `de-template`) — эмпирический тюнинг триггеров через `claude -p`. Опционально: descriptions уже канон-pushy; запускать отдельным заходом (тяжёлый), приоритетно `scaffold` (одно description на 5 интентов).
- MinIO-примитив (`test/infra/objstore.py` + minio-сервис в compose) — завести когда форк добавит реального потребителя объектного хранилища (сейчас потребителя нет).
