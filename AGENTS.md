# AGENTS.md

Шаблон Go HTTP-бэкенда на стеке stit.tech: huma/v2 + chi + pgx/v5 + sqlc + uber/fx + golibinfra (telemetry, authn, outbox, brute-force, cache, NATS, audit, clock).

Этот файл — постоянный гайд (грузится в контекст всегда). Контекстные правила лежат в `.claude/rules/`. Процедурные воркфлоу — в `.claude/skills/`. Описание дневного флоу — в `.claude/WORKFLOW.md`.

**Точка входа в сессию: `/smartdev`** — даёт сводку о состоянии репо (ветка, drift, активные планы, висящие TODO) и предлагает следующее действие. Если задача нетривиальная — оттуда переключайся в `/plan` для декомпозиции на атомы (план переживает `/clear` через `.claude/plans/<slug>.md`).

**Проектирование первично, код вторичен.** DDD-артефакты живут в `docs/domains/<context>/` и являются source of truth для бизнес-смысла. Перед скаффолдингом домена → артефакт; `/domain-deepen <ctx>` углубляет драфты до stable. См. `docs/domains/README.md` и `docs/domains/MAPPING.md` (DDD ↔ Go).

## 1. Команды

Стандартные Make-таргеты (`Makefile`):

| Команда | Что делает |
|---|---|
| `make tools` | Поставить pinned-версии dev-инструментов (gofumpt, golangci-lint, sqlc, mockery) |
| `make test` | Юнит-тесты, `-short -race` |
| `make test-int` | Интеграционные тесты через testcontainers (нужен Docker) |
| `make test-cover` | Coverage по domain + platform |
| `make lint` | golangci-lint |
| `make fmt` | gofumpt -w |
| `make build` / `make build-worker` | Собрать api / worker бинарники |
| `make sqlc` / `make sqlc-diff` | Регенерация / проверка pgx-кода |
| `make mocks` / `make mocks-check` | Регенерация / проверка mockery v3 моков |
| `make migrate-up` / `make migrate-down` | Накатить / откатить одну миграцию (нужен `PG_DSN`) |
| `make new-domain NAME=order` | Скаффолд скелета нового домена |
| `make pre-merge` | Канонический gate: fmt + vet + lint + test + build + sqlc-diff + mocks-check. Гонится pre-push хуком. |
| `make release-check` / `make release` | Pre-flight (main / clean / annotated `vX.Y.Z` / buildx) + push `<host>/<org>/backend{,-migrate,-worker}:<tag>`. См. README §Release. |
| `make hooks-install` | Установить git-хуки (commit-msg, pre-commit, pre-push) |

**Agent-friendly обёртки** в `scripts/agent-tools/` дают сжатый вывод (≤25k токенов, сначала фейлы, в конце канонический status-line). В работе агента предпочитай их сырым командам:

| Обёртка | Заменяет | Форма вывода |
|---|---|---|
| `agent-tools/test.sh [pkg…]` | `make test` | Счётчики PASS на зелёном; `pkg/file_test.go:line FAIL TestName` на каждое падение |
| `agent-tools/lint.sh [paths…]` | `make lint` | Топ замечаний, отсортированные по severity, формат `file:line:col [linter] msg` |
| `agent-tools/build.sh` | `go build ./...` | Первые 5 ошибок с `file:line:col`, дальше count остальных |
| `agent-tools/search.sh PATTERN [path]` | `grep -r` | По умолчанию список файлов с матчами; `--lines` — с контекстом строк |
| `agent-tools/repomap.sh [pkg]` | (нет) | Сводка публичных символов по пакетам, размер ограничен бюджетом |
| `agent-tools/status.sh [--full]` | (нет) | Сводка состояния репо для `/smartdev`: ветка, working tree, активные планы, домены, миграции, TODO. `--full` добавляет sqlc-diff/mocks/lint. |
| `agent-tools/validate-domain-doc.sh [path]` | (нет) | Валидатор DDD-артефактов в `docs/domains/`. Проверяет frontmatter, обязательные секции, при `status: stable` — non-empty bodies + ref-целостность. |

Все обёртки принимают `--format=concise|detailed` и заканчиваются одной канонической строкой статуса (`OK` / `FAIL: <reason>`).

## 2. Тестирование

