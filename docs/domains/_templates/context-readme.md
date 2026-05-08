# <Context> — Bounded Context

> 2-3 предложения: какую часть бизнеса этот контекст представляет. Чем не занимается (граница).

## Purpose

- _что делаем_
- _чего не делаем_

## Aggregates

| aggregate | role |
|---|---|
| [`<Name>`](<Name>.md) | <бизнес-роль> |

## Use cases

| operation | actor | brief |
|---|---|---|
| [`Register<Name>`](use-cases/Register<Name>.md) | anonymous | регистрация нового <Name> |

## Events

### Published (outgoing)

| event | trigger | consumers |
|---|---|---|
| [`<Name>Registered`](events/<Name>Registered.md) | после `Register<Name>` | verification, audit |

### Subscribed (incoming)

| event | source | reaction |
|---|---|---|
| [`verification.EmailVerified`](../verification/events/EmailVerified.md) | verification | выставляет EmailVerified=true |

## Cross-context dependencies

| direction | other context | nature |
|---|---|---|
| outbound | `verification` | port `EmailVerificationIssuer` для запуска подтверждения |
| inbound | `auth` | читает `<Name>` через port `UserReader` |

## Ubiquitous Language

> Бизнес-термины этого контекста ↔ имена в коде. Также синонимы и **что НЕ называть как**.

| Бизнес-термин | В коде | Синонимы | Не путать с |
|---|---|---|---|
| Пользователь | `user.User` | account, member | `auth.Session` (это сессия, не пользователь) |
| Заблокирован | `user.StatusBlocked` | banned (избегать) | удалён (`StatusDeleted`) |
| E-mail | `user.Email` (VO) | login (избегать) | externalEmail (для интеграций) |
| Регистрация | `user.RegisterUser` (use-case) | sign up | onboarding (это шире) |

## Glossary notes

> Дополнения, неоднозначности, исторические причины именований. Все вопросы "почему так названо" — сюда.

- _пусто_

## Architecture notes

> Архитектурные решения специфичные для этого контекста. Например: aggregate-of-one, нет child entities; outbox обязателен для всех мутаций.

- _пусто_
