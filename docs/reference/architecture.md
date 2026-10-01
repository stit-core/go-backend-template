# Architecture

Каталоги, слои бизнес-домена, ports/adapters, события, граница с golibinfra. Path-scoped правила
(`.claude/rules/*`) держат terse-инварианты и ссылаются сюда за «как и почему».

## Каталоги

| Каталог | Что в нём | Что импортирует |
|---|---|---|
| `internal/domain/<d>/` | бизнес-код домена: слои ниже | `internal/platform`, `pkg`, киты; чужой домен — только `module/` |
| `internal/domain/<d>/infra/<kit>/` | адаптер порта домена к киту, внешнему каналу (почта, платёжный провайдер) или клиенту из `pkg/` | корень своего домена, кит, клиент |
| `internal/platform/` | клей сервиса, общий доменам: конфиг и prod-гард (`config`), сессия (`authgate`), личность (`identity`), outbox доменов (`events`) | `pkg`, киты; доменов не знает |
| `pkg/` | правила и типы сервиса без I/O, общие доменам (`policy`, `paging`, `outcome`); сгенерированные клиенты (gRPC, OpenAPI) — их зовёт `infra/` домена | `pkg`, киты |
| `cmd/` | входы: `api`, `migrate`, `seed-admin` | всё |
| `internal/testkit/`, `internal/archtest/` | тестовая обвязка и машинные правила раскладки | — |

Generic-примитив, нужный не только этому сервису (HTTP-клиент, middleware, «kit»), — в golibinfra
(§ Граница с golibinfra). Других каталогов в `internal/` нет (archtest `TestInternalLayout`).

## Слои домена

Каждый домен `internal/domain/<name>/` — изолированный bounded context:

```
<domain>/
├── entity.go        доменные типы (User, Session); без инфраструктуры
├── dto.go           input/output типы usecase (не HTTP-DTO)
├── errors.go        sentinel-ошибки: var ErrNotFound = errors.New(...)
├── events.go        outbox-эвенты: payload-типы + unexported subject-const
├── policy.go        authorization + чистые правила; решения без I/O
├── usecase.go       бизнес-логика; зависимости через интерфейсы (порты)
├── trusted.go       операции для других доменов без policy (Trusted)
├── metrics.go       метрики для алертов (biz_<domain>_*_total); nil-safe Record*
├── delivery/http/   huma/v2 + chi: handler.go, dto.go, view.go
├── delivery/nats/   NATS-subscriber (опц.): handler.go, dto.go
├── infra/postgres/  repository.go + queries/*.sql + db/ (sqlc, read-only)
├── infra/<kit>/     адаптер порта домена к киту (infra/jwt, infra/bruteforce)
└── module/          module.go (fx) + adapters.go (cross-domain)
```

| Тип кода | Где |
|---|---|
| Доменные типы | `entity.go` |
| Ошибки (sentinel) | `errors.go` |
| Логика | `usecase.go` |
| Операции для других доменов (без policy) | `trusted.go` |
| Authorization + чистые валидационные правила | `policy.go` |
| Outbox-события | `events.go` |
| Domain-метрики для алертов (bizmetric) | `metrics.go` |
| HTTP handler / DTO / mapping | `delivery/http/{handler,dto,view}.go` |
| sqlc-запросы / repository | `infra/postgres/queries/*.sql`, `repository.go` |
| Адаптер порта к киту | `infra/<kit>/` |
| fx-wiring / cross-domain адаптеры | `module/{module,adapters}.go` |

### Обязательность файлов

Файл существует, только если существует его содержимое — пустышек-плейсхолдеров нет.
Условия существования:

