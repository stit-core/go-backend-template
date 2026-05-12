# Release flow: build + push + version embedding

**Slug:** `release-flow`
**Создан:** 2026-05-11
**Статус:** done
**Последний чекпоинт:** 2026-05-11 (DONE)

## Контекст

Темплейт `go-backend-template` ожидает от инфра-репо (`stit/project-infra-template-compose`)
готовый docker image в `git.stit.tech/<org>/backend:<BACKEND_TAG>`. Тег приходит из Vault
(`kv/projects/<p>/<env>/meta.backend_tag` → vault-agent → `.runtime/env/compose.env`).
Контракт зафиксирован в `project-infra-template-compose/docs/architecture.md:107`:
build локальный, никаких gitea-runner.

В этом репо нет:
- make-таргетов для build+push (только локальный `go build -o ./bin/api`);
- version-embedding в бинарник (`-X main.version` отсутствует);
- документации release-флоу (README §"Контракт" утверждает "build через gitea-runner" — ложь);
- skill'а `/release` для агентского флоу;
- permissions для release-related bash в `.claude/settings.json`.

Цель — закрыть всё перечисленное в одном проходе по этому репо. Правки в infra-репо
(`compose/migrate.yml` + `depends_on` у backend) — отдельным планом позже.

## Не входит в скоуп

- Правки в `project-infra-template-compose` (migrate-сервис, vault-agent meta-поля,
  documentation cross-reference) — отдельный план.
- `/version` HTTP-endpoint в huma — только лог при старте через `logkit.Logger`.
  Endpoint можно добавить как follow-up, если попросят.
- Auto `docker login git.stit.tech` — security-чувствительно, оставляем
  оператору; release-check только проверяет что push работает.
- `make release` НЕ вызывает `make pre-merge` автоматически — это отдельный
  шаг оператора (документируем в README). Иначе release-команда станет медленной
  и непредсказуемой по времени.
