---
name: dev-tokens
description: Формат и правила использования ./credentials.md — локальный файл с токенами для dev-tooling. Gitignored, не читается Claude'ом напрямую (deny в settings + pre-write-protect hook). Грузится при работе с credentials.md или agent-tools/logs.sh.
globs: "credentials.md,scripts/agent-tools/logs.sh"
---

# Dev tokens — credentials.md

Локальный файл `./credentials.md` хранит токены для **dev-tooling**: bearer'ы для внешних API (observability, registry, и т.п.), которые нужны Claude'у/агент-тулам при работе с конкретным проектом. Это **не** runtime-секреты приложения — те живут в `.env` (локально) или Vault + vault-agent (на VPS, см. infra-repo `docs/project-secrets.md`).

## Где живёт

- Корень репо: `./credentials.md` (и алиас `credential.md`).
- В `.gitignore`.
- Не уезжает в автоматические backup'ы. Юзер сам отвечает за копию (Vaultwarden / локальная заметка).

## Формат

Markdown с произвольной структурой. **Обязательное правило**: для каждого секрета — отдельная строка `<KEY>=<value>`. Можно в bullet-листе или в fenced code-блоке — парсер делает `grep -oE 'KEY=...'`. Пример:

```markdown
# Credentials

## Observability (loki.stit.tech / tempo.stit.tech)

- `STIT_OBS_TOKEN=eyJhbGciOi...` — bearer для read API.
  Tenant зашит в токене (`tenant-auth` ForwardAuth). Источник:
  `kv/projects/<p>/<env>/otel` в корп-Vault.

## Что-то ещё

FOO_TOKEN=bar
```

## Кто читает

- **Claude НЕ может читать `credentials.md` напрямую.** `Read(**/credentials*)` в `.claude/settings.json` → deny. Страховка от утечки в чат/память.
- **Agent-tools читают через shell.** Пример: `scripts/agent-tools/logs.sh` делает `grep -m1 -oE 'STIT_OBS_TOKEN=...'`, передаёт `logcli` как `LOKI_BEARER_TOKEN`. Токен остаётся внутри bash-процесса, в stdout не попадает.
- **Pre-write hook** (`scripts/claude-hooks/pre-write-protect.sh`) блокирует `Edit`/`Write` на пути `*credentials*` — обновление файла идёт **юзером руками**, не Claude'ом.

## Правила

### Делать

- Одна строка `KEY=value` на секрет. Без многострочных значений.
- Рядом — комментарий: откуда токен, для какого env, как ротировать.
- При ротации — обновить файл локально + в источнике (корп-Vault / Vaultwarden).

### Не делать

- ❌ Цитировать токен в чат — он попадёт в transcript/память. Если Claude'у нужен токен — клади в `credentials.md` и зови agent-tool.
- ❌ Коммитить файл. Gitignore страхует, но `git add -A` всё равно может зацепить.
- ❌ Писать значения в `.env`/`.env.example`. `.env` — runtime приложения, `credentials.md` — dev-tooling.
- ❌ Хранить здесь долгоживущие прод-секреты, которые должны быть в Vault.

## Известные ключи

| Ключ | Используется | Документация |
|---|---|---|
| `STIT_OBS_TOKEN` | `scripts/agent-tools/logs.sh` | `.claude/skills/query-logs/SKILL.md` + infra-repo `docs/project-secrets.md` |