| Файл | Когда обязателен |
|---|---|
| `entity.go`, `errors.go`, `usecase.go`, `module/module.go` | всегда |
| `metrics.go` | у домена есть сигнал, на который ставят алерт (исходы входа, повтор refresh, письма сброса, которые не ушли); счётчиков операций «на всякий случай» нет |
| `dto.go` | есть хоть один Input/Output-тип (метод с 1–2 скалярами обходится без DTO) |
| `policy.go` | есть authenticated actor (authorization) **или** чистые правила валидации домена (общее для доменов правило пароля — `pkg/policy`) |
| `events.go` | домен публикует хотя бы одно outbox-событие; вместе с ним — `infra/outbox/outbox.go` |
| `delivery/http/view.go` | есть хоть один response с телом (mapping entity → view) |
| `delivery/nats/` | домен подписан хотя бы на один subject |
| `trusted.go` | другой домен берёт данные или действия домена через порт |
| `module/adapters.go` | у домена есть хоть один cross-domain порт с логикой маппинга |

Мёртвый слой (тип/файл без потребителя) не хранится «на будущее» — удаляется, история в git.

### Что запрещено класть в каждый слой

| Файл | Запрещено |
|---|---|
| `entity.go` | любой I/O, импорт `infra/`/`delivery/`/kit'ов: только stdlib, `internal/platform/identity` и `pkg/policy` |
| `policy.go` | I/O, импорт `infra/`/`delivery/`; только чистые решения (`pkg/policy.Decision` / bool) |
| `dto.go` | huma-теги, json-теги транспорта (это `delivery/http/dto.go`), методы с I/O |
| `errors.go` | всё, кроме sentinel-`var` и error-типов домена (отказ policy — общий `policy.ForbiddenError` из `pkg/policy`) |
| `events.go` | всё, кроме payload-типов и unexported subject-констант |
| `usecase.go` | прямой транспорт: `httpkit`/`natskit`/huma/`net/http`; ctx-геттеры транспорта — только через `internal/platform/identity`; inline-константы правил (место — `policy.go`) |
| `delivery/*` | бизнес-решения (все ветвления — в usecase), импорт чужих доменов и своего `infra/` (archtest) |
| `infra/postgres` | бизнес-логика в SQL (сложные CASE-ветвления политик); запросы держат CRUD/фильтры |
| `module/` | бизнес-логика и обёртки китов (место — `infra/<kit>/`); только wiring + маппинг типов/ошибок между доменами |

### Канон именования

- **Domain-DTO** (`dto.go`): `<Op>Input` / `<Op>Output` (`LoginInput`, `CreateInput`).
  Вспомогательные value-типы — описательно (`ListFilter`); страница списка — общий `paging.Page`
  (`pkg/paging`), он же встраивается во вход list-операции huma.
  Output-DTO заводится, когда результат ≠ entity и ≠ скаляр.
- **События** (`events.go`): `<Aggregate><PastEvent>` (`UserRegistered`,
  `EmailVerificationRequested`); payload — без json-тегов (wire = Go-имена; смена формата =
  новый subject `.v2`), timestamp-поле — `At time.Time`. Subject — unexported const
  `subject<Event>` рядом с payload-типом.
- **Sentinel** (`errors.go`): `Err<What> = errors.New("<domain>: <lowercase text>")`.
- **HTTP-DTO** (`delivery/http/dto.go`): `<Domain><Op>Input/Output` — глобально уникальные
  имена (huma registry, см. ниже). Response-envelope — `Body.Data` (`json:"data"`); списки —
  `data` + `total`.
- **OperationID**: lowerCamel `<verb><Noun>` (`loginUser`, `listUsers`), уникален глобально.
  Path-префикс и `Tags` роута могут не совпадать с доменом-владельцем (регистрация живёт в
  `user`, но путь `/api/auth/register`): владелец = тот, чей usecase вызывается.
- **Метрики** (`metrics.go`): первый аргумент bizmetric — имя домена = имени пакета
  (`biz_<domain>_<event>_total`); struct `Metrics`, конструктор `NewMetrics(m metric.Meter) *Metrics`,
  все `Record*` — nil-safe. Исход (метка `result`) — `outcome.Result`: общие исходы — `pkg/outcome`,
  особые для домена — переменные `outcome.New(…)` в `metrics.go`; строка вместо исхода не
  компилируется. Экспорт — `telemetry.Module` в `cmd/api`: при `OTEL_SDK_DISABLED=true` сигналы не
  уходят; запись метрики тест сверяет SDK-ридером (`sdkmetric.NewManualReader`).
