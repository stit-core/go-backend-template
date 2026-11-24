# scaffold › domain — новый бизнес-домен

**Шаг 0** — прочитай инструментом Read `.claude/rules/domain-architecture.md`,
`.claude/rules/sqlc-and-migrations.md` и `.claude/rules/tests-pipeline.md`.

Создаёт новый домен по конвенции этого шаблона.

> `<module-path>` в примерах ниже — module path репо из `go.mod` (строка `module`).
> В форке после `/de-template` это уже свой путь (`git.stit.tech/<org>/backend`),
> не путь шаблона. Подставляй фактический — не копируй буквально.

**Gate (soft).** Домены проектируются не в этом репо, а в соседнем strategy-репо орга
(`<org>/strategy`, форк `business-strategy-template`) — внутри сервиса-владельца. Скаффолдь по
**stable**-артефактам оттуда: `<strategy-path>/docs/projects/<slug>/services/<svc>/domains/<ctx>/`,
где `<slug>` / `<svc>` / `<strategy-path>` — это `project-slug` / `service-slug` / `strategy-path`
из `docs/stack/service.md § Identity` (дефолт `strategy-path` = `../strategy`, sibling-чекаут
рядом; контекст уникален в проекте, так что его папку находит и `services/*/domains/<ctx>/`).
Код разворачивается по дизайну, не наоборот: если артефакт ещё `draft` (или sibling-репо не
склонирован рядом) — сначала доведи домен до `stable` в strategy-репо его скиллом
`/domain-deep-dive <slug> <ctx>`, потом сюда. Жёсткого хука нет — это дисциплина. Как секции
артефакта ложатся в Go-код этого репо — `docs/reference/domain-mapping.md`. Strategy-репо у проекта
нет (`strategy-path` — `—`) — вход тот же по смыслу, от юзера: контракт домена (сущность, операции,
права, события) одним блоком до кода, юзер подтверждает.

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
- Агрегат среза — главный агрегат контекста в единственном числе (`order`, `object`, `employee`): по умолчанию —
  контекст в единственном числе (`objects` → `object`); у контекста, чьё имя — не его агрегат (`ordering` → `order`,
  `people` → `employee`), — явно, `AGG=`.
- Что нужно: CRUD? только read? с outbox-событиями? с HTTP-API или только subscriber/job?

Если неясно — спроси одним вопросом до начала.

### 2. Срез Create и Get агрегата

```bash
make new-domain NAME=<name>              # агрегат — <name> в единственном числе
make new-domain NAME=<name> AGG=<agg>    # агрегат контекста с другим именем
```

`scripts/new-domain.sh` создаёт рабочий вертикальный срез одного агрегата по канону и подключает его
(`<Agg>` — агрегат в CamelCase, `<aggs>` — его множественное число, `<Name>` — домен):

```
migrations/NNNN_<aggs>.{up,down}.sql    таблица <aggs> (id, name, created_at); номер — следующий
internal/domain/<name>/
├── entity.go          <Agg>{ID, Name, CreatedAt}, New<Agg> проверяет имя
├── dto.go             Create<Agg>Input
├── errors.go          ErrNotFound, ErrInvalidName
├── policy.go          CanCreate<Agg>, CanGet<Agg> — только admin; предел имени
├── usecase.go         Create<Agg>, Get<Agg>; порты <Agg>Repository и Authorizer
├── *_test.go          entity (+fuzz), policy, usecase на строгом моке репозитория и настоящей policy
├── delivery/http/     POST /api/<aggs> и GET /api/<aggs>/{id} под authgate.Secured (OperationID
│                      create<Agg>, get<Agg>), DTO с префиксом <Name><Agg> (<Name>, если агрегат — домен)
│                      и envelope data, таблицы
│                      ошибок create<Agg>Errors и get<Agg>Errors с Declare, handler- и errors-тесты
├── infra/postgres/    queries/<agg>.sql (Insert<Agg>, Get<Agg>ByID), repository.go,
│                      repository_test.go (integration), db/ — sqlc
└── module/module.go   fx.Module "internal/domain/<name>/module"
```

Имена агрегата — его, а не домена: следующий агрегат контекста встаёт рядом без переименований среза (§ Следующий
агрегат ниже). Множественное число — по правилам английского (`status` → `statuses`, `category` → `categories`; имя на
`-s`, кроме `-ss`, `-us`, `-is`, — уже множественное или неизменяемое: `series`); исключения
(`person` → `people`) правят руками.

Ещё скрипт вписывает блок домена в `sqlc.yaml` и интерфейсы `<Agg>Repository` и `delivery/http.Usecase`
в `.mockery.yml`, подключает модуль в `domainModules` (`cmd/api/main.go`) и HTTP-вход в
`httpHandlers` и `registerHTTP` (`cmd/api/http.go`), гонит `make gen`, проверяет, что срез собирается
(`go vet` с тегом integration), и ставит созданное в индекс git. Границы домена держит
`internal/archtest` без правки конфигов.

