# Stack artifacts

Дизайн платформы/стека сервиса — **первичный source of truth** и **вход для skill
`de-template`**. Зеркалит модель доменов (она живёт в соседнем strategy-репо): 8 singleton-
артефактов (по одному на kind), заполняются через `/stack-deep-dive`, валидируются
`make validate-stack-docs`.

Где домены (соседний strategy-репо) отвечают на «какая бизнес-логика», `docs/stack/` отвечает на «на
какой платформе она крутится»: какой runtime, какие env, какие kit'ы golibinfra, какие
subjects, как версионируется и деплоится образ.

## Kinds

| kind | файл | что описывает |
|---|---|---|
| `service` | `service.md` | идентичность: имя, module path, версионирование, бинари |
| `runtime` | `runtime.md` | что крутится в `cmd/api`: HTTP / outbox-drain / NATS-subs / jobs |
| `config` | `config.md` | required/optional env, секреты (env-класс), валидация |
| `persistence` | `persistence.md` | Postgres: driver, миграции, kit-таблицы, пул |
| `messaging` | `messaging.md` | NATS/outbox subjects (published / consumed), delivery |
| `auth` | `auth.md` | authn/authz, authgate, роли, seed-admin, brute-force |
| `observability` | `observability.md` | структурные логи в stdout (logkit), уровни, что логируется |
| `dependencies` | `dependencies.md` | golibinfra kits, внешние runtime-deps, ёмкость |

Каждый артефакт обязан иметь секцию **Generates** — что из него родится в коде. Это
контракт для `de-template`: маппинг живёт рядом с источником, не отдельным файлом.

## Frontmatter

```yaml
id: stack.service      # stack.<name>
kind: service          # один из 8 выше
name: service          # == basename файла без .md
status: draft          # draft → stable (через валидатор)
```

## Workflow

1. Скопировать `_templates/<kind>.md` → `docs/stack/<kind>.md` (8 штук).
2. `/stack-deep-dive` — заполнить волнами вопросов inside-out.
3. `make validate-stack-docs` зелёный → промоушн `status: draft → stable`.
4. `/de-template` читает stable-артефакты и разворачивает фундамент проекта по секциям `Generates`.

## Gate

`/de-template` не запускается, пока не все артефакты `stable` и валидатор не зелёный.
Зеркалит дисциплину доменов (она в соседнем strategy-репо): код разворачивается только из
stable-артефактов, не из draft.
