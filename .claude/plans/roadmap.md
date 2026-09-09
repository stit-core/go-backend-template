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

**Current focus:** migrate-golibinfra-v1

## Active

### migrate-golibinfra-v1 — миграция шаблона на golibinfra v1.1.0
**Создан:** 2026-09-09 · **Статус:** blocked (blocked by: передача INFRA-HANDOFF инфра-админу — секция закрывается по `make release`, который агрегирует **Infra impact.**)

**Контекст.** golibinfra ушёл с `v0.55.0` на `v1.1.0` (`git tag -l --format='%(contents)' v1.0.0 v1.1.0`
в `../golibinfra`) — breaking по всем китам шаблона: logkit (`New(cfg, opts…)`, `LOG_SILENT`),
healthkit/timerkit (именованные пробы, группы `healthkit.probes`/`timerkit.jobs` через `Provide`),
pgkit (`Source{Name,FS,Dir}`, `MigrateUpAll`, таблица версий на источник), outboxkit
(`Instance` + `pgstore.FactoryModule`, `TableMigrations`), authnkit (ctx у Hasher/JWT,
`SignAccess/SignRefresh`, `ParseAccess/ParseRefresh`, `VerifyDummy`), bruteforcekit (cleanup-job
внутри `pgstore.Module`, `RecordFailure → (*LockState, error)`, ключ требует настоящий IP),
natskit (`Subscribe* → (Subscription, error)`, `Module` не провайдит Config), testkit
(`txkit → pgtest`, `eventbus → outboxtest`, `StartPostgres(t, opts…)`). go.mod уже на `v1.1.0`
(`go 1.26.6`), `go vet ./...` даёт 32 ошибки в 9 пакетах — карта работ ниже.

**Не входит в скоуп.** Перевод `cmd/api` на `pgkit.MigrateModule` (lockstep-контракт
api+migrate сохраняется), TOTP/RBAC-фичи authnkit, `telemetry`-кит (корп-стека нет),
NeedsRehash-перехэш на логине (→ backlog), security-находка по `auth.Refresh`/`CanLogin` (backlog).

**Открытые вопросы.**
- [x] Q1: `authnkit.Module` тянет TOTP → обязательный `TOTP_ISSUER`. → Проводим Hasher+JWT явно
  (`internal/platform/authgate`), без TOTP: новый required-env ради неиспользуемой фичи — нет.
- [x] Q2: Рукописная `0003_outbox_domains` vs `pgstore.TableMigrations`. → Схема per-domain
  outbox-таблиц переезжает к киту (`infra/outbox.Migrations`), 0003 удаляется, 0004 → 0003
  (валидатор требует непрерывной нумерации; прод-БД у шаблона нет — существующие БД форков в
  handoff).
- [x] Q3: Доменные порты `Tokens`/`TokenParser` без ctx. → Добавляем ctx (kit требует его для
  спанов/метрик), моки перегенерировать `make gen`.
- [x] Q4: Client-IP для bruteforcekit (v1 отвергает пустой/`host:port` ключ — guard молча
  выключился бы). → middleware `authgate.ClientIP` в группе `httpkit.middleware` поверх
  `httpkit.ClientIP(r, HTTP_RATE_LIMIT_TRUST_PROXY)`.

**Атомы.**

#### Phase 1 — фундамент (кит-проводка, компилируется прод-код)
- [x] **A1:** go.mod/go.sum → golibinfra v1.1.0, `go 1.26.6`; импорты `testkit/txkit→pgtest`, `testkit/eventbus→outboxtest`.
  - Verify: `go list -m git.stit.tech/stit-core/golibinfra` → v1.1.0 · Зависит от: —
  - **Done:** go get + tidy прошли; вывод `go vet ./...` — карта остальных атомов.
