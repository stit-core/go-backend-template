# scaffold › domain — новый бизнес-домен

Создаёт новый домен по конвенции этого шаблона. Соответствует `.claude/rules/domain-architecture.md` — открой его если ещё не загружено.

> `<module-path>` в примерах ниже — module path репо из `go.mod` (строка `module`).
> В форке после `/de-template` это уже свой путь (`git.stit.tech/<org>/backend`),
> не путь шаблона. Подставляй фактический — не копируй буквально.

**Gate (soft).** Домены проектируются **не в этом репо**, а в соседнем strategy-репо орга
(`<org>/strategy`, форк `business-strategy-template`) — внутри сервиса-владельца. Скаффолдь по
**stable**-артефактам оттуда: `<strategy-path>/docs/projects/<slug>/services/<svc>/domains/<ctx>/`,
где `<slug>` / `<svc>` / `<strategy-path>` — это `project-slug` / `service-slug` / `strategy-path`
из `docs/stack/service.md § Identity` (дефолт `strategy-path` = `../strategy`, sibling-чекаут
рядом; контекст уникален в проекте, так что его папку находит и `services/*/domains/<ctx>/`).
Код разворачивается по дизайну, не наоборот: если артефакт ещё `draft` (или sibling-репо не
склонирован рядом) — сначала доведи домен до `stable` в strategy-репо его скиллом
`/domain-deep-dive <slug> <ctx>`, потом сюда. Жёсткого хука нет — это дисциплина. Как секции
артефакта ложатся в Go-код этого репо — `docs/reference/domain-mapping.md`.

## Когда применять

- "Добавь домен `order`"
- "Создай новый домен для управления подписками"
- "Scaffold a `notification` domain with CRUD"

## Когда НЕ применять

- Изменение существующего домена → не активируй skill, работай напрямую.
- Только миграция / только handler / только событие → точечная правка, skill не нужен.
- Артефакты домена в strategy-репо ещё `draft` → сначала `/domain-deep-dive <slug> <ctx>` там (см. Gate выше).

## Шаги

### 1. Подтвердить вход

Прежде чем что-то делать:
- Имя домена в `snake_case` (например, `order`, `notification`, `subscription`).
- Что нужно: CRUD? только read? с outbox-событиями? с HTTP-API или только subscriber/job?

Если неясно — спроси одним вопросом до начала.

### 2. Скаффолд директорий

```bash
make new-domain NAME=<name>
```

Скрипт `scripts/new-domain.sh` создаёт stub-файлы и сам вписывает конфиги:
```
internal/domain/<name>/
├── entity.go
├── dto.go
├── errors.go
├── metrics.go
├── policy.go
├── policy_test.go
├── usecase.go
├── usecase_test.go
├── delivery/http/
│   ├── handler.go
│   └── handler_test.go
├── infra/postgres/
│   ├── repository.go
│   └── queries/<name>.sql
└── module/
    └── module.go
```
+ интерфейсы `Repository`, `Authorizer`, `delivery/http.Usecase` в `.mockery.yml`. Границы
нового домена держит `internal/archtest` — правка конфигов не нужна.

Остальные файлы появляются вместе с содержимым (таблица обязательности —
`docs/reference/architecture.md`): `delivery/http/dto.go` и `view.go` — с первым endpoint'ом,
`events.go` + `infra/outbox/outbox.go` — с первой публикацией (`references/event.md`),
`module/adapters.go` — с первым cross-domain портом, `trusted.go` — с первым доменом-потребителем
(`references/port.md`).

### 3. Миграция

Добавь `migrations/NNNN_<name>.up.sql` + `.down.sql`. Нумерация — следующая после максимальной существующей.

Структура `up`:
```sql
CREATE TABLE <name>s (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    -- ...поля
    version INT NOT NULL DEFAULT 1,  -- для optimistic lock
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
      omit_unused_structs: true
```

### 5. Запросы

В `internal/domain/<name>/infra/postgres/queries/<name>.sql` (создание — `Insert<Entity>`, rule `sqlc-and-migrations`):
```sql
-- name: Get<Name>ByID :one
SELECT * FROM <name>s WHERE id = $1;

-- name: Insert<Name> :exec
INSERT INTO <name>s (id, ...) VALUES ($1, ...);

-- name: Update<Name> :execrows
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
    ErrConcurrentUpdate = errors.New("<name>: concurrent update")
)
```

