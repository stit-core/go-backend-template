# DDD-артефакт → Go-код

Маппинг секций доменных артефактов на конкретные файлы и конструкции в `internal/domain/<context>/`.
Если у тебя есть сомнение «куда это положить в коде» — смотри сюда.

Артефакты доменов живут **не в этом репо**, а в соседнем strategy-репо орга (`<org>/strategy`,
форк `business-strategy-template`) — design-time SoT бизнес-стратегии, где продукт и его
DDD-домены опциональны (заводятся, только когда стратегия рождает продукт):
`../strategy/docs/projects/<slug>/domains/<context>/`, где `<slug>` — `project-slug` из
`docs/stack/service.md`. Этот файл — **мост** от их формы к Go-коду именно этого бэкенда; его
потребляет `scaffold` (вариант domain), разворачивая код по stable-артефактам.

## Bounded context

`../strategy/docs/projects/<slug>/domains/<context>/` ↔ `internal/domain/<context>/` — один Go-пакет, один FX-модуль (`module/module.go`).

`../strategy/docs/projects/<slug>/domains/_shared/<Name>.md` (kind=vo) ↔ `internal/platform/shared/` — платформенные cross-context VO в одном пакете. Не зависят от бизнес-доменов; бизнес-домены импортят их напрямую.

## Aggregate / Entity

| Артефакт | Go-код |
|---|---|
| `kind: aggregate` файл | `entity.go` — корневой struct |
| Root entity name | имя struct (`User`, `Session`) |
| Properties → таблица | поля struct'а |
| Property `Source: input` | задаётся в конструкторе/setter'е |
| Property `Source: derived` | метод-геттер, не хранимое поле |
| Property `Source: computed` | метод, может кэшироваться |
| Property `Source: system` | проставляется репозиторием/БД (`CreatedAt`, `UpdatedAt`) |
| Property `Required: yes` | конструктор `New<Name>` падает если zero-value/nil |
| Property `Constraints` | проверка в конструкторе → возврат sentinel-ошибки из `errors.go` |
| Methods (pure) | методы на `*<Name>` без I/O |
| Method `preconditions` | `if !ok { return Err* }` в начале метода |
| Method `postconditions` | гарантируется state'ом после return; покрыто entity-test'ом |
| Method `events emitted` | возвращается из метода или копится в `aggregate.uncommittedEvents` (если используется); use-case кладёт в outbox |
| Invariants (локальные) | проверяются в методах агрегата + покрыты `entity_test.go` (+ fuzz при наличии валидаций строк) |
| Lifecycle table | поле-enum (`Status`) + методы перехода (`Block`, `Unblock`, `Delete`); switch по from-state в начале метода |
| Lifecycle `guard` | условие перед сменой состояния → возврат ошибки если не выполнено |
| Lifecycle `side effects` | event(ы), эмитируемые методом перехода |
| Boundary | конкретный набор полей и контейнеров внутри `entity.go`; репозиторий загружает/сохраняет всё это атомарно |
| Spanning Invariants | методы на root, имеющие доступ ко всем child'ам |
| Contained Entities | дополнительные struct'ы в `entity.go` или `entity_<child>.go`; не имеют публичного репозитория |
| Identity → ID type | поле `ID` (рекомендуется `type <Name>ID string` для type-safety) |
| Visibility `published` | разрешено упоминать в DTO других контекстов через port-адаптер |

## Value Objects

| Артефакт | Go-код |
|---|---|
| VO subsection в aggregate | typed-обёртка в `entity.go`: `type Email string` или `type Money struct{...}` |
| VO `Constructor: New<VO>` | функция `NewEmail(s) (Email, error)` рядом со struct'ом |
| VO `Invariants` | проверки в конструкторе |
| VO `Equality: by value` | сравнение `==` (только если все поля comparable) |
| VO `Equality: structural`/`custom` | метод `func (a VO) Equal(b VO) bool` |

> **Не обязательно типизировать каждый VO.** Голый `string` для Email допустим если валидация делается в конструкторе агрегата. Типизация — рекомендация для часто употребимых VO.

## Shared Value Objects (`_shared/`, kind: vo)

Для платформенных VO, которые структурно идентичны в ≥2 контекстах и не несут доменной семантики (`PhotoRef`, etc).

| Артефакт | Go-код |
|---|---|
| `kind: vo` файл (`_shared/<Name>.md`) | `internal/platform/shared/<name>.go` (`type <Name> struct{...}`) |
| Properties → таблица | поля struct'а |
| Constructor секция | `New<Name>(...) (<Name>, error)` рядом со struct'ом |
| Invariants | проверки в конструкторе → возврат sentinel-ошибки |
| Equality | `==` (если все поля comparable) или метод `Equal` |
| Consumed by | импорты из бизнес-доменов; **не** наоборот — `_shared/` ничего не импортит из `internal/domain/` |
| Errors секция | sentinel в `internal/platform/shared/errors.go`; домены оборачивают через `%w` |

`_shared/` — узкий список (большинство VO остаются inline в aggregate-файле своего контекста). Доменные VO (`Phone`, `Email`, `OrderTotal`) — не сюда: их семантика принадлежит контексту, а не платформе.

## Events

