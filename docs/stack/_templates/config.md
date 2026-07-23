---
id: stack.config
kind: config
name: config
status: draft
---

# Config — конфигурация и секреты

Этот артефакт — материализованная таблица всех env (тип, default, sensitive) и
release-checklist. Конфиг = app-сторона (`internal/platform/config` через `golibinfra/config`) +
golibinfra-сторона (envconfig-теги kit-структур). Новый `envconfig:"..."` дописывается сюда тем же PR.

## Required env
<Без них приложение не стартует (`config.Load` → envconfig fail-fast). Таблица `| Env | Тип | Назначение | Где живёт |`. Эти переменные едут в INFRA-HANDOFF.>

## Optional env
<С code-default в kit-структурах; переопределяй только когда дефолт не подходит. Группируй по префиксу подсекциями `### <Группа>` с таблицей `| Env | Default | Назначение |` по подключённым kit'ам (напр. Postgres `PG_*`, HTTP `HTTP_*`, Auth `JWT_*`/`ARGON2_*`, Log `LOG_*`).>

## Secrets
<Sensitive env (JWT/OIDC-ключи, DB-пароль в `PG_DSN`, ...): таблица `| Env | Тип | Назначение |`. Факт «нужен» и тип — НЕ значение; кто выдаёт (проектная инфра). Прокатка значений — зона infra-репо (инжектятся в окружение контейнера), не здесь.>

## Validation
<Как валидируется конфиг на старте (fail-fast): обязательность, формат, диапазоны. Добавь подсекцию `### Release-checklist` — перед релизом сверять: новые `envconfig` дописаны в таблицы выше, Sensitive-ключи проброшены проектной инфрой в окружение контейнера.>

## Generates
<Что de-template родит:
- `internal/platform/config` (структуры + загрузка)
- `.env.example` (required+optional с дефолтами)
- seed для INFRA-HANDOFF (required env + секреты)>
