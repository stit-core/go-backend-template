---
title: Kit store-port refactor (bruteforcekit → core+port+pgstore, multi-source migrate)
slug: kit-store-port-refactor
status: in-progress
created: 2026-05-12
last_checkpoint: 2026-05-12 — approved, фазы 1+2; фаза 3 откладывается
---

# Kit store-port refactor

**Slug:** `kit-store-port-refactor`
**Создан:** 2026-05-12
**Статус:** in-progress — фазы 1+2 закрыты (7/9), фаза 3 (A8-A9) отложена до запроса юзера
**Последний чекпоинт:** 2026-05-12 20:00 — фазы 1+2 done

## Контекст

В текущем шаблоне `bruteforcekit` сам лезет в БД через `pgkit.DB` и держит DDL в
`golibinfra/bruteforcekit/migrations/*.sql` + `embed.FS`. **App не использует
embed.FS** — `template/migrations/0002_auth.up.sql` буквально содержит копию DDL
`bruteforce_attempts` (см. строки 18-28 этого файла). Дублирование, kit не подменяем.

Аналогичные дыры в `auditkit` и `outboxkit` (тот же паттерн pgguard в kit + копия
DDL в app). **Они в этом плане не трогаются** — на v1 наш downstream-проект
(`ROC/admin-panel-backend`) их выключает (нет audit/outbox-эвентов), а template
сохранит их как технический долг для отдельного плана.

`authnkit` чист (Argon2id хешер + JWT signer/parser — без БД). `refresh_sessions`
живёт app-side как и должно. Refactor не нужен.

Решение юзера: подход **Core + Store-port + opt pgstore-adapter** + **только нужные
для iam kit'ы (bruteforce)** в этой итерации. Goal — разблокировать
`ROC/admin-panel-backend/.claude/plans/cleanup-and-iam.md`.

Consumers golibinfra: только этот шаблон + admin-panel-backend. **Можно ломать API
без backward compatibility** — нет внешних команд на пути.

## Цели

1. `bruteforcekit` — разделение на `Module` (только interface `Guard`) + подпакет
   `pgstore` (Postgres implementation + `embed.FS migrations`). App-уровень решает
   что подключать.
2. `pgkit.MigrateUpMulti` — multi-source API, принимает ordered list `embed.FS`
   от kit'ов + app's `Path`, склеивает в tmp/ с глобальной перенумерацией, гонит
   через golang-migrate как сейчас.
3. `cmd/migrate` шаблона переключён на `MigrateUpMulti` с
   `bruteforcekit/pgstore.MigrationsFS` в списке источников.
4. Копия DDL `bruteforce_attempts` удалена из `template/migrations/0002_auth.up.sql`.

## Не входит в скоуп

- `auditkit` / `outboxkit` рефакторинг — отдельный план если/когда понадобятся
  (в `admin-panel-backend` на v1 выключаются полностью).
- `cachekit` — не использует БД, не имеет миграций, паттерн не дырявый.
- `authnkit` — уже clean (port-only, без БД).
- Backward compatibility / migration path для существующих БД — на этот момент
  единственный downstream (`admin-panel-backend`) проходит cleanup-плана с drop
  схемы, breaking change безопасен.
- Auto-discovery migrations через build-tag / link-time registry — overkill, явная
  передача `[]embed.FS` достаточна.

## Открытые вопросы

- [x] **Q1:** **Renumber 1..N в порядке sources** + инвариант "новые kit-источники
  всегда добавляются в **конец** списка sources в `cmd/migrate`" (чтобы не сдвигать
  существующие версии). Простая реализация, прозрачная `schema_migrations` таблица.
  Документировать инвариант в comment-строке над `Sources: []Source{...}` в `cmd/migrate/main.go`.
- [x] **Q2:** `MigrateDownMulti` откатывает на N шагов в обратном порядке через
  golang-migrate (как `migrate down` сейчас — на 1 шаг). Kit-миграции участвуют
  на равных. Норм для dev-перезапуска; в prod down не пускаем.

## Атомы

### Фаза 1 — golibinfra

- [x] **A1: `bruteforcekit` — расщепить Module** ✓ — root `module.go` удалён (kit-root теперь только Guard interface + Config). Все pg-binding'и переехали в `pgstore/`.
  - Что делаем: в `golibinfra/bruteforcekit/module.go` оставить **только** provide
    `fx.Provide(NewPgGuard, fx.As(new(Guard)))` → ❌ нет, заменить на:
    `Module` теперь пустой (только log "bruteforcekit/core loaded"), без provide.
    App обязан подключить ИЛИ `bruteforcekit/pgstore.Module`, ИЛИ свой адаптер.
    Альтернативно: `Module` оставить deprecated alias, добавить `CoreModule` пустой.
    Решить точную форму при A1 — но контракт: **kit-root больше не provide'ит pgguard**.
  - Файлы: `golibinfra/bruteforcekit/module.go`, `pgguard.go` (move), `migrations.go` (move), `migrations/*.sql` (move)
  - Verify: `cd golibinfra && go build ./bruteforcekit/...` → OK
  - Зависит от: —

