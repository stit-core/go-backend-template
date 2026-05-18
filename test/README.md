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
| Фикстуры | `conftest.py` | session-стек (shell-out в `stack/up.sh`), `api`, `user`, `pg`, `redis`, `nats_collector`. |

Папки `smoke/`/`e2e/` организуют, маркеры селектят: `pytest -m smoke` / `pytest -m e2e`.

## Side-effect ассерты

Порты `postgres:5432` / `nats:4222` / `redis:6379` проброшены на `127.0.0.1`
(loopback) — `infra/`-хелперы ходят туда напрямую. Эвенты доменов идут через
`outbox`-таблицу (детерминированно, на каждый эвент) → outboxkit-worker
дренит в **core NATS** (реплея нет). Поэтому `NatsCollector` подписывается
**до** HTTP-действия; `outbox.wait_for(..., published=True)` подтверждает дрен.
Showcase полного async-пути — `e2e/test_event_flow.py`. Bruteforce-счётчики
в Postgres (`bruteforcekit/pgstore`), не Redis.

## Запуск

Нужен Docker и `./.env.local` с `STIT_OBS_TOKEN` + `OTEL_TENANT`
(шаблон — `.env.local.example`; формат — `.claude/rules/env-local.md`).

```
make e2e          # весь набор (поднимает стек, гоняет, сносит)
make e2e-smoke    # только -m smoke (быстрый gate)
```

Под капотом: `test/.venv` (из `test/requirements.txt`) + pytest. Конкретно —
`scripts/agent-tools/e2e.sh` (сжатый вывод, каноничная строка `OK`/`FAIL`).

## Зависимости

Пины в `requirements.txt` (обновление — через ревью, как `go.mod`).
Tool-конфиг (pytest/ruff/mypy) — `pyproject.toml`.
