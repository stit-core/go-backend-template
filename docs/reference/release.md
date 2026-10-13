# Release

**Зона ответственности этого репо: код → image в registry. На этом — STOP.** Выкатка,
прокатка env, проверка на проде — хаб `<org>/infra` (см. [ecosystem.md](ecosystem.md)). Skill — `/release`.

## Одна команда

На чистом `main` после зелёного `make check`:

```
make release VERSION=vX.Y.Z
```

`scripts/release.sh` одним проходом: confirm-промпт с полным путём образов → `release-preflight env` —
окружение релиза: ветка `main`, чистое дерево, доступ сборки к приватным модулям, buildx и `make doctor` →
`make check`, `make e2e-smoke` и `govulncheck` (уязвимость, которую вызывает код, — стоп до тега) →
annotated tag (rich changelog — `git tag -a` заранее) → `release-preflight tag` (на HEAD annotated
semver-тег, и он — запрошенный) → `docker buildx --push --ssh default` двух образов lockstep:
- `<host>/<org>/<репо>:vX.Y.Z` — `cmd/api` (runtime)
- `<host>/<org>/<репо>-migrate:vX.Y.Z` — `cmd/migrate` + вшитые SQL (init-контейнер)

→ агрегированный INFRA-HANDOFF → STOP.

Оба образа — distroless без shell и `wget`, процесс — UID 65532. Проверка здоровья контейнера api —
сам бинарь, подкоманда `healthcheck` (`GET /health` по `HTTP_ADDR`, код выхода 0 — здоров); e2e-стек
ждёт `healthy` по ней же.

Путь образа `<host>/<org>/<репо>` — из `git remote get-url origin`: он следует за местом репо в Gitea,
без правок при форке, и у каждого сервиса орги свой. Порт https-origin входит в хост (registry Gitea — на
её HTTP-порту), порт ssh — нет. Имя репо — имя сервиса для хаба: тег — `<РЕПО>_TAG` так, как его строит
`make bump` хаба (верхний регистр, `-` → `_`), выкатка — `make bump SVC=<репо>`; у форка `<org>/backend`
— `backend` и `BACKEND_TAG`. Отказ до тега — origin, из которого путь не вывести (локальный путь,
подгруппа), и имя репо, из которого не выходит имя переменной окружения (`1c-gateway`, `my.service`):
такой сервис хаб не выкатит. Build локальный.
`docker login <host>` — креды оператора; `make doctor` в preflight ловит их отсутствие
fail-fast. Lightweight / `:latest` / `:edge` / тег не на HEAD отвергаются by design.

Приватные модули `go.mod` (golibinfra) сборка образа берёт по SSH ключом ssh-agent оператора — тем
же путём, что `go` на его машине (README § Как начать): `docker buildx build --ssh default`, в
`Dockerfile` и `Dockerfile.migrate` — `RUN --mount=type=ssh` у `go mod download`. Сокет агента виден
только этому шагу: ни в слои образа, ни в его историю ключ не попадает. Ключ с доступом к git-хосту
модулей — в агенте (`ssh-add`): `release-preflight env` читает им каждый приватный модуль `go.mod` —
только агентом, без `~/.ssh/config` и файлов ключей, как увидит сборка, — и без доступа останавливает
релиз до гейта и тега. Пробная сборка образа — с тем же флагом: `docker buildx build --ssh default`.

## INFRA-HANDOFF

`make release` печатает агрегированный блок (`<РЕПО>_TAG` с путём образов, накопитель `.claude/plans/handoff.md`
с разделами `## Handoff` активных планов и команду выкатки `make bump`) — что передать в хаб
`<org>/infra` — и очищает накопитель коммитом. Заранее, без проверок, тега и образов:
`make release VERSION=vX.Y.Z DRY_RUN=1`. Сам репо `make bump` / `make ship` / `ssh` **не выполняет**.

## Handoff (другие репо)

| Зона | Где | Что делает |
|---|---|---|
| Проект | `<org>/infra` (← project-infra-template-compose) | `make bump SVC=<репо> TAG=vX.Y.Z` — правит `<РЕПО>_TAG` в `.env` на VPS + force-recreate, compose пуллит образ (канон rollout'а; `make ship` там — про доставку самого infra-репо) |
| Verify | там же | `make ps`, лог `"starting" component=api version=vX.Y.Z commit=<sha>` |

## Откат

Image уже в registry — пересборка не нужна. Полностью в infra-репо:
`make bump SVC=<репо> TAG=<предыдущий>`. Релиз с миграцией приложения — любой, не только с
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