| Артефакт | Go-код |
|---|---|
| `kind: event` файл | struct в `events.go` (имя файла = имя struct'а) |
| Payload table | поля struct'а |
| Trigger `source aggregate` | use-case этого aggregate'а кладёт событие в outbox |
| Trigger `triggering use-case` | конкретный метод `Usecase` |
| Consumers | `internal/domain/<consumer>/delivery/nats/handler.go` — handler рядом с `delivery/http/`, на равных правах. Регистрируется fx того же `cmd/api`. |
| Delivery `idempotency key` | поле в payload; subscriber хранит уже обработанные ID |
| Delivery `at-least-once` | `outboxkit.Publisher` гарантирует, subscriber обязан быть идемпотентен |
| Delivery `ordering: per-aggregate` | в текущем `natskit`/core-NATS upgrade'а до partition'а **нет**: ordering держится только тем, что outbox-drain — single-instance worker, читает строки в порядке `id`. Если subscriber'у важен per-aggregate order — он сам сериализует обработку по `<Aggregate>ID` (например, очередь in-memory keyed by ID). |

## Use Case

| Артефакт | Go-код |
|---|---|
| `kind: use-case` файл | метод на `*Usecase` в `usecase.go` |
| Actor | проверяется на handler-уровне (`delivery/http`) или в первом шаге метода |
| Inputs | DTO в `dto.go` (доменный) и `delivery/http/dto.go` (huma-теги) |
| Outputs | `dto.go` или domain entity (mapping в `delivery/http/view.go`) |
| Dependencies (ports) | интерфейсы рядом в `usecase.go`; реализации — в `module/adapters.go` чужого домена или `infra/postgres/repository.go` |
| Steps | последовательность вызовов внутри метода `Usecase` |
| Called aggregate methods | вызовы методов на entity внутри метода Usecase |
| Events emitted | `outboxkit.Publisher` + struct из `events.go`, в одной транзакции с persist |
| Errors | sentinel-ошибки из `errors.go`; маппинг на HTTP — в `delivery/http/handler.go` (errMapping) |
| Side effects beyond aggregate | external clients (email, billing) — через отдельные ports, инжектируются |

## Policy

| Артефакт | Go-код |
|---|---|
| `kind: policy` файл | функции/методы в `policy.go` (без I/O) |
| Inputs | параметры функции |
| Rules | реализуются как набор условий → возвращают `allow`/`deny` |
| Outcomes | `nil` для allow, `ErrForbidden`/специфичная ошибка для deny |
| Used by | вызывается из `Usecase.<Operation>` на шаге Authorize/Validate |
| Pure | `policy.go` не должен импортить `infra/` или `delivery/` |

## Context README

| Артефакт | Go-код |
|---|---|
| Purpose | пакет-doc в `<context>/doc.go` (опционально) |
| Ubiquitous Language | проверяется на код-ревью; имена в коде должны соответствовать |
| Cross-context dependencies (outbound) | порт в `<context>/usecase.go` + адаптер в чужом `module/adapters.go` |
| Cross-context dependencies (inbound) | другой контекст имеет порт; этот контекст реализует через адаптер |
| Architecture notes | живёт в `.claude/rules/<context>-*.md` если требует rule-уровня |

## Cross-context relations

| Artifact `relations[].type` | Что это в коде |
|---|---|
| `composition` | внутри одного контекста; struct содержит другой struct |
| `reference (by ID)` | хранится только ID; для чтения — port + adapter |
| `uses` | вызов через port (cross-context use-case или policy) |

depguard в `.golangci.yml` блокирует прямые импорты между бизнес-доменами; всё cross-context идёт через `module/adapters.go`.

## Migrations

| Артефакт | Go-код |
|---|---|
| Property → колонка | `migrations/NNNN_<context>_<change>.up.sql` |
| Property `Constraints` | `CHECK`-constraint или валидируется только в коде (если бизнес-уровень) |
| Lifecycle enum | `CHECK (status IN ('active', 'blocked', 'deleted'))` или отдельная enum-таблица |
| Identity ID type | `id UUID PRIMARY KEY` или `TEXT` для composite |
| Outbox-эмит | таблица `outbox` — **не в `migrations/` этого репо**; схему держит `golibinfra/outboxkit/pgstore` (`MigrationsFS`), `cmd/migrate/main.go` накатывает её рядом с миграциями приложения. То же про `audit_log` (`auditkit/pgstore`) и `bruteforce_attempts` (`bruteforcekit/pgstore`). |

## Что НЕ маппится 1:1

- **Domain Service** — в этом репо не вводим. Любая I/O-операция → use-case. Любое чистое правило → policy. Если возникает реальная необходимость (редко) — отдельный файл рядом, но это будет архитектурное решение, требующее обсуждения.
- **Read models / projections** — не моделируем как entity. Если в проекте возникнет необходимость — вводится отдельный kind `query` (архитектурное решение, требует обсуждения с владельцем).
- **CQRS write/read split** — не используется; один Usecase на контекст.

## Ступоры — куда положить?

> Если артефакт описывает что-то, для чего непонятно место:

| Признак | Куда |
|---|---|
| Метод нуждается в БД/сети/времени | use-case, не aggregate-method |
| Логика работает с двумя aggregates | use-case (orchestration) |
| Решение без эффектов | policy |
| Перевод состояния агрегата | aggregate.method (через Lifecycle row) |
| Выставление `CreatedAt` | репозиторий, не конструктор |
| Уникальность по полю | use-case (нужен `repo.GetBy*`), не aggregate-invariant |
| Нотификация (email, webhook) | port в use-case + адаптер в `module/` |
| Идемпотентность subscriber'а | поле события `EventID` + хранилище обработанных ID |
