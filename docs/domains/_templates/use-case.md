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

- **Role:** anonymous | client | admin | system
- **Channel:** HTTP API | NATS subscriber | scheduled job | internal

## Inputs

| field | type | required | description |
|---|---|---|---|
| `Email` | string | yes | e-mail для регистрации |
| `Password` | string | yes | сырое значение, хешируется внутри |

## Outputs

- **On success:** `<Aggregate>` (или его DTO-проекция)
- **On failure:** см. Errors

## Dependencies

| port | purpose | adapter |
|---|---|---|
| `<Aggregate>Repository` | persistence корневого агрегата | postgres |
| `Authorizer` | проверка прав актора | platform/authgate |
| `EmailVerificationIssuer` | cross-domain port в verification | adapter в `module/adapters.go` |
| `Clock` | детерминируемое время | golibinfra/clock |

## Steps

1. **Authorize:** `Authorizer.Can(actor, <action>, <resource>)` → `ErrForbidden` если нет
2. **Validate inputs:** базовая валидация формата (детальная — в VO-конструкторах)
3. **Load:** `repo.GetByEmail(email)` → если найден и не deleted → `ErrAlreadyExists`
4. **Construct aggregate:** `<Aggregate>.New(...)` → может вернуть domain-error
5. **Apply policy:** [`PasswordPolicy.Check(raw)`](../policies/PasswordPolicy.md)
6. **Persist:** `repo.Insert(ctx, agg)` — в одной транзакции с outbox-записью
7. **Emit:** `<Aggregate>Registered` через outboxkit

## Called aggregate methods

- [`<Aggregate>.New<Name>`](../<Aggregate>.md#newname)
- [`<Aggregate>.SetPasswordHash`](../<Aggregate>.md#setpasswordhash)

## Events emitted

- [`<context>.<Aggregate>Registered`](../events/<Aggregate>Registered.md)

## Errors

| sentinel | when | HTTP status |
|---|---|---|
| `ErrInvalidEmail` | формат email невалиден | 400 |
| `ErrAlreadyExists` | пользователь с этим email уже есть | 409 |
| `ErrWeakPassword` | пароль не прошёл policy | 400 |
| `ErrForbidden` | actor не имеет прав | 403 |

## Side effects beyond the aggregate

> Что меняется кроме самого агрегата. Outbox события — нет (они часть транзакции). Sending real email, billing, audit log — да, перечислить.

- — нет
