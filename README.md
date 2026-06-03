# go-backend-template

Шаблон Go HTTP-бэкенда на стеке stit.tech: huma/v2 + chi + pgx/v5 + sqlc + uber/fx +
[`golibinfra`](https://git.stit.tech/stit-infra/golibinfra) (telemetry, authn, outbox,
brute-force, cache, NATS, audit, clock).

Часть экосистемы из 5 репо: [`stit-infra/infra`](https://git.stit.tech/stit-infra/infra)
(corp control plane) ←── **этот** (форк в `<org>/backend`, build+push) ──►
[`project-infra-template-compose`](https://git.stit.tech/stit-template/project-infra-template-compose)
(форк в `<org>/infra`, deploy). Домены продукта — в соседнем docs-репо (`<org>/docs`, форк
`project-docs-template`). Подробнее — [`docs/reference/ecosystem.md`](docs/reference/ecosystem.md).

## Использовать как шаблон

В Gitea помечен `template: true`. **Полный пошаговый путь** от форка к своему проекту —
[`docs/getting-started.md`](docs/getting-started.md): форк → `/init-service` →
проектирование стека (`docs/stack/`; домены уже готовы в соседнем docs-репо) → `/de-template` →
scaffold → проект, который ничего не помнит про шаблон.

Быстрый старт (чтобы собралось):

1. **Use this template** → `<org>/<service>`, склонировать.
2. (Один раз на машину) доступ к приватным модулям `git.stit.tech`:
   ```bash
   git config --global url.git@git.stit.tech:.insteadOf https://git.stit.tech/
   ```
3. `make setup && make check` — зелёное.
4. `cp .env.example .env`, заполнить. Запустить `claude` → `/init-service`.

Module path, README, фундамент и удаление reference-доменов — делает флоу `/init-service`
→ `/de-template` по заполненным `docs/`. Не правь фундамент руками до этого.

## Что внутри

- **HTTP**: huma/v2 поверх chi, OpenAPI 3.1, structured errors.
- **БД**: pgx/v5 + sqlc (typed queries), golang-migrate.
- **DI**: uber/fx. Один runtime-бинарь `cmd/api` (HTTP + outbox-drain + NATS-subs + jobs);
  `cmd/migrate` — отдельный image, init-контейнер.
- **Платформа**: `golibinfra` — OTLP telemetry, JWT/session authn, outbox, brute-force,
  cache, NATS, audit, clock.
- **DDD**: артефакты доменов — в соседнем docs-репо; маппинг в Go-код — `docs/reference/domain-mapping.md`.
- **Тесты**: unit (`-race`), integration (testcontainers), fuzz, mockery v3, e2e (pytest).
- **Gate**: `make check` = fmt+vet+lint+test+build+gen-check (pre-push хук).

## Команды

`make help` — полный список; concise-by-default. Канонический gate — `make check`. Полный
гайд агента — [`CLAUDE.md`](CLAUDE.md); точка входа в сессию — `/smartdev`.

## Документация

Карта — [`docs/README.md`](docs/README.md):
- Потребителю — [`getting-started.md`](docs/getting-started.md); контрибьютору —
  [`contributing.md`](docs/contributing.md).
- Reference — [architecture](docs/reference/architecture.md) ·
  [observability](docs/reference/observability.md) · [release](docs/reference/release.md) ·
  [ecosystem](docs/reference/ecosystem.md).
- Design SoT — домены в соседнем docs-репо (`../docs`) · [`docs/stack/`](docs/stack/README.md) · маппинг [`domain-mapping`](docs/reference/domain-mapping.md).

## E2E

Типизированный pytest-харнес `test/` поверх docker-compose стека
(postgres/nats/redis/otel-agent/migrate/api): `test/stack/` (docker+bash), `test/client/`
(httpx+pydantic), `test/scenarios/`, `test/infra/` (side-effect ассерты), `test/{smoke,e2e}/`.

```
make e2e          # весь набор: поднять стек → pytest → снести
make e2e-smoke    # только -m smoke (быстрый gate)
```

Нужен Docker и `./.env.local` (`STIT_OBS_TOKEN` + `OTEL_TENANT`; шаблон — `.env.local.example`,
формат — `.claude/rules/env-local.md`). Детали — `test/README.md`.

## Release

Зона репо: **код → image в registry, STOP.** Деплой — другие репо. Одна команда
`make release VERSION=vX.Y.Z` (skill `/release`). Полностью — [`docs/reference/release.md`](docs/reference/release.md).
