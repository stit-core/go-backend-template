# Чек-листы полей по kind

По разделу на каждый из 6 kinds + context-README. В каждом разделе:

- **Обязательные H2** — что должно быть и непусто к stable.
- **Опциональные H2** — допустимо `_N/A_` / `_нет_`.
- **Ссылается на** — какие соседи во frontmatter / теле.
- **Кто ссылается на это** — обратные связи.
- **Главные anti-patterns** — короткий список (из `docs/domains/README.md`
  и подсказок шаблонов).
- **Порядок копания** — последовательность вопросов в режиме A/C.

Полный канон — `docs/domains/README.md` + `_templates/<kind>.md`. Здесь
— выжимка для скилла.

---

## aggregate

**Обязательные H2**: `Identity`, `Properties`, `Methods`, `Invariants`,
`Lifecycle`, `Boundary`, `Contained Entities`, `Spanning Invariants`,
`Value Objects`, `Events`, `Relations`, `Use Cases`.

**Опциональные** (можно `_N/A_`): нет — все 12 обязательны, но при
stateless / aggregate-of-one явно пишется `_N/A (stateless)_` /
`_нет (aggregate-of-one)_` соответственно (это считается заполненным).

**Frontmatter**: `id: <ctx>.<Name>`, `kind: aggregate`, `context`,
`name`, `status`, `relations`, `emits`, `consumes`.

**Ссылается на**: `<ctx>.<Event>` (events), `<other-ctx>.<Aggregate>`
(reference-relation by ID), `_shared.<VO>` (если используется
платформенный VO).
**Кто ссылается**: `<ctx>.<UseCase>` (called methods), `<ctx>.<Event>`
(triggered by aggregate method), `<ctx>.<ChildEntity>` (relation
composition).

**Anti-patterns**:
- I/O-операция как метод агрегата (`SendEmail`, `Charge`, `Fetch`) —
  это use-case.
- Spanning invariant между двумя aggregate'ами — запрещено DDD,
  переразбить или использовать policy/event.
- Cross-context relation типа `composition` — только `reference` (by ID).
- Внутри-контекстный VO как отдельный файл — должен жить inline в
  секции `Value Objects` этого же файла.
- Mutator без events при том, что мутация бизнес-значимая.

**Порядок копания**:

1. **Identity** (раунд enum): `ID type` (Other), `Mutable` (yes/no),
   `Visibility` (in-context / published).
2. **Properties (таблица + детали)**:
   - таблица `name | type | required | brief` (имена + типы + required)
     — через Other по каждому полю;
   - для каждого property подсекция `### <Name>` с `Constraints`,
     `Default`, `Source`, `Description` — Other для значений; `Source`
     — enum (`input` / `derived` / `computed` / `system`).
3. **Methods (таблица + детали)**:
   - таблица `name | brief`;
   - на каждый метод — подсекция с `params`, `returns`, `preconditions`,
     `postconditions`, `events emitted` (через Other).
4. **Invariants** — нумерованный список falsifiable. На каждом этапе —
   AskUserQuestion с примерами противоречий: «Может ли существовать
   `<Name>` с `<combo>`?» (✅/❌-режим бинарного теста).
5. **Lifecycle** — таблица переходов либо `_N/A (stateless)_`.
   Если есть переходы — AskUserQuestion: «есть state-machine?» (yes/no),
   при yes — построить таблицу `from | event/method | to | guard | side`.
6. **Boundary** — что внутри транзакционной границы, что снаружи (по ID).
7. **Contained Entities** — root + child entities (inline-подсекция или
   отдельный entity-файл если сложно).
8. **Spanning Invariants** — инварианты между несколькими entity внутри
   агрегата; для aggregate-of-one — `_нет (aggregate-of-one)_`.
9. **Value Objects** — inline VO внутри агрегата. Раунд: на каждый VO
   — Properties, Invariants, Equality (by value | structural),
   Constructor. Cross-context VO — ссылка одной строкой на `_shared/<X>.md`.
10. **Events** — emits (список ссылок на `events/<X>.md`) + consumes
    (список `<other-ctx>.<Event>`).
11. **Relations** — таблица `target | cardinality | type | description`.
    Anti-pattern alert: `type=composition` между разными context'ами.
12. **Use Cases** — список ссылок на `use-cases/*.md` (раздел читается
    тривиально из существующих use-case-файлов контекста).

**Тест-критерий**: см. `draft-stable.md § aggregate`.

---

## entity (child)

**Обязательные H2**: `Identity`, `Properties`, `Methods`, `Invariants`,
`Lifecycle`, `Relations`.

**Frontmatter**: `id: <ctx>.<Name>`, `kind: entity`, `relations:
[<ctx>.<ParentAggregate>]`.

