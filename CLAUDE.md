# go-backend-template — гайд для агентов

Шаблон Go HTTP-бэкенда stit: huma/v2 + chi + pgx/v5 + sqlc + uber/fx + `golibinfra`
(logging, authn, outbox, brute-force, cache, NATS, audit, clock).

Этот файл — **ядро (always-on) + роутер**. Детали — в `docs/`, контекстные правила — в
`.claude/rules/` (path-scoped, грузятся когда matching-файл в контексте), процедуры — в
`.claude/skills/`. Карта документации — `docs/README.md`.

## Точка входа

- **Как сюда попадают.** Обычно — агентом из операционного хаба `<org>/infra`: общий план
  проекта ведёт хаб, задача приходит оттуда, а работаешь ты обвязкой этого репо (этот файл,
  скиллы, гейт `make check`). Результат уходит обратно в хаб: образ в registry (`/release`) и
  INFRA-HANDOFF-блок. Прямой заход разработчика — тоже нормальный режим с той же обвязкой;
  последствия для соседей оформляются INFRA-HANDOFF-блоком для хаба (`/infra-handoff`). Карта —
  `docs/reference/ecosystem.md`.
- **`/smartdev`** — cockpit сессии: сводка состояния (ветка, drift, активный план, TODO) +
  переключение в нужный режим. Запускай в начале / после `/clear`.
- **Потребителю шаблона** — `docs/getting-started.md`: форк → `/init-service` → проектирование
  стека (домены уже готовы в соседнем strategy-репо) → `/de-template` → scaffold → свой проект.
- **Planning-first.** Любое содержательное изменение этого репо идёт через план — файл в
  `.claude/plans/` (skill `/plan`: критерии приёмки, шаги с проверками, «Где мы сейчас» для
  продолжения после `/clear`); фаза программы хаба — план с `parent: infra:<slug>`. Прямая
  правка — только truly-atomic и багфиксы (`/triage-failure`, RED-first).

## Проектирование первично, код вторичен

DDD-артефакты доменов живут в **соседнем strategy-репо** орга (`../strategy`, форк
`business-strategy-template`): `docs/projects/<slug>/services/<svc>/domains/<ctx>/` — домены
сервиса `<svc>`, который реализует этот репо; артефакты платформы/стека — локально `docs/stack/`.
Это **source of truth**; код пишется/генерируется по ним, не наоборот. `auth`/`user`/`verification`
— code-only референс-примеры (изучить, потом удалить под свой проект). Роль репо в экосистеме
(образ в registry; выкатка — `make bump` в `<org>/infra`) — `docs/reference/ecosystem.md`.

## Команды

Единый интерфейс `make <verb>`; `make help` — полный список. Конциз-by-default
(`FORMAT=detailed`/`VERBOSE=1` — полно; `PKG=./x/...` — подмножество; `make check` их не читает).
Канонический gate перед claim «готово» / push — **`make check`** (fmt-check + vet + lint +
test-int (юнит + integration, нужен Docker) + build + gen-check + migrations-check + cover-check
≥75%). `make gen` — регенерация sqlc+mockery. Реализация за таргетами — `scripts/tools/*`
(напрямую не зови).

## Карта: где что

| Тема | Где |
|---|---|
| Архитектура, слои, ports/adapters, события, non-goals | `docs/reference/architecture.md` + rule `domain-architecture` |
| Логирование (структурные логи в stdout, `logkit.Logger`) | CLAUDE.md § Стиль кода + артефакт `docs/stack/observability.md` (форма — `docs/stack/_templates/observability.md`) |
| Release (образы, handoff, откат) | `docs/reference/release.md` + skill `/release` |
| Экосистема, контракт с `<org>/infra` | `docs/reference/ecosystem.md` |
| Тесты, моки, TDD, e2e | rule `tests-pipeline` + `test/README.md` |
| Миграции, sqlc | rule `sqlc-and-migrations` + skill `/scaffold` (вариант migration) |
| Outbox / NATS / jobs | rule `events-and-messaging` |
| DDD-домены (проектирование) | соседний strategy-репо `../strategy` (skill `/domain-deep-dive` там); маппинг DDD→Go-код — `docs/reference/domain-mapping.md` |
| Stack-артефакты (8 kinds, вход de-template) | `docs/stack/README.md` + skill `/stack-deep-dive` |
| Устройство `.claude`-системы, troubleshooting | `docs/contributing.md` |

## Стиль кода (инварианты)