- **Repository**: создание — `Insert*` (не `Create*`), sqlc-аннотации — `Insert<Entity>`.

### Разбиение переростков

Рукописный файл > ~400 строк — сигнал к разбиению по потокам: `usecase_<flow>.go`
(`usecase_login.go`), `handler_<op>.go`. Порты, `UsecaseDeps`, `Usecase`, `NewUsecase`
остаются в `usecase.go`. Разбиение — внутри пакета, без смены API.

### Usecase-канон

- Порты-интерфейсы объявлены в `usecase.go` сверху; зависимости — `UsecaseDeps{fx.In}`;
  конструктор `NewUsecase(d UsecaseDeps)`; хвост deps в порядке `Audit?, Log, Clock, Metrics`.
  `Usecase` хранит deps целиком (`type Usecase struct{ d UsecaseDeps }`, обращение `uc.d.Repo`):
  зависимость названа один раз, забыть её присвоить нельзя.
- Named returns не используются (линтер `nonamedreturns`).
- Метод с метрикой пишет исход ровно раз на любом пути: исход возвращает внутренняя функция
  (`Login` → `login`), запись — одна, в публичном методе.
- Security-значимые операции (login/logout/смена+сброс пароля/verification) пишут `audit`
  из usecase одним хелпером домена (`uc.audit`: время, IP, запрос) — в записи ID учётной записи, не
  адрес; попытка входа на неизвестный адрес и блокировка guard'а — только исход в метрике.
  След таких отказов по клиенту (IP, частота) — access-лог прокси перед сервисом.
  Техническая запись канала (факт доставки письма) — в его адаптере.
- Транспортные значения из ctx (`IPFrom`, `RequestIDFrom`) — только `internal/platform/identity`.
- ID, которые создаёт код (не БД), — `idkit.NewV7()`.

## Ports & adapters (cross-domain)

`internal/archtest` (`TestImportBoundaries`, шаг тестов) строго изолирует бизнес-домены: для
любого каталога `internal/domain/*`, включая ещё не созданные, импорт чужого домена запрещён —
без перечисления доменов в конфигах. Исключения: `module/` (адаптеры; чужой домен — только
корневым пакетом) и `*_test.go`. Там же держатся слои: `delivery` не импортирует `infra`,
`infra/postgres/db` — только `infra/postgres`, корень домена — ни своих подпакетов, ни
транспорта, драйверов и I/O ОС (в том числе через `pkg/`), `infra/outbox` — только свой `module/`
и `cmd/migrate`, моки —
только из тестов; вне доменов домен импортирует только `platform` и `pkg`, а они — только друг
друга (другой пакет стал бы мостом в обход портов). Когда домену A нужны данные из B —
объявляешь **порт** (интерфейс) в A (`usecase.go`) и **адаптер** в `A/module/adapters.go`,
который оборачивает B. Адаптер в `module/` имеет право импортить B; usecase — нет. Cross-domain
DTO в порте — локальные (не re-export из B).

Канон адаптера:

- **Поставщик отдаёт `Trusted`** (`trusted.go`): операции для других доменов без policy —
  решение принял вызывающий. Методы с policy (`Usecase`) — только входам самого домена (HTTP,
  NATS); `Trusted` берут только адаптеры `module/` потребителей, свой `module/` лишь провайдит
  `NewTrusted` (archtest `TestTrustedOnlyInModules`). Ошибки `Trusted` — список `TrustedErrors`: что
  других нет, держат тест `Trusted` (исходы репозитория) и archtest `TestTrustedErrors` (`return` в
  `trusted.go`), что каждая переводится — тест адаптера.
