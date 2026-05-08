---
id: <context>.<PolicyName>
kind: policy
context: <context>
name: <PolicyName>
status: draft
relations: []
emits: []
consumes: []
---

# <PolicyName>

> Одна фраза: какое бизнес-решение принимает эта policy. Без I/O, без сторонних эффектов.

## Kind of decision

- **Type:** authorization | validation | rate-limit | rule
- **Pure:** yes (без БД/сети/времени) | impure (зависит от Clock/random)

## Inputs

| field | type | description |
|---|---|---|
| `Subject` | identity.Principal | актор, для которого принимается решение |
| `Resource` | <type> | объект решения |
| `Action` | string | действие (`read`, `write`, ...) |

## Rules

> Нумерованный список бизнес-правил. Каждое — falsifiable утверждение.

1. Admin может всё.
2. Client может читать только свой ресурс (`subject.UserID == resource.OwnerID`).
3. Заблокированный аккаунт (`subject.Status == blocked`) не может ничего, кроме unblock-операций.

## Outcomes

| outcome | when | error |
|---|---|---|
| `allow` | хотя бы одно правило разрешает и ни одно не запрещает | — |
| `deny` | подходит правило явного запрета | `ErrForbidden` |

## Composition

> Как эта policy сочетается с другими (если есть). Например: PasswordPolicy + LoginPolicy перед `auth.Login`.

- — самостоятельна

## Used by

- [`<context>.<UseCase>`](../use-cases/<UseCase>.md) — на шаге Authorize/Validate
