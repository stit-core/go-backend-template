# Ecosystem

Место шаблона в экосистеме stit: **5 форкаемых шаблонов** `stit-core/`
(`corp-infra-template`, `project-infra-template-compose`, `go-backend-template`,
`spa-frontend-template`, `project-docs-template`) + 2 общие библиотеки
`stit-core/golibinfra` (Go-kits) и `stit-core/ui-lib` (`@stit/ui`,
frontend design-system). Корп control-plane — это репо `corp-infra-template`
(managed = инстанс на `*.stit.tech`; self-hosted = форк `<org>/infra`). Канон
контрактов — `corp-infra-template/docs/ecosystem.md`.

## Роль

Шаблон Go-бэкенда. Форкается в `<client-org>/backend`; разработчик собирает + пушит образ
`git.${DOMAIN}/<client-org>/backend:<vX.Y.Z>`. Деплоем **не занимается** — handoff в
`<client-org>/infra`.

```
   <org>/docs (← project-docs-template)  — design-time SoT: домены продукта
        │ side-input: читается scaffold'ом при разработке (рантайм не зависит)
        ▼
   ЭТОТ РЕПО ◄──контракт (tenant, OTLP, registry)── corp-infra-template (corp control plane)
        │ ──build+push image──► git.${DOMAIN}/<org>/backend{,-migrate}
        │                                   │
   golibinfra (kit'ы)                       ▼
                              <org>/infra (← project-infra-template-compose) ──deploy──► VPS
```

## Зависит от / используется

- **Зависит от:** `stit-core/golibinfra` (kit'ы: telemetry, httpkit, pgkit, authnkit,
  outboxkit, natskit, …); `corp-infra-template` (контракт `service_namespace == tenant`, OTLP
  endpoint, registry); `git.${DOMAIN}` (push images).
- **Используется:** `<client-org>/infra` (форк `project-infra-template-compose`) — пуллит
  образ через `vault kv patch …/meta backend_tag=<tag>` + `make ship` на проектной VPS.
- **Зависит (design-time) от:** `<client-org>/docs` (форк `project-docs-template`) — DDD-артефакты
  доменов продукта (`docs/projects/<slug>/domains/`). `scaffold` читает их при разработке кода;
  рантайм-образ от docs-репо не зависит (это source-of-truth проектирования, не deploy-звено).

## Контракт с корп-инфрой

- `git.${DOMAIN}` — registry для `<org>/backend{,-migrate}` (build локальный, см.
  [release.md](release.md)).
- `vault.${DOMAIN}` — secrets backend, AppRole `projects-<p>-<env>` (per env, flat-дефис;
  `corp-infra-template` → `scripts/onboard-project.sh`).
- `otel.${DOMAIN}` — OTLP ingest (Bearer = `OTEL_TOKEN`). Tenant определяется на edge
  middleware'ом `tenant-auth` по токену → `X-Scope-OrgID` → resource attr `tenant`.
  `service.namespace` приложения перезатирается на actual tenant (корп-инвариант
  `service_namespace == tenant`, см. `corp-infra-template/docs/ecosystem.md` § Tenant identity).
- Корп-дашборды на golibinfra-метриках работают автоматически — достаточно выставить
  `service.name`/`service.version`/`deployment.environment.name`; `tenant` ставит infra.
- Деплой — через `project-infra-template-compose` (docker-compose стек на проектный VPS).

## Self-hosted путь

Эндпоинты выше (`vault.${DOMAIN}`, `otel.${DOMAIN}`, `git.${DOMAIN}`) на managed-пути
указывают на хостинг stit (`*.stit.tech`); на self-hosted — на ваш форк
`corp-infra-template`. Адреса задаются через `OTEL_EXPORTER_OTLP_ENDPOINT`, `VAULT_ADDR`,
`REGISTRY_HOST` в `.env` (см. `.env.example`) — архитектура кода при этом не меняется,
меняется только хост. Вендор-лока нет.

## Границы ответственности

Этот репо STOP'ает на image-in-registry. Деплой-команды чужих репо (`make ship`, `ssh`,
`vault kv patch`) отсюда **не запускаются**. Инфра-поверхность (env/секрет/миграция-с-
эффектом/cross-service-subject/runtime-dep) фиксируется INFRA-HANDOFF-блоком — см.
`.claude/rules/infra-impact.md` и skill `infra-handoff`.
