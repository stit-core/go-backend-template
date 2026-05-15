# AGENTS.md

Шаблон Go HTTP-бэкенда на стеке stit.tech: huma/v2 + chi + pgx/v5 + sqlc + uber/fx + golibinfra (telemetry, authn, outbox, brute-force, cache, NATS, audit, clock).

Этот файл — постоянный гайд (грузится в контекст всегда). Контекстные правила лежат в `.claude/rules/`. Процедурные воркфлоу — в `.claude/skills/`. Описание дневного флоу — в `.claude/WORKFLOW.md`.

## Где это в экосистеме

**Роль:** шаблон Go-бэкенда. Форкается в `<client-org>/backend`, разработчик собирает + пушит образ в `git.stit.tech/<client-org>/backend:<vX.Y.Z>`. Деплоем этот репо **не занимается** — handoff в `<client-org>/infra`.
**Зависит от:** `stit-infra/golibinfra` (импорт всех kit'ов: telemetry, httpkit, pgkit, authnkit, outboxkit, natskit, …); `stit-infra/infra` (контракт — `service_namespace == tenant`, OTLP endpoint, registry); `git.stit.tech` (push images).
**Используется:** `<client-org>/infra` (форк `stit-infra/project-infra-template-compose`) — пуллит образ через `vault kv patch …/meta backend_tag=<tag>` + `make ship` на проектной VPS.

Карта 4 репо, tenant/AppRole conventions и handoff-флоу — `stit-infra/infra/docs/ecosystem.md`. Локально про observability: `README.md §Observability` и `golibinfra/README.md` (ADR-0004 invariants).

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
| `make build` | Собрать api-бинарник (`bin/api`). Воркер как отдельный бинарь не используется — outbox drain, NATS subscribers и background jobs живут внутри `cmd/api/` через fx.Lifecycle. |
| `make sqlc` / `make sqlc-diff` | Регенерация / проверка pgx-кода |
| `make mocks` / `make mocks-check` | Регенерация / проверка mockery v3 моков |
| `make migrate-up` / `make migrate-down` | Накатить / откатить одну миграцию (нужен `PG_DSN`) |
| `make new-domain NAME=order` | Скаффолд скелета нового домена |
| `make pre-merge` | Канонический gate: fmt + vet + lint + test + build + sqlc-diff + mocks-check. Гонится pre-push хуком. |
| `make release-check` / `make release` | Pre-flight (main / clean / annotated `vX.Y.Z` / buildx) + push `<host>/<org>/backend{,-migrate}:<tag>` (lockstep). См. README §Release. |
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
| `agent-tools/logs.sh {query\|errors\|watch}` | (нет) | Чтение прод-логов через корп-Loki (`loki.stit.tech`) под bearer-токеном из `./credentials.md`. Локальный dev — логи в stdout, тулза не нужна. Подробнее — skill `query-logs`. |

Все обёртки принимают `--format=concise|detailed` и заканчиваются одной канонической строкой статуса (`OK` / `FAIL: <reason>`).

## 2. Тестирование

- **Юнит-тесты** лежат рядом с кодом как `<file>_test.go`. В корне домена тесты на `usecase.go`, `policy.go`. В `delivery/http/` — на хендлеры.
- **Интеграционные тесты** под build-tag `integration`: `//go:build integration`. Запускаются через `make test-int`. Поднимают Postgres + NATS через `internal/testkit/pgseed` и `testcontainers-go`.
- **Моки** — mockery v3, файлы `<interface_snake>.go` в подпакете `mocks/` рядом с интерфейсом (пакет `mocks`, тип `Mock<Interface>`). Конфиг — `.mockery.yml`. Руками никогда не править — `make mocks` после изменения интерфейсов.
- **Fuzz**-таргеты лежат рядом с тестами; `make test-fuzz` гоняет 10s-смок на каждую цель.
- **TDD обязателен** для багфиксов: сначала пишешь падающий тест, видишь red, потом фиксишь до green.

## 3. Структура проекта

Один runtime-бинарь (`cmd/api`) + standalone-CLI миграций (`cmd/migrate`). HTTP, outbox drain, NATS subscribers, background jobs — всё внутри `cmd/api` через одну fx.Lifecycle. Отдельного worker-процесса нет.

```
cmd/
  api/           Единственный runtime-бинарь: HTTP-сервер (huma+chi) + outbox drain
                 (golibinfra/outboxkit) + NATS subscribers + background jobs.
  migrate/       Standalone CLI: golang-migrate up/down. Запускается init-контейнером
                 (отдельный image `<org>/backend-migrate:<tag>` со вшитыми SQL-файлами).
internal/
  domain/<name>/                Бизнес-домен (bounded context).
    dto.go, entity.go, errors.go, events.go, policy.go, usecase.go, *_test.go
    delivery/http/              huma/v2 + chi REST-хендлеры
    delivery/nats/              NATS-subscriber (опц.) — обрабатывает входящие
                                события других доменов. Регистрируется fx того же
                                `cmd/api`. Реф. пример: `verification/delivery/nats/`
                                ловит `user.registered` и запускает email-verification.
    infra/postgres/             repository.go + queries/*.sql + db/ (sqlc, read-only)
    module/                     fx.Module + cross-domain адаптеры (`adapters.go`)
  platform/
    authgate/    Гейт authn/authz (JWT + identity.Principal), виден хендлерам
    config/      Прикладной конфиг, fx.Module
    identity/    Типы subject/principal
  jobs/          Background jobs (например, `bruteforce.go` — cleanup brute-force
                 counters). Поднимаются fx.Lifecycle того же `cmd/api`.
  testkit/       pgseed/, identitytest/ — хелперы для тестов
pkg/
  policy/        Публичные policy-примитивы
  tokens/        Публичные token-примитивы
migrations/      NNNN_name.up.sql + NNNN_name.down.sql, последовательная нумерация
scripts/
  agent-tools/   Обёртки с agent-friendly выводом (см. §1)
  claude-hooks/  Детерминированные хуки для Claude Code
  git-hooks/     commit-msg, pre-commit, pre-push (ставятся через `make hooks-install`)
.claude/
  settings.json        Конфиг Claude Code (хуки, permissions, model) — committed
  settings.local.json  Локальные user-overrides — .gitignore'нутые
  rules/               Path-scoped правила (грузятся когда соотв. файлы в контексте)
  skills/              Процедурные воркфлоу: smartdev, plan, add-domain, add-migration,
                       add-endpoint, add-event, add-port, triage-failure, run-pre-merge,
                       domain-deepen, query-logs, release
  agents/              Subagent'ы (explorer, reviewer, migration-author)
  plans/               roadmap.md (индекс) + <slug>.md (per-task) — committable, ревьюются в PR
  WORKFLOW.md          Описание дневного флоу для людей
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
- **Production**: `OTEL_TENANT` + `OTEL_TOKEN` непустые → к каждому OTLP-запросу добавляется `Authorization: Bearer <token>`. Resource attrs приложения: `service.name`, `service.version`, `deployment.environment.name`. `tenant` и `service.namespace` **ставит/перезатирает корп-otel-collector** на edge (по Bearer→`X-Scope-OrgID` через `tenant-auth` middleware → `resource/service_namespace` processor; `service_namespace == tenant` — инвариант, см. `stit-infra/infra` ADR-0004 §2). Приложение может выставить `service.namespace` сам — оно перезатрётся.

Корп-сторона маршрутизирует по `X-Scope-OrgID` header (не по `service.namespace`), но логи/метрики/трейсы получают `service_namespace=<tenant>` как label/attr автоматически — это даёт фильтр в Loki (`{service_namespace="<tenant>"}`) и Tempo (`{resource.service.namespace="<tenant>"}`).

Production-значения **в этот repo не попадают**. Они выдаются корп-onboarding (`stit-infra/infra/scripts/onboard-project.sh`) и рендерятся vault-agent'ом в env_file на проектном VPS — см. `stit-infra/project-infra-template-compose` (ADR 0002 о централизации env'ов в infra-repo).

## 5. Git workflow

- **Conventional Commits** проверяются `scripts/git-hooks/commit-msg`. Допустимые типы: `feat`, `fix`, `refactor`, `docs`, `test`, `chore`, `meta`. Формат: `<type>(<scope>)?: <subject>`, subject ≤72 символа.
- **Линейная история**. Squash-merge в `main` через PR.
- **Хуки** ставятся `make hooks-install` — commit-msg, pre-commit (gofumpt + vet + lint), pre-push (`make pre-merge` — полный gate). Skip только через `SKIP_PREPUSH=1`, не `--no-verify`.
- **Релизы** — annotated semver-tag (`git tag -a vX.Y.Z -m "..."`) на чистом `main` после зелёного `make pre-merge`. Lightweight-теги отвергаются `release-preflight`. Тег = lockstep для api+migrate images.
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
- Править сгенерированные файлы руками (`internal/domain/*/infra/postgres/db/*.go`, `**/mocks/*.go`). Регенерируй через `make sqlc` / `make mocks`.
- Использовать `fmt.Print*` или `slog.*` напрямую. Только `logkit.Logger`.
- Пушить в `main` напрямую. PR + squash-merge.
- Image-теги вида `:latest` / `:edge` / lightweight git-tags. `release-preflight` отвергает; infra-template их и не примет.
- Запускать deploy-команды чужих репо из этого: `make ship`, `bash scripts/deploy.sh`, `ssh <vps>`, `vault kv patch`. Зона ответственности этого репо — код → image в registry. После `make release` — STOP; дальше `<client-org>/infra` + корп-сторона.
- Принимать архитектурные решения без согласования. Skills задают процедуры; отклонение — это решение и требует явного OK от пользователя.