**Anti-patterns**: прямой репозиторий снаружи aggregate'а; внешние
ID-ссылки на child entity (только через parent).

**Порядок копания** — аналог aggregate, но `Identity.ID scope =
"локальный внутри <ParentAggregate>"`, `Relations` всегда содержит
`type=composition` к родителю.

**Тест-критерий**: см. `draft-stable.md § entity`.

---

## event

**Обязательные H2**: `Trigger`, `Payload`, `Consumers`, `Delivery semantics`.

**Опциональные**: `Notes`.

**Frontmatter**: `id: <ctx>.<Name>`, `kind: event`, `relations:
[<ctx>.<SourceAggregate>]`.

**Ссылается на**: `<ctx>.<Aggregate>` (source), `<ctx>.<UseCase>`
(triggering use-case).
**Кто ссылается**: `<ctx>.<Aggregate>.Methods` (events emitted),
`<ctx>.<UseCase>.Events emitted`, `<other-ctx>` (consumes).

**Anti-patterns**:
- Имя в настоящем/будущем времени (`SubmitOrder`, `WillCharge`) — только
  прошедшее (`OrderSubmitted`, `Charged`).
- Суффикс `Event` (`OrderSubmittedEvent`) — имя самодостаточно.
- Payload содержит UI-state (scroll, focus), sensitive (пароли, сырые
  токены), вычисляемые-из-других-полей данные.
- `Ordering: none` при том, что consumer'ы зависят от порядка событий
  одного агрегата.

**Порядок копания**:

1. `Trigger`: `Source aggregate` (выбор из существующих aggregate'ов
   контекста + Other), `Triggering use-case` (выбор из use-cases/), `When`
   (Other — обычно «после успешного коммита транзакции»).
2. `Payload` (таблица): обязательно `EventID` (UUID v7), `OccurredAt`,
   `<Aggregate>ID`; кастомные поля — Other.
3. `Consumers` (таблица): по каждому consumer — `<HandlerName>` (Other),
   `bounded context` (выбор из существующих), `reaction` (Other). Если
   нет consumer'ов на старте — `_нет на старте_`.
4. `Delivery semantics`:
   - `Delivery`: enum `at-least-once` / `at-most-once` / `exactly-once-effective`;
   - `Ordering`: enum `none` / `per-aggregate (partition key = <X>ID)` /
     `global`;
   - `Idempotency key`: Other — обычно `EventID`;
   - `Retention`: Other.
5. `Notes` — опционально (миграционные хвосты, breaking-change policy).

**Тест-критерий**: см. `draft-stable.md § event`.

---

## use-case

**Обязательные H2**: `Actor`, `Inputs`, `Outputs`, `Dependencies`,
`Steps`, `Called aggregate methods`, `Events emitted`, `Errors`.

**Опциональные**: `Side effects beyond the aggregate`.

**Frontmatter**: `id: <ctx>.<Name>`, `kind: use-case`, `relations:
[<ctx>.<Aggregate>]`, `emits`, `consumes`.

**Ссылается на**: `<ctx>.<Aggregate>` (called methods), `<ctx>.<Event>`
(emitted), `<other-ctx>.<X>Reader` (cross-domain ports).
**Кто ссылается**: `<ctx>/README.md § Use cases`,
`<ctx>.<Aggregate>.Use Cases`, `<ctx>.<Policy>.Used by`.

**Anti-patterns**:
- Actor неясен / «system» при том, что есть HTTP-вход.
- Steps — прозой «обрабатывает запрос» вместо observable действий
  (Validate / Load / Construct / Apply / Persist / Emit / Return).