- [x] **A2:** logkit: `cmd/api` → `logkit.Module` + `NewConfig`/`NewRedactConfig`; `cmd/migrate`, `cmd/seed-admin` → `logkit.New(cfg)`; `authgate.RegisterAttr("user_id", …)`.
  - Verify: `go build ./cmd/... ./internal/platform/...` → OK · Зависит от: A1
  - **Done:** cmd/api на `logkit.Module` (+`NewConfig`/`NewRedactConfig`), cmd/migrate и seed-admin — `logkit.New(cfg)`, `authgate.userIDAttr` через `RegisterAttr(key, fn)`; `go build ./...` → OK.
- [x] **A3:** healthkit/timerkit/natskit-config в `cmd/api`: `healthkit.Provide(pgkit/natskit/cachekit.Probe)`, `fx.Provide(natskit.NewConfig, outboxpg.NewConfig)`; удалить `internal/jobs` (cleanup-job даёт `bruteforcepg.Module`), archtest без `internal/jobs`.
  - Verify: `go build ./...` (кроме A4–A7 ошибок) · Зависит от: A2
  - **Done:** `healthkit.Provide(pgkit/natskit/cachekit.Probe)`, `natskit.NewConfig`/`outboxpg.NewConfig` в kitConfigs, `config.Module` вместо ручного `[]Source`; `internal/jobs` удалён, archtest обходит только `internal/domain`.
- [x] **A4:** authnkit: порты `Tokens`/`TokenParser` с ctx; адаптеры на `SignAccess/SignRefresh/ParseRefresh`; authgate `ParseAccess`; usecases `Hash/Verify(ctx…)` + `VerifyDummy` на unknown-user/blocked; явная проводка Hasher+JWT; seed-admin. `make gen` (моки).
  - Verify: `go build ./... && make gen-check` → OK · Зависит от: A3
  - **Done:** порты `Tokens`/`TokenParser` с ctx, адаптеры на `SignAccess/SignRefresh/ParseRefresh` (typ проверяет кит), authgate `ParseAccess`, `VerifyDummy` на unknown-user/blocked, Hasher+JWT провайдятся явно (`authgate/authn.go`); `make gen` → моки; `make gen-check` → OK.
- [x] **A5:** bruteforcekit: адаптер `RecordFailure → (*LockState, error)`; `authgate.ClientIP` middleware в `httpkit.MiddlewareGroup` + unit-тест.
  - Verify: `make test PKG=./internal/platform/...` → PASS · Зависит от: A4
  - **Done:** `loginGuardAdapter.RecordFailure` отбрасывает `*LockState`; `authgate.ClientIP` (группа `httpkit.middleware`, `HTTP_RATE_LIMIT_TRUST_PROXY`) + `clientip_test.go`; `make test PKG=./internal/platform/...` → PASS.
- [x] **A6:** pgkit-миграции: `cmd/migrate` на `MigrateUpAll/MigrateDown/MigrateVersion` + `NewMigrateRunnerConfig` (`PG_MIGRATE_TIMEOUT`), источники `bruteforcepg/auditpg.Migrations`, `useroutbox/verifoutbox.Migrations` (`pgstore.TableMigrations`), `DirSource(app)`; удалить `0003_outbox_domains`, `0004 → 0003`.
  - Verify: `go build ./cmd/migrate && bash scripts/tools/migrations-check.sh` → OK · Зависит от: A3
  - **Done:** `cmd/migrate`: `NewMigrateRunnerConfig` (PG_MIGRATIONS_PATH/PG_MIGRATE_TIMEOUT), `MigrateUpAll` по источникам bruteforcekit·auditkit·outbox_user·outbox_verification·app, `down` — одна миграция приложения, `version` — по источникам; `0003_outbox_domains` удалена, `0004→0003`; `migrations-check` → OK (3 пары).
- [x] **A7:** outbox: `events.OutboxFor` поверх `outboxkit.Instance` + приватный алиас Publisher; `events.Module` = `outboxpg.FactoryModule`; убрать ручной `RegisterBacklogGauges`.
  - Verify: `go build ./...` → OK · Зависит от: A3
  - **Done:** `events.OutboxFor` = `outboxkit.Instance` + приватный алиас Publisher (`fx.ParamTags(name)`), `events.Module` = `pgstore.FactoryModule`; `infra/outbox.Migrations = TableMigrations(Table)`.
