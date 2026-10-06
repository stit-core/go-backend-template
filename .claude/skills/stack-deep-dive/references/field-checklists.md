# Чек-листы полей по kind

По разделу на каждый из 8 stack-kinds. В каждом разделе:

- **Обязательные H2** — что должно быть и непусто к stable.
- **Что в Generates** — конкретика, которую ждёт `de-template`.
- **Cross-kind ссылки** — куда роллапается это решение
  (`config`/`runtime`/другой kit).
- **Главные anti-patterns**.
- **Порядок копания**.

Полный канон — `docs/stack/README.md` + `_templates/<kind>.md`. Здесь
— выжимка для скилла.

---

## service

**Обязательные H2**: `Identity`, `Versioning`, `Binaries`, `Generates`.

**Generates содержит**: `go.mod module path`, README-шапка, ldflags
`-X main.version` / `-X main.commit` в Makefile/build, лог
`"starting" component/version/commit` в `cmd/*`.

**Cross-kind**: `module path` — общий якорь, на него опираются все
импорты; старт-лог `component`/`version`/`commit` уезжает в
`observability § Log levels & content`.

**Anti-patterns**:
- module path остался шаблонным (`git.stit.tech/stit-core/go-backend-template`).
- `Versioning: semver` без указания как версия попадает в бинарь
  (ldflags? VERSION-файл?).
- Image-теги `:latest`/`:edge` (CLAUDE.md § Не делать никогда).

**Порядок копания**:

1. `Identity`: имя сервиса (Other), module path (Other), одно-фразовое
   назначение (Other), `project-slug` — папка продукта в соседнем strategy-репо
   (Other), `service-slug` — этот сервис в реестре `services/service.md`
   strategy-репо, откуда `scaffold` берёт домены (Other), `strategy-path` —
   путь к форку `<org>/strategy` (дефолт `../strategy`; Other для иной раскладки).
2. `Versioning`: схема (semver `vX.Y.Z` — стандарт; Other для отклонения),
   как попадает в бинарь (ldflags — Other), что логируется (стандарт —
   `"starting" component/version/commit`).
3. `Binaries`: какие `cmd/<x>` собираются (HTTP `cmd/api`,
   `cmd/migrate` как init, `cmd/seed-admin` one-off — выбор +
   Other для дополнительных).
4. `Generates`: построить из 1–3 (module path, ldflags, лог-формат) —
   через Other по каждому пункту.

---

## runtime

**Обязательные H2**: `Entrypoints`, `Lifecycle`, `Shutdown`, `Generates`.

**Generates содержит**: композиция `cmd/api/main.go`
(`fx.New(...)` с нужными модулями), регистрация subscribers/jobs,
порядок и бюджет остановки (`fx.StopTimeout`; хуки остановки — в китах,
канон — `docs/reference/architecture.md § fx`).

**Cross-kind (это роллап)**: читает решения из всех 6 kit-артефактов —
HTTP из `auth § Authgate`, drain из `messaging § Published`,
subs из `messaging § Consumed`, jobs из `auth § Brute-force` +
`persistence` (любые cleanup).

**Anti-patterns**:
- `Entrypoints: HTTP` при том, что в `messaging` объявлены consumed
  subjects → subscribers упущены.
- Shutdown «закрывается всё» без порядка (HTTP first → subs → drain
  → pool — стандартный порядок).

**Порядок копания**:

1. `Entrypoints`: множественный выбор из {HTTP, outbox-drain,
   NATS-subscribers, scheduled jobs} + Other.
2. `Lifecycle`: fx-композиция модулей (Other), интервалы jobs (Other),
   что регистрируется через `fx.Invoke` (Other).
3. `Shutdown`: порядок закрытия (Other), таймауты (Other), как
   дренится in-flight (HTTP graceful, drain полное, subs — drain
   текущих обработок).
4. `Generates`: список фактических fx-модулей по результатам 1–3.

---

## config

**Обязательные H2**: `Required env`, `Optional env`, `Secrets`,
`Validation`, `Generates`.

**Generates содержит**: `internal/platform/config` (структура `App`
конфигов приложения, загрузка из env через envconfig), `.env.example`
(required+optional с дефолтами), seed для INFRA-HANDOFF (required + секреты).

**Cross-kind (это роллап)**: читает решения kit-артефактов —
их envconfig-теги попадают сюда. Каждый env, упомянутый где-либо
в `Generates`/теле kit'а, должен быть в `Required` или `Optional`.

**Anti-patterns**:
- Секрет в `Optional env` с plain-default (значение).
- Required env без указания «где живёт» (kit-структура vs app-структура).
- Release-checklist отсутствует → новый envconfig забывают добавить.

**Порядок копания**:

1. `Required env` — таблица `Env | Тип | Назначение | Где живёт`.
   Источник — обход всех kit-артефактов на required envconfig.
   Other для каждого ряда (имя), но «где живёт» — enum (kit-структура
   golibinfra / `internal/platform/config`).
