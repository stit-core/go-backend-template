# Roadmap

Индекс активных планов. Каждый детальный план — в `<slug>.md` рядом. Управляется skill'ом `plan`.

## Active

- [kit-store-port-refactor](./kit-store-port-refactor.md) — `bruteforcekit` → core+port+pgstore split, `pgkit.MigrateUpMulti`, удалить копию DDL из template. Разблокирует `ROC/admin-panel-backend` cleanup-and-iam. — `in-progress` 7/9 atoms (фазы 1+2 закрыты, фаза 3 ждёт запроса)

## Recently done

- rename-stit-and-audit-docs — done 2026-05-12 — module path `akadmin/go-backend-template`→`stit/go-backend-template` (51 .go + go.mod + dev-tooling configs + 3 skills + 1 rule) и сквозной аудит docs/skills/rules/agents/domains: убрана фикция про `cmd/worker/` + `internal/worker/subscribers/` + `make build-worker` + `image-worker` (реально всё в одном `cmd/api/` через fx.Lifecycle), переписан правило про subscribers на in-domain `delivery/nats/`, актуализированы номера миграций (0003_audit, 0004_outbox), убраны stale `EmailVerificationIssuer` / `.claude/research/`. 12 atoms через 2 коммита (`chore(module): rename` + `docs(audit): align`).
- telemetry-otlp-logs — done 2026-05-12 — app→OTLP-only logger через logkit/otelslog bridge, prod-like smoke c otel-agent sidecar, tenant-per-env, end-to-end в corp Loki (`{service_name="backend",deployment_environment_name="smoke",service_namespace="roc-dev"}`). 12 atoms через 2 репо (`golibinfra` + этот) + handoff в `stit/infra` для verification logs-leg в otel-collector (`stit/infra/.claude/plans/observability-logs-leg-verification.md`).

## Backlog (idea state)

- Smoke `otel-agent` config — переименовать `otlphttp/corp` → `otlp_http/corp` (deprecated alias). Синхронно с тем же в `stit/project-infra-template-compose/configs/otel-agent/config.yaml`.
- audit/outbox kit refactor по тому же паттерну (core+port+pgstore). На v1 не нужен (admin-panel-backend их выключает), но тех. долг template'а — рано или поздно делать.
