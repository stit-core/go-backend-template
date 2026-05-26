# go-backend-template — гайд для агентов

Шаблон Go HTTP-бэкенда stit.tech: huma/v2 + chi + pgx/v5 + sqlc + uber/fx + `golibinfra`
(telemetry, authn, outbox, brute-force, cache, NATS, audit, clock).

Этот файл — **ядро (always-on) + роутер**. Детали — в `docs/`, контекстные правила — в
`.claude/rules/` (path-scoped, грузятся когда matching-файл в контексте), процедуры — в
`.claude/skills/`. Карта документации — `docs/README.md`.

## Точка входа

- **`/smartdev`** — cockpit сессии: сводка состояния (ветка, drift, активный план, TODO) +
  переключение в нужный режим. Запускай в начале / после `/clear`.
- **Потребителю шаблона** — `docs/getting-started.md`: форк → `/init-service` → проектирование
  доменов/стека → `/de-template` → свой проект.
- **Planning-first.** Любое содержательное изменение идёт через план — секцию в едином
  `.claude/plans/roadmap.md` (skill `/plan`). Прямая правка — только truly-atomic и багфиксы
  (`/triage-failure`, RED-first). План переживает `/clear`.

## Проектирование первично, код вторичен

DDD-артефакты доменов — `docs/domains/`, артефакты платформы/стека — `docs/stack/`. Это
**source of truth**; код пишется/генерируется по ним, не наоборот. `auth`/`user`/`verification`
— референс-примеры (изучить, потом удалить под свой проект). Роль репо в экосистеме 4 репо
(build+push image, деплой — в `<org>/infra`) — `docs/reference/ecosystem.md`.

## Команды

Единый интерфейс `make <verb>`; `make help` — полный список. Конциз-by-default
(`FORMAT=detailed`/`VERBOSE=1` — полно; `PKG=./x/...` — подмножество). Канонический gate перед
claim «готово» / push — **`make check`** (fmt+vet+lint+test+build+gen-check). `make gen` —
регенерация sqlc+mockery. Реализация за таргетами — `scripts/tools/*` (напрямую не зови).

## Карта: где что

| Тема | Где |
|---|---|
| Архитектура, слои, ports/adapters, события, non-goals | `docs/reference/architecture.md` + rule `domain-architecture` |
| Observability (метрики/логи/трейсы, Grafana) | `docs/reference/observability.md` |
| Release (образы, handoff, откат) | `docs/reference/release.md` + skill `/release` |
| Экосистема, контракт со stit-инфрой | `docs/reference/ecosystem.md` |
| Тесты, моки, TDD, e2e | rule `tests-pipeline` + `test/README.md` |
| Миграции, sqlc | rule `sqlc-and-migrations` + skill `/scaffold` (вариант migration) |
| Outbox / NATS / jobs | rule `events-and-messaging` |
| DDD-артефакты (kinds, workflow) | `docs/domains/README.md` + `docs/domains/MAPPING.md` |
| Stack-артефакты (8 kinds, вход de-template) | `docs/stack/README.md` |
| Устройство `.claude`-системы, troubleshooting | `docs/contributing.md` |

## Стиль кода (инварианты)

- **gofumpt** обязателен (`make fmt`).
- **golangci-lint** (`.golangci.yml`): **forbidigo** — нельзя `fmt.Print*`/`slog.*`, только
  `logkit.Logger`; **depguard** — бизнес-домены не импортят друг друга, cross-domain через
  порты + адаптеры в `<domain>/module/adapters.go`. errcheck/errorlint/gosec/govet/contextcheck
  /noctx/rowserrcheck/bodyclose/sqlclosecheck — на.
- Ошибки — `%w` + `errors.Is/As`; sentinel в `errors.go` корня домена.
- Комментарии — только WHY (rule `comments`).
- **Не городить велосипеды**: переиспользуемый/инфра-примитив (generic util, HTTP-client,
  middleware, любой «kit») → `golibinfra`, не локальная полукалека. Нет нужного — PR в
  `golibinfra` + строка в roadmap-backlog; временный shim только с явным OK юзера. Граница и
  обоснование — `docs/reference/architecture.md § Граница с golibinfra`.

## Git workflow

- **Conventional Commits** (`scripts/git-hooks/commit-msg`): типы `feat|fix|refactor|docs|test|chore|meta`,
  формат `<type>(<scope>)?: <subject>`, subject ≤72.
- **Push в `main` разрешён** (прямой или через PR); линейная история. Gate — git
  `pre-push → make check` (skip только `SKIP_PREPUSH=1`, не `--no-verify`).
- git-хуки ставятся `make setup` (commit-msg, pre-commit, pre-push). Релиз — `/release`.

## Границы

### Делать всегда
- Перед claim «готово» — `make check`. Без верификации ты единственный feedback-loop.
- **Изменение — через план** (planning-first; skill `/plan`). Отклонение — осознанное, проговаривается.
- **Тронул инфра-поверхность** (required env/Vault-секрет; миграция destructive/lock/backfill/extension;
  cross-service NATS/outbox-subject; новый порт/breaking API; новая runtime-dep в `go.mod`) →
  оформи **INFRA-HANDOFF**-блок и попроси юзера передать инфра-админу до claim/коммита (rule
  `infra-impact` + skill `/infra-handoff`).
- Cross-domain — только порты + адаптеры. Логи — `logkit.Logger`. Generic-примитив → `golibinfra`.
- `make gen` после правки `queries/`, `migrations/` или интерфейсов из `.mockery.yml`.
- Conventional Commits. Дёргай `make <verb>`, не скрипты напрямую.

### Сначала спросить
- `migrations/` (skill `/scaffold`, вариант migration), `pkg/policy` + `pkg/tokens`, `internal/platform/authgate/`,
  `go.mod`/`go.sum`, любой `--no-verify`, `git tag -a` / `make release` (артефакт вне репо).

### Не делать никогда
- `--no-verify` (блокирует `permissions.deny`). Коммитить/цитировать `.env*`/`*.pem`/`*.key`.
- Править сгенерированное руками (`infra/postgres/db/*.go`, `**/mocks/*.go`) — только `make gen`.
- `fmt.Print*`/`slog.*` напрямую. Image-теги `:latest`/`:edge`/lightweight.
- Deploy-команды чужих репо (`make ship`, `ssh`, `vault kv patch`) — после `make release` STOP.
- Принимать архитектурные решения без согласования — отклонение от skill/плана требует OK юзера.

## Forward-only

Любой текст в репо описывает **только текущее состояние** — не «было/стало». История — в
`git log` + Conventional Commits. Полное правило — `.claude/rules/forward-only.md`.
