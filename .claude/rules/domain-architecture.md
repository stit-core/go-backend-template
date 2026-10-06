---
name: domain-architecture
description: Слои и границы бизнес-доменов internal/domain/. Terse-инварианты; «как и почему» — docs/reference/architecture.md. Грузится при работе с internal/domain/.
paths:
  - "internal/domain/**"
---

# Архитектура домена

Полное описание слоёв, обязательности файлов, naming-канонов, ports/adapters, событий —
`docs/reference/architecture.md`. Здесь — что обязан соблюдать при правке `internal/domain/`.

## Инварианты

Пометка archtest, имя теста или линтера — инвариант держит машина (`make check`; на archtest
`//nolint` не действует); без пометки — правило и `/code-review`.

- **Раскладка по слоям.** `entity.go` (типы, без I/O) · `dto.go` (`<Op>Input/Output`) ·
  `errors.go` (все sentinel и error-типы домена — только здесь) · `policy.go` (authorization
  + чистые правила, без I/O) · `usecase.go` (логика, зависимости через порты) · `trusted.go`
  (операции для других доменов без policy) · `events.go` (payload + unexported subject-const) ·
  `metrics.go` (`biz_<domain>_*` под алерт, nil-safe) · `delivery/{http,nats}/` · `infra/postgres/` ·
  `infra/<kit>/` (адаптер порта к киту, внешнему каналу или клиенту из `pkg/`) · `module/`. Файл существует
  только вместе со своим содержимым (таблица обязательности — architecture.md); мёртвые
  типы/файлы удаляются.
- **Логика — в usecase и policy, края тонкие.** Хендлер: разбор входа → вызов usecase →
  `<домен>Errors.Map` и view, без ветвлений бизнес-правил и без repo или кита мимо usecase.
  Репозиторий и SQL — CRUD, фильтры и условия атомарности (`WHERE version = $2`,
  `used_at IS NULL`): кому что видно, решает usecase и передаёт параметром, `CASE` по ролям в SQL
  нет. Адаптер `module/` — только перевод типов и ошибок.
- **Право — policy в usecase.** Метод usecase за операцией под `Security` спрашивает policy
  (`Can<Verb>(ctx, caller, …)`) до чтения и записи — и по id, и для списка; исключение — чтение
  самого вызывающего (`Me`). Без policy — публичные операции (решает секрет запроса: пароль,
  токен), входы событий и `Trusted`.
- **Cross-domain — только порты + адаптеры.** archtest (`internal/archtest`) держит импорты
  любого каталога `internal/domain/*`: чужой домен — только из `module/` и только корневым
  пакетом (и из `*_test.go`); `delivery` не импортирует `infra`; `infra/postgres/db` — только
  `infra/postgres`; корень домена — ни своих подпакетов, ни транспорта, драйверов и I/O ОС, в том
  числе через пакет `pkg/`; `infra/outbox` — только свой `module/` и `cmd/migrate`. Порт объявляешь
  в домене-потребителе (`usecase.go`), адаптер — в `<consumer>/module/adapters.go` поверх `Trusted` поставщика
  (`trusted.go`: без policy, ошибки — только `TrustedErrors`, берут только адаптеры `module/` —
  archtest). DTO в порте — локальные; порт возвращает **sentinel'ы потребителя** (адаптер мапит каждую
  ошибку `TrustedErrors`, тест — по этому списку на каждом методе) и **вычисленные свойства** — методы
  сущности поставщика (`CanLogin()`): в адаптере нет условий по полям поставщика. Identity-closure
  (`return uc`) — только внутри домена (delivery-порты своего usecase); порт между доменами — типовой
  адаптер, тип и конструктор в `adapters.go` (archtest). Время во всём домене — инжектированный
  `clockkit.Clock`, настенных часов нет (archtest `TestNoWallClockInDomains`).
- **DI — fx** (канон — `architecture.md` § fx). Модуль домена — `fx.Module` с именем-путём пакета
  в `module/module.go`, подключён в `cmd/api` (`TestDomainsAreWired`). Порт в графе — тип
  результата провайдера, тип с несколькими портами — identity-closure над своим типом; `fx.As`
  нет (archtest `TestNoFxAs`), `fx.Annotate` — только теги `name:`/`group:`. Outbox `Providers` —
  внутри модуля домена (`fx.Private`). `context.Context` из графа не берут.
- **HTTP DTO — domain-specific имена.** huma собирает все DTO в одну OpenAPI registry по
  имени типа; `ListOutput` в двух доменах → panic. Префиксуй: `UserListOutput`, не `ListOutput`.
  Операция либо под `Security` (`authgate.Secured(op)`), либо открыта пометкой `httpkit.Public(op)`:
  без того и другого сервис не стартует (httpkit); регистрация — в `registerHTTP`
  (`cmd/api/http.go`). Ошибки — таблица домена `<домен>Errors`: правило на каждую ошибку `errors.go`,
  хендлер отдаёт `<домен>Errors.Map(err)`, таблица операции из тех же правил — `Declare` при
  регистрации (оба — archtest `TestDomainErrorTables`); ошибка поля — 422 с `Field`.
  Каждая ручка — операция huma в контракте `openapi.json`: сырой маршрут chi (`Router()`) и
  `Hidden: true` в контракт не попадают, `TestBoot` их не пускает. Envelope — `data`;
  префикс, уникальность имён и envelope держит archtest `TestHTTPTypes`.
  OperationID — lowerCamel `<verb><Noun>`, глобально уникален.
- **I/O — это use-case, не метод агрегата.** Нужен repo/сеть/время → метод `*Usecase`.
  Чистое правило/константа правила → `policy.go` домена, общее для доменов (правило пароля,
  `ForbiddenError`) — `pkg/policy`; не inline в usecase. Исход метрики — `outcome.Result`, ID,
  которые создаёт код, — `idkit.NewV7()`, страница списка — `paging.Page`.
- **Usecase не знает транспорта.** `httpkit`/`natskit`/huma в usecase запрещены; ctx-геттеры
  (`IPFrom`, `RequestIDFrom`) — только `internal/platform/identity`. Named returns не используем
  (линтер `nonamedreturns`).
  Метрика — только под алерт; у метода с метрикой исход пишется ровно раз на любом пути, запись
  проверяет тест. Security-операции пишут audit из usecase (`uc.audit`, в записи — ID, не адрес).
  Состояния между запросами в `*Usecase` и переменных пакета нет — оно в БД или ките.
- **Логи — `logkit.Logger`.** Не `fmt.Print*` / `log.*` / `print(ln)` / `slog.*` и не запись в
  `os.Stdout`/`os.Stderr` напрямую (forbidigo). В лог и payload события — ID и факты: токенов,
  паролей, секретов и PII (email, телефон, имя) там нет — лог и стрим читают многие и хранят
  долго; данные потребитель берёт по ID портом или API поставщика.
- Файл > ~400 строк → разбей по потокам (`usecase_<flow>.go`, `handler_<op>.go`), порты и
  конструкторы остаются в `usecase.go`.
- Pure-методы агрегата покрыты `*_test.go`; интерфейсы из `usecase.go` и `delivery/*` →
  `.mockery.yml` → `make gen`, кроме policy-порта `Authorizer`: тесты берут настоящую policy.

Outbox / subscribers / jobs — `.claude/rules/events-and-messaging.md`. Миграции / sqlc —
`.claude/rules/sqlc-and-migrations.md`. Переиспользуемые примитивы → golibinfra (CLAUDE.md § Стиль кода).
