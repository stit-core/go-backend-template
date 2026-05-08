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
| `Email` | Email (VO) | yes | e-mail пользователя |
| `Status` | UserStatus (enum) | yes | текущее состояние |

### `Email`

- **Constraints:** валидный RFC 5322 формат, ≤254 символов
- **Default:** —
- **Source:** input
- **Description:** уникальный идентификатор для логина

### `Status`

- **Constraints:** один из enum'а UserStatus
- **Default:** `Active` при создании
- **Source:** system (управляется методами)
- **Description:** состояние жизненного цикла, см. Lifecycle

<!-- enum'ы выносятся отдельной секцией -->
## Enums

### `UserStatus`

| value | meaning |
|---|---|
| `active` | может выполнять операции |
| `blocked` | временно заблокирован, восстановим |
| `deleted` | удалён, неоперабелен |

## Methods

> Только pure-методы агрегата (без I/O, без ports). Если нужна оркестрация → use-case.

| name | brief |
|---|---|
| `New<Name>` | конструктор с валидацией |
| `Block` | перевод в `blocked` |

### `New<Name>(email Email, ...) (*<Name>, error)`

- **params:**
  - `email` — VO Email, обязателен
- **returns:** `*<Name>`, либо `ErrInvalidEmail`/`ErrInvalidRole`
- **preconditions:** email валиден; role из допустимых значений
- **postconditions:** Status=Active; Version=1; CreatedAt=now
- **events emitted:** `<Context>.<Name>Registered`

### `Block(reason string) error`

- **params:** `reason` — нетехническая причина
- **returns:** `error` — `ErrAlreadyBlocked` если уже blocked
- **preconditions:** Status ∈ {active}
- **postconditions:** Status=blocked
- **events emitted:** `<Context>.<Name>Blocked`

## Invariants

1. **<Name>.Email уникален в контексте.** Проверяется в use-case (нужен репо), не в pure методе.
2. **PasswordHash непустой если Status=active.** Проверяется в `New<Name>` и в методах смены пароля.
3. **Version монотонно растёт.** Гарантируется репозиторием через optimistic lock.

## Lifecycle

| from | event/method | to | guard | side effects |
|---|---|---|---|---|
| ∅ | `New<Name>` | `active` | — | emit `<Name>Registered` |
| `active` | `Block` | `blocked` | reason ≠ "" | emit `<Name>Blocked` |
| `blocked` | `Unblock` | `active` | — | emit `<Name>Unblocked` |
| `active` | `Delete` | `deleted` | — | emit `<Name>Deleted` |
| `blocked` | `Delete` | `deleted` | — | emit `<Name>Deleted` |

<!-- если stateless: одной строкой:  -->
<!-- _Lifecycle: N/A (stateless aggregate)._ -->

## Boundary

> Что внутри транзакционной границы агрегата. Что снаружи — только по ID.

- **Внутри:** root entity `<Name>`, VO Email, VO PasswordHash
- **Снаружи (только по ID):** Session (другой агрегат, другой контекст)

## Contained Entities

- root: `<Name>` (этот файл)
- _нет child entities — aggregate-of-one_

<!-- если есть child:  -->
<!-- - child: <ChildName> — см. <ChildName>.md, идентифицируется внутри <Name> -->

## Spanning Invariants

> Инварианты, пересекающие несколько entity внутри агрегата. Если aggregate-of-one — совпадают с Invariants выше или пусто.

- _нет (aggregate-of-one)_

## Value Objects

### `Email`

- **Properties:** `value: string`
- **Invariants:** RFC 5322; lowercase canonical form; len ≤254
- **Equality:** by value
- **Constructor:** `NewEmail(s string) (Email, error)` — нормализует и валидирует

### `PasswordHash`

- **Properties:** `value: string`
- **Invariants:** непустая строка; формат argon2id-encoded
- **Equality:** by value
- **Constructor:** `NewPasswordHash(raw string) (PasswordHash, error)`

## Events

### Emits

- [`<context>.<Name>Registered`](events/<Name>Registered.md) — после успешной регистрации
- [`<context>.<Name>Blocked`](events/<Name>Blocked.md) — после блокировки
- [`<context>.<Name>Unblocked`](events/<Name>Unblocked.md)
- [`<context>.<Name>Deleted`](events/<Name>Deleted.md)

### Consumes

- [`verification.EmailVerified`](../verification/events/EmailVerified.md) — выставляет EmailVerified=true

## Relations

| target | cardinality | type | description |
|---|---|---|---|
| `auth.Session` | 1..N | reference (by ID) | у пользователя могут быть активные сессии |
| `verification.Code` | 0..1 | reference (by ID) | в момент регистрации |

## Use Cases

> Список бизнес-операций над этим агрегатом. Детали — в use-cases/.

- [`Register<Name>`](use-cases/Register<Name>.md)
- [`Block<Name>`](use-cases/Block<Name>.md)
- [`Change<Name>Email`](use-cases/Change<Name>Email.md)
