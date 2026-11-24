# de-template › doc-driven wiring

Шесть `Generates`-секций, которые движок разворачивает как reviewable diff поверх скелета
(в отличие от детерминированной механики Phase A). Каждый kind: прочитай его `Generates` в
`docs/stack/<kind>.md`, наложи дифф в названном месте, покажи юзеру на ревью. Порядок —
фундамент → роллап (зеркалит волны `stack-deep-dive`); `runtime` последним, он стягивает остальное.

## Порядок и маппинг

### 1. persistence → `docs/stack/persistence.md`
- pgkit pool + transactor в fx (обычно `internal/platform` или модуль-провайдер).
- база `sqlc.yaml` (per-domain блоки добавляет `scaffold` domain — здесь только общий каркас).
- `cmd/migrate` + порядок накатки app+kit миграций.

### 2. dependencies → `docs/stack/dependencies.md`
- киты golibinfra — из `golibinfra kits`: нужный — его `<kit>.NewConfig` в `kitConfigs` и модуль в графе
  (`cmd/api/main.go`); кит шаблона, которого там нет, — вон из графа, импорты — `go mod tidy`. Кроме
  китов, которые берут домены идентичности (сессии, антибрутфорс, аудит — `auditkit.Logger` у `auth`,
  `user`, `verification`): без них граф не собирается (`TestFxGraphValidates`), а при `identity: external`
  их снимают шаги «Подключение» и «Подключение по стеку» prune (`/init-service`) — после своих доменов.
- `timerkit.Module` и пробы healthkit (`healthkit.ProvideProbe(<kit>.Probe)`, у NATS —
  `natskit.ProvideProbe("")`) — потребители групп китов: без первого не идут джобы (дренаж outbox,
  cleanup — `TestFxGraphValidates` красный), без проб `/ready` не видит зависимость (список проб сверяет
  `TestBoot`). Их не убирают,
  пока в графе есть кит с джобом или пробой.
- wiring внешних runtime-deps (Redis/NATS/…), если стек их объявил. Зависимость кита графа (Redis у `cachekit`,
  MinIO у `storagekit`) boot-тест, smoke-стек и `make local-deps-up` поднимают сами (`scripts/tools/kits.sh`); своя
  зависимость сервиса в smoke-стеке — его второй compose-файл test/stack/compose.service.yml.
- golibinfra-first: generic-примитив → golibinfra, не локально (CLAUDE.md § Стиль; обоснование — `docs/reference/architecture.md § Граница с golibinfra`).

### 3. messaging → `docs/stack/messaging.md`
- subject-константы у consumer'ов (`internal/domain/<c>/delivery/nats/dto.go`) — локальные.
- регистрация подписчиков в fx-модуле потребителя.
- outbox-publisher в `UsecaseDeps` домена-источника.
- (детали паттерна — `scaffold` вариант event + rule `events-and-messaging`.)

#### Сервис без событий (`events: none`)

Решение — `events: none` во frontmatter `messaging.md` (`/stack-deep-dive`): сервис ничего не публикует и не слушает,
и только при `identity: external` — домены идентичности публикуют события, поэтому платформу событий снимают после
их prune (`/init-service`, этап 4). Платформа событий уходит из сервиса одним коммитом (`refactor: сервис без событий
— …`):
- граф `cmd/api/main.go`: из `kitConfigs` — `natskit.NewConfig`, `outboxkit.NewConfig`, `outboxpg.NewConfig`; из
  модулей — `natskit.Module("")`, модуль событий `internal/platform/events` и проба `natskit.ProvideProbe("")`;
  `checkStopBudget` — без `natskit.Config` (бюджет — только HTTP);
- пути `.template/events` (`internal/platform/events`) — `git rm`; источников outbox в `cmd/migrate/sources` нет;
- `.env.example` — без `NATS_*` и `OUTBOX_*` с их комментариями; тесты графа (`cmd/api/main_test.go`: что граф даёт
  доменам, бюджет остановки) — без шины и outbox; boot-тест, smoke-стек и `make local-deps-up` NATS не поднимают:
  он зависимость кита `natskit` (`scripts/tools/kits.sh`);
- мутации платформы событий (`EV*`, блок пробы NATS) — из `scripts/tools/mutants.txt`; харнес `test/` — без
  `infra/nats.py`, `infra/outbox.py` и фикстуры `nats_collector`;
- `go mod tidy` (уходят модули nats.go, nkeys и nuid), `make check`: `TestEventsPaths` держит решение — путей
  `.template/events` нет, код не знает natskit и outboxkit; проза шаблона о платформе событий сверяется по тегу
  шаблона.

Событие появилось в strategy — платформу событий возвращает skill `scaffold`, вариант event («Сервис без событий»).

### 4. auth → `docs/stack/auth.md`
- authgate-wiring + middleware (`internal/platform/authgate`), authnkit-провайдеры (JWT verifier,
  hasher): при `identity: external` — проверка токенов внешней идентичности (issuer, audience, ключ).
- `identity: own` — домены идентичности, `cmd/seed-admin`, кит сессий и bruteforce guard остаются
  (роли, TTL, пороги — по артефакту); `external` — их удаляет prune после своих доменов (`/init-service`).
- Если стек объявил «без authn» — пропусти и не подключай authgate.

### 5. observability → `docs/stack/observability.md`
- logging-wiring: logkit → stdout (формат/уровень из `LOG_*` env).
- старт-лог `"starting" component/version/commit` (версия и commit — ldflags из `docs/stack/service.md`).
- конвенция логирования ключевых событий (по `Log levels & content` артефакта).

### 6. runtime → `docs/stack/runtime.md` (последним)
- композиция `cmd/api/main.go`: `fx.New(...)` с нужными модулями (домены + platform + kit'ы).
- регистрация subscribers/jobs (то, что messaging/auth подняли).
- порядок и бюджет остановки (`fx.StopTimeout`, `checkStopBudget`): хуки остановки — в китах, порядок —
  `docs/reference/architecture.md § fx`.
- это роллап: то, что предыдущие kinds объявили, здесь собирается в один бинарь.

## Принцип

Не сочиняй wiring «по памяти» — `Generates`-секция артефакта это source of truth. Пустую или из
одной заглушки (`TBD…`, `TODO…`) секцию валидатор Gate (`SKILL.md`) не пропускает; размытая («нужный
кит») — не решение: вернись в `stack-deep-dive`.

Каждый kind — отдельный reviewable дифф; после каждого — `go build ./...` зелёный, прежде чем
переходить к следующему. `runtime` закрывает композицию — после него `make build` целиком.
