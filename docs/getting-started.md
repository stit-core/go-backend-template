# Getting started — от шаблона к своему проекту

**[META]** Этот файл — для потребителя шаблона. Описывает путь от форка до проекта,
который ничего не помнит про шаблон. Удаляется на шаге де-темплейтизации.

Принцип: **не правь фундамент руками — сначала спроектируй, потом дай скиллам развернуть.**
Документация в `docs/` первична; код вторичен и разворачивается по ней.

## 0. Форк

1. Gitea → `go-backend-template` → **Use this template** → `<org>/<service>`.
2. `git clone git@git.sftbmb.ru:<org>/<service>.git && cd <service>`.
3. Один раз на машину — доступ к приватным модулям той же Gitea:
   `git config --global url.git@git.sftbmb.ru:.insteadOf https://git.sftbmb.ru/`.
4. `make setup` — pinned dev-tools + git-хуки.

> `git.sftbmb.ru` здесь — managed-хост Gitea. Self-hosted форк живёт на своей Gitea:
> подставь её хост в clone-URL и в `insteadOf`-rewrite (это тот же хост, что в module
> path `go.mod`). `make setup` выводит хост из `go.mod` автоматически и предупредит,
> если rewrite не настроен.

## 1. `/init-service`

Скилл-оркестратор ведёт весь онбординг: домены — **готовый вход** из соседнего docs-репо;
он проектирует стек, скаффолдит домены в код и в конце де-темплейтит + прунит reference.
Дальше шаги 2–7 — под его управлением.

## 2. Домены — уже спроектированы (в соседнем docs-репо)

- Домены продукта проектируются **не здесь и не на этом шаге** — они уже готовы к моменту форка
  бэкенда. DDD-артефакты живут в форке `<org>/docs` (из `project-docs-template`), склонированном
  рядом (`../docs`): `docs/projects/<slug>/domains/<ctx>/`.
- Тебе нужны они в `status: stable` как **вход**. Если какой-то домен ещё не доведён — это
  делается в docs-репо (`/domain-deep-dive <slug> <ctx>`) до онбординга бэка, не здесь.
- Reference-домены `auth` / `user` / `verification` в **этом** репо — **code-only** учебные
  примеры (артефактов в docs у них нет). Изучи как образец Go-раскладки; удаление — шаг 6.

## 3. Спроектируй стек — `/stack-deep-dive`

- 8 артефактов в `docs/stack/` (service/runtime/config/persistence/messaging/auth/
  observability/dependencies). См. [stack/README.md](stack/README.md).
- В `service.md` запиши `project-slug` + `docs-path` (дефолт `../docs`) — по ним `scaffold`
  найдёт домены в docs-репо.
- Цель: `status: stable` + `make validate-stack-docs` зелёный. Это вход для де-темплейтизации.

## 4. Разверни фундамент — `/de-template`

- Читает stable-артефакты `docs/stack` и разворачивает фундамент по секциям
  `Generates`: module path, README, `internal/platform/*`, `cmd/api` композиция, `go.mod`,
  `.env.example`, нужные kit'ы golibinfra. Домены он не трогает — только платформу.
- Гибрид: механика (module path, README, env, prune, strip шаблонной мета) —
  детерминирована; platform/main wiring — reviewable diff поверх скелета.

## 5. Сгенерируй свои домены — `/scaffold` (вариант domain)

По каждому stable-домену из docs-репо разверни Go-код: `/scaffold domain <name>` читает
`../docs/projects/<slug>/domains/<ctx>/` (slug/docs-path из `docs/stack/service.md`) и
разворачивает слои `internal/domain/<ctx>/`. Маппинг секций артефакта → код —
[reference/domain-mapping.md](reference/domain-mapping.md). Делай это **до** удаления reference
(шаг 6) — иначе теряешь живой Go-образец.

## 6. Удали reference-домены

Когда свои домены сгенерированы (шаг 5), убери учебные `auth`/`user`/`verification`:
- `internal/domain/{auth,user,verification}/` (DDD-артефактов в docs у них нет — code-only).
- блоки в `sqlc.yaml`, их `migrations/`, fx-wiring в `cmd/api`, depguard-правила в `.golangci.yml`.
- их e2e-сценарии в `test/`.
- `/de-template` подсказывает/выполняет этот чеклист.

## 7. Готово

Проект больше ничего не помнит про шаблон: свой module path, README, свои домены, стек, deps.
Домены продукта остаются в соседнем docs-репо; этот репо — их потребитель. Дальше — обычный
дневной флоу через `/smartdev` (см. `docs/contributing.md` про устройство `.claude`-системы).
Релиз образов — `docs/reference/release.md`.
