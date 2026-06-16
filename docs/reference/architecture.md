# Architecture

Слои бизнес-домена, ports/adapters, события, граница с golibinfra. Path-scoped правила
(`.claude/rules/*`) держат terse-инварианты и ссылаются сюда за «как и почему».

## Слои домена

Каждый домен `internal/domain/<name>/` — изолированный bounded context:

```
<domain>/
├── entity.go        доменные типы (User, Session); без инфраструктуры
├── dto.go           input/output типы usecase (не HTTP-DTO)
├── errors.go        sentinel-ошибки: var ErrNotFound = errors.New(...)
├── events.go        outbox-эвенты: типы и payload
├── policy.go        authorization; решения без I/O
├── usecase.go       бизнес-логика; зависимости через интерфейсы (порты)
├── metrics.go       bizmetric-счётчики домена (biz_<plural>_*_total); nil-safe Record*
├── delivery/http/   huma/v2 + chi: handler.go, dto.go, view.go
├── delivery/nats/   NATS-subscriber (опц.): handler.go, dto.go
├── infra/postgres/  repository.go + queries/*.sql + db/ (sqlc, read-only)
└── module/          module.go (fx) + adapters.go (cross-domain)
```

| Тип кода | Где |
|---|---|
| Доменные типы | `entity.go` |
| Ошибки (sentinel) | `errors.go` |
| Логика | `usecase.go` |
| Authorization | `policy.go` |
| Outbox-события | `events.go` |
| Domain-метрики (bizmetric-счётчики) | `metrics.go` |
| HTTP handler / DTO / mapping | `delivery/http/{handler,dto,view}.go` |
| sqlc-запросы / repository | `infra/postgres/queries/*.sql`, `repository.go` |
| fx-wiring / cross-domain адаптеры | `module/{module,adapters}.go` |

## Ports & adapters (cross-domain)

depguard в `.golangci.yml` строго изолирует бизнес-домены: правило `<domain>-domain-isolation`
запрещает домену импортить что-либо **вне своего поддерева** `internal/domain/<domain>/` —
deny префикса `internal/domain` + allow только своего поддерева (`list-mode: lax`,
longest-prefix override: свой импорт ловит более длинный allow, любой сосед ловит umbrella-deny).
Любой соседний домен, включая ещё не созданные, блокируется автоматически — без перечисления
по имени. Исключения: `module/` (адаптеры) и `*_test.go`. Когда домену A нужны данные из B —
объявляешь **порт** (интерфейс) в A (`usecase.go`) и **адаптер** в `A/module/adapters.go`,
который оборачивает B. Адаптер в `module/` имеет право импортить B; usecase — нет. Cross-domain
DTO в порте — локальные (не re-export из B).

## HTTP DTO naming (huma)

huma собирает все DTO-структуры всех доменов в одну OpenAPI schema registry по имени
Go-типа. Два домена с одинаковым `ListOutput` → коллизия `ListOutputBody` → panic. Поэтому
имена publicly-named DTO в `delivery/http/dto.go` — domain-specific: `EventListOutput`,
не `ListOutput`. Inline `Body struct{...}` именуется по Outer-типу — уникальность Outer
гарантирует уникальность Body.

## События и messaging

Отдельного worker-процесса нет: HTTP, outbox-drain, NATS-subscribers, background jobs —
в одном бинаре `cmd/api` через одну fx.Lifecycle.

- **Outbox (per-domain).** Событие пишется в БД в одной транзакции с основным изменением
  (`pgkit.Transactor.WithTx` → `outboxkit.Publisher.Publish(txCtx, subject, payload)`). Каждый
  домен-публикатор держит свой transactional-outbox (таблица `outbox_<domain>`,
  Outbox+Publisher+drain) через `events.OutboxFor` (`internal/platform/events`), включённый в
  его `module.Module` (`fx.Private` скоупит publisher доменом). Drain батчами публикует в
  JetStream через `natskit.StreamBus` (PubAck-гарантия; id строки уходит как `Nats-Msg-Id` →
  переотправка на ретрае дедуплицируется стримом); published метятся только подтверждённые.
  Subject — литерал `<domain>.events.<event>`; источник его не экспортит. Poison-строка
  (исчерпан `OUTBOX_MAX_ATTEMPTS`) метится dead и больше не вычитывается.
- **Subscriber'ы in-domain.** Durable JetStream-консьюмеры в `<consumer>/delivery/nats/`
  рядом с `delivery/http/` — равноправные «входы» в usecase (`natskit.SubscribeStreamJSON`,
  стрим+durable; `ErrJetStreamDisabled` → graceful-skip для local-dev без `-js`). Объявляют
  **локальный** payload-DTO (depguard запрещает импорт домена-источника). Регистрируются в
  `module/module.go` consumer'а.
- **Idempotency.** JetStream = at-least-once → subscriber обязан быть идемпотентен (`ON
  CONFLICT DO NOTHING`, `UPDATE … WHERE status='pending'`, idempotency-key). Не «вряд ли
  задвоится». Poison у consumer'а — `NATS_JS_MAX_DELIVER`→DLQ/Term (natskit).
- **Jobs.** Периодические задачи без события-триггера — `internal/jobs/<name>.go`,
  поднимаются той же fx.Lifecycle. Если job чистит таблицу kit'а — зовёт метод kit'а, не
  сырой SQL.
- **JetStream-стримы.** drain и subscriber'ы требуют включённого JetStream
  (`NATS_JETSTREAM_ENABLED=true`) и стримов, покрывающих subject-префиксы доменов
  (`NATS_JS_STREAM_NAMES`, напр. `user,verification` → `user.>`/`verification.>`).
- **Таблицы.** `audit_log`/`bruteforce_*`/default `outbox` — схему держит golibinfra-kit
  (`MigrationsFS`). Per-domain `outbox_<domain>` заводит миграция этого репо (`migrations/`,
  DDL зеркалит `outboxkit/pgstore.SchemaSQL`); default-таблица `outbox` не используется.

Terse-инварианты — `.claude/rules/{events-and-messaging,domain-architecture,sqlc-and-migrations}.md`.

## Граница с golibinfra

Переиспользуемое/инфраструктурное (generic util, HTTP-client, middleware, kit-подобный
примитив) — **кандидат в `golibinfra`, не в этот репо**. Не плодить полукалеки здесь.
Доменная логика — сюда; платформенный примитив — upstream (PR в golibinfra). Так шаблон
остаётся тонким слоем над общей платформой, а улучшения едут всем сервисам сразу. Нет
нужного примитива — PR в golibinfra + строка в roadmap-backlog; временный shim только с
явным OK юзера. Инвариант для агента — CLAUDE.md § Стиль кода.

## Non-goals (осознанно не делаем)

- **Multi-agent swarm** — Cognition + Anthropic Multi-Agent Research caveat про код.
- **Иерархия team-lead→workers** — «share full traces» сложнее в иерархии, чем в едином
  агенте с subagent-изоляцией.
- **Десятки специализированных агентов** — успех = чёткие границы в *правилах*, не
  множественность *агентов*.
- **Vector-DB retrieval** — ripgrep + `gopls` достаточно для Go-репо.
- **Bloated CLAUDE.md** — bloat снижает adherence; лимит 200 строк.
- **JSON-wrapped edit-формат** — нативный string-replace через Edit.

Это present-state policy (триггер пересмотра — обсуждение с владельцем), не история.
