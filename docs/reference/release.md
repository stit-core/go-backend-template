# Release

**Зона ответственности этого репо: код → image в registry. На этом — STOP.** Выкатка,
прокатка env, проверка на проде — хаб `<org>/infra` (см. [ecosystem.md](ecosystem.md)). Skill — `/release`.

## Одна команда

На чистом `main` после зелёного `make check`:

```
make release VERSION=vX.Y.Z
```

`scripts/release.sh` одним проходом: confirm-промпт → чистое дерево, `make check` и
`make e2e-smoke` → annotated
tag (rich changelog — `git tag -a` заранее) → `release-preflight` (ветка/clean/semver/buildx +
`make doctor`) →
`docker buildx --push` двух образов lockstep:
- `<host>/<org>/backend:vX.Y.Z` — `cmd/api` (runtime)
- `<host>/<org>/backend-migrate:vX.Y.Z` — `cmd/migrate` + вшитые SQL (init-контейнер)

→ агрегированный INFRA-HANDOFF → STOP.

Оба образа — distroless без shell и `wget`, процесс — UID 65532. Проверка здоровья контейнера api —
сам бинарь: `/app healthcheck` (`GET /health` по `HTTP_ADDR`, код выхода 0 — здоров); e2e-стек
ждёт `healthy` по ней же.

`<host>`/`<org>` берутся из `git remote get-url origin` — image-path следует за местом
репо в Gitea, без правок при форке. Build локальный (без gitea-runner'ов).
`docker login <host>` — креды оператора; `make doctor` в preflight ловит их отсутствие
fail-fast. Lightweight / `:latest` / `:edge` / тег не на HEAD отвергаются by design.

## INFRA-HANDOFF

`make release` печатает агрегированный блок (`BACKEND_TAG`, накопитель `.claude/plans/handoff.md`
с разделами `## Handoff` активных планов и команду выкатки `make bump`) — что передать в хаб
`<org>/infra` — и очищает накопитель коммитом. Заранее, без проверок, тега и образов:
`make release VERSION=vX.Y.Z DRY_RUN=1`. Сам репо `make bump` / `make ship` / `ssh` **не выполняет**.

## Handoff (другие репо)

| Зона | Где | Что делает |
|---|---|---|
| Проект | `<org>/infra` (← project-infra-template-compose) | `make bump SVC=backend TAG=vX.Y.Z` — правит `BACKEND_TAG` в `.env` на VPS + force-recreate, compose пуллит образ (канон rollout'а; `make ship` там — про доставку самого infra-репо) |
| Verify | там же | `make ps`, лог `"starting" component=api version=vX.Y.Z commit=<sha>` |

## Откат

Image уже в registry — пересборка не нужна. Полностью в infra-репо:
`make bump SVC=backend TAG=<предыдущий>`. Релиз с миграцией приложения — любой, не только с
эффектом, — откатывается в обратном порядке: сначала `migrate down` образом migrate откатываемого
релиза, по разу на каждую его миграцию (`down` откатывает одну), затем `make bump`. Прежний образ
migrate версий схемы новее своих не знает и падает на `up` (golang-migrate не находит версию); после
`down` схема — та, которой ждёт прежний api. У миграции с эффектом (маркер `-- infra-handoff:`) окно
и порядок отката — ещё и в её блоке INFRA-HANDOFF. Релиз, где bump golibinfra добавил миграцию в источник
кита, так не откатить: `migrate down` откатывает только миграции приложения (схему китов ведёт
golibinfra), а прежний образ migrate падает на версии кита новее своей. Его откат — по аннотации тега
golibinfra (работает ли прежний код кита с новой схемой — тогда прежний api без прогона прежнего
migrate) или выкаткой вперёд.

## Versioning

Версия и commit зашиваются в бинарь через `-ldflags` (`-X main.version=$TAG -X
main.commit=$GIT_SHA`). На старте бинарь логирует `"starting" component=<api|migrate>
version=vX.Y.Z commit=<short-sha>` — на проде сразу виден крутящийся коммит без map'а
tag↔SHA. Схема версий и бинари — артефакт `docs/stack/service.md`. Теги `template/vX.Y.Z` — версии
обвязки шаблона (`docs/contributing.md § Версия обвязки`), не релизы образа: `make release` их не видит.
