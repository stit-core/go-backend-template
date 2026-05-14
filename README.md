# go-backend-template

Шаблон Go HTTP-бэкенда на стеке stit.tech: huma/v2 + chi + pgx/v5 + sqlc + uber/fx + `golibinfra` (telemetry, authn, outbox, brute-force, cache, NATS, audit, clock).

## Использовать как шаблон

В Gitea этот репо помечен `template: true`. Старт нового сервиса:

1. Открыть https://git.stit.tech/stit/go-backend-template → **Use this template** → создать `<org>/<service>`.
2. Склонировать новый репо локально:
   ```bash
   git clone git@git.stit.tech:<org>/<service>.git && cd <service>
   ```
3. (Один раз на машину) Настроить git и Go под приватные модули `git.stit.tech` —
   `golibinfra` и любые stit-private deps. Без этого `go mod download` для
   шаблона упадёт по auth/proxy:
   ```bash
   git config --global url.git@git.stit.tech:.insteadOf https://git.stit.tech/
   ```
   (`GOPRIVATE` ставится автоматически в `make tools` ниже.)
4. Заменить module path:
   ```bash
   OLD=git.stit.tech/stit/go-backend-template
   NEW=git.stit.tech/<org>/<service>
   grep -rl "$OLD" --exclude-dir=.git | xargs sed -i "s|$OLD|$NEW|g"
   go mod edit -module "$NEW" && go mod tidy
   ```
5. `make tools && make hooks-install && make pre-merge` — убедиться что зелёное.
6. `cp .env.example .env` и заполнить локальные переменные.
7. Первый домен: `make new-domain NAME=<context>` → `docs/domains/<context>/` (см. `docs/domains/README.md`).

Альтернатива через Gitea API:
```bash
curl -X POST -H "Authorization: token $GITEA_TOKEN" -H "Content-Type: application/json" \
  -d '{"owner":"<org>","name":"<service>","private":false}' \
  https://git.stit.tech/api/v1/repos/generate \
  --url-query 'template_owner=stit' --url-query 'template_repo=go-backend-template'
```

## Что внутри

- **HTTP**: huma/v2 поверх chi, OpenAPI 3.1 авто-генерация, structured errors.
- **БД**: pgx/v5 + sqlc (typed queries), миграции через golang-migrate.
- **DI**: uber/fx, lifecycle hooks. Один runtime-бинарь (`cmd/api`) — внутри него HTTP, outbox drain, NATS subscribers и background jobs. Standalone CLI миграций (`cmd/migrate`) собирается отдельным image и крутится init-контейнером.
- **Платформа**: `golibinfra` — OTLP telemetry, JWT/session authn, outbox-pattern publisher, brute-force guard, cache (Redis/in-mem), NATS pub/sub, audit log, clock.
- **DDD**: артефакты доменов в `docs/domains/<context>/` — source of truth перед кодом. См. `docs/domains/MAPPING.md` (DDD ↔ Go).
- **Тесты**: unit (`-short -race`), integration (`//go:build integration` + testcontainers), fuzz, mockery v3.
- **CI gate**: `make pre-merge` = fmt + vet + lint + test + build + sqlc-diff + mocks-check. Гонится pre-push хуком.

## Команды

| Команда | Что делает |
|---|---|
| `make tools` | Pinned dev-инструменты (gofumpt, golangci-lint, sqlc, mockery) |
| `make test` / `make test-int` / `make test-cover` | Unit / integration / coverage |
| `make lint` / `make fmt` | golangci-lint / gofumpt |
| `make build` | Бинарник `bin/api` (единственный runtime) |
| `make sqlc` / `make sqlc-diff` | Регенерация / проверка pgx-кода |
| `make mocks` / `make mocks-check` | Регенерация / проверка mockery v3 моков |
| `make migrate-up` / `make migrate-down` | Миграции (нужен `PG_DSN`) |
| `make new-domain NAME=order` | Скаффолд нового домена |
| `make pre-merge` | Канонический gate перед push |
| `make release-check` / `make release` | Pre-flight + push api/migrate images в registry под lockstep tag |
| `make hooks-install` | Поставить git-хуки |

Полный гайд для агентов и человеков — `AGENTS.md`. Дневной флоу — `.claude/WORKFLOW.md`. Точка входа в сессию — `/smartdev`.

## Структура