2. `Optional env` — подсекции по префиксу (Logging `LOG_*`,
   Postgres `PG_*`, HTTP `HTTP_*`, Auth `JWT_*`). Дефолты из шаблонов
   kit'ов.
3. `Secrets` — секреты-класс с таблицей `Env | Тип | Назначение`.
   **Значение не коммитим** — только имя env.
4. `Validation`: как fail-fast (envconfig); release-checklist
   подсекцией.
5. `Generates`: пути файлов (`internal/platform/config`,
   `.env.example`).

---

## persistence

**Обязательные H2**: `Engine`, `Migration strategy`, `Kit-owned tables`,
`Connection pool`, `Generates`.

**Generates содержит**: pgkit-wiring (pool, transactor) в fx, база
`sqlc.yaml`, `cmd/migrate` + порядок накатки app+kit миграций.

**Cross-kind**:
- `config § Required` — `PG_DSN`;
- `config § Optional` — `PG_MAX_CONNS`, `PG_MIN_CONNS`, `PG_MAX_CONN_*`, `PG_MIGRATE_TIMEOUT`;
- `dependencies § External` — Postgres как обязательная;
- `runtime § Lifecycle` — `cmd/migrate` как init-container? отдельный
  бинарь?

**Anti-patterns**:
- Дублирование CREATE TABLE для kit-таблиц (`outbox_<domain>`, `audit_log`,
  `bruteforce_*`, `authn_sessions`) — должно подключаться как `pgkit.Source` кита (`pgstore.Migrations` /
  `TableMigrations`).
- Размер пула без обоснования / без метрики.

**Порядок копания**:

1. `Engine`: Postgres+pgx/v5 (стандарт; Other для отклонения); PK-конвенции
   (uuid, `gen_random_uuid()` — стандарт); naming (snake_case, мн. число); optimistic-lock
   стратегия (через `version`-колонку — yes/no).
2. `Migration strategy`: инструмент (golang-migrate — стандарт),
   парные up/down (yes), нумерация (последовательная),
   `cmd/migrate` как init-контейнер (yes/no).
3. `Kit-owned tables`: какие подключаются источниками китов `pgstore.Migrations` (множественный
   выбор: outboxkit, auditkit, bruteforcekit, authnkit/session).
4. `Connection pool`: размер (Other — число), timeout acquire (Other),
   нужны ли метрики `pg_pool_*` (yes/no).
5. `Generates`: пути pgkit-wiring, `sqlc.yaml` каркас, `cmd/migrate`.

---

## messaging

**Обязательные H2**: `Published subjects`, `Consumed subjects`,
`Delivery semantics`, `Generates`.

**Generates содержит**: subject-константы у consumer'ов
(`internal/domain/<c>/delivery/nats/dto.go`), регистрация
подписчиков в fx, outbox-publisher wiring в usecase-deps.

**Cross-kind**:
- `config` — NATS URL, subjects (если параметризуются);
- `runtime § Entrypoints` — есть ли NATS-subs, есть ли outbox-drain;
- `dependencies § External` — NATS как зависимость.

**Anti-patterns**:
- `Delivery: at-most-once` при том, что у consumer нет idempotency.
- `Ordering: none` при cross-instance subscriber'ах с
  state-mutation на одном aggregate.
- Breaking-change payload без нового subject (`<domain>.events.<event>.v2`).
- В `Generates` упомянуты consumers, но в `runtime § Entrypoints` нет
  NATS-subs.

**Порядок копания**:

1. `Published subjects` (таблица). Source-aggregate — выбор из
   aggregate'ов соседнего strategy-репо (`../strategy/docs/projects/<slug>/services/<svc>/domains/`) + Other.
2. `Consumed subjects` (таблица). Source-context — выбор + Other.
3. `Delivery semantics`:
   - `Delivery`: enum at-least-once / at-most-once / exactly-once-effective;
   - `Ordering`: enum none / per-aggregate (key) / global;
   - `Idempotency`: Other (стандарт — natural key + unique-индекс, rule `events-and-messaging`);
   - `Versioning`: как меняем subject при breaking (Other).
4. `Generates`: пути `delivery/nats/dto.go`, fx-регистрация, outbox.

---

## auth

**Обязательные H2**: `Authn`, `Authgate`, `Roles`, `Seed-admin`,
`Brute-force`, `Generates`.

**Generates содержит**: authgate-wiring + middleware, authnkit-провайдеры
(JWT verifier, hasher), `cmd/seed-admin`, bruteforce guard + cleanup-job.

**Cross-kind**:
- `config § Required/Secrets` — `JWT_SECRET`, `SEED_ADMIN_*`;
- `runtime § Lifecycle` — cleanup-job интервал;
- `dependencies § kits` — authnkit, bruteforcekit.

**Anti-patterns**:
- `JWT_SECRET` вне `Secrets` или с plain-default: authnkit подписывает HS256, секрет и есть
  ключ подписи (не короче 32 байт — иначе старт падает).
