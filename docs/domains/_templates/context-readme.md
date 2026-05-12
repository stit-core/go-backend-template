# <Context> — Bounded Context

> 2-3 предложения: какую часть бизнеса этот контекст представляет. Чем не занимается (граница).

## Purpose

- _что делаем_: <бизнес-обязательства контекста>
- _чего не делаем_: <границы — чем занимается другой контекст или платформа>

## Aggregates

| aggregate | role |
|---|---|
| [`<Name>`](<Name>.md) | <бизнес-роль> |

## Use cases

| operation | actor | brief |
|---|---|---|
| [`<Operation>`](use-cases/<Operation>.md) | <actor> | <что делает> |

Read use-case'ы (`Get<Name>`, `List<Name>s`) — добавим при скаффолдинге HTTP-endpoint'ов, если нужны.

## Events

### Published (outgoing)

| event | trigger | consumers |
|---|---|---|
| [`<Name>`](events/<Name>.md) | после `<UseCase>` | <consumer-context> |

<!-- Если событий нет: -->
<!-- | _нет на старте_ | — | — | -->

### Subscribed (incoming)

| event | source | reaction |
|---|---|---|
| [`<other-context>.<Name>`](../<other-context>/events/<Name>.md) | `<other-context>` | <что делаем в ответ> |

<!-- Если нет: -->
<!-- | _нет_ | — | — | -->

## Cross-context dependencies

| direction | other context | nature |
|---|---|---|
| outbound | `<other-context>` | port `<OtherDomain>Reader` для чтения данных кросс-контекстно |
| inbound | `iam` / `platform/authgate` | проверка валидной admin-сессии через HTTP-middleware |

## Ubiquitous Language

> Бизнес-термины этого контекста ↔ имена в коде. Также синонимы и **что НЕ называть как**.

| Бизнес-термин | В коде | Синонимы | Не путать с |
|---|---|---|---|
| <термин 1> | `<context>.<Type>` | <синонимы; «избегать» помечать явно> | <ложный друг> |
| <термин 2> | `<context>.<Type>` | — | — |

## Glossary notes

> Дополнения, неоднозначности, исторические причины именований. Все вопросы «почему так названо» — сюда.

- _пусто_

## Architecture notes

> Архитектурные решения, специфичные для этого контекста. Что нестандартно? Что отличается от других доменов? Где живут cross-aggregate чтения? Семантика PUT/PATCH? Hard- vs soft-delete? Auth — middleware или policy?

- _пусто_
