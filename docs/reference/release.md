# Release

**Зона ответственности этого репо: код → image в registry. На этом — STOP.** Выкатка,
прокатка env, проверка на проде — хаб `<org>/infra` (см. [ecosystem.md](ecosystem.md)). Skill — `/release`.

## Одна команда

На чистом `main` после зелёного `make check`:

```
make release VERSION=vX.Y.Z
```

`scripts/release.sh` одним проходом: confirm-промпт → annotated tag (rich changelog —
`git tag -a` заранее) → `release-preflight` (ветка/clean/semver/buildx + `make doctor`) →
`docker buildx --push` двух образов lockstep:
- `<host>/<org>/backend:vX.Y.Z` — `cmd/api` (runtime)
- `<host>/<org>/backend-migrate:vX.Y.Z` — `cmd/migrate` + вшитые SQL (init-контейнер)

→ агрегированный INFRA-HANDOFF → STOP.

`<host>`/`<org>` берутся из `git remote get-url origin` — image-path следует за местом
репо в Gitea, без правок при форке. Build локальный (без gitea-runner'ов).
`docker login <host>` — креды оператора; `make doctor` в preflight ловит их отсутствие
fail-fast. Lightweight / `:latest` / `:edge` / тег не на HEAD отвергаются by design.

## INFRA-HANDOFF

`make release` печатает агрегированный блок (`BACKEND_TAG`, накопленные разделы
`## Handoff` активных планов `.claude/plans/` и команду выкатки `make bump`) —
что передать в хаб `<org>/infra`. Сам репо `make bump` / `make ship` / `ssh` **не выполняет**.

## Handoff (другие репо)

| Зона | Где | Что делает |
|---|---|---|
| Проект | `<org>/infra` (← project-infra-template-compose) | `make bump SVC=backend TAG=vX.Y.Z` — правит `BACKEND_TAG` в `.env` на VPS + force-recreate, compose пуллит образ (канон rollout'а; `make ship` там — про доставку самого infra-репо) |
| Verify | там же | `make ps`, лог `"starting" component=api version=vX.Y.Z commit=<sha>` |

## Откат

Image уже в registry — пересборка не нужна. Полностью в infra-репо:
`make bump SVC=backend TAG=<предыдущий>`.

## Versioning

Версия и commit зашиваются в бинарь через `-ldflags` (`-X main.version=$TAG -X
main.commit=$GIT_SHA`). На старте бинарь логирует `"starting" component=<api|migrate>
version=vX.Y.Z commit=<short-sha>` — на проде сразу виден крутящийся коммит без map'а
tag↔SHA. Схема версий и бинари — артефакт `docs/stack/service.md`.