- **gofumpt** обязателен (`make fmt`).
- **golangci-lint** (`.golangci.yml`): **forbidigo** — нельзя `fmt.Print*`/`log.*`/`print(ln)`/
  `slog.*` и запись в `os.Stdout`/`os.Stderr`, только `logkit.Logger`; **depguard** — бизнес-домены
  не импортят друг друга, cross-domain через порты + адаптеры в `<domain>/module/adapters.go`.
  errcheck (и `_ = f()`)/errorlint/gosec/govet/contextcheck/noctx/rowserrcheck/bodyclose
  /sqlclosecheck/testifylint — на; `//nolint` — только с именем линтера и объяснением (nolintlint).
- Ошибки — `%w` + `errors.Is/As`; sentinel в `errors.go` корня домена.
- Комментарии — только WHY (rule `comments`).
- **Не городить велосипеды**: переиспользуемый/инфра-примитив (generic util, HTTP-client,
  middleware, любой «kit») → `golibinfra`, не локальная полукалека. Нет нужного — PR в
  `golibinfra` + строка в `.claude/plans/backlog.md`; временный shim только с явным OK юзера. Граница и
  обоснование — `docs/reference/architecture.md § Граница с golibinfra`.

## Git workflow

- **Conventional Commits** (`scripts/git-hooks/commit-msg`): типы `feat|fix|refactor|docs|test|chore|meta`,
  формат `<type>(<scope>)?: <subject>`, subject ≤72; сообщения самого git (`Revert "…"`,
  `fixup!`/`squash!`) тоже проходят.
- **Push в `main` разрешён** (прямой или через PR); линейная история. Gate — git
  `pre-push → make check` на чистом дереве, пушится только HEAD. Обойти хуки может только
  человек (`SKIP_PREPUSH=1`); из сессии Claude обходы блокирует хук `.claude/hooks/guard-bash.sh`.
- git-хуки подключает `make setup` (`core.hooksPath=scripts/git-hooks`). Релиз — `/release`.

## Границы

### Делать всегда
- Перед claim «готово» — `make check`: CI нет, этот гейт (его же гоняет pre-push) — единственная
  проверка изменения.
- **Изменение — через план** (planning-first; skill `/plan`). Отклонение — осознанное, проговаривается.
- **Тронул инфра-поверхность** (required env / sensitive-env-секрет; миграция destructive/lock/backfill/extension;
  cross-service NATS/outbox-subject; новый порт/breaking API; новая runtime-dep в `go.mod`) →
  до claim/коммита оформи **INFRA-HANDOFF**-блок для хаба `<org>/infra`: пришёл из хаба — блок
  входит в твой отчёт хабу, прямой заход — попроси юзера передать его туда (rule `infra-impact` +
  skill `/infra-handoff`).
- Cross-domain — только порты + адаптеры. Логи — `logkit.Logger`. Generic-примитив → `golibinfra`.
- `make gen` после правки `queries/`, `migrations/` или интерфейсов из `.mockery.yml`.
- Conventional Commits. Дёргай `make <verb>`, не скрипты напрямую.

### Сначала спросить
- `migrations/` (skill `/scaffold`, вариант migration), `pkg/policy` + `pkg/tokens`, `internal/platform/authgate/`,
  `go.mod`/`go.sum`, `git tag -a` / `make release` (артефакт вне репо).

### Не делать никогда
- Обходить хуки и гейт: `--no-verify`, `git commit -n`, `SKIP_PREPUSH`, `core.hooksPath`,
  `GOFLAGS=` (блокируют `permissions.deny` и хук `guard-bash`). Коммитить/цитировать `.env`,
  `.env.local`, `*.pem`, `*.key` (в репо живут только `*.example`-шаблоны).
- Править сгенерированное руками (`internal/domain/*/infra/postgres/db/*.go`, `**/mocks/*.go`) — только `make gen`.
- `fmt.Print*`/`log.*`/`print(ln)`/`slog.*`/`os.Stdout`/`os.Stderr` напрямую. Image-теги `:latest`/`:edge`/lightweight.
- Deploy-команды (`make bump`, `make ship`, `ssh`, env-прокатка) — выкатка идёт только из
  `<org>/infra`; после `make release` STOP.
- Принимать архитектурные решения без согласования — отклонение от skill/плана требует OK юзера.

## Forward-only

Любой текст в репо описывает **только текущее состояние** — не «было/стало». История — в
`git log` + Conventional Commits. Полное правило — `.claude/rules/forward-only.md`.