- **Юнит-тесты** лежат рядом с кодом как `<file>_test.go`. В корне домена тесты на `usecase.go`, `policy.go`. В `delivery/http/` — на хендлеры.
- **Интеграционные тесты** под build-tag `integration`: `//go:build integration`. Запускаются через `make test-int`. Поднимают Postgres + NATS через `internal/testkit/pgseed` и `testcontainers-go`.
- **Моки** — mockery v3, файлы `mock_<interface>_test.go`, конфиг в `.mockery.yml`. Руками никогда не править — `make mocks` после изменения интерфейсов из `.mockery.yml`.
- **Fuzz**-таргеты лежат рядом с тестами; `make test-fuzz` гоняет 10s-смок на каждую цель.
- **TDD обязателен** для багфиксов: сначала пишешь падающий тест, видишь red, потом фиксишь до green.

## 3. Структура проекта

```
cmd/
  api/           HTTP-сервер entrypoint, fx wiring
  worker/        Фоновые подписчики, jobs, outbox drain
  migrate/       Раннер миграций (golang-migrate)
internal/
  domain/<name>/                Бизнес-домен
    dto.go, entity.go, errors.go, events.go, policy.go, usecase.go, *_test.go
    delivery/http/              huma/v2 + chi хендлеры
    infra/postgres/             repository.go + queries/*.sql + db/ (sqlc-сгенерированный)
    module/                     fx.Module + cross-domain адаптеры
  platform/
    authgate/    Гейт authn/authz, виден хендлерам
    config/      Прикладной конфиг, fx.Module
    identity/    Типы subject/principal
  worker/        bruteforce.go + subscribers/ (NATS-консьюмеры)
  testkit/       pgseed/, identitytest/ — хелперы для тестов
pkg/
  policy/        Публичные policy-примитивы
  tokens/        Публичные token-примитивы
migrations/      NNNN_name.up.sql + NNNN_name.down.sql, последовательная нумерация
scripts/
  agent-tools/   Обёртки с agent-friendly выводом
  claude-hooks/  Детерминированные хуки для Claude Code
  git-hooks/     commit-msg, pre-commit, pre-push (ставятся через `make hooks-install`)
.claude/
  settings.json  Конфиг Claude Code (хуки, permissions, model)
  rules/         Path-scoped правила (грузятся когда соотв. файлы в контексте)
  skills/        Процедурные воркфлоу: smartdev, plan, add-domain, add-migration,
                 add-endpoint, add-event, add-port, triage-failure, run-pre-merge
  agents/        Subagent'ы (explorer, reviewer, migration-author)
  plans/         roadmap.md (индекс) + <slug>.md (per-task) — committable, ревьюются в PR
  WORKFLOW.md    Описание дневного флоу для людей
  research/      synthesis.md + raw-briefings.md (кросс-вендорное исследование)
```

`internal/domain/auth`, `user`, `verification` — это **референс-примеры**. Для реального сервиса изучи их, потом удали и замени своими доменами.

## 4. Стиль кода

- **gofumpt** (строгий gofmt) обязателен. `make fmt` или PostToolUse-хук (он сам гонит gofumpt на каждый `.go`-edit).
- **golangci-lint** конфиг в `.golangci.yml`. Критичные правила:
  - **forbidigo**: запрещены `fmt.Print*` и прямые `slog.*`. Использовать `golibinfra/logkit.Logger`.
  - **depguard**: бизнес-домены не импортят друг друга напрямую. `auth` ↛ `user`, `user` ↛ `auth`, `verification` ↛ `{user, auth}`. Используй порты + адаптеры в `<domain>/module/adapters.go`.
  - errcheck, errorlint, gosec, govet, contextcheck, noctx, rowserrcheck, bodyclose, sqlclosecheck — всё на.
- Комментарии — только когда *зачем* неочевидно. Не пересказывай что делает код.
- Ошибки оборачивай через `%w`, используй `errors.Is` / `errors.As`. Sentinel-ошибки в `errors.go` корня домена.

## 4a. Telemetry

`golibinfra/telemetry` поддерживает два режима:

- **Local dev** (дефолт `.env.example`): `OTEL_STDOUT_ONLY=true` — никакого OTLP, всё в stdout. Или: указать локальный коллектор через `OTEL_EXPORTER_OTLP_ENDPOINT`.
- **Production**: `OTEL_TENANT` + `OTEL_TOKEN` непустые → к каждому OTLP-запросу добавляется `Authorization: Bearer <token>`, в resource'ы добавляется `service.namespace=<tenant>` и `tenant=<tenant>`. Корп-сторона использует это для per-tenant маршрутизации.

Production-значения **в этот repo не попадают**. Они выдаются корп-onboarding (`stit/infra/scripts/onboard-project.sh`) и рендерятся vault-agent'ом в env_file на проектном VPS — см. `stit/project-infra-template-compose` (ADR 0002 о централизации env'ов в infra-repo).