- [x] **A8:** natskit-subscriber: `(Subscription, error)`, `Drain(ctx)` на OnStop; правка scaffold-референса `event.md`.
  - Verify: `go build ./internal/domain/verification/...` → OK · Зависит от: A3
  - **Done:** `SubscribeStreamJSON` → `(Subscription, error)`, OnStop: `Drain(stopCtx)` до `cancel()`; scaffold `event.md` — тот же шаблон.

#### Phase 2 — граф, тесты, гейт
- [x] **A9:** `cmd/api` граф целиком: `TestFxGraphValidates` зелёный.
  - Verify: `make test PKG=./cmd/...` → PASS · Зависит от: A2,A3,A4,A5,A7,A8
  - **Done:** `TestFxGraphValidates` зелёный (`make test` → PASS 181/38 пакетов).
- [x] **A10:** тесты доменов/платформы: `pgtest.Identity`, `outboxtest.Recorder`, моки с ctx, stub-парсер authgate, интеграционные `StartPostgres(t, WithMigrationsDir(...))`.
  - Verify: `make test && go vet -tags integration ./...` → PASS · Зависит от: A9
  - **Done:** `pgtest.Identity`/`outboxtest.Recorder`, моки с ctx (`mock.Anything` ×2), stub-парсер с `Parse/ParseAccess/ParseRefresh`, `StartPostgres(t, WithMigrationsDir(...))`; `make test` → PASS, `go vet -tags integration ./...` → OK.
- [x] **A11:** ENV/e2e/доки: `.env.example` (`LOG_SILENT`/`LOG_FORMAT`), `test/stack/compose.yml` (без `BRUTEFORCE_WINDOW`), `test/infra/outbox.py` (per-domain таблица), rules `events-and-messaging`/`sqlc-and-migrations`/`tests-pipeline`, `docs/reference/{architecture,domain-mapping}.md`, `docs/stack/_templates/*`, stack-deep-dive refs (`MigrationsFS → Migrations`), `scripts/new-domain.sh`.
  - Verify: `grep -rn 'MigrationsFS\|txkit\|eventbus\|internal/jobs\|LOG_STDOUT\|BRUTEFORCE_WINDOW' --exclude-dir=.git .` → пусто · Зависит от: A10
  - **Done:** .env.example (LOG_FORMAT/LOG_SILENT), compose без BRUTEFORCE_WINDOW, `test/infra/outbox.py` — `table_for(subject)`; правила events-and-messaging/sqlc-and-migrations/tests-pipeline, architecture/domain-mapping, stack-шаблоны, stack-deep-dive refs, scaffold event/migration, reviewer.md; grep старых идентификаторов (вне roadmap) → пусто.
- [x] **A12:** `make check` зелёный; INFRA-HANDOFF предъявлен юзеру; секция закрывается.
  - Verify: `make check` → green · Зависит от: A11
  - **Done:** `make check` → ✓ check clean (fmt, vet, lint, test, build, gen-check, migrations, coverage 75.1%).

