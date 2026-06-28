# de-template › doc-driven wiring

Шесть `Generates`-секций, которые движок разворачивает как **reviewable diff поверх скелета**
(в отличие от детерминированной механики Phase A). Каждый kind: прочитай его `Generates` в
`docs/stack/<kind>.md`, наложи дифф в названном месте, покажи юзеру на ревью. Порядок —
фундамент → роллап (зеркалит волны `stack-deep-dive`); `runtime` последним, он стягивает остальное.

## Порядок и маппинг

### 1. persistence → `docs/stack/persistence.md`
- pgkit pool + transactor в fx (обычно `internal/platform` или модуль-провайдер).
- база `sqlc.yaml` (per-domain блоки добавляет `scaffold` domain — здесь только общий каркас).
- `cmd/migrate` + порядок накатки app+kit миграций.

### 2. dependencies → `docs/stack/dependencies.md`
- kit-импорты + fx-провайдеры **только нужных** kit'ов golibinfra; прунь неиспользуемые из `go.mod`.
- wiring внешних runtime-deps (Redis/NATS/…), если стек их объявил.
- golibinfra-first: generic-примитив → golibinfra, не локально (CLAUDE.md § Стиль; обоснование — `docs/reference/architecture.md § Граница с golibinfra`).

### 3. messaging → `docs/stack/messaging.md`
- subject-константы у consumer'ов (`internal/domain/<c>/delivery/nats/dto.go`) — локальные.
- регистрация подписчиков в fx-модуле потребителя.
- outbox-publisher в `UsecaseDeps` домена-источника.
- (детали паттерна — `scaffold` вариант event + rule `events-and-messaging`.)

### 4. auth → `docs/stack/auth.md`
- authgate-wiring + middleware (`internal/platform/authgate`).
- authnkit-провайдеры (JWT verifier, hasher).
- `cmd/seed-admin` (если форк его сохраняет под свой домен).
- bruteforce guard + cleanup-job.
- Если стек объявил «без authn» — пропусти и не подключай authgate.

### 5. observability → `docs/stack/observability.md`
- telemetry-wiring: OTLP-exporters (dev — без коллектора дропается; prod — +Bearer); логи в stdout по `LOG_STDOUT`.
- resource attrs (service.name/version/commit) из `docs/stack/service.md`.
- bizmetric-конвенция + примеры `metrics.go` (по trace-points артефакта).

### 6. runtime → `docs/stack/runtime.md` (последним)
- композиция `cmd/api/main.go`: `fx.New(...)` с нужными модулями (домены + platform + kit'ы).
- регистрация subscribers/jobs (то, что messaging/auth подняли).
- shutdown-хуки (graceful drain outbox, close pool).
- это роллап: то, что предыдущие kinds объявили, здесь собирается в один бинарь.

## Принцип

Не сочиняй wiring «по памяти» — `Generates`-секция артефакта это source of truth. Если в ней
`TBD`/пусто — артефакт не `stable`, движок не должен был стартовать (см. Gate в `SKILL.md`):
вернись в `stack-deep-dive`.

Каждый kind — отдельный reviewable дифф; после каждого — `go build ./...` зелёный, прежде чем
переходить к следующему. `runtime` закрывает композицию — после него `make build` целиком.