- Errors отсутствуют или нет HTTP-статусов.
- I/O-операция как метод агрегата (anti-pattern aggregate'а) частая
  причина того, что use-case «голый» — переразбить.
- `Authorizer`-порт внутри use-case (авторизация — middleware
  `authgate`, если не нужны fine-grained policy).

**Порядок копания**:

1. `Actor`:
   - `Role`: enum `anonymous` / `admin` / `client` / `system`;
   - `Channel`: enum `HTTP API (<METHOD> <path>)` / `NATS subscriber` /
     `scheduled job` / `internal` (Other для конкретного path).
2. `Inputs` (таблица) — Other per field.
3. `Outputs` — `On success` (Other), `On failure` ссылается на Errors.
4. `Dependencies` (таблица `port | purpose | adapter`) — выбор из
   известных портов проекта (репо агрегата, `Clock`, cross-domain
   readers) + Other для нового. Anti-pattern alert: `Authorizer`.
5. `Steps` — нумерованный observable список. Other на каждый шаг.
6. `Called aggregate methods` — выбор из методов соответствующего
   `<Aggregate>.md` + «нужен новый метод» (Other → правка aggregate'а).
7. `Events emitted` — выбор из существующих `events/*.md` + «нужно
   новое событие» (Other → stub event).
8. `Errors` (таблица `sentinel | when | HTTP status`) — Other; типичные
   `ErrInvalid<X>` 400, `ErrNotFound` 404, `ErrAlreadyExists` 409,
   `ErrForbidden` 403 — как подсказки в свободной форме.
9. `Side effects beyond the aggregate` — опционально, явно `_нет_` если
   нет.

**Тест-критерий**: см. `draft-stable.md § use-case`.

---

## policy

**Обязательные H2**: `Kind of decision`, `Inputs`, `Rules`, `Outcomes`,
`Used by`.

**Опциональные**: `Composition`.

**Frontmatter**: `id: <ctx>.<Name>`, `kind: policy`.

**Ссылается на**: `<ctx>.<UseCase>` (used by).
**Кто ссылается**: `<ctx>.<UseCase>.Dependencies` (если policy
явно — порт; чаще inline в Steps), `<ctx>.<UseCase>.Steps`
(шаг Authorize/Validate).

**Anti-patterns**:
- I/O в policy (БД-вызов, HTTP). Policy — pure-функция.
- Скрытое состояние (random, clock без Inputs).
- Не используется ни одним use-case'ом — «policy ради policy».

**Порядок копания**:

1. `Kind of decision`:
   - `Type`: enum `authorization` / `validation` / `rate-limit` / `rule`;
   - `Pure`: enum `yes (без БД/сети/времени)` / `impure (зависит от
     Clock/random)` — Other для уточнения.
2. `Inputs` (таблица) — Other.
3. `Rules` — нумерованный falsifiable-список. Other на каждый.
4. `Outcomes` — таблица `outcome | when | error`. `outcome` — enum
   `allow` / `deny`; `error` — sentinel либо `—`.
5. `Composition` — опционально, как policy сочетается с другими.
6. `Used by` — выбор из существующих use-cases контекста + Other.

**Тест-критерий**: см. `draft-stable.md § policy`.

---

## vo (cross-context, `_shared/`)

**Обязательные H2**: `Properties`, `Invariants`, `Constructor`,
`Consumed by`.

**Опциональные**: `Errors`, `Notes`.

**Frontmatter**: `id: _shared.<Name>`, `kind: vo`, `context: _shared`.

**Ссылается на**: ничего.
**Кто ссылается**: `<ctx>.<Aggregate>.Properties` / inline-VO ссылается
одной строкой; `Consumed by` — обратный список.

**Anti-patterns**:
- Доменная семантика (`Phone`, `Email`, `OrderId`) — должно жить inline
  в aggregate-файле, не в `_shared/`.
- Используется только одним контекстом — не нужен `_shared/`.

**Порядок копания**:

1. `Properties` (таблица).
2. `Invariants` (нумерованный falsifiable-список).
3. `Constructor` (сигнатура Go + params/returns/pre/post).
4. `Consumed by` — список aggregate'ов из ≥2 контекстов, иначе anti-pattern.
5. `Errors` — sentinel-ошибки.
6. `Notes` — equality semantics, источник данных и т.п.

**Тест-критерий**: см. `draft-stable.md § vo`.

---

## context-README (`<ctx>/README.md`)

**Обязательные H2**: `Purpose`, `Aggregates`, `Use cases`, `Events`
(вложенно `Published (outgoing)` + `Subscribed (incoming)`),
`Cross-context dependencies`, `Ubiquitous Language`.

**Опциональные**: `Glossary notes`, `Architecture notes`.

**Anti-patterns**:
- Половина aggregate'ов из директории не упомянуты — README обязан их
  слинковать (валидатор проверяет).
- `Cross-context dependencies` указано только в одну сторону
  (только outbound) — inbound события забыты.
- `Ubiquitous Language` без колонки «Не путать с» — теряется главное.

**Порядок копания**:

1. `Purpose`: «что делаем» (Other) + «чего не делаем — граница» (Other).
2. `Aggregates` — таблица; автозаполнение из `<ctx>/*.md` aggregate-файлов.
3. `Use cases` — таблица; автозаполнение из `use-cases/*.md`.
4. `Events`:
   - `Published` — автозаполнение из `events/*.md`;
   - `Subscribed` — выбор из `events/*.md` других контекстов + Other.
5. `Cross-context dependencies` — таблица direction/other-context/nature.
6. `Ubiquitous Language` — таблица с явной колонкой «Не путать с».
   Каждый ряд — Other.
7. `Glossary notes` / `Architecture notes` — Other.

**Тест-критерий**: см. `draft-stable.md § context-README`.