## 5. Git workflow

- **Conventional Commits** проверяются `scripts/git-hooks/commit-msg`. Допустимые типы: `feat`, `fix`, `refactor`, `docs`, `test`, `chore`, `meta`. Формат: `<type>(<scope>)?: <subject>`, subject ≤72 символа.
- **Линейная история**. Squash-merge в `main` через PR.
- **Хуки** ставятся `make hooks-install` — commit-msg, pre-commit (gofumpt + vet + lint), pre-push (`make pre-merge` — полный gate). Skip только через `SKIP_PREPUSH=1`, не `--no-verify`.
- **Релизы** — annotated semver-tag (`git tag -a vX.Y.Z -m "..."`) на чистом `main` после зелёного `make pre-merge`. Lightweight-теги отвергаются `release-preflight`. Тег = lockstep для api+migrate+worker images.
- Примеры:
  - `feat(user): add password reset flow`
  - `fix(auth): block reused refresh tokens by validating jti`
  - `meta(claude): rebuild agent skills per agentskills.io spec`

## 6. Domain artifacts (DDD)

Бизнес-смысл живёт в `docs/domains/<context>/`. Это **первичный source of truth**, код вторичен.

```
docs/domains/
├── README.md         ← структура, workflow
├── MAPPING.md        ← DDD ↔ Go (куда что ляжет в коде)
├── _templates/       ← шаблоны для копирования
└── <context>/
    ├── README.md     ← purpose + ubiquitous language (роллап)
    ├── <Aggregate>.md
    ├── events/<Event>.md
    ├── use-cases/<Operation>.md
    └── policies/<PolicyName>.md
```

**Kinds:** `aggregate`, `entity`, `event`, `use-case`, `policy`. Каждый имеет обязательный набор H2-секций, проверяемый валидатором.

**Workflow:**
1. Копируешь шаблоны в `docs/domains/<ctx>/`, заполняешь минимум (frontmatter + Properties + Methods names).
2. `/domain-deepen <ctx>` — skill ведёт волнами вопросов inside-out, добивая секции.
3. `scripts/agent-tools/validate-domain-doc.sh docs/domains/<ctx>` зелёный → промоушн `status: draft → stable`.
4. `/smartdev` → `/plan <task>` → `add-domain` / `add-endpoint` / `add-event`. Скаффолдинг-скиллы блокируются если есть `status: draft` или валидатор красный.

См. `.claude/rules/domain-artifacts.md` и `.claude/skills/domain-deepen/SKILL.md`.

## 7. Границы

### Делать всегда
- Перед claim "готово" гнать `make pre-merge` (или эквивалент через `agent-tools/*`). Без верификации ты единственный feedback-loop.
- Cross-domain вызовы — только через порты + адаптеры. depguard ловит прямые импорты.
- Логи — через `logkit.Logger`.
- Регенерить сгенерированный код при изменении источника:
  - `make sqlc` после правки `internal/domain/*/infra/postgres/queries/` или `migrations/`
  - `make mocks` после изменения интерфейсов из `.mockery.yml`
- Conventional Commits.
- В агентских флоу — обёртки `agent-tools/*`. Они режут вывод и подтягивают фейлы наверх.

### Сначала спросить
- `migrations/` — схему БД сложно откатывать. Используй skill `add-migration`.
- `pkg/policy` и `pkg/tokens` — security-sensitive примитивы.
- `internal/platform/authgate/` — auth-обвязка задевает все домены.
- `go.mod` / `go.sum` — изменения зависимостей идут через ревью владельца.
- Любой `--no-verify` на git-команде.
- `git tag -a vX.Y.Z` и `make release` — артефакт видимый вне репо (image в registry), сначала explicit OK от пользователя. Используй skill `/release`.

### Не делать никогда
- Скипать pre-commit / pre-push через `--no-verify`. PreToolUse-хук это блокирует.
- Коммитить `.env` / `.env.local` / креды. Pre-write хук защищает эти пути.
- Править сгенерированные файлы руками (`internal/domain/*/infra/postgres/db/*.go`, `mock_*_test.go`). Регенерируй.
- Использовать `fmt.Print*` или `slog.*` напрямую. Только `logkit.Logger`.
- Пушить в `main` напрямую. PR + squash-merge.
- Image-теги вида `:latest` / `:edge` / lightweight git-tags. `release-preflight` отвергает; infra-template их и не примет.
- Принимать архитектурные решения без согласования. Skills задают процедуры; отклонение — это решение и требует явного OK от пользователя.
