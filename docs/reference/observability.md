# Observability

Шаблон эмитит **метрики, логи, трейсы** через OTel в `otel.stit.tech` (auth bearer =
`OTEL_TOKEN`). Все три сигнала коррелируются через `trace_id`. Telemetry-режим задаётся
артефактом `docs/stack/observability.md`.

## Режимы telemetry

`golibinfra/telemetry`:
- **Local dev** (дефолт `.env.example`): `OTEL_STDOUT_ONLY=true` — всё в stdout. Или
  локальный коллектор через `OTEL_EXPORTER_OTLP_ENDPOINT`.
- **Production**: `OTEL_TENANT` + `OTEL_TOKEN` непустые → `Authorization: Bearer <token>`
  на каждый OTLP-запрос. Resource attrs приложения: `service.name`, `service.version`,
  `deployment.environment.name`. `tenant` и `service.namespace` ставит/перезатирает
  корп-otel-collector на edge (Bearer→`X-Scope-OrgID`→`resource/service_namespace`;
  инвариант `service_namespace == tenant`, см. `stit-infra/infra` ADR-0004 §2).

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
`outbox_batch_{size,duration_seconds}`, `outbox_publish_duration_seconds{subject}`,
`outbox_lag_seconds{subject}` (если приложение подключило `LagReporter`).

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

Retention: logs/traces — 7 дней, metrics — 90 дней (`stit-infra/infra` ADR-0004).

## Project-specific дашборды (опц.)

Domain-specific funnel'и, не покрытые `Apps · Business` — **PR в `stit-infra/infra`** под
`configs/grafana/dashboards/Tenants/<project>/` (не в этот репо). Скопировать
`_skeleton.json.example`, заменить `<PROJECT>`/`<TOPIC>`, добавить panels; provisioner
подхватит ≤60 сек. Гайд — `stit-infra/infra/docs/observability-app-guide.md § 8`.
