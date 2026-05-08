---
id: <context>.<Name>
kind: entity
context: <context>
name: <Name>
status: draft
relations: [<context>.<ParentAggregate>]
emits: []
consumes: []
---

# <Name>

> Child entity внутри aggregate <ParentAggregate>. Снаружи агрегата напрямую недоступна.

> **Используй этот kind только когда child entity действительно сложна и заслуживает отдельного файла.** В большинстве случаев документируй child entities как подсекцию в файле aggregate'а.

## Identity

- **ID type:** <тип, обычно UUID v7>
- **ID scope:** локальный внутри `<ParentAggregate>` (внешние ссылки запрещены)
- **Mutable:** yes/no

## Properties

| name | type | required | brief |
|---|---|---|---|
| | | | |

### `<field>`

- **Constraints:**
- **Default:**
- **Source:** input | derived | computed | system
- **Description:**

## Methods

| name | brief |
|---|---|
| | |

### `<Method>(...)`

- **params:**
- **returns:**
- **preconditions:**
- **postconditions:**
- **events emitted:**

## Invariants

1.

## Lifecycle

| from | event/method | to | guard | side effects |
|---|---|---|---|---|

<!-- если stateless: -->
<!-- _Lifecycle: N/A (stateless)._ -->

## Relations

| target | cardinality | type | description |
|---|---|---|---|
| `<context>.<ParentAggregate>` | N..1 | composition | принадлежит родительскому агрегату |
