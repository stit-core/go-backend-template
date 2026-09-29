<div align="center">

# go-backend-template

### Go HTTP-бэкенд со стандартами из коробки

Шаблон Go-бэкенда экосистемы stit: huma/v2 + chi + pgx/v5 + sqlc + uber/fx,
поверх [`golibinfra`](https://git.stit.tech/stit-core/golibinfra) (authn,
outbox, cache, NATS, audit, logging). Структурные логи, тесты и релиз — встроены;
вы пишете домены, не обвязку.

[![License](https://img.shields.io/badge/license-Apache--2.0-2563eb)](LICENSE)
[![Часть stit](https://img.shields.io/badge/stit-открытые_шаблоны-6366f1)](https://git.stit.tech/stit-core/stit-public)
[![Go](https://img.shields.io/badge/Go-00ADD8)](#что-внутри)
[![API](https://img.shields.io/badge/API-huma%2Fv2_%2B_OpenAPI_3.1-0ea5e9)](#что-внутри)
[![DB](https://img.shields.io/badge/DB-pgx_%2B_sqlc-22c55e)](#что-внутри)
[![Logging](https://img.shields.io/badge/logging-structured_slog-8b5cf6)](#что-внутри)

</div>

---

## Место в экосистеме

```
business-strategy-template (<org>/strategy)   ← проектирование: сервисы и их домены
        │  scaffold доменов в Go по stable-артефактам
        ▼
go-backend-template (<org>/backend)  ← вы здесь   ──► stit-core/golibinfra
        │  make release → образы в registry
        ▼
project-infra-template-compose (<org>/infra)   ← хаб проекта: план, агенты, выкатка (make bump)
```

Образы уходят в container registry Gitea (`git.stit.tech`): хост и орга выводятся из
`git remote get-url origin`, в коде не зашиты. Конфиг и секреты — обычные env, их задаёт
`<org>/infra`. Как сюда приходят агенты и что репо отдаёт и берёт —
[`docs/reference/ecosystem.md`](docs/reference/ecosystem.md).

## Использовать как шаблон

В Gitea помечен `template: true`. **Полный путь** от форка к своему проекту —
[`docs/getting-started.md`](docs/getting-started.md): форк → `/init-service` →
проектирование стека (`docs/stack/`; домены — в соседнем strategy-репо) →
`/de-template` → scaffold → проект, который ничего не помнит про шаблон.

Быстрый старт (чтобы собралось):

```bash
# 1. Use this template → <org>/<service>, склонировать
# 2. (один раз на машину) доступ к приватным модулям той же Gitea:
git config --global url.git@git.stit.tech:.insteadOf https://git.stit.tech/
# 3. зелёный gate (make setup выводит хост из go.mod):
make setup && make check
# 4. запустить онбординг:
cp .env.example .env   # заполнить
claude                 # → /init-service
```

Module path, README, фундамент и удаление reference-доменов делает флоу
`/init-service` → `/de-template` по заполненным `docs/`. Не правьте фундамент
руками до этого.

## Что внутри

- **HTTP** — huma/v2 поверх chi, OpenAPI 3.1, structured errors.
- **БД** — pgx/v5 + sqlc (typed queries), golang-migrate.
- **DI** — uber/fx. Один runtime-бинарь `cmd/api` (HTTP + outbox-drain + NATS-subs
  + jobs); `cmd/migrate` — отдельный image, init-контейнер.
- **Платформа** — [`golibinfra`](https://git.stit.tech/stit-core/golibinfra):
  structured logging, JWT/session authn, outbox, brute-force, NATS, audit, clock.
- **DDD** — артефакты доменов в соседнем strategy-репо; маппинг в Go —
  [`docs/reference/domain-mapping.md`](docs/reference/domain-mapping.md).
- **Тесты** — unit (`-race`), integration (testcontainers), fuzz, mockery v3, e2e (pytest).
- **Gate** — `make check` = fmt-check + vet + lint + migrations-check + test-int (юнит +
  integration, нужен Docker) + build + gen-check + cover-check ≥75% (pre-push хук).

## Команды

`make help` — полный список; concise-by-default. Канонический gate — **`make check`**.
Полный гайд агента — [`CLAUDE.md`](CLAUDE.md); точка входа в сессию — `/smartdev`.

```bash
make check        # полный gate перед push (fmt, vet, lint, тесты с integration, build, gen, миграции, coverage)
make gen          # регенерация sqlc, mockery и openapi.json
make e2e          # поднять docker-стек → pytest → снести
make release VERSION=vX.Y.Z   # api + migrate образы в registry (skill /release)
```

## Документация

Карта — [`docs/README.md`](docs/README.md):
- Потребителю — [`getting-started.md`](docs/getting-started.md); контрибьютору —
  [`contributing.md`](docs/contributing.md).
- Reference — [architecture](docs/reference/architecture.md) ·
  [release](docs/reference/release.md) · [ecosystem](docs/reference/ecosystem.md).
- Design SoT — домены в соседнем strategy-репо · [`docs/stack/`](docs/stack/README.md).

## E2E

Типизированный pytest-харнес `test/` поверх docker-compose стека
(postgres/nats/migrate/api). Нужен Docker и `./.env.local`
(`E2E_ADMIN_*`; шаблон — `.env.local.example`). Детали — `test/README.md`.

```bash
make e2e          # весь набор
make e2e-smoke    # только -m smoke (быстрый gate)
```

## Release

Зона репо: **код → image в registry, STOP.** Выкатка — `make bump` в `<org>/infra`. Одна
команда `make release VERSION=vX.Y.Z` (skill `/release`). Полностью —
[`docs/reference/release.md`](docs/reference/release.md).

## Часть экосистемы stit

Это один из открытых шаблонов stit (Apache-2.0) — фундамент «-1 уровня»,
берите бесплатно и владейте. Карта всех шаблонов и точка входа —
**[витрина stit](https://git.stit.tech/stit-core/stit-public)**. Нужна
команда, стандарт или поддержка — **[наш подход и контакты (vision)](https://git.stit.tech/stit/vision)**.

## Лицензия

**Apache-2.0** — см. [`LICENSE`](LICENSE).
