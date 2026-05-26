---
id: stack.observability
kind: observability
name: observability
status: draft
---

# Observability — телеметрия

## Telemetry mode
<Режим: local dev (`OTEL_STDOUT_ONLY=true`) vs production (OTLP + Bearer `OTEL_TOKEN`).
Что эмитится автоматически из golibinfra (HTTP/DB/NATS/outbox/auth метрики, логи, трейсы).>

## Resource attributes
<Что выставляет приложение: `service.name`, `service.version`,
`deployment.environment.name`. `tenant`/`service.namespace` перезатирает корп-edge
(инвариант `service_namespace == tenant`).>

## Business metrics
<Конвенция `biz_<domain>_<event>_{total|seconds}` (автодискавер на Grafana
`Apps/business`). Какие biz-метрики у сервиса.>

## Trace points
<Где открываются span'ы: HTTP-входы (otelhttp), исходы (httpclient), NATS (header
propagation), PG (otelpgx), domain usecase'ы (`telemetry.Span`).>

## Generates
<Что de-template родит:
- telemetry-wiring (exporters по режиму)
- bizmetric-конвенция + примеры `metrics.go`
- resource attrs из service-артефакта>