### 8. Repository

`infra/postgres/repository.go` — имплементит порт `Repository` из `usecase.go`. Использует sqlc-методы.

### 9. Usecase

`usecase.go`:
- Объяви интерфейсы зависимостей (порты) и `UsecaseDeps{fx.In}`; `Usecase` хранит deps целиком
  (`uc.d.Repo`), `NewUsecase` меняет только `d.Log`.
- Методы — бизнес-логика; мутирующие инструментированы `metrics.Record*` (`metrics.go`).

### 10. Adapters (если нужны cross-domain)

Если domain нужны данные другого домена — порт в `usecase.go`, адаптер в `module/adapters.go` (`references/port.md`). archtest блокирует прямые импорты.

### 11. fx-модуль

`module/module.go` генерирует `new-domain.sh`:
```go
package module

var Module = fx.Module(
    "internal/domain/<name>/module",
    fx.Provide(
        <name>pg.NewRepository,
        <name>.NewPolicy,
        <name>.NewMetrics,
        <name>.NewUsecase,
        <name>http.NewHandler,
        func(u *<name>.Usecase) <name>http.Usecase { return u },
    ),
)
```

Имя модуля — путь пакета; порт в графе — тип результата провайдера (`NewPolicy` возвращает
`Authorizer`), `fx.As` не нужен (`docs/reference/architecture.md` § fx).
Дальше дополняй его провайдерами по мере роста: адаптеры портов (`new<Provider>Adapter` из
`adapters.go`, `infra/<kit>` — для китов), `<name>.NewTrusted` — когда домен нужен другим,
`outbox.Providers` домена-публикатора, NATS-handler (`references/event.md`).
HTTP-операции домен сам не регистрирует — это делает `registerHTTP` в `cmd/api` (шаг 12).

### 12. Подключить в cmd/api

`cmd/api/main.go` — модуль:
```go
import <name>mod "<module-path>/internal/domain/<name>/module"

domainModules = fx.Options(
    usermod.Module,
    authmod.Module,
    verifmod.Module,
    <name>mod.Module,  // добавить
)
```

`cmd/api/http.go` — HTTP-вход: поле `<Name> *<name>http.Handler` в `httpHandlers` и
`<name>http.Register(api, h.<Name>)` в `registerHTTP`. Публичная операция (без `Security`) —
ещё и в `publicOperations` (`cmd/api/api_test.go`). Неподключённый домен (модуль, HTTP- или
NATS-вход) валит `TestDomainsAreWired`; outbox-таблица домена вне `sources.Kits`
(`cmd/migrate/sources/sources.go`) — `TestBoot`.

### 13. Моки

`new-domain.sh` уже вписал в `.mockery.yml` `Repository`, `Authorizer` и `delivery/http.Usecase`.
Новые интерфейсы из `usecase.go` и `delivery/*` добавляй туда же:
```yaml
<module-path>/internal/domain/<name>:
  interfaces:
    Repository:
    # другие интерфейсы из usecase.go
```

```bash
make gen
```

### 14. Тесты

`usecase_test.go` — тесты основной логики через моки портов.
`policy_test.go` — тесты authorization policy.
`delivery/http/handler_test.go` — тесты HTTP-уровня через моки Usecase.

Минимальный набор — happy path + основная error path.

### 15. Верификация

Перед claim "готово":
```bash
make check
```

Отдельные шаги (`make lint`, `make test`, `make build`, `make gen-check`) — для быстрой итерации;
«готово» и коммит — только по зелёному `make check`.

### 16. Коммит

`feat(<name>): scaffold <name> domain with CRUD`

Если есть outbox-события: `feat(<name>): scaffold <name> domain with CRUD and outbox events`.

## Что не делать

- Не копируй существующий домен (`auth`/`user`/`verification`) целиком — это референсы, у них своя специфика. Используй `make new-domain` который даст чистый stub.
- Не правь сгенерированные `db/`-файлы. Регенерируй через `make gen`.
- Не пропускай миграцию (`down`-файл обязателен).
- Не подключай домен в main.go до того как он компилируется в изоляции.
