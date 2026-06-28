# Observability

Шаблон эмитит **метрики, логи, трейсы** через OTel в корп OTLP-ingest (auth bearer =
`OTEL_TOKEN`; managed-адрес `otel.stit.tech`, self-hosted — свой через
`OTEL_EXPORTER_OTLP_ENDPOINT`). Все три сигнала коррелируются через `trace_id`.
Telemetry-режим задаётся артефактом `docs/stack/observability.md`.

## Режимы telemetry

`golibinfra/telemetry`:
- **Local dev**: метрики/трейсы идут в OTLP; без локального коллектора
  (`OTEL_EXPORTER_OTLP_ENDPOINT`) экспорт молча дропается. Логи всегда в stdout
  (`LOG_STDOUT=true` по умолчанию) — видны без коллектора.
- **Production**: `OTEL_TENANT` + `OTEL_TOKEN` непустые → `Authorization: Bearer <token>`
  на каждый OTLP-запрос. Resource attrs приложения: `service.name`, `service.version`,
  `deployment.environment.name`. `tenant` и `service.namespace` ставит/перезатирает
  корп-сторона (edge `tenant-auth` мапит Bearer→`X-Scope-OrgID`, otel-collector →
  `resource/service_namespace`; инвариант `service_namespace == tenant`, см.
  `corp-infra-template/docs/ecosystem.md` § Tenant identity).

Корп-сторона маршрутизирует по `X-Scope-OrgID` (не по `service.namespace`), но
логи/метрики/трейсы получают `service_namespace=<tenant>` автоматически — фильтр в Loki
(`{service_namespace="<tenant>"}`) и Tempo (`{resource.service.namespace="<tenant>"}`).
Production-значения в репо не попадают — их выдаёт корп-onboarding и рендерит vault-agent.

## Что эмитится автоматически (golibinfra ≥ v0.21.2)

**Process & runtime** — `runtime.go.goroutines`, `runtime.go.mem.heap_alloc/inuse`,
`runtime.go.gc.duration`, `process.*`.

**HTTP сервер** (`httpkit`): `http_requests_total{method,path,status}`,
`http_request_duration_seconds`, `http_panics_total{path}`,
`http_max_body_rejections_total{path}`, `http_idempotency_total{path,result}`.

**HTTP клиент** (`httpclient`): `http_client_request_duration_seconds{host,method,status}`,
`http_client_breaker_transitions_total`, `http_client_breaker_open{name}`,
`http_client_retry_attempts_total{host}`.

**Postgres** (`pgkit`): `db_pool_{acquired,idle,total,max,constructing}_conns`,
`db_pool_{empty,canceled,acquire}_total`, `db_pool_acquire_duration_seconds_total`; span на
каждый query (`otelpgx`).

**NATS** (`natskit`): `nats_messages_{published,consumed}_total{subject,status,transport}`,
`nats_{publish,handle}_duration_seconds`, `nats_inflight_messages`; producer/consumer span'ы
с W3C trace-propagation через `nats.Header`.

**Outbox** (`outboxkit`): `outbox_events_{inserted,processed}_total{subject,status}`,
`outbox_events_dead_total{subject}`, `outbox_batch_{size,duration_seconds}`,
`outbox_publish_duration_seconds`; `outbox_lag_seconds{subject}` и
`outbox_pending_count{subject}` — opt-in (если приложение подключило `BacklogReporter`).

**Auth** (`authnkit`): `jwt_verifications_total{result}`, `password_hash_duration_seconds`,
`password_verify_duration_seconds`, `totp_verifications_total{result}`.

**Логи** (`logkit`): структурированный slog → OTLP → Loki, `trace_id`/`span_id` в каждом
сообщении; `log_messages_total{level}` (errors/min без LogQL-парсинга).

## Business-метрики

Конвенция `biz_<domain>_<event>_{total|seconds}` — автодискавер на Grafana-дашборде
`Apps/business` через `{__name__=~"biz_.*"}`. Добавить свою:

1. `internal/domain/<x>/metrics.go`: `bizmetric.Counter(m, "<domain>", "<event>", ...)` в
   `NewMetrics(m metric.Meter)`; `Record*`-методы nil-safe (`if m == nil { return }`).
2. Зарегистрировать `NewMetrics` в `module.go` (`fx.Provide`).
3. Добавить в `UsecaseDeps`, вызывать `Record*` в нужных точках.

Канонический пример — `internal/domain/auth/metrics.go`.

## Trace-context propagation

- HTTP-входы → `otelhttp.NewHandler` (server-span); исходы → `otelhttp.NewTransport`.
- NATS → trace-context в `nats.Header` (`traceparent`/`tracestate`).
- Postgres → `otelpgx` span на каждый Query/Exec.
- Domain usecase'ы → `telemetry.Span(ctx, "domain.usecase.method")`.

## Где смотреть (Grafana)

Корп-Grafana (`grafana.stit.tech`), папка **Apps/** — общие дашборды для всех
golibinfra-сервисов, фильтр по `$tenant`/`$service`/`$env`: Service Overview, HTTP, Database,
Bus, Outbox, Logs, Traces, Business, Auth. В шапке: `tenant=<project>`, `service=backend`,
`env=<env>`.

Retention: logs/traces — 7 дней, metrics — 90 дней (задаётся на корп control-plane
`corp-infra-template`).

## Project-specific дашборды (опц.)

Domain-specific funnel'и, не покрытые `Apps · Business` — **PR в корп control-plane
(`corp-infra-template`)** под `configs/grafana/dashboards/Tenants/<project>/` (не в этот
репо). Скопировать `_skeleton.json.example`, заменить `<PROJECT>`/`<TOPIC>`, добавить
panels; provisioner подхватит ≤60 сек. Гайд —
`corp-infra-template/docs/observability-app-guide.md § 8`.
