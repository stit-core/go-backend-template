---
id: _shared.<Name>
kind: vo
context: _shared
name: <Name>
status: draft
relations: []
emits: []
consumes: []
---

# <Name>

> Одна-две фразы: что это значение. Должно быть платформенным (не нести доменной семантики). Если несёт — заводи inline VO внутри aggregate-файла домена, не сюда.

## Properties

| name | type | required | brief |
|---|---|---|---|
| `Field1` | string | yes | <что это> |

### `Field1`

- **Constraints:** <правила валидации>
- **Default:** —
- **Source:** input

## Invariants

1. **<инвариант>.** <как гарантируется>

## Constructor

```go
New<Name>(...) (<Name>, error)
```

- **params:** <список полей>
- **returns:** `<Name>` или sentinel-ошибка
- **preconditions:** см. invariants
- **postconditions:** zero-value становится валидным; equality — by value

## Consumed by

- [`<ctx>.<Aggregate>`](../<ctx>/<Aggregate>.md) — <где используется (поле, кардинальность)>

## Errors

| sentinel | when |
|---|---|
| `ErrInvalid<Name>` | <условия> |

Sentinel-ошибка живёт в `internal/platform/<area>/errors.go`.

## Notes

- <Опциональные пояснения: equality semantics, где загружается источник данных, и т.п.>
