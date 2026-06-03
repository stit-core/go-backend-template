# Ecosystem

Место шаблона в экосистеме stit.tech из 5 репо (4 runtime/control-plane + design-time
docs-репо). Канон — `stit-infra/infra/docs/ecosystem.md`.

## Роль

Шаблон Go-бэкенда. Форкается в `<client-org>/backend`; разработчик собирает + пушит образ
`git.stit.tech/<client-org>/backend:<vX.Y.Z>`. Деплоем **не занимается** — handoff в
`<client-org>/infra`.

```
   <org>/docs (← project-docs-template)  ← design-time SoT: домены продукта
        │ читается scaffold'ом при разработке (рантайм от него не зависит)
        ▼
stit-infra/infra (corp control plane)
        ▲ контракт (tenant, OTLP, registry)
        │
   ЭТОТ РЕПО ──build+push image──► git.stit.tech/<org>/backend{,-migrate}
        │                                   │
   golibinfra (kit'ы)                       ▼
                              <org>/infra (← project-infra-template-compose) ──deploy──► VPS
```

## Зависит от / используется

- **Зависит от:** `stit-infra/golibinfra` (kit'ы: telemetry, httpkit, pgkit, authnkit,
  outboxkit, natskit, …); `stit-infra/infra` (контракт `service_namespace == tenant`, OTLP
  endpoint, registry); `git.stit.tech` (push images).
- **Используется:** `<client-org>/infra` (форк `project-infra-template-compose`) — пуллит
  образ через `vault kv patch …/meta backend_tag=<tag>` + `make ship` на проектной VPS.
- **Зависит (design-time) от:** `<client-org>/docs` (форк `project-docs-template`) — DDD-артефакты
  доменов продукта (`docs/projects/<slug>/domains/`). `scaffold` читает их при разработке кода;
  рантайм-образ от docs-репо не зависит (это source-of-truth проектирования, не deploy-звено).

## Контракт со stit-инфрой

- `git.stit.tech` — registry для `<org>/backend{,-migrate}` (build локальный, см.
  [release.md](release.md)).
- `vault.stit.tech` — secrets backend, AppRole `project-<p>` (`stit-infra/infra` →
  `scripts/onboard-project.sh`).
- `otel.stit.tech` — OTLP ingest (Bearer = `OTEL_TOKEN`). Tenant определяется на edge
  middleware'ом `tenant-auth` по токену → `X-Scope-OrgID` → resource attr `tenant`.
  `service.namespace` приложения перезатирается на actual tenant (ADR-0004 §2 — инвариант
  `service_namespace == tenant`).
- Корп-дашборды на golibinfra-метриках работают автоматически — достаточно выставить
  `service.name`/`service.version`/`deployment.environment.name`; `tenant` ставит infra.
- Деплой — через `project-infra-template-compose` (docker-compose стек на проектный VPS).

## Границы ответственности

Этот репо STOP'ает на image-in-registry. Деплой-команды чужих репо (`make ship`, `ssh`,
`vault kv patch`) отсюда **не запускаются**. Инфра-поверхность (env/секрет/миграция-с-
эффектом/cross-service-subject/runtime-dep) фиксируется INFRA-HANDOFF-блоком — см.
`.claude/rules/infra-impact.md` и skill `infra-handoff`.
