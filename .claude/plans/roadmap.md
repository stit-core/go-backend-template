# Roadmap

Индекс активных планов. Каждый детальный план — в `<slug>.md` рядом. Управляется skill'ом `plan`.

## Active

—

## Recently done

- kit-store-port-refactor — done 2026-05-12 — `bruteforcekit`/`auditkit`/`outboxkit` расщеплены на core (interfaces+Config) + `*/pgstore` (Store+`MigrationsFS`+Module), `pgkit.MigrateUpMulti` принимает упорядоченный список `Source{Name,FS,Prefix}`, `cmd/migrate` Sources: 4 источника append-only (bruteforce → audit → outbox → app), `internal/jobs/bruteforce.go` делегирует cleanup в `Guard.Cleanup` вместо raw SQL, дубли DDL удалены из `migrations/0002-0004`, `outbox` теперь partitioned (kit-owned). golibinfra `v0.18.0→v0.19.0` (breaking minor). 7/9 атомов в этом репо — фаза 3 (A8-A9, перенос в `ROC/admin-panel-backend` + unblock `cleanup-and-iam`) отложена по запросу юзера. Re-smoke зелёный на v0.19.0 (`schema_migrations.version=5`, outbox→NATS→verification chain отрабатывает за 1с = `OUTBOX_INTERVAL`).
- rename-stit-and-audit-docs — done 2026-05-12 — module path `akadmin/go-backend-template`→`stit/go-backend-template` (51 .go + go.mod + dev-tooling configs + 3 skills + 1 rule) и сквозной аудит docs/skills/rules/agents/domains: убрана фикция про `cmd/worker/` + `internal/worker/subscribers/` + `make build-worker` + `image-worker` (реально всё в одном `cmd/api/` через fx.Lifecycle), переписан правило про subscribers на in-domain `delivery/nats/`, актуализированы номера миграций (0003_audit, 0004_outbox), убраны stale `EmailVerificationIssuer` / `.claude/research/`. 12 atoms через 2 коммита (`chore(module): rename` + `docs(audit): align`).
- telemetry-otlp-logs — done 2026-05-12 — app→OTLP-only logger через logkit/otelslog bridge, prod-like smoke c otel-agent sidecar, tenant-per-env, end-to-end в corp Loki (`{service_name="backend",deployment_environment_name="smoke",service_namespace="roc-dev"}`). 12 atoms через 2 репо (`golibinfra` + этот) + handoff в `stit/infra` для verification logs-leg в otel-collector (`stit/infra/.claude/plans/observability-logs-leg-verification.md`).

## Backlog (idea state)

- Smoke `otel-agent` config — переименовать `otlphttp/corp` → `otlp_http/corp` (deprecated alias). Синхронно с тем же в `stit/project-infra-template-compose/configs/otel-agent/config.yaml`.
