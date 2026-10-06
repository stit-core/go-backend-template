# E2E / smoke harness

Типизированный pytest-харнес поверх docker-compose стека. Колхозный bash+curl
здесь не живёт: ассерты, setup/teardown, выбор smoke/e2e — на pytest; «своё»
там где ценно — типизированный API-клиент и переиспользуемые сценарии.

## Разделение зон

| Зона | Где | Отвечает за |
|---|---|---|
| Стек | `stack/` | docker-compose + креды seed-admin (bash). `up.sh`/`down.sh`, `lib.sh`, `compose.yml`, `Dockerfile.local`. |
| Транспорт+модели | `client/` | типизированная httpx-обёртка + pydantic-DTO (строгое зеркало huma: лишнее поле или тип — ошибка разбора). |
| Сценарии | `scenarios/` | переиспользуемые step-функции над `client` (без ассертов). |
| Infra-интроспекция | `infra/` | типизированный доступ к side-effects: `pg.py` (psycopg, users/сессии refresh/bruteforce/email_verifications), `outbox.py` (per-domain `outbox_<domain>`), `nats.py` (core-NATS collector). |
| Smoke-тесты | `smoke/` | boot + критический happy-path, `@pytest.mark.smoke`. |
| E2E-тесты | `e2e/` | сценарии по доменам, `@pytest.mark.e2e`. |
| Фикстуры | `conftest.py` | session-стек, `endpoints` (резолвнутые адреса), `api`, `user`, `admin`, `other_user`, `pg`, `nats_collector`. |

Папки `smoke/`/`e2e/` организуют, маркеры селектят: `pytest -m smoke` / `pytest -m e2e`.

## Side-effect ассерты

Сервисы публикуют порты эфемерно на `127.0.0.1` (Docker берёт свободный
host-порт); `up.sh` резолвит фактические адреса в `stack/.e2e-endpoints.env`,
`conftest` инжектит их в `infra/`-хелперы — те ходят на резолвнутый адрес
напрямую (детали — §Изоляция). Эвенты доменов идут через per-domain
outbox-таблицу `outbox_<domain>` (детерминированно, на каждый эвент) → outboxkit-drain
публикует в JetStream. `NatsCollector` слушает subject обычной core-подпиской (без
реплея), поэтому подписывается **до** HTTP-действия; `outbox.wait_for(..., published=True)`
подтверждает дрен.
Showcase полного async-пути — `e2e/test_event_flow.py`. Bruteforce-счётчики
в Postgres (`bruteforcekit/pgstore`).

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
  PATCH (поля одной записью, `version` из ответа; устаревшая → 409, неизвестный
  статус → 422, нет такого id → 404), DELETE (soft → `status=deleted`).
- **authz** (`user/policy.go`): list и смена статуса = admin-only (свой статус
  клиент не меняет → 403); get/update/delete = selfOrAdmin; cross-user → 403.
- **negatives**: huma schema-валидация (4xx, у v2 — 422), 404, 401-без-cookie.
- **cross-domain**: register → outbox → JetStream → verification consume →
  `email_verifications` (`e2e/test_event_flow.py`).

Сознательно вне e2e: **погашение ссылок из писем** (email-verify /
password-reset confirm). У шаблона нет канала доставки: dev-заглушка
`verification/infra/logmail` письмо не отправляет и токен не пишет никуда, в
outbox и NATS токена тоже нет. Путь писем целиком — запрос сброса, оба письма,
погашение обеих ссылок — проходит `TestBoot` (`cmd/api`, шаг `make check`) с
перехватом mailer графа; правила погашения держат usecase- и
integration-тесты verification. Реальный Mailer + mail-capture (Mailpit) —
extension point форка.

## Изоляция

Несколько форков шаблона гоняют `make e2e` на одной машине параллельно без
пересечений:

- **Project name** детерминирован из пути репо: `e2e-<sha1(git-toplevel)>`
  (`stack/lib.sh`, sourced в `up.sh`/`down.sh`). Контейнеры, сети, volumes и
  локальный образ (`${COMPOSE_PROJECT_NAME}:local`) project-scoped → форки не
  схлопывают ничего по именам, `down.sh` бьёт ровно свой стек без передачи
  имени между процессами.
- **Host-порты эфемерны** (`127.0.0.1::<port>`) — Docker выбирает свободный.
  `up.sh` после `compose up` резолвит фактические порты и пишет
  `stack/.e2e-endpoints.env` (gitignored, чистится `down.sh`). `conftest`
  читает его: фикстура `stack` отдаёт `E2E_API_BASE_URL`, фикстуры
  `pg`/`nats_collector` инжектят DSN/URL явным аргументом
  (module-дефолты `infra/*.py` замораживаются на import — обход через
  фикстуру обязателен).
- Один и тот же репо параллельно сам с собой не поддерживается: имя
  стабильно по пути, не по запуску.

## Запуск

Нужен Docker. `./.env.local` опционален: без него `up.sh` берёт дефолтные
креды seed-admin (шаблон — `.env.local.example`; формат — `.claude/rules/env-local.md`).

```
make e2e          # весь набор (поднимает стек, гоняет, сносит)
make e2e-smoke    # только -m smoke (быстрый gate)
```

Под капотом: `test/.venv` (из `test/requirements.txt`) + pytest. Конкретно —
`scripts/tools/e2e.sh` (сжатый вывод, каноничная строка `OK`/`FAIL`).

## Зависимости

Пины в `requirements.txt` (обновление — через ревью, как `go.mod`).
Tool-конфиг (pytest/ruff/mypy) — `pyproject.toml`.
