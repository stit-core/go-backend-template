# Ecosystem

Место шаблона в экосистеме stit: **5 форкаемых шаблонов** `stit-core/`
(`corp-infra-template`, `project-infra-template-compose`, `go-backend-template`,
`spa-frontend-template`, `business-strategy-template`) + 2 общие библиотеки
`stit-core/golibinfra` (Go-kits) и `stit-core/ui-lib` (`@stit/ui`,
frontend design-system). Корп control-plane — это репо `corp-infra-template`
(managed = инстанс на `*.stit.tech`; self-hosted = форк `<org>/infra`). Канон
контрактов — `corp-infra-template/docs/ecosystem.md`.

## Роль

Шаблон Go-бэкенда. Форкается в `<client-org>/backend`; разработчик собирает + пушит образ
`git.${DOMAIN}/<client-org>/backend:<vX.Y.Z>`. Деплоем **не занимается** — handoff в
`<client-org>/infra`.

```
   <org>/strategy (← business-strategy-template)  — design-time SoT: бизнес-стратегия;
        │ домены продукта (опц.) — её downstream под docs/projects/<slug>/
        │ side-input: читается scaffold'ом при разработке (рантайм не зависит)
        ▼
   ЭТОТ РЕПО ◄──контракт (image registry)── corp-infra-template (corp control plane)
        │ ──build+push image──► git.${DOMAIN}/<org>/backend{,-migrate}
        │                                   │
   golibinfra (kit'ы)                       ▼
                              <org>/infra (← project-infra-template-compose) ──deploy──► VPS
```

## Зависит от / используется

- **Зависит от:** `stit-core/golibinfra` (kit'ы: httpkit, pgkit, authnkit, outboxkit,
  natskit, logkit, …); `corp-infra-template` (контракт: image registry `git.${DOMAIN}`);
  `git.${DOMAIN}` (push images).
- **Используется:** `<client-org>/infra` (форк `project-infra-template-compose`) — пуллит
  образ, выставив `BACKEND_TAG=<tag>` в своём `.env`/compose + `make ship` на проектной VPS.
- **Зависит (design-time) от:** `<client-org>/strategy` (форк `business-strategy-template`) —
  design-time SoT бизнес-стратегии; когда стратегия рождает продукт, его DDD-артефакты доменов
  живут под `docs/projects/<slug>/domains/`. `scaffold` читает их при разработке кода; рантайм-образ
  от strategy-репо не зависит (это source-of-truth проектирования, не deploy-звено).

## Контракт с корп-инфрой

- `git.${DOMAIN}` — registry для `<org>/backend{,-migrate}` (build локальный, см.
  [release.md](release.md)). Единственная runtime-зависимость этого репо от корп-инфры.
- Конфиг и секреты (`PG_DSN`, JWT/OIDC-ключи, …) — обычные env-переменные; в проде их
  инжектит проектная инфра прямо в окружение контейнера. Схема ключей — артефакт
  `docs/stack/config.md`; прокатка значений — зона `<org>/infra`, не этого репо.
- Деплой — через `project-infra-template-compose` (docker-compose стек на проектный VPS).

## Self-hosted путь

Registry `git.${DOMAIN}` на managed-пути указывает на хостинг stit (`git.stit.tech`); на
self-hosted — на вашу Gitea (тот же хост, что в module path `go.mod`). Хост выводится из
`git remote get-url origin` при `make release` — правок при форке не требует. Архитектура
кода не меняется, меняется только хост. Вендор-лока нет.

## Границы ответственности

Этот репо STOP'ает на image-in-registry. Деплой-команды чужих репо (`make ship`, `ssh`,
env-прокатка) отсюда **не запускаются**. Инфра-поверхность (env/секрет/миграция-с-
эффектом/cross-service-subject/runtime-dep) фиксируется INFRA-HANDOFF-блоком — см.
`.claude/rules/infra-impact.md` и skill `infra-handoff`.
