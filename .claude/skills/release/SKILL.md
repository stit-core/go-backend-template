---
name: release
description: Выпустить версию бэкенда одной командой — `make release VERSION=vX.Y.Z`: annotated tag, образы api+migrate одним тегом в registry, агрегированный INFRA-HANDOFF для хаба `<org>/infra`, затем STOP (выкатка — `make bump` в infra). Используй, когда юзер просит релиз, новый тег или образ в registry.
---

# Skill: release

Канонический release-флоу. **Зона ответственности этого репо: код → image в registry. На STOP — всё.** Build локальный, push в `<host>/<org>/<репо>{,-migrate}` (путь — из origin, у каждого сервиса орги свой) под одним lockstep `vX.Y.Z`-тегом. Воркер отдельным образом не собирается — outbox drain, NATS subscribers, jobs живут в `cmd/api/`.

Дальше — handoff в хаб `<org>/infra`: rollout — `make bump SVC=<репо> TAG=<v>` там (правит `<РЕПО>_TAG` в `.env` на VPS + force-recreate; у `<org>/backend` — `BACKEND_TAG`). Эти шаги не выполняются здесь — `make release` печатает агрегированный INFRA-HANDOFF-блок, и он уходит в хаб.

## Когда применять

- Юзер просит релиз или новую версию образа в registry.
- После закрытия задачи и зелёного `make check`, когда юзер сказал «пора катить».

## Когда НЕ применять

- "Собери image локально для теста" → `docker buildx build --ssh default --load -t local-test .`, не `make release`.
- Пуш feature-веточного билда → `release-preflight` отвергает не-main.
- Юзер не озвучил OK на релиз → STOP, спроси.

## Pre-flight (BEFORE команды)

Любой пункт не закрыт — не лезь:

- [ ] Юзер явно сказал OK на релиз (тег + push в registry — действие видимое вне репо; `make release` ещё раз спросит confirm в TTY).
- [ ] `make status` показывает: `main`, working tree clean, нет активных не-release планов.
- [ ] `make check` зелёный (если нет — STOP, переключайся в `triage-failure`).
- [ ] Юзер знает версию (semver-bump по уровню изменений).
- [ ] Оператор залогинен в registry (`docker login <host>` — вне процесса, не делай сам; `make doctor` это проверяет fail-fast).
- [ ] Ключ с доступом к приватным модулям (golibinfra) — в ssh-agent оператора (`ssh-add -l`): сборка образа берёт их по SSH этим ключом, `ssh-add` — тоже вне процесса.

## Шаги в этом репо

### 1. Подтвердить версию (semver-bump)

Спроси у юзера версию в формате `vX.Y.Z`. Не угадывай. Подскажи уровень по changelog:
- patch (`v1.2.3 → v1.2.4`) — bugfix без API-изменений.
- minor (`v1.2.3 → v1.3.0`) — новые endpoint'ы/поля, обратно совместимо.
- major (`v1.2.3 → v2.0.0`) — breaking change в API или схеме.

Нет предыдущих тегов — старт с `v0.1.0`.

**Rich changelog (опц.):** если нужен содержательный annotated-tag message — оператор создаёт тег заранее: `git tag -a vX.Y.Z -m "<changelog>"`. `make release` детектит существующий тег на HEAD и использует его. Иначе команда сама создаёт annotated tag с generic-сообщением.

### 2. Одна команда

```bash
make release VERSION=vX.Y.Z
```

