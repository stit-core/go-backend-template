# Getting started — от шаблона к своему проекту

**[META]** Этот файл — для потребителя шаблона. Описывает путь от форка до проекта,
который ничего не помнит про шаблон. Удаляется на шаге де-темплейтизации.

Принцип: **не правь фундамент руками — сначала спроектируй, потом дай скиллам развернуть.**
Документация в `docs/` первична; код вторичен и разворачивается по ней.

## 0. Форк

1. Gitea → `go-backend-template` → **Use this template** → `<org>/<service>`.
2. `git clone git@git.stit.tech:<org>/<service>.git && cd <service>`.
3. Один раз на машину — доступ к приватным модулям той же Gitea:
   `git config --global url.git@git.stit.tech:.insteadOf https://git.stit.tech/`.
4. `make setup` — pinned dev-tools + git-хуки.

> Хост в clone-URL и в `insteadOf`-rewrite — Gitea, где живёт орга (тот же хост, что в
> module path `go.mod`). `make setup` берёт хост из `go.mod` и предупреждает, если rewrite
> не настроен.

## 1. `/init-service`

Скилл-оркестратор ведёт весь онбординг: домены — **готовый вход** из соседнего strategy-репо;
он ведёт проектирование стека, код доменов и в конце де-темплейт с удалением reference.
Дальше шаги 2–7 — под его управлением.

## 2. Домены — уже спроектированы (в соседнем strategy-репо)

- Домены продукта проектируются **не здесь и не на этом шаге** — они уже готовы к моменту форка
  бэкенда. DDD-артефакты живут в форке `<org>/strategy` (из `business-strategy-template`),
  склонированном рядом (`../strategy`), внутри сервиса, который реализует этот бэкенд:
  `docs/projects/<slug>/services/<svc>/domains/<ctx>/`.
- Тебе нужны они в `status: stable` как **вход**. Если какой-то домен ещё не доведён — это
  делается в strategy-репо (`/domain-deep-dive <slug> <ctx>`) до онбординга бэка, не здесь.
- Reference-домены `auth` / `user` / `verification` в **этом** репо — **code-only** учебные
  примеры (артефактов в docs у них нет). Изучи как образец Go-раскладки; удаление — шаг 6.

## 3. Спроектируй стек — `/stack-deep-dive`

- 8 артефактов в `docs/stack/` (service/runtime/config/persistence/messaging/auth/
  observability/dependencies). См. [stack/README.md](stack/README.md).
- В `service.md § Identity` запиши `project-slug`, `service-slug` (сервис этого репо в реестре
  `services/service.md` strategy-репо) и `strategy-path` (дефолт `../strategy`) — по ним `scaffold`
  найдёт домены в strategy-репо.
- Цель: `status: stable` + `make validate-stack-docs` зелёный. Это вход для де-темплейтизации.

## 4. Разверни фундамент — `/de-template`

- Читает stable-артефакты `docs/stack` и разворачивает фундамент по секциям
  `Generates`: module path, README, `internal/platform/*`, `cmd/api` композиция, `go.mod`,
  `.env.example`, нужные kit'ы golibinfra. Домены он не трогает — только платформу.
- Гибрид: механика (module path, README, env, prune, strip шаблонной мета) —
  детерминирована; platform/main wiring — reviewable diff поверх скелета.

## 5. Свои домены — по таблице «Маршрут задачи»

Каждый stable-домен из strategy-репо идёт в Go-код по таблице «Маршрут задачи» (`CLAUDE.md`). Слои
домена разворачивает `/scaffold` (вариант domain): читает
`../strategy/docs/projects/<slug>/services/<svc>/domains/<ctx>/` (slug, сервис и strategy-path — из
`docs/stack/service.md`) и разворачивает `internal/domain/<ctx>/`. Маппинг секций артефакта → код —
[reference/domain-mapping.md](reference/domain-mapping.md). Делай это **до** удаления reference
(шаг 6) — иначе теряешь живой Go-образец.

## 6. Удали reference-домены

Когда свои домены сгенерированы (шаг 5), убери учебные `auth`/`user`/`verification` (полный
чеклист — `.claude/skills/de-template/references/prune.md`, его выполняет `/de-template`):
- `internal/domain/{auth,user,verification}/` (DDD-артефактов в docs у них нет — code-only).
- блоки в `sqlc.yaml` и `.mockery.yml`, их `migrations/` (свои — перенумеровать подряд с `0001`),
  fx-wiring в `cmd/api`.
- их e2e-сценарии в `test/` и `cmd/seed-admin`.

## 7. Готово

Проект больше ничего не помнит про шаблон: свой module path, README, свои домены, стек, deps.
Домены продукта остаются в соседнем strategy-репо; этот репо — их потребитель. Дальше — обычный
дневной флоу через `/smartdev` (см. `docs/contributing.md` про устройство `.claude`-системы).
Релиз образов — `docs/reference/release.md`.