- Multi-arch (arm64). Только `linux/amd64`.
- Multi-package layout vs single-package layout в Gitea: фиксируем
  *separate packages* (`backend`, `backend-migrate`, `backend-worker`) — каждый
  свой image c одинаковым тегом `vX.Y.Z`. Альтернативу (один image, разные
  entrypoint'ы) сознательно отвергаем — меньше size, чище pull-policy.

## Открытые вопросы

- [x] Q1: Worker в release сразу? → **Да**, `make release` пушит api+migrate+worker.
- [x] Q2: Один тег на все image vs раздельные? → **Lockstep**: один `vX.Y.Z` на всё.
- [x] Q3: Откуда брать image-path? → **Конвенция** `<host>/<org>/backend{,-migrate,-worker}`.
  Host+org парсим из `go list -m`, repo-name всегда `backend` (совпадает с infra-template).
- [x] Q4: Multi-arch? → Только `linux/amd64`.
- [x] Q5: `/version` HTTP-endpoint? → **Нет, только лог на старте**.
- [x] Q6: Pre-flight checks для `release-check`? → На `main`, working tree clean,
  annotated tag на HEAD (`git describe --tags --exact-match`), `docker buildx` доступен.
  `make pre-merge` НЕ встроен — отдельный шаг, документирован в README.
- [x] Q7: Что если `cmd/worker/` удалили из темплейта? → `image-worker` skip'ается
  без ошибки (директорию не находим → silent skip + WARN в stderr).
- [x] Q8: Что если `docker login` не сделан? → `make release-check` пытается
  `docker buildx imagetools inspect <repo>` для smoke; если 401 → внятное сообщение
  "залогинься в git.stit.tech через `docker login`".
- [x] Q9: Release-tag не на HEAD (тег на ст. коммите)? → Fail-fast. `git describe
  --exact-match HEAD` — единственный путь.

## Атомы

- [x] **A1:** ldflags-переменные + лог на старте в `cmd/{api,worker,migrate}/main.go`
  - Что делаем: добавить `var (version = "dev"; commit = "unknown")` в каждый main,
    залогировать через `logkit.Logger`: `logger.Info("starting", "service", "<name>",
    "version", version, "commit", commit)` сразу после получения логгера.
  - Файлы: `cmd/api/main.go`, `cmd/worker/main.go`, `cmd/migrate/main.go`
  - Verify: `go build -ldflags="-X main.version=test1 -X main.commit=abc" -o /tmp/api ./cmd/api`
    → бинарник содержит `test1` (`strings /tmp/api | grep test1` → match).
  - Зависит от: —

- [x] **A2:** Dockerfile + Dockerfile.migrate: `ARG VERSION`, `ARG GIT_SHA`, прокинуть в ldflags
  - Что делаем: `ARG VERSION=dev`, `ARG GIT_SHA=unknown` в build-stage обоих
    Dockerfile'ов, `-ldflags="-s -w -X main.version=$VERSION -X main.commit=$GIT_SHA"`.
  - Файлы: `Dockerfile`, `Dockerfile.migrate`
  - Verify: `docker buildx build --build-arg VERSION=v0.0.0-test --build-arg GIT_SHA=deadbee
    -t local-api . --load && docker run --rm local-api 2>&1 | head -5 | grep v0.0.0-test`.
  - Зависит от: A1 (нужен var в main, чтобы было что зашивать).

- [x] **A3:** Makefile: `release-check` — pre-flight gate
  - Что делаем: новый таргет проверяет:
    - текущая ветка = `main`;
    - working tree clean (`git status --porcelain` пустой);
    - есть annotated tag на HEAD (`git describe --tags --exact-match HEAD`
      + `git cat-file -t $TAG` = `tag`, не `commit`);
    - формат тега `vX.Y.Z` (regex);
    - `docker buildx` доступен (`docker buildx version`);
    - не падает если нет залогинены в registry — это проверит сама push-операция.
  - Файлы: `Makefile`
  - Verify: `make release-check` на чистом main с annotated tag → exit 0;
    с грязным деревом → exit 2 с понятным сообщением.
  - Зависит от: —

- [x] **A4:** Makefile: `image-api`/`image-migrate`/`image-worker` + `release` orchestrator
  - Что делаем:
    - `IMAGE_BASE := $(shell go list -m | awk -F/ '{print $$1"/"$$2}')/backend`
      (host+org из module path + хардкод `backend`).
    - `TAG ?= $(shell git describe --tags --exact-match HEAD 2>/dev/null)`;
      если пуст → fail с сообщением.
    - `GIT_SHA := $(shell git rev-parse --short HEAD)`.
    - `image-api`: `docker buildx build --platform linux/amd64 --push
      --build-arg TARGET=api --build-arg VERSION=$(TAG) --build-arg GIT_SHA=$(GIT_SHA)
      -t $(IMAGE_BASE):$(TAG) .`
    - `image-migrate`: то же с `-f Dockerfile.migrate -t $(IMAGE_BASE)-migrate:$(TAG)`
      (без `--build-arg TARGET`).
    - `image-worker`: silent skip если нет `cmd/worker/`; иначе `--build-arg TARGET=worker
      -t $(IMAGE_BASE)-worker:$(TAG)`.
    - `release`: depends on `release-check`, потом `image-api image-migrate image-worker`,
      финальный echo: "✓ pushed $(IMAGE_BASE){,-migrate,-worker}:$(TAG)".
  - Файлы: `Makefile`
  - Verify: на чистом main с тегом `v0.0.0-test` → `make release` пушит три image
    в `git.stit.tech/<org>/backend{,-migrate,-worker}:v0.0.0-test`. Проверяем через
    `docker buildx imagetools inspect git.stit.tech/<org>/backend:v0.0.0-test`.
  - Зависит от: A2, A3.

- [x] **A5:** README.md: раздел "Release"; удалить ложный "build через gitea-runner"
  - Что делаем: новый раздел между "Команды" и "Структура":
    1. `make pre-merge` (опционально, gate перед релизом);
    2. `git tag -a vX.Y.Z -m "<changelog>"`;
    3. `git push origin vX.Y.Z`;
    4. `make release` (push api+migrate+worker в registry с lockstep tag);
    5. Корпу: запросить `vault kv patch kv/projects/<p>/<env>/meta backend_tag=vX.Y.Z`
       через skill `/request-secret` в infra-репо;
    6. На VPS: `make ship` (force-recreate consumer'ов под новый image).
    Удалить из §"Контракт со stit-инфрой" фразу "(build через gitea-runner)" —
    заменить на ссылку на новый раздел.
  - Файлы: `README.md`
  - Verify: visual review, no broken links.
  - Зависит от: A4 (команда `make release` должна реально существовать).

- [x] **A6:** AGENTS.md: блок про release-флоу
  - Что делаем:
    - §1 "Команды": добавить строку про `make release` / `make release-check`
      / `make image-{api,migrate,worker}` (link на README для деталей).
    - §5 "Git workflow": annotated tags для релизов, формат `vX.Y.Z` (semver),
      тег ставится на чистый main после зелёного `make pre-merge`.
    - §7 "Границы → Сначала спросить": "release tag (`git tag -a`) — действие
      видно вне репо, нужен явный OK от пользователя".
    - §7 "Границы → Не делать никогда": "Image-теги вида `:latest` / `:edge`
      запрещены. `make release` принимает только annotated semver".
  - Файлы: `AGENTS.md`
  - Verify: visual.
  - Зависит от: A4 (имена команд должны существовать).

- [x] **A7:** `.claude/skills/release/SKILL.md`: процедурный воркфлоу
  - Что делаем: новый skill в `.claude/skills/release/`. Описание + триггеры
    ("/release", "релиз", "запушим версию"). Шаги:
    1. Проверить state через `scripts/agent-tools/status.sh` (чистое дерево, на main).
    2. Прогнать `make pre-merge`; если красный → STOP, идти в `triage-failure`.
    3. Спросить у юзера TAG (формат `vX.Y.Z`, semver-bump по уровню изменений).
    4. `git tag -a $TAG -m "<changelog>"` — agent НЕ создаёт сам без OK.
    5. `git push origin $TAG`.
    6. `make release`.
    7. Сформировать stdout-блок для корпа: `vault kv patch kv/projects/<p>/<env>/meta backend_tag=$TAG`.
    8. Напомнить юзеру: на VPS — `make ship` в infra-репо.
    Anti-patterns: не push'ить tag до зелёного pre-merge; не делать `make release`
    минуя release-check; не предлагать `:latest`.
  - Файлы: `.claude/skills/release/SKILL.md` (new)
  - Verify: skill auto-loads (`/release` в available-skills).
  - Зависит от: A4 (команды), A5 (документация для cross-link).

- [x] **A8:** `.claude/settings.json`: permissions для release-related bash
  - Что делаем: добавить в `permissions.allow`:
    - `Bash(make release*)`, `Bash(make release-check*)`, `Bash(make image-*)`
    - `Bash(docker buildx*)` (для inspect, не для login)
    - `Bash(git tag*)` (read-only вариант — `git tag -l`; create — оставляем
      под общий prompt, чтобы не auto-approve создание тегов)
    - `Bash(git describe*)`, `Bash(git rev-parse*)`
    - `Bash(go list*)` уже есть.
    НЕ добавлять: `docker login`, `docker push` (push идёт через buildx, allowed),
    `git push --tags*` (push tag'а — significant action, через prompt).
  - Файлы: `.claude/settings.json`
  - Verify: `make release-check` от агента без prompt'а; `git tag -a vX.Y.Z`
    через prompt (правильно).
  - Зависит от: A4 (команды).

## Чекпоинты

### 2026-05-11

Закрыто: A1-A8 ✓ в один проход.

Evidence:
- A1: `go build -ldflags="-X main.version=test1 -X main.commit=abcd1234"
  -o /tmp/api ./cmd/api` → `strings /tmp/api | grep -E '^(test1|abcd1234)$'` matched
  для всех трёх бинарников.
- A2: `Dockerfile` / `Dockerfile.migrate` содержат `ARG VERSION=dev`, `ARG GIT_SHA=unknown`,
  ldflags `-X main.version=${VERSION} -X main.commit=${GIT_SHA}`. Реальный `docker buildx`
  смотри в A4 verify (требует креды к git.stit.tech, юзер запускает).
- A3: `scripts/release-preflight.sh` + интеграция в Makefile. Off-repo матрица тестов:
  no-tag→fail, lightweight→fail, wrong-format→fail, OK→exit 0, TAG arg mismatch→fail,
  not-on-main→fail, dirty→fail. На текущем дереве `make release-check` упал с
  "working tree dirty" — корректно.
- A4: `make -n release TAG=v0.0.0` показывает три buildx push-команды с одинаковым
  TAG и правильным IMAGE_BASE (`git.stit.tech/akadmin/backend{,-migrate,-worker}`).
  Worker recipe содержит `if [ ! -d cmd/worker ]; ...skip; fi`.
- A5: README раздел "Release" вставлен перед "Контракт со stit-инфрой"; в Команды
  таблице строка про `make release-check / make release`; ложный "build через
  gitea-runner" в Контракте удалён, заменён ссылкой на §Release.
- A6: AGENTS.md §1 строка про release-команды; §5 bullet про annotated semver
  на чистом main; §7 "Сначала спросить" про `git tag -a` + `make release`; §7
  "Не делать никогда" про `:latest` / lightweight tags.
- A7: `.claude/skills/release/SKILL.md` — frontmatter валидный, skill подхватился
  в available-skills.
- A8: `.claude/settings.json` — добавлены allow для `make release*`, `make image-*`,
  `docker buildx*`, preflight script, `git describe/rev-parse/tag -l/cat-file`.
  Валидный JSON (python json.load OK).

Что НЕ верифицировано в этом чекпоинте (требует реальной среды юзера):
- Реальный `docker buildx build --push` — требует `docker login git.stit.tech`
  и доступ к private go modules (ssh + .netrc) на машине билдера. Проверка
  переезжает на момент первого реального релиза.
- `make pre-merge` целиком (только cmd/ smoke: go vet/build/gofumpt — все чистые).

Следующий шаг: коммит изменений + чекпоинт в roadmap → план DONE; follow-up plan
для правок в infra-репо (`compose/migrate.yml` + `backend.depends_on.migrate`).