`scripts/release.sh` делает за тебя, по порядку:
1. валидирует `VERSION` (semver; lightweight/`:latest`/`:edge` отвергаются);
2. **confirm-промпт** `⚠ confirm release <v> from main@<sha> → <host>/<org>/<репо>{,-migrate}:<v>? [y/N]` — путь образов сверь с тем, что ждёт хаб (нет TTY/EOF → отмена, accidental release невозможен; origin, из которого путь не вывести, и имя репо, которое хаб не выкатит, — `1c-gateway`, `my.service`, — отказ);
3. `release-preflight.sh env` — окружение релиза, fail-fast до гейта и тега: ветка=main / clean / ключи ssh-agent читают приватные модули `go.mod` / `docker buildx` / **`make doctor`** (docker-auth, sqlc-drift, непарные миграции, git-хуки);
4. чистое дерево, `make check`, `make e2e-smoke` (собранный бинарь со стеком в docker) и `govulncheck` (уязвимость, которую вызывает код; нужна сеть) — тег и образы только у проверенного HEAD (красный → fail до тега);
5. annotated tag, если на HEAD ещё нет (иначе использует существующий; конфликт версий → fail);
6. `release-preflight.sh tag` — на HEAD annotated semver-тег, и он — запрошенный;
7. `docker buildx --platform linux/amd64 --push --ssh default` двух image'ей lockstep: `<host>/<org>/<репо>:<v>` (`cmd/api`) и `<host>/<org>/<репо>-migrate:<v>` (`cmd/migrate` + вшитые SQL, init-контейнер), `VERSION`/`GIT_SHA` зашиты в бинарник; приватные модули `go mod download` берёт по SSH ключом ssh-agent — в слои образа ключ не попадает;
8. печатает агрегированный **INFRA-HANDOFF** (см. ниже), очищает накопитель коммитом и STOP.

Что уйдёт в хаб, видно заранее, без проверок, тега и образов: `make release VERSION=vX.Y.Z DRY_RUN=1`.

`401 unauthorized` от registry → `make doctor` поймает это в preflight ещё до гейта; remediation — `docker login <host>` (креды оператора, не делай сам). «в ssh-agent нет ключей» или «… не читается ключами ssh-agent» от preflight → оператор добавляет ключ: `ssh-add <ключ>` (не делай сам); в той же строке причина ssh — у сетевого сбоя ключ ни при чём.

---

**После команды image'ы в registry. Зона этого репо закрыта.**

## Handoff (НЕ в этом репо — генерируется автоматически)

`make release` сам печатает блок `=== INFRA HANDOFF ===`:

- `<РЕПО>_TAG → <v>` с путём образов (api+migrate lockstep);
- **накопленный handoff** — накопитель `.claude/plans/handoff.md` (его пополняет skill `infra-handoff`) и разделы `## Handoff` активных планов. Если пусто — печатает напоминание убедиться, что инфра-затрагивающее зафиксировано. После печати накопитель очищается коммитом `meta(handoff): …`; отданное — `git show vX.Y.Z:.claude/plans/handoff.md`;
- действие инфра-стороны:
  - проект: в `<org>/infra` — `make bump SVC=<репо> TAG=<v>` (правит `<РЕПО>_TAG` в `.env` на VPS + force-recreate).

Твоя роль после команды: отдать этот блок в хаб `<org>/infra` — пришёл агентом из хаба, блок входит в твой отчёт; прямой заход — покажи юзеру и попроси передать. `make bump`/`make ship`/`ssh` отсюда не выполняются — другой репо, другие креды.

### Verify (в infra-репо, не здесь)

`make ps` → backend `healthy`; в логах на старте `"starting" component=api version=<v> commit=<sha>`.

## Откат

Полностью в infra-репо (этот репо не участвует — image в registry):
`make bump SVC=<репо> TAG=<предыдущий>`. Релиз с миграцией приложения — сначала `migrate down`
образом откатываемого релиза, по разу на миграцию, затем `make bump` (`docs/reference/release.md`
§ Откат).

## Anti-patterns

- ❌ `git tag vX.Y.Z` без `-a`. Lightweight tag отвергнет release-preflight.
- ❌ `make release` минуя зелёный `make check`. Если красный — STOP, в `triage-failure`.
- ❌ `make release` не на main. Preflight отвергнет — не трать время.
- ❌ `docker login <host>` или `ssh-add` сам. Креды оператора.
- ❌ `:latest` / `:edge` / любой не-semver tag.
- ❌ Релизить, не передав INFRA-HANDOFF-блок в `<org>/infra` — без `make bump` на инфра-стороне продолжит крутиться старый image.
- ❌ Выполнять `make bump` / `make ship` / `ssh <vps>` из этого репо. Другой репо/креды. Наша роль — image в registry + текст-handoff в хаб.
