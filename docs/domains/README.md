# Domain Artifacts

DDD-first документация бизнес-домена. Артефакты — **первичный source of truth** проектирования; код вторичен и генерируется/пишется по ним.

## Структура

```
docs/domains/
├── README.md                  ← этот файл
├── MAPPING.md                 ← как DDD-артефакт → Go-код в этом репо
├── _templates/                ← пустые шаблоны для копирования
│   ├── aggregate.md
│   ├── entity.md              (опционально, для сложных child entities)
│   ├── vo-shared.md           (kind: vo — для _shared/, не для inline-VO внутри aggregate)
│   ├── event.md
│   ├── use-case.md
│   ├── policy.md
│   └── context-readme.md
├── _shared/                   ← cross-context VO и integration events
│   ├── README.md              ← роллап: какие VO живут здесь и для кого
│   └── <Name>.md              ← kind: vo, платформенный VO (PhotoRef, MoneyAmount, …)
└── <bounded-context>/
    ├── README.md              ← purpose, ubiquitous language (роллап на контекст)
    ├── <Aggregate>.md         ← kind: aggregate, root entity + inline-VO внутри
    ├── events/
    │   └── <Event>.md         ← kind: event
    ├── use-cases/
    │   └── <Operation>.md     ← kind: use-case
    └── policies/
        └── <PolicyName>.md    ← kind: policy
```

`_shared/` — заводим только когда VO реально нужен в ≥2 контекстах и не несёт доменной семантики. Внутри-контекстные VO остаются inline в aggregate-файле.

## Kinds

| kind | назначение | где живёт |
|---|---|---|
| `aggregate` | бизнес-сущность с консистентной границей: root entity + inline-VO внутри | `<ctx>/<Name>.md` |
| `entity` | child entity сильной сложности (опционально) | `<ctx>/<Name>.md` |
| `event` | domain event для outbox | `<ctx>/events/<Name>.md` |
| `use-case` | бизнес-операция (orchestration) | `<ctx>/use-cases/<Name>.md` |
| `policy` | pure правило / authz / rate-limit | `<ctx>/policies/<Name>.md` |
| `vo` | cross-context платформенный VO (только для `_shared/`) | `_shared/<Name>.md` |

## Frontmatter (общий контракт)

```yaml
id: user.User                 # <context>.<Name>, стабильный кросс-реф
kind: aggregate               # aggregate|entity|vo|event|use-case|policy
context: user
name: User                    # human-readable
status: draft                 # draft → stable (через валидатор)
relations: [user.UserId, auth.Session]
emits: [user.UserRegistered]
consumes: [verification.EmailVerified]
```

## Обязательные секции по kind

См. валидатор `scripts/tools/validate-domain-doc.sh`. Кратко:

| kind | обязательные H2 |
|---|---|
| `aggregate` | Identity, Properties, Methods, Invariants, Lifecycle, Boundary, Contained Entities, Spanning Invariants, Value Objects, Events, Relations, Use Cases |
| `entity` | Identity, Properties, Methods, Invariants, Lifecycle, Relations |
| `event` | Trigger, Payload, Consumers, Delivery semantics |
| `use-case` | Actor, Inputs, Outputs, Dependencies, Steps, Called aggregate methods, Events emitted, Errors |
| `policy` | Kind of decision, Inputs, Rules, Outcomes, Used by |
| `vo` | Properties, Invariants, Constructor, Consumed by |

Контекст-`README.md`: Purpose, Aggregates, Use cases, Events, Cross-context dependencies, Ubiquitous Language.

Stateless / aggregate-of-one / пустые consumers — пишутся явной пометкой `N/A` или `_нет_`. Пустых таблиц быть не должно.

## Workflow

```
1. Скопировать шаблоны:
     mkdir -p docs/domains/<ctx>/{events,use-cases,policies}
     cp _templates/aggregate.md       docs/domains/<ctx>/<Aggregate>.md
     cp _templates/event.md           docs/domains/<ctx>/events/<Event>.md
     cp _templates/use-case.md        docs/domains/<ctx>/use-cases/<Op>.md
     cp _templates/policy.md          docs/domains/<ctx>/policies/<Pol>.md
     cp _templates/context-readme.md  docs/domains/<ctx>/README.md
   # Если обнаружен cross-context платформенный VO:
     cp _templates/vo-shared.md       docs/domains/_shared/<Name>.md

2. Заполнить вручную минимум: frontmatter + properties (имена, типы)
   + methods/operations (имена). Принести Claude.

3. /domain-deepen <ctx>
   — пачкой по волнам, AskUserQuestion'ами добиваем секции inside-out.

4. scripts/tools/validate-domain-doc.sh docs/domains/<ctx>
   — зелёный → промоушн status: draft → stable.

5. /smartdev → /plan <task>
   — планировщик читает stable-артефакты, разносит работу по слоям.

6. add-domain / add-endpoint / add-event
   — gate проверяет наличие stable-артефактов; скаффолдит код по ним.
```

## Gate

- **Pre-commit hook** (`scripts/git-hooks/pre-commit`) гонит валидатор на изменённых `docs/domains/**.md`.
- **Claude PreToolUse hook** на запуск skill `add-domain` фейлит если в `docs/domains/<context>/` есть `status: draft` или валидатор красный.

## Маппинг DDD → код

См. [MAPPING.md](MAPPING.md). Если не уверен куда что ляжет в коде — туда.

## Anti-patterns

- ❌ Создавать отдельный файл для **внутри-контекстной** VO. Она живёт inline в aggregate-файле. (Исключение: cross-context платформенный VO — `_shared/<Name>.md` с `kind: vo`.)
- ❌ Описывать I/O-операции как методы агрегата. Если нужен репо/порт — это use-case.
- ❌ Spanning invariant между двумя aggregate'ами. Запрещено DDD.
- ❌ Cross-context relation типа `composition`. Только `reference` (по ID).
- ❌ Промоутить `status: stable` руками без зелёного валидатора.
- ❌ Класть в `_shared/` доменный VO (например `Phone`, `Email`). `_shared/` — только для платформенных типов без доменной семантики.
