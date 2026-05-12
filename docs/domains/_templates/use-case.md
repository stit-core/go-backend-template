---
id: <context>.<Operation>
kind: use-case
context: <context>
name: <Operation>
status: draft
relations: [<context>.<Aggregate>]
emits: []
consumes: []
---

# <Operation>

> Одна фраза описывающая бизнес-операцию. Что хочет сделать актор.

## Actor

- **Role:** anonymous | admin | client | system
- **Channel:** HTTP API (`<METHOD> <path>`) | NATS subscriber | scheduled job | internal

## Inputs

| field | type | required | description |
|---|---|---|---|
| `<Field>` | <type> | yes | <смысл> |

## Outputs

- **On success:** `<Aggregate>DTO` (или void для DELETE-операций)
- **On failure:** см. Errors

## Dependencies

| port | purpose | adapter |
|---|---|---|
| `<Aggregate>Repository` | persistence корневого агрегата | postgres (pgx + sqlc) |
| `Clock` | детерминированное время | `golibinfra/clock` |
| `<OtherDomain>Reader` | cross-domain port (если нужны данные из другого контекста) | adapter в `module/adapters.go` чужого домена |

> **Авторизация** — на уровне HTTP-middleware (`platform/authgate`), не в use-case'е. Use-case не имеет `Authorizer`-порта и не возвращает `ErrForbidden`, если репо-конвенция «есть валидная сессия → can do everything». Если в конкретном домене нужны fine-grained роли — заводи policy в `<ctx>/policies/` и **явно** добавь её здесь.

## Steps

> Нумерованный список шагов. Каждый — observable действие (Validate / Load / Construct / Apply / Persist / Emit / Return).

1. **Validate inputs:** базовая валидация формата (детальная — в VO-конструкторах)
2. **Load:** `repo.GetByID(ctx, id)` → `ErrNotFound` если нет (для Update/Delete; не нужно для Create)
3. **Construct aggregate / Apply method:** `<Aggregate>.New<Name>(...)` или `agg.<Mutator>(...)`
4. **Persist:** `repo.Insert(ctx, agg)` / `repo.Update(ctx, agg)` — в одной транзакции с outbox-записью (если есть events)
5. **Emit:** `<Event>` через `outboxkit.Publisher` (если есть events)
6. **Return DTO**

## Called aggregate methods

- [`<Aggregate>.New<Name>`](../<Aggregate>.md#new<name>)
- [`<Aggregate>.<Mutator>`](../<Aggregate>.md#<mutator>)

<!-- Если методов агрегата нет (например DELETE напрямую в репозитории): -->
<!-- - _нет_ (удаление выполняется репозиторием напрямую) -->

## Events emitted

- [`<context>.<EventName>`](../events/<EventName>.md)

<!-- Или: -->
<!-- - _нет_ -->

## Errors

| sentinel | when | HTTP status |
|---|---|---|
| `ErrInvalid<Field>` | валидация поля упала | 400 |
| `ErrNotFound` | агрегат не найден (для Update/Delete) | 404 |
| `ErrAlreadyExists` | уникальность нарушена | 409 |

## Side effects beyond the aggregate

> Что меняется кроме самого агрегата. Outbox-события — **нет** (они часть транзакции). Sending real email, billing, audit log, FK-каскады в БД — да, перечислить.

- _нет_

<!-- Пример при наличии side effects: -->
<!-- - Через FK `ON DELETE SET NULL` БД обнуляет `<other_table>.<this>_id` (если был связан). -->