- **Identity-closure** (`func(u *user.Usecase) userhttp.Usecase { return u }`) — только внутри
  домена: delivery-порт своего usecase. Порт между доменами closure не реализует — чужие
  sentinel'ы уйдут потребителю без маппинга (archtest `TestNoCrossDomainIdentityClosure`).
- **Типовой адаптер** — тип **и его конструктор** в `adapters.go` (не расщеплять между
  module.go/adapters.go); один тип на домен-поставщика реализует все порты потребителя к нему.
- **Sentinel-маппинг исчерпывающий:** каждая ошибка из `TrustedErrors` поставщика → sentinel
  потребителя (`user.ErrConcurrentUpdate` → `auth.ErrConcurrentUpdate`), usecase-потребитель не
  может `errors.Is` по чужим sentinel'ам (archtest); тест адаптера идёт по `TrustedErrors` на каждом
  методе.
- **Порт переносит решения, не сырьё:** вычисленное свойство — метод сущности поставщика
  (`CanLogin()`), в адаптере нет условий по полям поставщика.
- Времени в адаптерах нет: `clockkit.Clock` инжектится так же, как в usecase.

## fx

Граф сервиса — `service` в `cmd/api/main.go`: его запускает `main`, его же собирают
`TestFxGraphValidates` (статический граф, `fx.ValidateApp`) и `TestBoot` (конструкторы, Invoke и
хуки на контейнерах).

- **Модуль** — `fx.Module` на пакет (доменный `module/`, пакет платформы, `cmd/api`); имя — путь
  пакета от корня репо (`internal/domain/user/module`).
- **Привязка к порту** — тип результата провайдера. Свой конструктор возвращает порт
  (repository, policy, адаптер); тип, который реализует несколько портов или приходит из кита,
  привязывает identity-closure (`func(u *user.Usecase) userhttp.Usecase { return u }`,
  `func(j *authnkit.HS256JWT) authnkit.JWTSigner { return j }`). `fx.As` не используется
  (archtest), `fx.Annotate` — только для тегов `name:` и `group:`.
- **Конфиги** — конфиг кита читает его `<kit>.NewConfig` (`kitConfigs` в `cmd/api`), конфиги
  самого приложения — структура `App` (`internal/platform/config`). Вне `APP_ENV=dev` старт не
  пускает небезопасное: cookie без Secure, логи без маскирования и docs — `config.Guard`, лог-mailer —
  его конструктор (`verification/infra/logmail`). `context.Context` граф не
  провайдит: свой ctx компонент получает от хука или запроса. ctx, доступный корню графа, ловит
  `TestFxGraphHasNoContext`; именованный и `fx.Private` он не видит.
- **Порядок хуков** держит httpkit: сервер слушает после OnStart всех модулей и гасится первым.
  Условие — `httpkit.Module` на верхнем уровне `fx.New`, `registerHTTP` — внутри `fx.Module`
  (`httpModule`): Invoke модулей fx выполняет раньше корневых. Порядок проверяет `TestBoot`.
- **Таймауты** — явные `fx.StartTimeout`/`fx.StopTimeout`. `StopTimeout` (30s) — общий бюджет
  хуков остановки: хуки китов укладываются в его остаток, `HTTP_SHUTDOWN_TIMEOUT` и
  `NATS_DRAIN_TIMEOUT` действуют не дольше него. Бюджет покрывает их дефолты (15s и 10s); подняли
  их — поднимите и `StopTimeout`, иначе остановка режется раньше.
- **События fx** — в лог через `logkit.FxLogger` (порядок хуков `TestBoot` читает из лога
  сервиса); паника конструктора или Invoke — ошибка старта (`fx.RecoverFromPanics`).
- **Группы** — `httpkit.security_scheme`, `healthkit.probes`, `timerkit.jobs` (через
  `timerkit.Provide`), `pgkit.migrations`; middleware и правки huma — одним значением
  `httpkit.Middlewares` и `httpkit.HumaConfigs` из `cmd/api`. Per-domain outbox
  (`events.OutboxFor`) включается внутрь доменного `fx.Module`: `fx.Private` скоупит publisher.
