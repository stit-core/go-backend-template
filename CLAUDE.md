# go-backend-template — гайд для агентов

Шаблон Go HTTP-бэкенда на стеке stit.tech: huma/v2 + chi + pgx/v5 + sqlc + uber/fx + golibinfra (telemetry, authn, outbox, brute-force, cache, NATS, audit, clock).

Этот файл — постоянный гайд (грузится в контекст всегда). Контекстные правила лежат в `.claude/rules/`. Процедурные воркфлоу — в `.claude/skills/`. Описание дневного флоу — в `.claude/WORKFLOW.md`.

**Forward-only документация.** Любой текст в репо (этот файл, `README.md`, `docs/**`, skills, rules, plans, `WORKFLOW.md`) описывает **только текущее состояние и намерение** — не «было X, стало Y, потому что Z». История изменений хранится в `git log` и Conventional Commits; дублировать её прозой — значит держать два источника правды. Полное правило, чего не пишем и как вместо этого — `.claude/rules/forward-only.md`.

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
| `make help` | Сгруппированный список всех таргетов |
| `make test` / `lint` / `build` | Concise-by-default; `FORMAT=detailed`/`VERBOSE=1` — полно; `PKG=./x/...` — подмножество |
| `make fmt` | gofumpt -w |
| `make check` | Канонический gate: fmt+vet+lint+test+build+gen-check. Гонится pre-push хуком |
| `make gen` / `make gen-check` | Регенерация (sqlc+mockery) / проверка их drift |
| `make migrate-up` / `make migrate-down` | Накатить / откатить одну миграцию (нужен `PG_DSN`) |
| `make e2e` / `make e2e-smoke` | pytest-харнес `test/` (Docker + `./.env.local`) |
| `make new-domain NAME=order` | Скаффолд скелета нового домена |
| `make status` / `map` / `search Q=` / `logs A=` | INTEL: cockpit-feed / пакеты / поиск / прод-логи |
| `make release VERSION=vX.Y.Z` | confirm→tag→preflight(+`doctor`)→buildx api+migrate push lockstep→INFRA-HANDOFF→STOP. См. README §Release |
| `make setup` / `make doctor` | dev-tools+git-хуки / fail-fast preflight (docker-auth/drift/миграции) |
| `make test-int` / `test-fuzz` / `test-bench` / `test-cover` / `tidy` | EXTRA вспомогательное |

`make build` собирает `bin/api`. Воркер отдельным бинарём не собирается — outbox drain, NATS subscribers и background jobs живут в `cmd/api/` через fx.Lifecycle.

Реализация за таргетами — `scripts/tools/*` (`test.sh`/`lint.sh`/`build.sh`/`status.sh`/`e2e.sh`/`logs.sh`/`doctor.sh`/`validate-domain-doc.sh`/`validate-plan.sh`). Напрямую не зовутся: агент дёргает `make <verb>`, скрипты дают сжатый вывод и каноническую строку (`OK` / `FAIL: <reason>` / `ATTENTION:`). `make <x> FORMAT=detailed` / `VERBOSE=1` — полный вывод для человека/отладки.

## 2. Тестирование

- **Юнит-тесты** лежат рядом с кодом как `<file>_test.go`. В корне домена тесты на `usecase.go`, `policy.go`. В `delivery/http/` — на хендлеры.
- **Интеграционные тесты** под build-tag `integration`: `//go:build integration`. Запускаются через `make test-int`. Поднимают Postgres + NATS через `internal/testkit/pgseed` и `testcontainers-go`.
- **Моки** — mockery v3, файлы `<interface_snake>.go` в подпакете `mocks/` рядом с интерфейсом (пакет `mocks`, тип `Mock<Interface>`). Конфиг — `.mockery.yml`. Руками никогда не править — `make gen` после изменения интерфейсов.
- **Fuzz**-таргеты лежат рядом с тестами; `make test-fuzz` гоняет 10s-смок на каждую цель.
- **E2E/smoke** — типизированный pytest-харнес в `test/` поверх docker-compose стека. `make e2e` / `make e2e-smoke` (или `scripts/tools/e2e.sh [--smoke]`). Зоны: `test/stack/` (docker+bash), `test/client/` (typed httpx+pydantic), `test/scenarios/` (step-функции), `test/infra/` (pg/outbox/nats/redis — side-effect ассерты, порты на 127.0.0.1), `test/smoke/` + `test/e2e/` (тесты, маркеры `smoke`/`e2e`). Тесты не только black-box: ассертят строки БД, outbox-эвенты, core-NATS доставку, authz-матрицу, bruteforce-lock. Verification confirm-roundtrip — дом Go usecase/integration-тестов (у шаблона нет mail-канала by design), не e2e-пробел. Нужен Docker + `./.env.local`. Детали — `test/README.md`.
- **TDD обязателен** для багфиксов: сначала пишешь падающий тест, видишь red, потом фиксишь до green.

