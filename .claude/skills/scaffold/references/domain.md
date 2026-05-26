# scaffold › domain — новый бизнес-домен

Создаёт новый домен по конвенции этого шаблона. Соответствует `.claude/rules/domain-architecture.md` — открой его если ещё не загружено.

**Gate (soft).** Скаффолдь по **stable** DDD-артефактам `docs/domains/<ctx>/`: код разворачивается
по дизайну, не наоборот. Если артефакты ещё `draft` или `scripts/tools/validate-domain-doc.sh
docs/domains/<ctx>` красный — сначала `domain-deep-dive` до `stable`, потом сюда. Жёсткого хука
нет (снят) — это дисциплина (см. `docs/domains/README.md § Gate`).

## Когда применять

- "Добавь домен `order`"
- "Создай новый домен для управления подписками"
- "Scaffold a `notification` domain with CRUD"

## Когда НЕ применять

- Изменение существующего домена → не активируй skill, работай напрямую.
- Только миграция / только handler / только событие → точечная правка, skill не нужен.
- Артефакты домена ещё `draft` / валидатор красный → сначала `domain-deep-dive` (см. Gate выше).

## Шаги

### 1. Подтвердить вход

Прежде чем что-то делать:
- Имя домена в `snake_case` (например, `order`, `notification`, `subscription`).
- Что нужно: CRUD? только read? с outbox-событиями? с HTTP-API или только worker?

Если неясно — спроси одним вопросом до начала.

### 2. Скаффолд директорий

```bash
make new-domain NAME=<name>
```

Скрипт `scripts/new-domain.sh` создаёт:
```
internal/domain/<name>/
├── entity.go
├── dto.go
├── errors.go
├── events.go
├── policy.go
├── policy_test.go
├── usecase.go
├── usecase_test.go
├── delivery/http/
│   ├── handler.go
│   ├── handler_test.go
│   ├── dto.go
│   └── view.go
├── infra/postgres/
│   ├── repository.go
│   └── queries/<name>.sql
└── module/
    ├── module.go
    └── adapters.go
```

Файлы — со stub-кодом, который дальше дописываешь.

### 3. Миграция

Добавь `migrations/NNNN_<name>.up.sql` + `.down.sql`. Нумерация — следующая после максимальной существующей.

Структура `up`:
```sql
CREATE TABLE <name>s (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    -- ...поля
    version INT NOT NULL DEFAULT 0,  -- для optimistic lock
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

`down`: `DROP TABLE <name>s;`

### 4. sqlc-конфиг

В `sqlc.yaml` добавь блок:
```yaml
- engine: postgresql
  schema: migrations/
  queries: internal/domain/<name>/infra/postgres/queries/
  gen:
    go:
      package: db
      out: internal/domain/<name>/infra/postgres/db
      sql_package: pgx/v5
      emit_interface: false
      emit_exact_table_names: false
      emit_pointers_for_null_types: true
      emit_json_tags: false
      emit_prepared_queries: false
      emit_methods_with_db_argument: false
```

### 5. Запросы

В `internal/domain/<name>/infra/postgres/queries/<name>.sql`:
```sql
-- name: GetByID :one
SELECT * FROM <name>s WHERE id = $1;

-- name: Insert :exec
INSERT INTO <name>s (id, ...) VALUES ($1, ...);

-- name: Update :execrows
UPDATE <name>s
SET ..., version = version + 1, updated_at = NOW()
WHERE id = $1 AND version = $2;
```

Минимально — чтобы домен компилировался. Расширяй по мере usecase.

### 6. Сгенерировать sqlc-код

```bash
make gen
```

Создаёт `internal/domain/<name>/infra/postgres/db/`. Не правь руками.

### 7. Доменные типы

`entity.go`:
```go
package <name>

type <Name> struct {
    ID        string
    // ...поля
    Version   int
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

`errors.go`:
```go
package <name>

import "errors"

var (
    ErrNotFound = errors.New("<name>: not found")
    ErrConflict = errors.New("<name>: version conflict")
)
```

### 8. Repository

`infra/postgres/repository.go` — имплементит порт `Repository` из `usecase.go`. Использует sqlc-методы.

### 9. Usecase

`usecase.go`:
- Объяви интерфейсы зависимостей (порты).
- Структура `Usecase` с зависимостями.
- Методы — бизнес-логика.

### 10. Adapters (если нужны cross-domain)

Если domain нужны данные другого домена — порт в `usecase.go`, адаптер в `module/adapters.go`. depguard блокирует прямые импорты.

### 11. fx-модуль

`module/module.go`:
```go
package module

var Module = fx.Module(
    "<name>",
    fx.Provide(
        postgres.NewRepository,
        <name>.NewUsecase,
        // ...
        http.NewHandler,
        func(u *<name>.Usecase) http.Usecase { return u },
    ),
    fx.Invoke(http.Register),
)
```

### 12. Подключить в cmd/api/main.go

```go
import <name>mod "git.stit.tech/stit-infra/go-backend-template/internal/domain/<name>/module"

domainModules = fx.Options(
    usermod.Module,
    authmod.Module,
    verifmod.Module,
    <name>mod.Module,  // добавить
)
```

### 13. Моки (если есть exported интерфейсы)

В `.mockery.yml`:
```yaml
git.stit.tech/stit-infra/go-backend-template/internal/domain/<name>:
  interfaces:
    Repository:
    # другие интерфейсы из usecase.go
git.stit.tech/stit-infra/go-backend-template/internal/domain/<name>/delivery/http:
  interfaces:
    Usecase:
```

```bash
make gen
```

### 14. Тесты

`usecase_test.go` — тесты основной логики через моки портов.
`policy_test.go` — тесты authorization policy.
`delivery/http/handler_test.go` — тесты HTTP-уровня через моки Usecase.

Минимальный набор — happy path + основная error path.

### 15. depguard правило

В `.golangci.yml` добавь правило изоляции для нового домена:
```yaml
<name>-domain-isolation:
  list-mode: lax
  files:
    - "**/internal/domain/<name>/**"
    - "!**/*_test.go"
  deny:
    - pkg: "git.stit.tech/stit-infra/go-backend-template/internal/domain/<other>"
      desc: "<name> domain must not import <other>"
```

(Только если домен реально не должен импортить другие — обычно так.)

### 16. Верификация

Перед claim "готово":
```bash
make check
```

Или поэтапно:
- `make fmt`
- `make lint`
- `make test`
- `make build`
- `make gen-check`

Всё green → коммит.

### 17. Коммит

`feat(<name>): scaffold <name> domain with CRUD`

Если есть outbox-события: `feat(<name>): scaffold <name> domain with CRUD and outbox events`.

## Что не делать

- Не копируй существующий домен (`auth`/`user`/`verification`) целиком — это референсы, у них своя специфика. Используй `make new-domain` который даст чистый stub.
- Не правь sgenerated `db/`-файлы. Регенерируй через `make gen`.
- Не пропускай миграцию (`down`-файл обязателен).
- Не подключай домен в main.go до того как он компилируется в изоляции.