- Кит без потребителя в граф не подключается: его конструкторы не выполняются, конфиг не
  проверяется, а env молча ничего не меняет.

## HTTP DTO naming (huma)

huma собирает все DTO-структуры всех доменов в одну OpenAPI schema registry по имени
Go-типа. Два домена с одинаковым `ListOutput` → коллизия `ListOutputBody` → panic. Поэтому
имена publicly-named DTO в `delivery/http/dto.go` — domain-specific: `EventListOutput`,
не `ListOutput`. Inline `Body struct{...}` именуется по Outer-типу — уникальность Outer
гарантирует уникальность Body. Префикс домена, уникальность и envelope `data` экспортируемых
типов `delivery/http` (кроме `Handler` и `Usecase`) держит archtest `TestHTTPTypes`.

Операции всех доменов регистрирует одна функция — `registerHTTP` в `cmd/api/http.go`; по ней
же `cmd/api/api_test.go` собирает API без БД и брокера и проверяет: у каждой операции
`Security` (с кодами 401 сессии) или место в `publicOperations` (операция без `Security` публична
в рантайме), пара метод+путь уникальна (huma и chi молча перезаписывают маршрут), дубль
`OperationID` — паника.

## Контракт ошибок API

Ответ-ошибка — `httpkit.APIError` (problem+json, RFC 9457): машинный `code`, `request_id`, у 422 —
`errors[]` полей; 5xx — без причины, она в логе сервиса с тем же `request_id` (контракт кита).

- Правило каждой ошибки `errors.go` — одно, переменной `delivery/http`. Таблица домена
  `<домен>Errors` (`httpkit.ErrorMap`) переводит их все, и хендлер отдаёт ошибку только
  `<домен>Errors.Map(err)`: ни одна ошибка домена не уходит 500 ни на одной операции. Таблица
  операции из тех же правил объявляет при регистрации (`Declare`) коды, которые отдаёт её usecase
  (`openapi.json`). Оба правила держит archtest `TestDomainErrorTables`, сверку таблицы операции с
  usecase — ручной список теста; операция без доменных ошибок таблицы операции не заводит.
- Код — `<DOMAIN>_<WHAT>`; разный смысл — разная ошибка домена: неверный пароль входа
  (`ErrInvalidCredentials`, 401) и старый пароль смены (`ErrWrongOldPassword`, 422 поля
  `body.old_password`: сессия цела).
- Ошибка поля — 422 с `Field`: та же форма, что у валидации huma.
- 401 `UNAUTHORIZED` (нет или битый cookie) и 401 refresh — «войти заново», 401
  `AUTH_ACCESS_EXPIRED` — «обновить сессию» (`POST /api/auth/refresh`), 403 `FORBIDDEN` — «нет
  прав». Операцию под cookie регистрирует `authgate.Secured(op)`: Security и коды 401 сессии в
  OpenAPI.
- 409 — свой код конфликта у каждой причины; 429 — `Retry-After` из `RetryAfter()` ошибки в цепочке
  (`auth.LockedError`).
- Ошибка, которую usecase переводит сам (ошибка порта), тоже имеет правило: если перевод пропадёт,
  клиент получит код, а не 500.

## События и messaging

Отдельного worker-процесса нет: HTTP, outbox-drain, NATS-subscribers, background jobs —
в одном бинаре `cmd/api` через одну fx.Lifecycle.