- Публичный путь сделать admin'а.
- Bruteforce не защищает login → возможен credentials stuffing.
- Authgate-middleware кладёт что-то отличное от plain Principal
  (например полный User-aggregate) — нарушает изоляцию.

**Порядок копания**:

1. `Authn`:
   - `type`: enum `JWT` / `session` / `none` / `mixed`;
   - алгоритм (если JWT): HS256 — authnkit другого не подписывает; иной алгоритм — план в
     golibinfra → тег → bump, не локально (`docs/reference/architecture.md § Граница с golibinfra`);
   - где верифицируется: enum `authgate middleware` / `per-domain`.
2. `Authgate`: что middleware кладёт в context (Other — в шаблоне
   `identity.Identity{UserID, Role}`), публичные vs защищённые роуты (Other).
3. `Roles`: список (Other по каждой), где назначаются (Other), где
   проверяются (enum `policy-слой доменов` / `middleware` / `mixed`).
4. `Seed-admin`: путь (Other — обычно `cmd/seed-admin`), идемпотентность
   (yes/no), env-список (Other), публичного пути «сделать admin» нет
   (yes/no — должно быть yes).
5. `Brute-force`: что защищается (login — стандарт; multi-select для
   расширений), счётчики (Other), lock-политика (Other), cleanup-job
   интервал (Other).
6. `Generates`: пути.

---

## observability

**Обязательные H2**: `Log sink`, `Log levels & content`, `Generates`.

**Generates содержит**: logging-wiring (logkit → stdout, формат/уровень
из `LOG_*`), старт-лог `component`/`version`/`commit`, конвенция
логирования ключевых событий.

**Cross-kind**:
- `config § Optional` — `LOG_*` envs (формат, уровень, redact);
- `service § Versioning` — `version`/`commit` старт-лога (ldflags);
- `dependencies § kits` — logkit.

**Anti-patterns**:
- Неструктурный вывод (`LOG_FORMAT=text`) в проде.
- Ключевые события (старт, ошибки usecase, обработка эвента) не логируются.
- Логи мимо `logkit.Logger` (`fmt.Print*` / `slog.*`).

**Порядок копания**:

1. `Log sink`: вывод в stdout (стандарт); формат enum `json` / `text`
   (`LOG_FORMAT`); уровень `LOG_LEVEL` (default `info`; Other).
2. `Log levels & content`: что пишется на каждом уровне; какие события
   логируются структурно (старт сервиса, ошибки usecase, результат обработки
   эвента — Other по каждому); доп. структурные поля (multi-select + Other).
3. `Generates`: пути logging-wiring (logkit в fx), конвенция логирования
   событий.

---

## dependencies

**Обязательные H2**: `golibinfra kits`, `Что НЕ подключено`,
`External runtime deps`, `Capacity`, `Generates`.

**Generates содержит**: kit-импорты + fx-провайдеры (только нужные;
prune ненужных), wiring внешних deps (Postgres/S3/NATS/…),
golibinfra-first принцип (generic util → golibinfra, не локально).

**Cross-kind**:
- `config § Optional` — envconfig-теги подключённых kit'ов;
- `runtime § Entrypoints` — что из объявленного реально поднимается;
- `dependencies § External` ↔ `persistence § Engine` (Postgres),
  `messaging` (NATS), `auth` (если нужны внешние JWKS).

**Anti-patterns**:
- Подключённый kit, чьи env'ы появились в `config § Optional`, но
  fx-провайдер не зарегистрирован → env читается впустую.
- «Что НЕ подключено» отсутствует → ложные ожидания «эта env читается».
- External provider без режима отказа / timeout / retry policy.

**Порядок копания**:

1. `golibinfra kits` (таблица): множественный выбор kit'ов golibinfra —
   в графе `cmd/api` шаблона: telemetry, logkit, pgkit, natskit, httpkit,
   timerkit, authnkit (с session), bruteforcekit, auditkit, clockkit,
   outboxkit, healthkit; в коде доменов — ещё bizmetric;
   полный набор — README golibinfra § Пакеты нужной версии (storagekit,
   httpclient, … — Other для редких). Версия golibinfra (Other).
2. `Что НЕ подключено`: автоматически = (полный набор) − (выбранные
   в 1). Юзер подтверждает «вот эти kit'ы действительно не нужны?» через
   AskUserQuestion (multi-confirm).
3. `External runtime deps`: на каждую (Postgres/NATS/Redis/S3/MinIO/email/
   sms/geo/billing/...):
   - название (Other);
   - режим отказа (enum `503` / `partial degrade` / `queue+retry`);
   - timeout (Other), retry (Other), quota (Other);
   - env-ключи (Other);
   - как менять провайдера (Other — обычно «через port + adapter»).
4. `Capacity`: CPU/RAM (Other), replicas (Other), pool (Other),
   rate-ожидания (Other).
5. `Generates`: фактические kit-импорты, fx-wiring внешних deps.
