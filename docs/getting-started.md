# Getting started — от шаблона к своему проекту

**[META]** Этот файл — для потребителя шаблона. Описывает путь от форка до проекта,
который ничего не помнит про шаблон. Удаляется на шаге де-темплейтизации.

Принцип: **не правь фундамент руками — сначала спроектируй, потом дай скиллам развернуть.**
Документация в `docs/` первична; код вторичен и разворачивается по ней.

## 0. Форк

1. Gitea → `go-backend-template` → **Use this template** → `<org>/<service>`.
2. `git clone git@git.stit.tech:<org>/<service>.git && cd <service>`.
3. Один раз на машину — доступ к приватным модулям `git.stit.tech`:
   `git config --global url.git@git.stit.tech:.insteadOf https://git.stit.tech/`.
4. `make setup` — pinned dev-tools + git-хуки.

## 1. `/init-service`

Скилл-оркестратор ведёт весь онбординг: чейнит проектирование доменов и стека, в конце
запускает `/de-template`. Дальше шаги 2–5 — под его управлением.

## 2. Спроектируй домены — `/domain-deep-dive`

- DDD-артефакты в `docs/domains/<context>/` — source of truth бизнес-смысла. См.
  [domains/README.md](domains/README.md).
- Reference-домены `auth` / `user` / `verification` — **учебные примеры**. Изучи их как
  образец, свои проектируй рядом. Удаление reference — шаг 5.
- Цель: `status: stable` + `validate-domain-doc.sh` зелёный.

## 3. Спроектируй стек — `/stack-deep-dive`

- 8 артефактов в `docs/stack/` (service/runtime/config/persistence/messaging/auth/
  observability/dependencies). См. [stack/README.md](stack/README.md).
- Цель: `status: stable` + `validate-stack-doc.sh` зелёный. Это вход для де-темплейтизации.

## 4. Разверни фундамент — `/de-template`

- Читает stable-артефакты `docs/{domains,stack}` и разворачивает фундамент по секциям
  `Generates`: module path, README, `internal/platform/*`, `cmd/api` композиция, `go.mod`,
  `.env.example`, нужные kit'ы golibinfra.
- Гибрид: механика (module path, README, env, prune, strip шаблонной мета) —
  детерминирована; platform/main wiring — reviewable diff поверх скелета.

## 5. Удали reference-домены

Когда свои домены спроектированы и сгенерированы, убери учебные `auth`/`user`/`verification`:
- `internal/domain/{auth,user,verification}/` + их `docs/domains/*`.
- блоки в `sqlc.yaml`, их `migrations/`, fx-wiring в `cmd/api`, depguard-правила в `.golangci.yml`.
- их e2e-сценарии в `test/`.
- `/de-template` подсказывает/выполняет этот чеклист.

## 6. Готово

Проект больше ничего не помнит про шаблон: свой module path, README, домены, стек, deps.
Дальше — обычный дневной флоу через `/smartdev` (см. `docs/contributing.md` про устройство
`.claude`-системы). Релиз образов — `docs/reference/release.md`.