- **Outbox (per-domain).** Событие пишется в БД в одной транзакции с основным изменением
  (`pgkit.Transactor.WithTx` → `outboxkit.Publisher.Publish(txCtx, subject, payload)`). Каждый
  домен-публикатор держит свой transactional-outbox (таблица `outbox_<domain>`: Outbox +
  Publisher + drain-job + партиции/retention + backlog-gauge'и — `outboxkit.Instance`) через
  `events.OutboxFor` (`internal/platform/events`), включённый в его `module.Module`
  (`fx.Private` скоупит publisher доменом); фабрику store'ов даёт `events.Module`
  (`pgstore.FactoryModule`). Drain батчами публикует в
  JetStream через `natskit.StreamBus` (PubAck-гарантия; id строки уходит как `Nats-Msg-Id` →
  переотправка на ретрае дедуплицируется стримом); published метятся только подтверждённые.
  Subject — литерал `<domain>.events.<event>`; источник его не экспортит. Poison-строка
  (исчерпан `OUTBOX_MAX_ATTEMPTS`) метится dead и больше не вычитывается.
- **Subscriber'ы in-domain.** Durable JetStream-консьюмеры в `<consumer>/delivery/nats/`
  рядом с `delivery/http/` — равноправные «входы» в usecase (`natskit.SubscribeOnStart` +
  `natskit.SubscribeStreamJSON`: стрим+durable, подписка в OnStart, `Drain` в OnStop; ошибка
  подписки и выключенный JetStream валят старт). Объявляют
  **локальный** payload-DTO (archtest запрещает импорт домена-источника). Регистрируются в
  `module/module.go` consumer'а.
- **Idempotency.** JetStream = at-least-once → subscriber обязан быть идемпотентен (unique-индекс
  + `ON CONFLICT`, `UPDATE … WHERE status='pending'`, idempotency-key). Не «вряд ли задвоится».
  Внешний эффект не теряется: схема помнит, выполнен ли он (`sent_at`), повтор выполненный
  пропускает, невыполненный повторяет — канон `verification.IssueEmailVerification`. Poison у
  consumer'а — `NATS_JS_MAX_DELIVER`→DLQ/Term (natskit).
- **Jobs.** Периодические задачи без события-триггера — `timerkit.Job`, регистрируются
  `timerkit.Provide` в модуле-владельце и гоняются `timerkit.Module` той же fx.Lifecycle.
  Обслуживание kit-таблиц приходит с модулями китов (`bruteforcepg.Module` — cleanup,
  `outboxkit.Instance` — drain/партиции/retention); свой job поверх kit-таблицы не пишется.
- **JetStream-стримы.** drain и subscriber'ы требуют включённого JetStream
  (`NATS_JETSTREAM_ENABLED=true`) и стримов, покрывающих subject-префиксы доменов
  (`NATS_JS_STREAM_NAMES`, напр. `user,verification` → `user.>`/`verification.>`). Без стрима
  подписки старт падает; без стрима публикатора сервис стартует, а строки его outbox не получают
  PubAck и копятся.
- **Таблицы.** `audit_log`/`bruteforce_attempts`/`outbox_<domain>` — схему держит
  golibinfra-kit (`auditpg.Migrations`, `bruteforcepg.Migrations`,
  `outboxpg.TableMigrations(Table)` из `infra/outbox/outbox.go`); источники перечислены в
  `cmd/migrate` (`sources.Kits`: отдельный бинарь графа сервиса не собирает — `TestBoot`
  сверяет список с kit-таблицами графа, группой `pgkit.migrations`), таблица версий у каждого
  своя (`schema_migrations_<source>`, приложение — `schema_migrations_app`). Default-таблица
  `outbox` не используется.

Terse-инварианты — `.claude/rules/{events-and-messaging,domain-architecture,sqlc-and-migrations}.md`.

## Граница с golibinfra

Переиспользуемое/инфраструктурное (generic util, HTTP-client, middleware, kit-подобный
примитив) — **кандидат в `golibinfra`, не в этот репо**. Не плодить полукалеки здесь.
Доменная логика — сюда; платформенный примитив — upstream (PR в golibinfra). Так шаблон
остаётся тонким слоем над общей платформой, а улучшения едут всем сервисам сразу. Нет
нужного примитива — PR в golibinfra + строка в `.claude/plans/backlog.md`; временный shim только с
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
