# scaffold › domain — новый бизнес-домен

**Шаг 0** — прочитай инструментом Read `.claude/rules/domain-architecture.md`,
`.claude/rules/sqlc-and-migrations.md` и `.claude/rules/tests-pipeline.md`.

Создаёт новый домен по конвенции этого шаблона.

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

- Блок в существующий домен — свой вариант (`endpoint`, `event`, `migration`, `port`); правка
  существующего — по таблице «Маршрут задачи» (CLAUDE.md).
- Артефакты домена в strategy-репо ещё `draft` → сначала `/domain-deep-dive <slug> <ctx>` там (см. Gate выше).

## Шаги

### 1. Подтвердить вход

Прежде чем что-то делать:
- Имя домена в `snake_case` (например, `order`, `notification`, `subscription`).
- Что нужно: CRUD? только read? с outbox-событиями? с HTTP-API или только subscriber/job?

Если неясно — спроси одним вопросом до начала.

### 2. Срез Create и Get

```bash
make new-domain NAME=<name>
```

`scripts/new-domain.sh` создаёт рабочий вертикальный срез по канону и подключает его:

```
migrations/NNNN_<names>.{up,down}.sql   таблица <names> (id, name, created_at); номер — следующий
internal/domain/<name>/
├── entity.go          <Name>{ID, Name, CreatedAt}, New<Name> проверяет имя
├── dto.go             CreateInput
├── errors.go          ErrNotFound, ErrInvalidName
├── policy.go          CanCreate, CanGet — только admin; предел имени
├── usecase.go         Create, Get; порты Repository и Authorizer
├── *_test.go          entity (+fuzz), policy, usecase на строгом моке репозитория и настоящей policy
├── delivery/http/     POST /api/<names> и GET /api/<names>/{id} под authgate.Secured, DTO с
│                      префиксом и envelope data, таблицы ошибок с Declare, handler- и errors-тесты
├── infra/postgres/    queries/<name>.sql (Insert<Name>, Get<Name>ByID), repository.go,
│                      repository_test.go (integration), db/ — sqlc
└── module/module.go   fx.Module "internal/domain/<name>/module"
```

Ещё скрипт вписывает блок домена в `sqlc.yaml` и интерфейсы `Repository` и `delivery/http.Usecase`
в `.mockery.yml`, подключает модуль в `domainModules` (`cmd/api/main.go`) и HTTP-вход в
`httpHandlers` и `registerHTTP` (`cmd/api/http.go`), гонит `make gen` и ставит созданное в индекс
git. Границы домена держит `internal/archtest` без правки конфигов.

`<names>` — множественное число по правилам английского (`status` → `statuses`, `category` →
`categories`; в пути `_` → `-`), исключения (`person` → `people`) правят руками. Имя, с которым срез не
собрался бы (ключевое слово или встроенное имя Go, имя из кода среза, алиас импорта другого домена),
скрипт отвергает до первой правки.

### 3. Гейт

`make check` — срез проходит его как есть, с покрытием. Красный — сначала причина (skill
`triage-failure`), не правка теста под код.

### 4. Домен по артефакту

Срез — основа, а не образец продукта: дальше домен растёт по stable-артефакту strategy
(`docs/reference/domain-mapping.md`), по правилам шага 0.

- **Поля** — новая миграция (вариант `migration`, номер — следующий), запросы в
  `infra/postgres/queries/` (создание — `Insert<Entity>`), поля сущности, `view.go`, `make gen`.
- **Права** — `policy.go`: срез пускает только admin, остальное открывает правило продукта.
- **Операции** — вариант `endpoint`; ошибка домена — sentinel в `errors.go` и правило в таблице
  `<домен>Errors`.
- **События** — вариант `event` (`events.go`, `infra/outbox/outbox.go`, источник в `cmd/migrate`).
- **Данные другого домена** — вариант `port` (`module/adapters.go`, `trusted.go` поставщика).
- **Метрика** — только под алерт (`docs/reference/architecture.md` § Обязательность файлов).

Остальные файлы появляются вместе с содержимым (таблица обязательности — architecture.md).
Модуль растёт провайдерами: адаптеры портов (`new<Provider>Adapter` из `adapters.go`, `infra/<kit>` —
для китов), `<name>.NewTrusted` — когда домен нужен другим, `outbox.Providers` домена-публикатора,
NATS-handler. Порт в графе — тип результата провайдера, `fx.As` не нужен (architecture.md § fx).
Новый интерфейс из `usecase.go` или `delivery/*` — в `.mockery.yml`, затем `make gen`; policy-порт
`Authorizer` мока не получает — тесты берут настоящую policy.

### 5. Готово и коммит

Готово и коммит — по CLAUDE.md § Готово; для итерации — `make test PKG=./internal/domain/<name>/...`,
`make lint PKG=...`. Коммит — `feat(<name>): scaffold <name> domain`; с событиями —
`feat(<name>): scaffold <name> domain with outbox events`.

## Что не делать

- Не копируй существующий домен (`auth`/`user`/`verification`) целиком — у референсов своя
  специфика; основа — `make new-domain`.
- Не правь сгенерированные `db/` и `mocks/` — только `make gen`.
- Не правь уже накатанную миграцию — поле добавляет новая.
