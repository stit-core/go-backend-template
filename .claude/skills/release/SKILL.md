---
name: release
description: Запушить новую версию backend image'ей в registry (api+migrate+worker lockstep) и подсказать оператору шаги в infra-репо. Активируй когда пользователь говорит "релиз", "запушим версию", "сделай release vX.Y.Z", "/release".
---

# Skill: release

Канонический release-флоу для этого шаблона. Build локальный (никаких gitea-runner), push в `git.stit.tech/<org>/backend{,-migrate,-worker}` под одним lockstep `vX.Y.Z`-тегом. Дальше — Vault + ship в infra-репо (вне этого репо).

## Когда применять

- "Релизим v1.2.3"
- "Запушь новую версию"
- "/release"
- После закрытия задачи и зелёного `make pre-merge`, когда юзер сказал "пора катить".

## Когда НЕ применять

- "Собери image локально для теста" → `docker buildx build --load -t local-test .`, не `make release`.
- Пуш в registry feature-веточного билда → не поддерживается, `release-preflight` отвергает не-main.
- Юзер не озвучил OK на тегирование/пуш → STOP, спроси.

## Pre-flight (BEFORE первого шага)

Один сценарий не закрыт юзером, не лезь:

- [ ] Юзер явно сказал OK на релиз (тег + push в registry — действие видимое вне репо).
- [ ] `scripts/agent-tools/status.sh` показывает: `main`, working tree clean, нет активных не-release планов.
- [ ] `make pre-merge` зелёный (если нет — STOP, переключайся в `triage-failure`).
- [ ] Юзер знает версию (semver-bump по уровню изменений: patch/minor/major).
- [ ] Оператор залогинен в registry (`docker login git.stit.tech` — это вне процесса, не делай сам).

## Шаги

### 1. Подтвердить версию

Спроси у юзера TAG в формате `vX.Y.Z`. Не угадывай. Подскажи bump-уровень по changelog:
- patch (`v1.2.3 → v1.2.4`) — bugfix без API-изменений.
- minor (`v1.2.3 → v1.3.0`) — новые endpoint'ы / поля, обратно совместимые.
- major (`v1.2.3 → v2.0.0`) — breaking change в API или схеме.

Если предыдущих тегов нет — стартуй с `v0.1.0`.

### 2. Annotated tag

```bash
git tag -a v1.2.3 -m "<conventional-style changelog block>"
```

**Не `git tag v1.2.3`** (lightweight) — `release-preflight` отвергнет. Не делай сам — попроси юзера подтвердить или дай команду на копирование.

### 3. Push tag

```bash
git push origin v1.2.3
```

После этого тег виден в Gitea, в `git describe --tags --exact-match HEAD` появится.

### 4. Release

```bash
make release
```

Делает (за тебя):
- `release-preflight.sh` (ветка / clean / annotated `vX.Y.Z` на HEAD / `docker buildx`),
- `docker buildx build --platform linux/amd64 --push` для api / migrate / worker (последний skip'ает если `cmd/worker/` нет),
- `--build-arg VERSION=$TAG --build-arg GIT_SHA=$(git rev-parse --short HEAD)` — версия зашита в бинарник,
- финальный echo с next-steps оператору.

Если падает на `401 unauthorized` от registry — попроси юзера `docker login git.stit.tech`, не делай сам.

### 5. Сформируй блок для корпа

После успешного `make release` — выведи юзеру блок, который он передаст корп-админу out-of-band:

```
=== корп-админу ===
vault kv patch kv/projects/<project>/<env>/meta backend_tag=v1.2.3
===
```

Если юзер сказал project+env заранее — подставь. Если нет — оставь плейсхолдеры и попроси заполнить перед отправкой.

В клиентском `infra`-репо есть skill `/request-secret` который форматирует этот блок — но он там, не здесь.

### 6. На VPS — `make ship`

Финальный шаг (в infra-репо, не в этом):

```bash
# В <client-org>/infra
make ship      # ssh-deploy на VPS: git pull + render + force-recreate consumer'ов
```

`vault-agent` рендерит новый `BACKEND_TAG` в `.runtime/env/compose.env`, `deploy.sh` детектит profile/template-change и пересоздаёт backend-контейнер с новым image.

### 7. Подтверждение

После `make ship`:
- `ssh <vps> 'cd /opt/<project>-infra && make ps'` — backend контейнер должен быть `healthy`.
- В логах backend'а на старте: `"starting" service=api version=v1.2.3 commit=<sha>` — sanity check что в проде именно тот image.

## Откат

Тот же путь с предыдущим тегом:
1. Корпу: `vault kv patch kv/.../meta backend_tag=v1.2.2` (предыдущий).
2. На VPS: `make ship`.

Image в registry никуда не делся, deploy моментальный. Re-build не нужен.

## Anti-patterns

- ❌ `git tag v1.2.3` без `-a`. Lightweight tag не валидится — отвергнет release-preflight.
- ❌ `make release` минуя `make pre-merge`. Pre-merge gate отдельный — оператор может skip'нуть для hotfix, но обычно гоним.
- ❌ Запуск `make release` не на main. Preflight отвергнет, но не трать time на попытку.
- ❌ `docker login git.stit.tech` сам. Креды юзера, оставляем оператору.
- ❌ `:latest` / `:edge` / любой не-semver tag. Запрещён в обоих репо.
- ❌ Tag без предварительного `make pre-merge` зелёного. Если pre-merge красный — STOP, идти в `triage-failure` skill.
- ❌ Сделать tag локально и забыть `git push origin <tag>` — `make release` тогда возьмёт правильный, но в Gitea его не будет, что путает при rollback.
- ❌ Скипать шаг 5 (стандартный блок для корпа) — без `vault kv patch` корп-сторонний `BACKEND_TAG` не обновится, и `make ship` поднимет старый image.
