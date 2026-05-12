---
id: <context>.<Name>
kind: aggregate
context: <context>
name: <Name>
status: draft
relations: []
emits: []
consumes: []
---

# <Name>

> Одна-две фразы: что это в бизнесе. Без технических терминов.

## Identity

- **ID type:** <тип, напр. UUID v7 / natural key / composite>
- **Mutable:** yes/no — изменяется ли entity после создания
- **Visibility:** in-context | published — видна ли снаружи bounded context'а

## Properties

| name | type | required | brief |
|---|---|---|---|
| `ID` | <Name>ID (UUID v7) | yes | стабильный идентификатор |
| `Field1` | <Type> | yes | <бизнес-значение> |
| `Field2` | <Type> (VO) | no | <бизнес-значение> |

### `Field1`

- **Constraints:** <правила валидации, длины, форматы>
- **Default:** —
- **Source:** input | derived | computed | system
- **Description:** <чем является в бизнесе>

### `Field2`

- **Constraints:** см. VO `<Type>` ниже
- **Default:** —
- **Source:** input
- **Description:** <чем является в бизнесе>

## Methods

> Только pure-методы агрегата (без I/O, без ports). Если нужна оркестрация → use-case.

| name | brief |
|---|---|
| `New<Name>` | конструктор с валидацией всех обязательных полей |
| `<Mutator>` | <что делает> |

### `New<Name>(...) (*<Name>, error)`

- **params:** обязательные поля + опциональные через struct
- **returns:** `*<Name>` или sentinel-ошибка из `errors.go`
- **preconditions:** все Required-поля заданы и проходят валидацию
- **postconditions:** `ID` сгенерирован; `CreatedAt = UpdatedAt = now` (если применимо)
- **events emitted:** <`<Context>.<Name>Created` или _нет_>

### `<Mutator>(args) error`

- **params:** <что приходит>
- **returns:** sentinel-ошибка при нарушении preconditions
- **preconditions:** <бизнес-условия>
- **postconditions:** <что становится истинным>; `UpdatedAt = now`
- **events emitted:** <event или _нет_>

## Invariants

> Нумерованный список falsifiable утверждений. Каждый — гарантируется конкретным методом / репозиторием / БД.

1. **<Инвариант 1>.** Гарантируется <конструктором / методом / БД-constraint>.
2. **<Инвариант 2>.** Гарантируется <…>.

## Lifecycle

> Если aggregate stateless — пишется явно: `_Lifecycle: N/A (stateless aggregate)._`. Иначе — таблица переходов.

| from | event/method | to | guard | side effects |
|---|---|---|---|---|
| ∅ | `New<Name>` | exists | — | record created (+ event если есть) |
| exists | `<Mutator>` | exists | <условие> | bumped UpdatedAt (+ event если есть) |
| exists | `Delete<Name>` (use-case) | ∅ | — | — |

## Boundary

> Что внутри транзакционной границы агрегата. Что снаружи — только по ID.

- **Внутри:** root entity `<Name>`, VO `<VO1>`, slice `<container>`
- **Снаружи (только по ID):** `<other-context>.<OtherAggregate>` (если есть relation)

## Contained Entities

- root: `<Name>` (этот файл)
- _нет child entities — aggregate-of-one_

<!-- Если есть child entity сильной сложности — отдельный файл `<Child>.md` (kind: entity); -->
<!-- остальные child'ы документируем подсекцией здесь.                                  -->

## Spanning Invariants

> Инварианты, пересекающие несколько entity внутри агрегата. Если aggregate-of-one — `_нет (aggregate-of-one)_`.

- _нет (aggregate-of-one)_

## Value Objects

> Внутри-контекстные VO живут здесь inline. Cross-context платформенные VO (`PhotoRef`, etc) — в `_shared/<Name>.md`, ссылка тут одной строкой.

### `<Name>ID`

- **Properties:** `value: string` (UUID v7 как строка)
- **Invariants:** валидный UUID v7
- **Equality:** by value
- **Constructor:** `New<Name>ID(s string) (<Name>ID, error)`; `Generate<Name>ID() <Name>ID`

### `<VO1>`

- **Properties:** `<field1>: <type>`, `<field2>: <type>`
- **Invariants:** <правила>
- **Equality:** by value | structural
- **Constructor:** `New<VO1>(...) (<VO1>, error)` — валидирует и нормализует

## Events

### Emits

- [`<context>.<EventName>`](events/<EventName>.md) — <когда>

<!-- или: -->
<!-- - _нет на старте_ — мутации не публикуют outbox-события -->

### Consumes

- [`<other-context>.<EventName>`](../<other-context>/events/<EventName>.md) — <как реагируем>

<!-- или: -->
<!-- - _нет_ -->

## Relations

| target | cardinality | type | description |
|---|---|---|---|
| `<other-context>.<OtherAggregate>` | 0..1 \| 1..N | reference (by ID) \| uses (через port) | <смысл связи> |

<!-- Если связей нет: -->
<!-- - _нет_ -->

## Use Cases

> Список бизнес-операций над этим агрегатом. Детали — в `use-cases/`.

- [`<Operation>`](use-cases/<Operation>.md)
