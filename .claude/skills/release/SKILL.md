---
name: release
description: Запушить новую версию backend image'ей в registry (api+migrate lockstep) одной командой и передать инфра-админу агрегированный handoff. Активируй когда пользователь говорит "релиз", "запушим версию", "сделай release vX.Y.Z", "/release".
---

# Skill: release

Канонический release-флоу. **Зона ответственности этого репо: код → image в registry. На STOP — всё.** Build локальный (никаких gitea-runner), push в `<host>/<org>/backend{,-migrate}` под одним lockstep `vX.Y.Z`-тегом. Воркер отдельным образом не собирается — outbox drain, NATS subscribers, jobs живут в `cmd/api/`.

Дальше — **handoff** в проектный infra-репо (`<client-org>/infra`): rollout — `make bump SVC=backend TAG=<v>` там (правит `BACKEND_TAG` в `.env` на VPS + force-recreate). Эти шаги не выполняются здесь — `make release` печатает агрегированный INFRA-HANDOFF-блок, оператор передаёт его out-of-band.

## Когда применять

- "Релизим v1.2.3" / "Запушь новую версию" / "/release"
- После закрытия задачи и зелёного `make check`, когда юзер сказал "пора катить".

## Когда НЕ применять

- "Собери image локально для теста" → `docker buildx build --load -t local-test .`, не `make release`.
- Пуш feature-веточного билда → `release-preflight` отвергает не-main.
- Юзер не озвучил OK на релиз → STOP, спроси.

## Pre-flight (BEFORE команды)

Любой пункт не закрыт — не лезь:

- [ ] Юзер явно сказал OK на релиз (тег + push в registry — действие видимое вне репо; `make release` ещё раз спросит confirm в TTY).
- [ ] `make status` показывает: `main`, working tree clean, нет активных не-release планов.
- [ ] `make check` зелёный (если нет — STOP, переключайся в `triage-failure`).
- [ ] Юзер знает версию (semver-bump по уровню изменений).
- [ ] Оператор залогинен в registry (`docker login <host>` — вне процесса, не делай сам; `make doctor` это проверяет fail-fast).

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
2. **confirm-промпт** `⚠ confirm release <v> from main@<sha>? [y/N]` (нет TTY/EOF → отмена, accidental release невозможен);
3. annotated tag, если на HEAD ещё нет (иначе использует существующий; конфликт версий → fail);
4. `release-preflight.sh` — ветка=main / clean / annotated semver на HEAD / `docker buildx` / **`make doctor`** (docker-auth, sqlc-drift, непарные миграции — fail-fast);
5. `docker buildx --platform linux/amd64 --push` двух image'ей lockstep: `<host>/<org>/backend:<v>` (`cmd/api`) и `<host>/<org>/backend-migrate:<v>` (`cmd/migrate` + вшитые SQL, init-контейнер), `VERSION`/`GIT_SHA` зашиты в бинарник;
6. печатает агрегированный **INFRA-HANDOFF** (см. ниже) и STOP.

`401 unauthorized` от registry → `make doctor` поймает это в preflight ещё до пуша; remediation — `docker login <host>` (креды оператора, не делай сам).

---

**После команды image'ы в registry. Зона этого репо закрыта.**

## Handoff (НЕ в этом репо — генерируется автоматически)

`make release` сам печатает блок `=== INFRA HANDOFF ===`:

- `BACKEND_TAG → <v>` (api+migrate lockstep);
- **накопленные `**Infra impact.**`** — `release.sh` агрегирует блоки `**Infra impact.**` из активных секций `.claude/plans/roadmap.md` (механизм — `.claude/rules/infra-impact.md` + skill `infra-handoff`). Если пусто — печатает напоминание убедиться, что инфра-затрагивающее зафиксировано;
- действие инфра-стороны:
  - проект: в `<client-org>/infra` — `make bump SVC=backend TAG=<v>` (правит `BACKEND_TAG` в `.env` на VPS + force-recreate).

Твоя роль после команды: **показать этот блок юзеру и сказать передать инфра-админу out-of-band**. Подставь `project`/`env`, если юзер их называл. `make ship`/`make bump`/`ssh` отсюда **не выполняются** — другой репо, другие креды.

### Verify (в infra-репо, не здесь)

`make ps` → backend `healthy`; в логах на старте `"starting" service=api version=<v> commit=<sha>`. Описано в release-skill **infra-репо**.

## Откат

**Полностью в infra-репо** (этот репо не участвует — image в registry):
`make bump SVC=backend TAG=<предыдущий>`.

## Anti-patterns

- ❌ `git tag vX.Y.Z` без `-a`. Lightweight tag отвергнет release-preflight.
- ❌ `make release` минуя зелёный `make check`. Если красный — STOP, в `triage-failure`.
- ❌ `make release` не на main. Preflight отвергнет — не трать время.
- ❌ `docker login <host>` сам. Креды оператора.
- ❌ `:latest` / `:edge` / любой не-semver tag.
- ❌ Релизить, не передав INFRA-HANDOFF-блок инфра-админу — без `make bump` на инфра-стороне продолжит крутиться старый image.
- ❌ Выполнять `make ship` / `ssh <vps>` из этого репо. Другой репо/креды. Наша роль — image в registry + текст-handoff юзеру.