**Infra impact.**
- **Таблицы версий миграций.** WHAT: `schema_migrations` (сквозная нумерация v0.55: bruteforcekit·auditkit·outboxkit·app) → `schema_migrations_app` + `schema_migrations_{bruteforcekit,auditkit,outbox_user,outbox_verification}` (по источнику). WHY: pgkit v1 — версия на источник; DDL kit-0001 (`audit_log`, `bruteforce_attempts`) не идемпотентен. ACTION (только существующие БД форков; чистые БД — ничего): один раз до первого `migrate up` на v1 — `ALTER TABLE schema_migrations RENAME TO schema_migrations_app; UPDATE schema_migrations_app SET version = 3;` (число миграций приложения), затем `CREATE TABLE schema_migrations_<kit> (LIKE schema_migrations_app INCLUDING ALL)` + `INSERT … (1, false)` для `bruteforcekit`, `auditkit`, `outbox_user`, `outbox_verification`; `DROP TABLE outbox` (default-таблица кита не используется). Далее `migrate up` доложит kit-0002 (audit `CREATE INDEX CONCURRENTLY` — старт ждёт построения на большой `audit_log`; bruteforce-индекс; `next_attempt_at`). WHO: проектная инфра форка. RISK: без bootstrap'а `migrate up` упадёт на `CREATE TABLE audit_log` (already exists).
- **ENV.** WHAT: `LOG_STDOUT` → `LOG_SILENT` (инверсия; дефолт прежний — stdout), `BRUTEFORCE_WINDOW` удалён (envconfig его игнорирует, но из compose/.env убрать); `JWT_SECRET` ≥ 32 байт — иначе старт падает; Redis ≥ 7.0 (проверка на старте, `REDIS_SKIP_VERSION_CHECK=true` снимает); `HTTP_RATE_LIMIT_TRUST_PROXY=true` за reverse-proxy теперь питает и client-IP bruteforce-guard'а (без него все клиенты за прокси — один IP-ключ). Новые опциональные с дефолтами: `LOG_FORMAT`, `LOG_SOURCE`, `LOG_REDACT_*`, `PG_MIGRATE_TIMEOUT` (migrate-job, 2m), `HTTP_READY_PROBE_TIMEOUT`, `HTTP_DOCS_ENABLED`, `HTTP_QUIET_PATHS`, `NATS_DRAIN_TIMEOUT`, `NATS_JS_HANDLER_TIMEOUT`, `OUTBOX_TICK_TIMEOUT`, `OUTBOX_RETRY_BACKOFF`, `OUTBOX_RETRY_MAX_BACKOFF`, `BRUTEFORCE_CLEANUP_INTERVAL`, `BRUTEFORCE_IPV6_PREFIX`, `JWT_LEEWAY`. WHO: проектная инфра (compose env). DECIDE: `HTTP_DOCS_ENABLED=false` в проде?
- **Метрики/логи (дашборды, алерты).** WHAT: `db_pool_*` → `pg_pool_*`; `jwt_verifications_total` → `authn_jwt_verifications_total`, `password_{hash,verify}_duration_seconds` → `authn_password_*`; access-log `dur` (нс) → `duration_ms`; все `*_seconds` гистограммы в секундах с общими бакетами; `outbox_*` с label `instance` (`user`/`verification`), timer-job `bruteforce.cleanup` → `bruteforcekit.cleanup`. WHO: владелец дашбордов.
- **Runtime.** WHAT: сборка требует Go ≥ 1.26.6 (`golang:1.26-alpine` в Dockerfile тянет актуальный patch; `GOTOOLCHAIN=auto` докачает); новых внешних зависимостей нет. WHO: проектная инфра (CI-образ).

**Чекпоинты.**
- **2026-09-09 · старт.** go.mod поднят, карта ошибок снята (`go vet`), API v1.1.0 изучен по аннотациям тегов и README/`go doc` китов.
- **2026-09-09 · код готов.**
  - Закрыто: A1–A12 ✓ (`make check` clean).
  - Обнаружено: в v0.55 client-IP в ctx никто не клал (`identity.WithIP` звался только из теста) — guard считал попытки по пустому IP; v1 отвергает такой ключ, поэтому появился `authgate.ClientIP`. e2e-helper `test/infra/outbox.py` читал default-таблицу `outbox`, которой сервис не пользуется — теперь per-domain.
  - Следующий шаг: юзер передаёт INFRA-HANDOFF, коммит (Conventional Commits), `make release` агрегирует handoff → секция удаляется.

## Backlog
- **Security-ревью:** `auth.Refresh` не проверяет `CanLogin` заблокированного пользователя —
  living refresh-токен выдаёт новый access после блокировки. Изменение поведения — решение
  юзера (находка strictness-аудита; сохранена при чистке roadmap как security-значимая).
- **Перехэш пароля на логине:** `authnkit.Hasher.NeedsRehash` после успешного `Verify` —
  обновлять `password_hash` при смене `ARGON2_*`; требует write-path в `auth.Login` и
  ожиданий в моках `PasswordSetter`. Решение по поведению — за юзером.
