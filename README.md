# go-backend-template

Шаблон Go HTTP-бэкенда на стеке stit.tech: huma/v2 + chi + pgx/v5 + sqlc + uber/fx + `golibinfra` (telemetry, authn, outbox, brute-force, cache, NATS, audit, clock).

## Использовать как шаблон

В Gitea этот репо помечен `template: true`. Старт нового сервиса:

1. Открыть https://git.stit.tech/stit/go-backend-template → **Use this template** → создать `<org>/<service>`.
2. Склонировать новый репо локально:
   ```bash
   git clone git@git.stit.tech:<org>/<service>.git && cd <service>
   ```
3. Заменить module path:
   ```bash
   OLD=git.stit.tech/stit/go-backend-template
   NEW=git.stit.tech/<org>/<service>
   grep -rl "$OLD" --exclude-dir=.git | xargs sed -i "s|$OLD|$NEW|g"
   go mod edit -module "$NEW" && go mod tidy
   ```
4. `make tools && make hooks-install && make pre-merge` — убедиться что зелёное.
5. `cp .env.example .env` и заполнить локальные переменные.
6. Первый домен: `make new-domain NAME=<context>` → `docs/domains/<context>/` (см. `docs/domains/README.md`).

Альтернатива через Gitea API:
```bash
curl -X POST -H "Authorization: token $GITEA_TOKEN" -H "Content-Type: application/json" \
  -d '{"owner":"<org>","name":"<service>","private":false}' \
  https://git.stit.tech/api/v1/repos/generate \
  --url-query 'template_owner=stit' --url-query 'template_repo=go-backend-template'
```

## Что внутри

- **HTTP**: huma/v2 поверх chi, OpenAPI 3.1 авто-генерация, structured errors.
- **БД**: pgx/v5 + sqlc (typed queries), миграции через golang-migrate.
- **DI**: uber/fx, lifecycle hooks, граф собирается в `cmd/api/main.go` и `cmd/worker/main.go`.
- **Платформа**: `golibinfra` — OTLP telemetry, JWT/session authn, outbox-pattern publisher, brute-force guard, cache (Redis/in-mem), NATS pub/sub, audit log, clock.
- **DDD**: артефакты доменов в `docs/domains/<context>/` — source of truth перед кодом. См. `docs/domains/MAPPING.md` (DDD ↔ Go).
- **Тесты**: unit (`-short -race`), integration (`//go:build integration` + testcontainers), fuzz, mockery v3.
- **CI gate**: `make pre-merge` = fmt + vet + lint + test + build + sqlc-diff + mocks-check. Гонится pre-push хуком.

## Команды

| Команда | Что делает |
|---|---|
| `make tools` | Pinned dev-инструменты (gofumpt, golangci-lint, sqlc, mockery) |
| `make test` / `make test-int` / `make test-cover` | Unit / integration / coverage |
| `make lint` / `make fmt` | golangci-lint / gofumpt |
| `make build` / `make build-worker` | Бинарники api / worker |
| `make sqlc` / `make sqlc-diff` | Регенерация / проверка pgx-кода |
| `make mocks` / `make mocks-check` | Регенерация / проверка mockery v3 моков |
| `make migrate-up` / `make migrate-down` | Миграции (нужен `PG_DSN`) |
| `make new-domain NAME=order` | Скаффолд нового домена |
| `make pre-merge` | Канонический gate перед push |
| `make hooks-install` | Поставить git-хуки |

Полный гайд для агентов и человеков — `AGENTS.md`. Дневной флоу — `.claude/WORKFLOW.md`. Точка входа в сессию — `/smartdev`.

## Структура

```
cmd/{api,worker,migrate}/   точки входа
internal/<domain>/          бизнес-логика по доменам (DDD)
internal/platform/          shared infra (db, http server, fx-модули)
internal/testkit/           pgseed, fixtures, helpers
pkg/                        публичные пакеты (если есть)
migrations/                 SQL-миграции (golang-migrate)
docs/domains/<context>/     DDD-артефакты — source of truth для домена
.claude/                    skills, rules, plans для Claude Code
scripts/agent-tools/        обёртки с компактным выводом для агентов
```

## Контракт со stit-инфрой

- `git.stit.tech` — registry для `<org>/<service>` images (build через gitea-runner).
- `vault.stit.tech` — secrets backend, AppRole `project-<p>` (см. `stit/infra` → `scripts/onboard-project.sh`).
- `otel.stit.tech` — OTLP ingest (Bearer-token), tenant routing по `service.namespace`.
- Деплой через `stit/project-infra-template-compose` (docker-compose стек на проектный VPS).