```
cmd/{api,migrate}/          точки входа: api — runtime, migrate — standalone CLI миграций
internal/<domain>/          бизнес-логика по доменам (DDD); subscribers — in-domain `delivery/nats/`
internal/platform/          shared infra (authgate, config, identity)
internal/jobs/              background jobs (поднимаются fx.Lifecycle того же cmd/api)
internal/testkit/           pgseed, identitytest helpers
pkg/                        публичные пакеты (policy, tokens)
migrations/                 SQL-миграции (golang-migrate), парные up/down
docs/domains/<context>/     DDD-артефакты — source of truth для домена
.claude/                    skills, rules, agents, plans для Claude Code
scripts/agent-tools/        обёртки с компактным выводом для агентов
```

См. `AGENTS.md §3` — структура расписана подробнее.

## Release

**Зона ответственности этого репо: код → image в registry. На этом — STOP.** Деплой, vault-patch и проверка контейнера на проде — другие репо (см. [Handoff](#handoff) ниже).

Build локальный (никаких gitea-runner'ов), push в `git.stit.tech/<org>/backend{,-migrate}` под одним lockstep `vX.Y.Z`-тегом.

Шаги **в этом репо**:

1. `make pre-merge` — gate (fmt + lint + test + build + sqlc-diff + mocks-check). Должен быть зелёным.
2. Аннотированный semver-tag на чистом `main`:
   ```bash
   git tag -a v1.2.3 -m "feat(user): password reset"
   git push origin v1.2.3
   ```
   `make release` не примет lightweight-тег и тег не на HEAD — это by design (см. `scripts/release-preflight.sh`).
3. `make release` — соберёт и запушит два image'а одним тегом:
   - `git.stit.tech/<org>/backend:v1.2.3` (`cmd/api` — runtime)
   - `git.stit.tech/<org>/backend-migrate:v1.2.3` (`cmd/migrate` + вложенные SQL-файлы, init-контейнер)

   `<host>` и `<org>` берутся из `git remote get-url origin` (host+первая path-компонента) — image-path следует за тем, где репо реально живёт в Gitea, без правок Makefile при копировании шаблона. Перед запуском убедись что залогинен: `docker login git.stit.tech`.

После шага 3 image'ы в registry. **Наша зона ответственности закончилась.** `make release` напечатает next-steps как подсказку оператору.

### Handoff

Дальше работают **другие репо** (этот их не вызывает и не знает их команды):

| Зона | Где | Что делает |
|---|---|---|
| Корп-сторона | `stit/infra` (root-токен Vault, только админ) | `vault kv patch kv/projects/<p>/<env>/meta backend_tag=v1.2.3` |
| Проектная инфра | `<client-org>/infra` (← `stit/project-infra-template-compose`) | `make ship` → ssh-deploy → `vault-agent` рендерит новый `BACKEND_TAG` → `deploy.sh` force-recreate'ит backend-контейнер |
| Verify | там же | `make ps`, логи `"starting" service=api version=v1.2.3 commit=<sha>` |

В инфра-репо есть skill `/request-secret` который формирует stdout-блок `vault kv patch …` для корп-админа out-of-band. Здесь, в backend-репо, мы лишь печатаем подсказку — кому что передать.

### Откат

Image в registry уже лежит — пересборка не нужна. Полностью **в infra-репо**: корп-патч на предыдущий тег + `make ship`.

### Versioning

Версия зашивается в бинарник через `-ldflags` (`-X main.version=$TAG -X main.commit=$GIT_SHA`). На старте каждый сервис логирует `"starting" service=<name> version=v1.2.3 commit=<short-sha>` через `logkit.Logger` — на проде сразу понятно какой коммит крутится, без захода в Gitea за map'ом tag↔SHA.

## Observability

Шаблон эмитит **метрики, логи, трейсы** через OTel в `otel.stit.tech` (auth bearer = `OTEL_TOKEN`). Все три сигнала корелируются через `trace_id`.

### Что эмитится автоматически (из golibinfra ≥ v0.21.2)

**Process & runtime** — `runtime.go.goroutines`, `runtime.go.mem.heap_alloc/inuse`, `runtime.go.gc.duration`, `process.*`.

**HTTP сервер** (`httpkit`):
- `http_requests_total{method,path,status}`
- `http_request_duration_seconds{method,path,status}`
- `http_panics_total{path}` — recovered панки
- `http_max_body_rejections_total{path}` — отказы 413
- `http_idempotency_total{path,result}` — replayed | stored

**HTTP клиент** (`httpclient`):
- `http_client_request_duration_seconds{host,method,status}`
- `http_client_breaker_transitions_total{name,from,to}`, `http_client_breaker_open{name}`
- `http_client_retry_attempts_total{host}`

**Postgres** (`pgkit`):
- `db_pool_{acquired,idle,total,max,constructing}_conns`
- `db_pool_{empty,canceled,acquire}_total`, `db_pool_acquire_duration_seconds_total`
- Span'ы на каждый query (через `otelpgx`)

**NATS** (`natskit`):
- `nats_messages_{published,consumed}_total{subject,status,transport}`
- `nats_{publish,handle}_duration_seconds{subject,transport}`
- `nats_inflight_messages{subject,transport}`
- Producer/Consumer span'ы с W3C trace-propagation через `nats.Header`

**Outbox** (`outboxkit`):
- `outbox_events_{inserted,processed}_total{subject,status}`
- `outbox_batch_{size,duration_seconds}`, `outbox_publish_duration_seconds{subject}`
- `outbox_lag_seconds{subject}` — возраст самого старого unpublished event'а (через `LagReporter` в `outboxkit/pgstore`)

**Auth primitives** (`authnkit`):
- `jwt_verifications_total{result=ok|expired|signature_invalid|malformed}`
- `password_hash_duration_seconds`, `password_verify_duration_seconds`
- `totp_verifications_total{result}`

**Логи** (`logkit`):
- Структурированный slog → OTLP → Loki, `trace_id`/`span_id` инжектятся в каждое сообщение
- `log_messages_total{level}` counter — `errors/min` без LogQL-парсинга

### Business-метрики

Конвенция: `biz_<domain>_<event>_{total|seconds}` — это позволяет автодискаверу на Grafana-дашборде `Apps/business` подхватить новые метрики через `{__name__=~"biz_.*"}` без ручного списка.

Сейчас в шаблоне:

```
biz_users_registered_total{result=ok|weak_password|invalid|error}
biz_users_updated_total{result=ok|forbidden|invalid|error}
biz_users_deleted_total{result=ok|forbidden|error}
biz_auth_login_attempts_total{result=ok|invalid_credentials|locked|blocked}
biz_auth_refresh_total{result=ok|expired|reused|invalid}
biz_auth_logouts_total
biz_auth_password_change_total{result=ok|forbidden|weak|invalid_old}
biz_verification_email_issued_total{result=ok|error}
biz_verification_email_confirmed_total{result=ok|invalid|expired|already_used}
biz_verification_password_reset_requested_total{result=ok|user_not_found|inactive|error}
biz_verification_password_reset_confirmed_total{result=ok|weak|invalid|expired|already_used}
```

### Как добавить свою бизнес-метрику

1. Создай (или расширь) `internal/domain/<x>/metrics.go`:
   ```go
   import "git.stit.tech/stit/golibinfra/bizmetric"

   type Metrics struct {
       myCounter metric.Int64Counter
   }

   func NewMetrics(m metric.Meter) (*Metrics, error) {
       return &Metrics{
           myCounter: bizmetric.Counter(m, "<domain>", "<event>",
               metric.WithDescription("...")),
       }, nil
   }

   func (m *Metrics) RecordSomething(ctx context.Context, result string) {
       if m == nil { return }  // nil-safe для unit-тестов
       m.myCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("result", result)))
   }
   ```
2. Зарегистрируй `NewMetrics` в `module.go` через `fx.Provide`.
3. Добавь в `UsecaseDeps`, инжектируй в usecase, вызывай `Record*` в нужных точках.

См. `internal/domain/auth/metrics.go` как канонический пример.

### Trace-context propagation

- HTTP-входы → `otelhttp.NewHandler` стартует server-span автоматически
- HTTP-исходы (`httpclient`) → `otelhttp.NewTransport` стартует client-span автоматически
- NATS → trace-context в `nats.Header` (`traceparent`/`tracestate`), `natskit.Publish` инжектит, `Subscribe` экстрактит
- Postgres → `otelpgx` создаёт span на каждый Query/Exec, привязанный к parent-span'у из ctx

Domain usecase'ы дополнительно открывают span через `telemetry.Span(ctx, "domain.usecase.method")` для семантической разметки операций.

### Локальная разработка

Если не нужны OTLP-экспортёры, поставь `OTEL_STDOUT_ONLY=true` — все три сигнала пойдут в stdout через `stdoutlog`/`stdoutmetric`/`stdouttrace`. Удобно для дебаггинга `trace_id`/`span_id` корреляций.

## Контракт со stit-инфрой

- `git.stit.tech` — registry для `<org>/backend{,-migrate}` images (build локальный, см. [Release](#release)).
- `vault.stit.tech` — secrets backend, AppRole `project-<p>` (см. `stit/infra` → `scripts/onboard-project.sh`).
- `otel.stit.tech` — OTLP ingest (Bearer-token), tenant routing по `service.namespace`.
- Деплой через `stit/project-infra-template-compose` (docker-compose стек на проектный VPS).
