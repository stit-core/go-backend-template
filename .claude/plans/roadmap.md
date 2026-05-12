# Roadmap

Индекс активных планов. Каждый детальный план — в `<slug>.md` рядом. Управляется skill'ом `plan`.

## Active

—

## Recently done

- telemetry-otlp-logs — done 2026-05-12 — app→OTLP-only logger через logkit/otelslog bridge, prod-like smoke c otel-agent sidecar, tenant-per-env, end-to-end в corp Loki (`{service_name="backend",deployment_environment_name="smoke",service_namespace="roc-dev"}`). 12 atoms через 2 репо (`golibinfra` + этот) + handoff в `stit/infra` для verification logs-leg в otel-collector (`stit/infra/.claude/plans/observability-logs-leg-verification.md`).

## Backlog (idea state)

- Smoke `otel-agent` config — переименовать `otlphttp/corp` → `otlp_http/corp` (deprecated alias). Синхронно с тем же в `stit/project-infra-template-compose/configs/otel-agent/config.yaml`.