## 3. Структура проекта

Один runtime-бинарь (`cmd/api`) + standalone-CLI миграций (`cmd/migrate`). HTTP, outbox drain, NATS subscribers, background jobs — всё внутри `cmd/api` через одну fx.Lifecycle. Отдельного worker-процесса нет.

```
cmd/
  api/           Единственный runtime-бинарь: HTTP-сервер (huma+chi) + outbox drain
                 (golibinfra/outboxkit) + NATS subscribers + background jobs.
  migrate/       Standalone CLI: golang-migrate up/down. Запускается init-контейнером
                 (отдельный image `<org>/backend-migrate:<tag>` со вшитыми SQL-файлами).
  seed-admin/    Standalone one-off: idempotent bootstrap первого admin
                 (SEED_ADMIN_EMAIL/PASSWORD, хеш authnkit). Публичного пути
                 сделать admin нет (register=client). Нужен PG_DSN; не runtime.
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
    shared/      Cross-context платформенные VO (`docs/domains/_shared/`,
                 kind: vo). Один файл = один VO + его errors.go. Не зависит
                 от `internal/domain/`; бизнес-домены импортят напрямую.
  jobs/          Background jobs (например, `bruteforce.go` — cleanup brute-force
                 counters). Поднимаются fx.Lifecycle того же `cmd/api`.
  testkit/       pgseed/, identitytest/ — хелперы для тестов
pkg/
  policy/        Публичные policy-примитивы
  tokens/        Публичные token-примитивы
migrations/      NNNN_name.up.sql + NNNN_name.down.sql, последовательная нумерация
test/            E2E/smoke-харнес (pytest): stack/ (docker+bash up.sh/down.sh),
                 client/ (typed httpx+pydantic), scenarios/ (step-функции),
                 infra/ (pg/outbox/nats/redis side-effect ассерты),
                 smoke/ + e2e/ (тесты, маркеры). Запуск — make e2e / e2e-smoke.
scripts/
  tools/   Обёртки с agent-friendly выводом (см. §1)
  claude-hooks/  Детерминированные хуки для Claude Code
  git-hooks/     commit-msg, pre-commit, pre-push (ставятся через `make setup`)
.claude/
  settings.json        Конфиг Claude Code (хуки, permissions, model) — committed
  settings.local.json  Локальные user-overrides — .gitignore'нутые
  rules/               Path-scoped правила (грузятся когда соотв. файлы в контексте)
  skills/              Процедурные воркфлоу: smartdev, plan, add-domain, add-migration,
                       add-endpoint, add-event, add-port, triage-failure, infra-handoff,
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
- **Push в `main` разрешён** (прямой или через PR); линейная история. Gate — git `pre-push → make check`.
- **Хуки** ставятся `make setup` — commit-msg, pre-commit (gofumpt + vet + lint + validate-domain-doc + validate-plan), pre-push (`make check` — полный gate). Skip только через `SKIP_PREPUSH=1`, не `--no-verify`.
- **Релизы** — `make release VERSION=vX.Y.Z` (один проход: confirm→annotated tag→preflight+doctor→buildx push lockstep→INFRA-HANDOFF→STOP) на чистом `main` после зелёного `make check`. Lightweight-теги отвергаются `release-preflight`.
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
├── _shared/          ← cross-context платформенные VO (kind: vo)
│   ├── README.md     ← роллап: какие VO здесь и для кого
│   └── <Name>.md     ← один cross-context VO (PhotoRef, MoneyAmount, …)
└── <context>/
    ├── README.md     ← purpose + ubiquitous language (роллап)
    ├── <Aggregate>.md
    ├── events/<Event>.md
    ├── use-cases/<Operation>.md
    └── policies/<PolicyName>.md
```

