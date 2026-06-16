# Release

**Зона ответственности этого репо: код → image в registry. На этом — STOP.** Деплой,
vault-patch, проверка на проде — другие репо (см. [ecosystem.md](ecosystem.md)). Skill — `/release`.

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

`make release` печатает агрегированный блок (накопленные подсекции `**Infra impact.**`
из активных секций `.claude/plans/roadmap.md` + vault/ship-подсказки) — кому что передать инфра-админу out-of-band. Сам репо `vault kv
patch` / `make ship` / `ssh` **не выполняет**.

## Handoff (другие репо)

| Зона | Где | Что делает |
|---|---|---|
| Корп | корп control-plane (`corp-infra-template`, root-Vault) | `vault kv patch kv/projects/<p>/<env>/meta backend_tag=vX.Y.Z` |
| Проект | `<org>/infra` (← project-infra-template-compose) | `make ship` → ssh-deploy → vault-agent рендерит `BACKEND_TAG` → force-recreate |
| Verify | там же | `make ps`, лог `"starting" service=api version=vX.Y.Z commit=<sha>` |

## Откат

Image уже в registry — пересборка не нужна. Полностью в infra-репо: корп-патч на
предыдущий тег + `make ship`.

## Versioning

Версия и commit зашиваются в бинарь через `-ldflags` (`-X main.version=$TAG -X
main.commit=$GIT_SHA`). На старте сервис логирует `"starting" service=<name>
version=vX.Y.Z commit=<short-sha>` — на проде сразу виден крутящийся коммит без map'а
tag↔SHA. Схема версий и бинари — артефакт `docs/stack/service.md`.