CamelCase — с инициализмом заглавными по правилу revive (`api_key` → `APIKey`, `urls` → `URLs`), в пути `_` → `-`.
Имена без правок — `bash scripts/new-domain.sh --names <name> [<agg>]`; первая строка прогона называет агрегат.
Домен — ключевое слово или встроенное имя Go, объявление пакета домена (`authorizer`, `policy`, `usecase`,
`usecase_deps`) у домена или агрегата, алиас импорта другого домена, таблицу, которую уже создаёт миграция, и
контекст, чьё единственное число имя не даёт (`-ches`, `-uses`, `-zes`), скрипт отвергает до первой правки; имя из
кода среза — сборкой с откатом всех его правок. Второй срез в тот же домен скрипт не кладёт.

#### Следующий агрегат

Следующий агрегат контекста — руками, рядом со срезом и в своих файлах: якорь мутации ищется по файлу, и код
второго агрегата в файле первого дал бы якорю среза два вхождения (`make mutate` — отказ «вхождений 2»).
- корень домена: `<agg2>.go` (сущность `<Agg2>`, `New<Agg2>`), DTO и методы usecase `Create<Agg2>`, `Get<Agg2>` —
  `usecase_<agg2>.go`, права `CanCreate<Agg2>` — в `policy.go`; порт `<Agg2>Repository` — в `usecase.go` рядом с
  первым, поле `<Agg2s>` — в `UsecaseDeps`; ошибки — в `errors.go`, свой код `<DOMAIN>_<AGG2>_<WHAT>`;
- `infra/postgres/`: `queries/<agg2>.sql`, адаптер `<agg2>_repository.go` (`<Agg2>Repository`,
  `New<Agg2>Repository`) с integration-тестом; миграция таблицы `<agg2s>` — `/scaffold` вариант migration;
- `delivery/http/`: `handler_<agg2>.go` (операции `create<Agg2>`, `get<Agg2>`, их таблицы ошибок), `view_<agg2>.go`,
  DTO `<Name><Agg2>…`, тесты; методы — в порт `Usecase`;
- `module/module.go` — адаптер в `fx.Provide`; блоки мутаций — свои, `<домен>-<код>` дальше номеров среза.

### 3. Гейт

`make check` — срез проходит его как есть, с покрытием. Красный — сначала причина (skill
`triage-failure`), не правка теста под код.

### 4. Домен по артефакту

Срез — основа, а не образец продукта: дальше домен растёт по stable-артефакту strategy
(`docs/reference/domain-mapping.md`), по правилам шага 0.

- **Поля** — колонки контракта пишутся в `CREATE TABLE` миграции среза (лишняя `name` оттуда уходит),
  пока срез не закоммичен: миграция ещё нигде не накатана, а `ALTER TABLE` над пустой таблицей —
  лишняя операция выкатки (`DROP COLUMN` — ещё и миграция с эффектом). Остальное — шаги 3–7 варианта
  `field` (запросы, репозиторий, сущность, HTTP, тесты). Поле после первого коммита среза — вариант
  `field` целиком, новой миграцией.
- **Права** — `policy.go`: срез пускает только admin, остальное открывает правило продукта.
- **Настройки** — вариант `env` (поле `App`, `Config` домена из модуля).
- **Операции** — вариант `endpoint`; ошибка домена — sentinel в `errors.go` и правило в таблице
  `<домен>Errors`.
- **События** — вариант `event` (`events.go`, `infra/outbox/outbox.go`, источник в `cmd/migrate`).
- **Данные другого домена** — вариант `port` (`module/adapters.go`, `trusted.go` поставщика).
- **Пользователь** — FK на `users`, порт к `user`, сид `pgseed` в integration-тесте: пока решение `identity`
  не `own`, путь каждого такого файла — в разделе «Свои домены поверх идентичности»
  `.claude/skills/init-service/references/prune.md` (Go-файл там требует archtest
  `TestPruneNamesIdentityDependents`).
- **Метрика** — только под алерт (`docs/reference/architecture.md` § Обязательность файлов).

Остальные файлы появляются вместе с содержимым (таблица обязательности — architecture.md).
Модуль растёт провайдерами: адаптеры портов (`new<Provider>Adapter` из `adapters.go`, `infra/<kit>` —
для китов), `<name>.NewTrusted` — когда домен нужен другим, `outbox.Providers` домена-публикатора,
NATS-handler. Порт в графе — тип результата провайдера, `fx.As` не нужен (architecture.md § fx).
Новый интерфейс из `usecase.go` или `delivery/*` — в `.mockery.yml`, затем `make gen`; мока не получают
policy-порт `Authorizer` (тесты берут настоящую policy) и порт с рукописным фейком (rule `tests-pipeline`).

### 5. Готово и коммит

Готово и коммит — по CLAUDE.md § Готово; для итерации — `make test PKG=./internal/domain/<name>/...`,
`make lint PKG=...`. Коммит — `feat(<name>): scaffold <name> domain`; с событиями —
`feat(<name>): scaffold <name> domain with outbox events`; в теле — строка `Review:`.

## Что не делать

- Не копируй существующий домен (домены идентичности `auth`/`user`/`verification`) целиком — у них
  своя специфика; основа — `make new-domain`.
- Не правь сгенерированные `db/` и `mocks/` — только `make gen`.
- Не правь миграцию после первого коммита среза — поле добавляет новая (шаг 4).
