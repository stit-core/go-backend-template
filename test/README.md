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
| Smoke-тесты | `smoke/` | boot + критический happy-path, `@pytest.mark.smoke`. |
| E2E-тесты | `e2e/` | сценарии по доменам, `@pytest.mark.e2e`. |
| Фикстуры | `conftest.py` | session-стек (shell-out в `stack/up.sh`), `api`, `user`. |

Папки `smoke/`/`e2e/` организуют, маркеры селектят: `pytest -m smoke` / `pytest -m e2e`.

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