Cross-context платформенный VO (структурно идентичен в ≥2 контекстах, без доменной семантики) — **только** отдельным файлом `_shared/<Name>.md` (`kind: vo`), маппится в `internal/platform/shared/`. Внутри-контекстные VO (`Phone`, `Email`, `OrderTotal`) остаются inline в aggregate-файле своего контекста — в `_shared/` их класть нельзя.

**Kinds:** `aggregate`, `entity`, `event`, `use-case`, `policy`, `vo` (`vo` — только `_shared/`). Каждый имеет обязательный набор H2-секций, проверяемый валидатором.

**Workflow:**
1. Копируешь шаблоны в `docs/domains/<ctx>/`, заполняешь минимум (frontmatter + Properties + Methods names).
2. `/domain-deepen <ctx>` — skill ведёт волнами вопросов inside-out, добивая секции.
3. `scripts/tools/validate-domain-doc.sh docs/domains/<ctx>` зелёный → промоушн `status: draft → stable`.
4. `/smartdev` → `/plan <task>` → `add-domain` / `add-endpoint` / `add-event`. Скаффолдинг-скиллы блокируются если есть `status: draft` или валидатор красный.

См. `.claude/rules/domain-artifacts.md` и `.claude/skills/domain-deepen/SKILL.md`.

## 7. Границы

### Делать всегда
- Перед claim "готово" гнать `make check`. Без верификации ты единственный feedback-loop.
- **Тронул инфра-поверхность** (новый/изменённый required env-var или Vault-секрет; миграция destructive/lock/backfill/extension; cross-service NATS/outbox-subject; новый порт/breaking API; новая runtime-зависимость в `go.mod`) → **обязан оформить INFRA-HANDOFF-блок и попросить юзера передать его инфра-админу** до claim «готово»/коммита. Не молчи. Механизм — `.claude/rules/infra-impact.md` + skill `infra-handoff`; `make release` агрегирует.
- Cross-domain вызовы — только через порты + адаптеры. depguard ловит прямые импорты.
- Логи — через `logkit.Logger`.
- Регенерить сгенерированный код `make gen` после правки `internal/domain/*/infra/postgres/queries/`, `migrations/` или интерфейсов из `.mockery.yml`.
- Conventional Commits.
- Дёргай `make <verb>` — concise-by-default; скрипты `scripts/tools/*` за таргетами, напрямую не зови.

### Сначала спросить
- `migrations/` — схему БД сложно откатывать. Используй skill `add-migration`.
- `pkg/policy` и `pkg/tokens` — security-sensitive примитивы.
- `internal/platform/authgate/` — auth-обвязка задевает все домены.
- `go.mod` / `go.sum` — изменения зависимостей идут через ревью владельца.
- Любой `--no-verify` на git-команде.
- `git tag -a vX.Y.Z` и `make release` — артефакт видимый вне репо (image в registry), сначала explicit OK от пользователя. Используй skill `/release`.

### Не делать никогда
- Скипать pre-commit / pre-push через `--no-verify`. PreToolUse-хук это блокирует.
- Коммитить `.env.local` / `.env` / `*.pem` / `*.key` или цитировать их содержимое в чат. От коммита страхует `.gitignore`; pre-write хук защищает от правки только `*.pem` / `*.key` + сгенерированные файлы.
- Править сгенерированные файлы руками (`internal/domain/*/infra/postgres/db/*.go`, `**/mocks/*.go`). Регенерируй через `make gen`.
- Использовать `fmt.Print*` или `slog.*` напрямую. Только `logkit.Logger`.
- Image-теги вида `:latest` / `:edge` / lightweight git-tags. `release-preflight` отвергает; infra-template их и не примет.
- Запускать deploy-команды чужих репо из этого: `make ship`, `bash scripts/deploy.sh`, `ssh <vps>`, `vault kv patch`. Зона ответственности этого репо — код → image в registry. После `make release` — STOP; дальше `<client-org>/infra` + корп-сторона.
- Принимать архитектурные решения без согласования. Skills задают процедуры; отклонение — это решение и требует явного OK от пользователя.
