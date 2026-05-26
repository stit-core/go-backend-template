---
name: env-local
description: Формат и правила ./.env.local — локальный gitignored файл с токенами для dev-tooling (e2e-стек, чтение прод-логов). Доступ к значению токена — через tools, не цитированием в чат. Грузится при работе с .env.local или tools/logs.sh / test/stack/up.sh.
paths:
  - ".env.local"
  - ".env.local.example"
  - "scripts/tools/logs.sh"
  - "test/stack/up.sh"
---

# Dev tokens — .env.local

Локальный файл `./.env.local` хранит токены для **dev-tooling**: bearer'ы для внешних API (observability и т.п.), которые нужны Claude'у/агент-тулам при работе с конкретным проектом. Это **не** runtime-секреты приложения — те живут в `.env` (локально, для `go run ./cmd/api`) или Vault + vault-agent (на VPS, см. infra-repo `docs/project-secrets.md`).

## Где живёт

- Корень репо: `./.env.local`. В `.gitignore`.
- Шаблон `./.env.local.example` — committed, с плейсхолдерами и комментариями (откуда токен, как ротировать). Реальные значения только в `.env.local`.
- Не уезжает в автоматические backup'ы. Юзер сам отвечает за копию (Vaultwarden / корп-Vault).

## Формат

Dotenv: одна строка `KEY=value` на секрет. Парсится `grep`'ом, **не** `source`'ом — без многострочных значений, без `$`-подстановок, без inline-команд. Опциональный `export `-префикс и окружающие кавычки допускаются. Пример — `.env.local.example`.

## Кто читает

- **Доступ к значению токена — только через tools.** `scripts/tools/logs.sh` и `test/stack/up.sh` сами `grep`-ают `KEY=` из `.env.local` и передают значение дальше внутри bash-процесса (`LOKI_BEARER_TOKEN` / env для compose). В stdout/transcript значение не попадает.
- **`Read`/`cat` ради токена — необратимая утечка** в transcript и `MEMORY.md` (придётся ротировать). Значение токена в чат не вытаскивай — зови agent-tool.
- **Обновление файла — предпочтительно юзером руками.** Если правит Claude — не эхо-ить значения в вывод.

## Правила

### Делать

- Одна строка `KEY=value` на секрет. Без многострочных значений.
- Комментарий рядом: откуда токен, для какого env, как ротировать (живёт в `.env.local.example`).
- При ротации — обновить `.env.local` локально + источник (корп-Vault / Vaultwarden).

### Не делать

- ❌ Цитировать токен в чат — попадёт в transcript/память. Нужен токен — клади в `.env.local` и зови agent-tool.
- ❌ Коммитить `.env.local`. Gitignore страхует, но `git add -A` может зацепить — проверяй `git status`.
- ❌ Писать dev-tooling токены в `.env`/`.env.example`. `.env` — runtime приложения, `.env.local` — dev-tooling.
- ❌ Хранить здесь долгоживущие прод-секреты, которые должны быть в Vault.

## Известные ключи

| Ключ | Используется | Документация |
|---|---|---|
| `STIT_OBS_TOKEN` | `scripts/tools/logs.sh`, `test/stack/up.sh` | `.claude/skills/query-logs/SKILL.md` + infra-repo `docs/project-secrets.md` |
| `OTEL_TENANT` | `test/stack/up.sh` (e2e otel-agent) | `test/stack/compose.yml`, stit-infra/infra ADR-0004 §2 |
| `E2E_ADMIN_EMAIL` / `E2E_ADMIN_PASSWORD` | `test/stack/up.sh` → `cmd/seed-admin` | `test/conftest.py` (`admin`-фикстура). Не прод-секрет — admin эфемерного e2e-стека; дефолты в up.sh |
