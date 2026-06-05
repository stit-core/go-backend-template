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

`./.env.local` хранит токены для **dev-tooling** (bearer'ы внешних API — observability и т.п.),
нужные агент-тулам при работе с проектом. Это **не** runtime-секреты приложения — те в `.env`
(локально) или Vault + vault-agent (VPS, см. infra-repo `docs/project-secrets.md`).

- Живёт в корне, в `.gitignore`. Committed только `./.env.local.example` (плейсхолдеры +
  откуда токен/как ротировать). В авто-backup'ы не уезжает — копия на юзере (Vaultwarden/Vault).
- **Формат:** dotenv, `KEY=value` на строку. Парсится `grep`'ом, **не** `source`'ом — без
  многострочных значений, `$`-подстановок, inline-команд. `export `-префикс и кавычки допустимы.

## Security-инвариант

- **Доступ к значению токена — только через tools.** `scripts/tools/logs.sh` и
  `test/stack/up.sh` сами `grep`-ают `KEY=` и передают значение внутри bash-процесса — в
  stdout/transcript оно не попадает.
- ❌ **`Read`/`cat` ради токена — необратимая утечка** в transcript (и память, если включена):
  придётся ротировать. Нужен токен — зови agent-tool, не вытаскивай значение в чат.
- ❌ Коммитить `.env.local` (gitignore страхует, но `git add -A` цепляет — проверяй `git status`).
- ❌ Класть dev-tooling токены в `.env`/`.env.example` (там runtime приложения) или хранить здесь
  долгоживущие прод-секреты (их место — Vault).
- Правит файл предпочтительно юзер руками; если Claude — не эхо-ить значения. Ротация — обновить
  `.env.local` + источник (Vault/Vaultwarden).
- **Граница одноимённого правила (не путать при копипасте между форками):** здесь `.env.local` —
  dev-tooling токены, которые ставит юзер, агент в файл не пишет. В `project-infra-template-compose`
  одноимённое `env-local` диктует обратное (workstation оператора: оператор кидает кред в чат → агент
  Write'ит его в `.env.local`). Это разные инварианты — не переносить поведение записи между репо.

## Известные ключи

| Ключ | Используется | Документация |
|---|---|---|
| `SFTBMB_OBS_TOKEN` | `scripts/tools/logs.sh`, `test/stack/up.sh` | `.claude/skills/query-logs/SKILL.md` + infra-repo `docs/project-secrets.md` |
| `OTEL_TENANT` | `test/stack/up.sh` (e2e otel-agent) | `test/stack/compose.yml`, sftbmb-infra/infra ADR-0004 §2 |
| `E2E_ADMIN_EMAIL` / `E2E_ADMIN_PASSWORD` | `test/stack/up.sh` → `cmd/seed-admin` | `test/conftest.py` (`admin`-фикстура). Не прод-секрет — admin эфемерного e2e-стека; дефолты в up.sh |
