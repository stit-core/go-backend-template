# E2E / smoke harness

Типизированный pytest-харнес поверх docker-compose стека. Колхозный bash+curl
здесь не живёт: ассерты, setup/teardown, выбор smoke/e2e — на pytest; «своё»
там где ценно — типизированный API-клиент и переиспользуемые сценарии.

## Разделение зон

| Зона | Где | Отвечает за |
|---|---|---|
| Стек | `stack/` | docker-compose + секреты (bash). `up.sh`/`down.sh`, `compose.yml`, `Dockerfile.local`, `configs/`. |
| Транспорт+модели | `client/` | типизированная httpx-обёртка + pydantic-DTO (зеркало huma). |
| Сценарии | `scenarios/` | переиспользуемые step-функции над `client` (без ассертов). |
| Infra-интроспекция | `infra/` | типизированный доступ к side-effects: `pg.py` (psycopg, users/refresh_sessions/bruteforce/email_verifications), `outbox.py` (outbox-таблица), `nats.py` (core-NATS collector), `redis.py` (cache-probe). |
| Smoke-тесты | `smoke/` | boot + критический happy-path, `@pytest.mark.smoke`. |
| E2E-тесты | `e2e/` | сценарии по доменам, `@pytest.mark.e2e`. |
| Фикстуры | `conftest.py` | session-стек, `api`, `user`, `admin`, `other_user`, `pg`, `redis`, `nats_collector`. |

Папки `smoke/`/`e2e/` организуют, маркеры селектят: `pytest -m smoke` / `pytest -m e2e`.

## Side-effect ассерты

Порты `postgres:5432` / `nats:4222` / `redis:6379` проброшены на `127.0.0.1`
(loopback) — `infra/`-хелперы ходят туда напрямую. Эвенты доменов идут через
`outbox`-таблицу (детерминированно, на каждый эвент) → outboxkit-worker
дренит в **core NATS** (реплея нет). Поэтому `NatsCollector` подписывается
**до** HTTP-действия; `outbox.wait_for(..., published=True)` подтверждает дрен.
Showcase полного async-пути — `e2e/test_event_flow.py`. Bruteforce-счётчики
в Postgres (`bruteforcekit/pgstore`), не Redis.

## Admin-принципал

Публичного пути сделать admin нет (`register` → `client`). Стек при старте
гоняет one-off `cmd/seed-admin` (creds `E2E_ADMIN_EMAIL`/`E2E_ADMIN_PASSWORD`
из `.env.local`, дефолты в `up.sh`), `api` ждёт его completed. Фикстура
`admin` логинится этими кредами; `other_user` — второй client для
cross-user 403.

## Покрытие и границы

Покрыто (side-effects сверяются в Postgres/outbox/NATS):
- **auth**: login/refresh-rotation/reuse-detect/logout, change-password,
  bruteforce-LOCK (429 + `locked_until`).
- **user**: register, /me, get-by-id, list (пагинация/`status`/границы),
  PATCH (optimistic version → 409), DELETE (soft → `status=deleted`).
- **authz** (`user/policy.go`): list = admin-only; get/update/delete =
  selfOrAdmin; cross-user → 403.
- **negatives**: huma schema-валидация (4xx, у v2 — 422), 404, 401-без-cookie.
- **cross-domain**: register → outbox → core-NATS → verification consume →
  `email_verifications` (`e2e/test_event_flow.py`).

Сознательно вне e2e: **verification confirm-roundtrip** (email-verify /
password-reset confirm). У шаблона нет канала доставки — `logMailerAdapter`
заглушка by design, raw-токен в outbox/NATS осознанно отсутствует. Это не
баг и не дыра харнеса: confirm-логика — дом Go usecase/integration-тестов,
request-side покрыт e2e. Реальный Mailer + mail-capture (Mailpit) — extension
point форка.

## Запуск

Нужен Docker и `./.env.local` с `STIT_OBS_TOKEN` + `OTEL_TENANT`
(шаблон — `.env.local.example`; формат — `.claude/rules/env-local.md`).

```
make e2e          # весь набор (поднимает стек, гоняет, сносит)
make e2e-smoke    # только -m smoke (быстрый gate)
```

Под капотом: `test/.venv` (из `test/requirements.txt`) + pytest. Конкретно —
`scripts/tools/e2e.sh` (сжатый вывод, каноничная строка `OK`/`FAIL`).

## Зависимости

Пины в `requirements.txt` (обновление — через ревью, как `go.mod`).
Tool-конфиг (pytest/ruff/mypy) — `pyproject.toml`.
