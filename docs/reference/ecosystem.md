# Ecosystem

Место шаблона в экосистеме sftbmb: 4 форкаемых шаблона `sftbmb-core/`
(`go-backend-template`, `spa-frontend-template`, `project-docs-template`,
`project-infra-template-compose`) + корп control-plane `sftbmb-infra/` (`sftbmb-infra/infra`)
+ lib `sftbmb-core/golibinfra` + дизайн-система `sftbmb-core/ui-lib` (`@sftbmb/ui`). Канон контрактов
— `sftbmb-infra/infra/docs/ecosystem.md`.

## Роль

Шаблон Go-бэкенда. Форкается в `<client-org>/backend`; разработчик собирает + пушит образ
`git.sftbmb.ru/<client-org>/backend:<vX.Y.Z>`. Деплоем **не занимается** — handoff в
`<client-org>/infra`.

```
   <org>/docs (← project-docs-template)  ← design-time SoT: домены продукта
        │ читается scaffold'ом при разработке (рантайм от него не зависит)
        ▼
sftbmb-infra/infra (corp control plane)
        ▲ контракт (tenant, OTLP, registry)
        │
   ЭТОТ РЕПО ──build+push image──► git.sftbmb.ru/<org>/backend{,-migrate}
        │                                   │
   golibinfra (kit'ы)                       ▼
                              <org>/infra (← project-infra-template-compose) ──deploy──► VPS
```

## Зависит от / используется

- **Зависит от:** `sftbmb-core/golibinfra` (kit'ы: telemetry, httpkit, pgkit, authnkit,
  outboxkit, natskit, …); `sftbmb-infra/infra` (контракт `service_namespace == tenant`, OTLP
  endpoint, registry); `git.sftbmb.ru` (push images).
- **Используется:** `<client-org>/infra` (форк `project-infra-template-compose`) — пуллит
  образ через `vault kv patch …/meta backend_tag=<tag>` + `make ship` на проектной VPS.
- **Зависит (design-time) от:** `<client-org>/docs` (форк `project-docs-template`) — DDD-артефакты
  доменов продукта (`docs/projects/<slug>/domains/`). `scaffold` читает их при разработке кода;
  рантайм-образ от docs-репо не зависит (это source-of-truth проектирования, не deploy-звено).

## Контракт со sftbmb-инфрой

- `git.sftbmb.ru` — registry для `<org>/backend{,-migrate}` (build локальный, см.
  [release.md](release.md)).
- `vault.sftbmb.ru` — secrets backend, AppRole `project-<p>` (`sftbmb-infra/infra` →
  `scripts/onboard-project.sh`).
- `otel.sftbmb.ru` — OTLP ingest (Bearer = `OTEL_TOKEN`). Tenant определяется на edge
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