- [x] **A2: `bruteforcekit/pgstore` подпакет** ✓ — `pgstore/{store,migrations,module}.go` + `migrations/0001_bruteforce_attempts.{up,down}.sql`. `pgGuard`→`Store`, `Migrations`→`MigrationsFS`. `go build ./bruteforcekit/...` + `go vet` зелёные.
  - Что делаем: создать `golibinfra/bruteforcekit/pgstore/`. Переместить туда
    `pgguard.go` → `store.go`, переименовать `pgGuard` → `Store`. Переместить
    `migrations/` + `migrations.go` (rename embed var `Migrations` → `MigrationsFS`
    для consistency с другими kit'ами). Создать `module.go`:
    `var Module = fx.Module("bruteforcekit/pgstore", fx.Provide(fx.Annotate(NewStore, fx.As(new(bruteforcekit.Guard)))))`.
  - Файлы: `golibinfra/bruteforcekit/pgstore/{module,store,migrations}.go`, `pgstore/migrations/*.sql`
  - Verify: `cd golibinfra && go build ./bruteforcekit/... && go test ./bruteforcekit/...` → OK; `bruteforcekit/pgstore.MigrationsFS` экспортирован.
  - Зависит от: A1

- [x] **A3: `pgkit.MigrateUpMulti` — multi-source API** ✓ — `migrate_multi.go` с `Source{Name,FS,Prefix}` + `MultiSourceConfig{DSN,Sources}` (без `AppPath` — app передаётся как `Source` через `os.DirFS`, тип однородный). Composer склеивает в tmp с global renumbering 1..N, defer cleanup. Тесты `TestMigrateUpMulti_TwoSources` (testcontainers postgres) + `TestComposeMigrations_MissingDown` + `TestComposeMigrations_NoSources` — PASS 1.7s.
  - Что делаем: `golibinfra/pgkit/migrate_multi.go`. Сигнатура:
    ```go
    type Source struct { Name string; FS fs.FS; Prefix string } // Prefix = subdir внутри FS, обычно "migrations"
    type MultiSourceConfig struct { DSN string; AppPath string; Sources []Source }
    func MigrateUpMulti(cfg MultiSourceConfig) error
    func MigrateDownMulti(cfg MultiSourceConfig) error
    func MigrateVersionMulti(cfg MultiSourceConfig) (uint, bool, error)
    ```
    Реализация: composer создаёт tmp/, копирует `*.up.sql`/`*.down.sql` из всех
    sources с глобальной перенумерацией (см. Q1), затем вызывает существующий
    `newMigrator` с `file://<tmpdir>`. tmp/ чистится через defer.
    Тесты: `migrate_multi_test.go` с testcontainers — один pg, два source (in-mem
    embed.FS из testdata), проверка что все таблицы созданы и `schema_migrations`
    содержит N версий.
  - Файлы: `golibinfra/pgkit/migrate_multi.go`, `migrate_multi_test.go`, `testdata/`
  - Verify: `cd golibinfra && go test ./pgkit/... -tags=integration` → PASS
  - Зависит от: —

### Фаза 2 — go-backend-template

- [x] **A4: `cmd/migrate` → MigrateUpMulti** ✓ — `Sources: [bruteforcekit/pgstore.MigrationsFS, os.DirFS("/migrations")]`. Добавлен dev-time `replace git.stit.tech/stit/golibinfra => /root/git/stit/golibinfra` в `go.mod` (убрать перед merge / после bump golibinfra версии). `go build ./cmd/migrate/` зелёный.
  - Что делаем: в `template/cmd/migrate/main.go` заменить `pgkit.MigrateUp(MigrateConfig{...})`
    на `pgkit.MigrateUpMulti(MultiSourceConfig{...})`. Sources на v1: только
    `bruteforcekit/pgstore.MigrationsFS` (потом сюда же auditkit/outboxkit когда
    дойдём). `AppPath` — `/migrations` (как сейчас).
  - Файлы: `template/cmd/migrate/main.go`
  - Verify: `make build` → OK; локально `make migrate-up` на пустой БД накатывает
    все таблицы (kit + app) в правильном порядке.
  - Зависит от: A2, A3

- [x] **A5: Удалить дубликат DDL `bruteforce_attempts` из template** ✓ — `migrations/0002_auth.{up,down}.sql` теперь только `refresh_sessions`. Шапка обновлена ("bruteforce_attempts владеет bruteforcekit/pgstore"). Реальная интеграция (migrate up на свежей БД) — в A7.
  - Что делаем: в `template/migrations/0002_auth.up.sql` оставить **только**
    блок `refresh_sessions` (это app-таблица); строки про `bruteforce_attempts`
    выкинуть. То же для `0002_auth.down.sql`. Поправить комментарий-шапку
    ("теперь bruteforcekit владеет своими таблицами через pgstore.MigrationsFS").
  - Файлы: `template/migrations/0002_auth.up.sql`, `0002_auth.down.sql`
  - Verify: `make migrate-down && make migrate-up` зелёный; `psql -c "\d bruteforce_attempts"` показывает таблицу (создана kit'ом, не app).
  - Зависит от: A4

- [x] **A6: `cmd/api/main.go` — подключить pgstore-модуль** ✓ — импорт `bruteforcepg "git.stit.tech/stit/golibinfra/bruteforcekit/pgstore"`, `bruteforcekit.Module` → `bruteforcepg.Module` в `platformModules`. `bruteforcekit.NewConfig` остался в `kitConfigs` (Config — root-level type). `go vet + go build ./cmd/api/...` зелёные.
  - Что делаем: в `template/cmd/api/main.go` заменить `bruteforcekit.Module` на
    `bruteforcekit.Module + bruteforcekitpg.Module` (импорт
    `git.stit.tech/stit/golibinfra/bruteforcekit/pgstore`). Альтернативно: один
    `bruteforcekitpg.Module` если core-Module теперь пустой (см. A1).
  - Файлы: `template/cmd/api/main.go`
  - Verify: `make build && ./bin/api` стартует без panic в fx graph.
  - Зависит от: A4

- [x] **A7: Template pre-merge + integration smoke** ✓ — `make pre-merge` → `✓ pre-merge clean` (fmt+vet+lint+test+build+sqlc-diff+mocks-check). Все mocks regenerated без изменений relevant for kit refactor.
  - Что делаем: `make pre-merge` → зелёный. Опционально гнать `make test-int`
    (testcontainers поднимает pg, прогоняет auth login flow с bruteforce-блоком).
  - Файлы: —
  - Verify: `make pre-merge` → OK; `make test-int` → PASS (если есть auth-int-тест).
  - Зависит от: A5, A6

### Фаза 3 — admin-panel-backend follow-up

- [ ] **A8: Перенести kit-обновления в `admin-panel-backend`**
  - Что делаем: bump `golibinfra` версии в `ROC/admin-panel-backend/go.mod`
    (если используется replace/local-path в dev — sanity check). Применить
    template-style композицию модулей: импорт `bruteforcekit/pgstore`, использование
    `MigrateUpMulti` в `cmd/migrate`. План `cleanup-and-iam` после этого
    переходит обратно из `blocked` в `planning` и стартует.
  - Файлы: `ROC/admin-panel-backend/go.mod`, `cmd/migrate/main.go`, `cmd/api/main.go`
  - Verify: `cd ROC/admin-panel-backend && make build` → OK
  - Зависит от: A7

- [ ] **A9: Снять BLOCKED с `cleanup-and-iam`**
  - Что делаем: в `ROC/admin-panel-backend/.claude/plans/cleanup-and-iam.md`
    `status: blocked → planning`, чекпоинт-запись "unblocked, kit-store-port-refactor
    done". Обновить **A4** (миграция `0003_bruteforce_attempts` больше не нужна —
    приходит из kit). Обновить **A2** (импорт `bruteforcekit/pgstore` теперь
    обязателен). После этого `cleanup-and-iam` готов к approve+старту.
  - Файлы: `ROC/admin-panel-backend/.claude/plans/cleanup-and-iam.md`, `roadmap.md`
  - Verify: `scripts/agent-tools/validate-domain-doc.sh` зелёный (не задеваем артефакты).
  - Зависит от: A8

## Чекпоинты

### 2026-05-12 19:56 — фаза 1 закрыта

- Закрыто: A1 ✓, A2 ✓, A3 ✓ (golibinfra side).
- Изменения дизайна vs. изначального плана:
  - **Нет `AppPath`** в `MultiSourceConfig` — app передаётся как обычный `Source{FS: os.DirFS(path), Prefix: "."}`. Однородный API, проще каллер.
  - **`bruteforcekit/module.go` удалён целиком** (не оставлен пустой alias) — kit-root теперь только Guard interface + Config. Callers ОБЯЗАНЫ импортировать `bruteforcekit/pgstore` (или свой store-adapter).
- Известные сломанные callers (фиксятся в фазе 2): `template/cmd/api/main.go:67` и `ROC/admin-panel-backend/cmd/api/main.go:67` ссылаются на `bruteforcekit.Module`. Первое — A6, второе — фаза 3 (A8, отложено).
- Следующий шаг: A4 — template `cmd/migrate` на новый API.

### 2026-05-12 20:00 — фаза 2 закрыта

- Закрыто: A4 ✓, A5 ✓, A6 ✓, A7 ✓ (template side).
- Решения по ходу:
  - Добавлен **dev-time `replace git.stit.tech/stit/golibinfra => /root/git/stit/golibinfra`** в `template/go.mod`. Уберётся когда golibinfra будет тегнут и опубликован (новая минорная — kit refactor breaking).
  - `bruteforcekit.Config` остался импортируемым из root-package (`bruteforcekit.NewConfig` в `kitConfigs`) — это правильно, root держит типы; импорт `bruteforcepg` нужен только для fx.Module.

### 2026-05-12 20:12 — обнаружен и закрыт остаточный leak

- При финальном аудите перед push'ем нашёл вторую дырявую абстракцию: `internal/jobs/bruteforce.go` чистил `bruteforce_attempts` через raw SQL — app знал private-схему kit'а. Это тот же класс проблем, что и DDL-копипаст, только в runtime-коде.
- Фикс (золотыми коммитами на той же branch):
  - **golibinfra `3ec743c`**: `bruteforcekit.Guard.Cleanup(ctx)` + `Config.CleanupRetention` (default 24h) + `pgstore.Store.Cleanup` impl + integration test (`TestStore_Cleanup`). `auth.LoginGuard` остаётся подмножеством Guard — структурный alias не сломан.
  - **template `34cb71c`**: `internal/jobs/bruteforce.go` теперь принимает `bruteforcekit.Guard` и зовёт `g.Cleanup(ctx)`. Pinned golibinfra `3ec743c`.
  - **template `00b58ae`**: `.claude/rules/jobs-and-subscribers.md` — пример переписан с raw SQL на `Guard.Cleanup`, добавлено правило "не лезь сырым SQL в kit-таблицу, зови kit-метод". Иначе rule учил бы будущих авторов на анти-паттерне.

### 2026-05-12 20:24 — audit + outbox добиты до конца

Юзер указал что остаются ещё outbox+audit с тем же паттерном. Добил:

- **golibinfra `556baa9` `refactor(auditkit)!`**: kit-root оставлен с Entry + Logger interface; `auditkit/pgstore.Store` (был `pgLogger`) + `MigrationsFS` + Module провайдит Store as Logger. Тесты переехали в pgstore.
- **golibinfra `07f25f1` `refactor(outboxkit)!`**: kit-root оставлен с Message + Repository/Publisher + Config + Bus + Worker + orchestration-Module. `outboxkit/pgstore.Store` (был `pgRepository`) + partitions.go (SchemaSQL, PartitionSQL, EnsurePartitions как метод Store) + `MigrationsFS`. Новый `pgstore.PartitionKeeperJob` — daily timerkit.Job создаёт next-month партицию (без него вставки ломались бы при rollover календаря). Тесты переехали в pgstore. Заодно фикс pgx-v5 quirk: `pg_class.relkind::text` (без cast pgx не сканит char OID 18 в *string).
- **template `32ff4df` `refactor: adopt audit + outbox pgstore split`**: `cmd/migrate` Sources += auditkit/pgstore + outboxkit/pgstore. `cmd/api` `auditkit.Module` → `auditpg.Module`, `outboxpg.Module` рядом с `outboxkit.Module`. Удалены `migrations/0003_audit.{up,down}.sql` и `migrations/0004_outbox.{up,down}.sql`. Побочный эффект: outbox теперь реально партиционирован (template'овая копия была flat-таблицей, разошлась с kit'ом).

### Финал (push state)

- golibinfra branch `refactor/kit-pluggable-store` @ `07f25f1`: 5 коммитов — `feat(pgkit)`, `refactor(bruteforcekit)!`, `feat(bruteforcekit) Cleanup`, `refactor(auditkit)!`, `refactor(outboxkit)!`.
- template branch `refactor/kit-pluggable-store` @ `32ff4df`: 5 коммитов — `refactor` (bruteforce compose), `refactor(jobs)`, `docs(rules)`, `docs(plans)`, `refactor: adopt audit+outbox`.
- Фаза 3 (A8-A9, admin-panel-backend) — отложена по запросу юзера.
- **Перед merge**: golibinfra → tag `v0.19.0` (breaking minor), template `go.mod` repin с pseudo-version на `v0.19.0`.

