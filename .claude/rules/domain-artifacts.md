---
name: domain-artifacts
description: DDD-артефакты в docs/domains/ — конвенции, kinds, frontmatter, валидатор. Грузится при работе с docs/domains/.
paths:
  - "docs/domains/**"
---

# Domain artifacts

DDD-артефакты — **первичный source of truth** проектирования. Код в `internal/domain/` пишется по ним, не наоборот.

См. `docs/domains/README.md` (структура, workflow) и `docs/domains/MAPPING.md` (DDD ↔ Go).

## Kinds (frontmatter `kind:`)

- `aggregate` — бизнес-сущность с консистентной границей. Root entity + inline-VO внутри. Один файл = один aggregate.
- `entity` — child entity сильной сложности (опционально, обычно описывается секцией внутри aggregate).
- `event` — domain event для outbox.
- `use-case` — бизнес-операция (orchestration). Маппится на метод `*Usecase`.
- `policy` — pure правило (authz/validation), без I/O.
- `vo` — cross-context платформенный VO. **Только для `_shared/`** (например `_shared/PhotoRef.md`). Внутри-контекстные VO остаются inline в aggregate-файле.

## Frontmatter — обязательные поля

```yaml
id: <context>.<Name>
kind: aggregate
context: <context>
name: <Name>
status: draft           # draft → stable
relations: []
emits: []
consumes: []
```

`id` стабильный — другие артефакты ссылаются по нему.

## Обязательные H2-секции

| kind | секции |
|---|---|
| aggregate | Identity, Properties, Methods, Invariants, Lifecycle, Boundary, Contained Entities, Spanning Invariants, Value Objects, Events, Relations, Use Cases |
| entity | Identity, Properties, Methods, Invariants, Lifecycle, Relations |
| event | Trigger, Payload, Consumers, Delivery semantics |
| use-case | Actor, Inputs, Outputs, Dependencies, Steps, Called aggregate methods, Events emitted, Errors |
| policy | Kind of decision, Inputs, Rules, Outcomes, Used by |
| vo | Properties, Invariants, Constructor, Consumed by |

Stateless / aggregate-of-one / пустые consumers — пишутся явно (`N/A`, `_нет_`), не оставляются пустыми.

## Workflow

1. Скопировать из `docs/domains/_templates/` в `docs/domains/<ctx>/`. Для cross-context VO — `_templates/vo-shared.md` → `docs/domains/_shared/<Name>.md`.
2. Заполнить frontmatter + минимум (Properties с именами+типами, Methods с именами).
3. `/domain-deep-dive <ctx>` — углубление по волнам inside-out.
4. `scripts/tools/validate-domain-doc.sh docs/domains/<ctx>` — зелёный → промоушн `status: stable`.
5. `scaffold` (вариант domain/endpoint/event) — только при зелёном валидаторе (gate — см. § Валидатор).

## Делать

- Один Aggregate = один файл. Внутри-контекстные VO inline в aggregate-файле; cross-context — только в `_shared/<Name>.md` (`kind: vo`).
- Pure-методы агрегата → секция Methods. Операции с I/O → use-case.
- Cross-context relation — только `reference (by ID)` или `uses (через port)`. Не `composition`.
- При status=stable все обязательные секции должны быть непустыми.
- Идемпотентность субскрайберов — `EventID` в payload + хранение обработанных ID.

## Не делать

- ❌ Отдельный файл на **внутри-контекстную** VO. Inline в aggregate-файле. (Исключение — cross-context VO в `_shared/`.)
- ❌ Класть доменный VO (`Phone`, `Email`, `OrderTotal`) в `_shared/`. `_shared/` — только платформенные типы без доменной семантики.
- ❌ Описывать I/O-операцию (с repo/ports) как метод агрегата.
- ❌ Spanning invariant между двумя aggregate'ами.
- ❌ Промоутить `status: stable` руками без зелёного валидатора.
- ❌ Запускать `scaffold` (вариант domain) пока есть `status: draft` в `docs/domains/<ctx>/`.

## Валидатор

`scripts/tools/validate-domain-doc.sh [path]` — детерминированный, без внешних зависимостей. Проверяет frontmatter, наличие секций, при `stable` — непустые секции и целостность ссылок (relations/emits/consumes указывают на существующие id).

Pre-commit hook гонит валидатор на изменённых `docs/domains/**.md`.
Жёсткого Claude-хука на запуск нет (снят); детерминированный gate — pre-commit-валидатор на изменённых `docs/domains/**.md`. `scaffold`/`domain-deep-dive` по дисциплине не стартуют на красном валидаторе или `status: draft`.
