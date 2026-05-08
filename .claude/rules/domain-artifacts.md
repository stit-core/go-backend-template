---
name: domain-artifacts
description: DDD-артефакты в docs/domains/ — конвенции, kinds, frontmatter, валидатор. Грузится при работе с docs/domains/.
globs: docs/domains/**
---

# Domain artifacts

DDD-артефакты — **первичный source of truth** проектирования. Код в `internal/domain/` пишется по ним, не наоборот.

См. `docs/domains/README.md` (структура, workflow) и `docs/domains/MAPPING.md` (DDD ↔ Go).

## Kinds (frontmatter `kind:`)

- `aggregate` — бизнес-сущность с консистентной границей. Root entity + VO внутри. Один файл = один aggregate.
- `entity` — child entity сильной сложности (опционально, обычно описывается секцией внутри aggregate).
- `event` — domain event для outbox.
- `use-case` — бизнес-операция (orchestration). Маппится на метод `*Usecase`.
- `policy` — pure правило (authz/validation), без I/O.

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

Stateless / aggregate-of-one / пустые consumers — пишутся явно (`N/A`, `_нет_`), не оставляются пустыми.

## Workflow

1. Скопировать из `docs/domains/_templates/` в `docs/domains/<ctx>/`.
2. Заполнить frontmatter + минимум (Properties с именами+типами, Methods с именами).
3. `/domain-deepen <ctx>` — углубление по волнам inside-out.
4. `scripts/agent-tools/validate-domain-doc.sh docs/domains/<ctx>` — зелёный → промоушн `status: stable`.
5. `add-domain` / `add-endpoint` / `add-event` теперь разрешены (claude-hook гейтит).

## Делать

- Один Aggregate = один файл. VO внутри aggregate-файла.
- Pure-методы агрегата → секция Methods. Операции с I/O → use-case.
- Cross-context relation — только `reference (by ID)` или `uses (через port)`. Не `composition`.
- При status=stable все обязательные секции должны быть непустыми.
- Идемпотентность субскрайберов — `EventID` в payload + хранение обработанных ID.

## Не делать

- ❌ Отдельный файл на каждую VO.
- ❌ Описывать I/O-операцию (с repo/ports) как метод агрегата.
- ❌ Spanning invariant между двумя aggregate'ами.
- ❌ Промоутить `status: stable` руками без зелёного валидатора.
- ❌ Запускать `add-domain` пока есть `status: draft` в `docs/domains/<ctx>/`.

## Валидатор

`scripts/agent-tools/validate-domain-doc.sh [path]` — детерминированный, без внешних зависимостей. Проверяет frontmatter, наличие секций, при `stable` — непустые секции и целостность ссылок (relations/emits/consumes указывают на существующие id).

Pre-commit hook гонит валидатор на изменённых `docs/domains/**.md`.
Claude PreToolUse hook (Skill matcher) блокирует `add-domain`/`add-endpoint`/`add-event` если валидатор красный или есть `status: draft`.
