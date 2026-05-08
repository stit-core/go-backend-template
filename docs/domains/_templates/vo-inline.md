<!--
Шаблон для вставки в секцию "Value Objects" файла aggregate'а.
Не предназначен для отдельного файла. Скопируй блок ниже как H3-секцию.
-->

### `<VOName>`

- **Properties:** `<field>: <type>` (для multi-field — `field1: t1, field2: t2`)
- **Invariants:** перечислить условия валидности (regex, range, length, ...)
- **Equality:** by value | structural | custom (`Equal` метод)
- **Constructor:** `New<VOName>(...) (<VOName>, error)` — нормализует и валидирует
- **Notes:** _опционально, если что-то требует пояснения_
