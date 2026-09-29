# Contributing — работа с шаблоном через Claude Code

**[META]** Для контрибьютора *самого шаблона*: как устроена `.claude`-система, что
enforce'ится детерминированно, как вмешаться. (Потребителю шаблона — `getting-started.md`.)

## Что где лежит и когда грузится

| Слой | Где | Когда грузится | Сила |
|---|---|---|---|
| Ядро + роутер | `CLAUDE.md` (реальный файл) | всегда, каждый запуск | advisory |
| Path-scoped правила | `.claude/rules/*.md` с `paths:` | когда matching-файл в контексте | advisory |
| Skills (процедуры) | `.claude/skills/<name>/SKILL.md` | по `description` или `/<name>` | procedural |
| Settings / permissions | `.claude/settings.json` | старт сессии | детерминированно (deny) |

Progressive disclosure: на холодном старте — только имена+описания; тела skills/rules и
`references/` грузятся по необходимости.

**Важно про `paths:`** — нативный ключ path-scoping в Claude Code именно `paths:` (не
`globs:`). Правило без `paths:` грузится всегда. Проверь, что новое правило path-scoped.

## Цикл одной задачи

```
prompt → /smartdev (cockpit: make status, читает активные планы, чейнит режим)
       → plan (planning-first: изменение идёт через план — файл в .claude/plans/)
       → act (RED→GREEN→REFACTOR; инфра-поверхность задета → infra-handoff)
       → verify (make test, make lint, make build; claim «готово» → make check)
       → review (опц. /code-review)
       → commit (Conventional Commits)
```

Планирование — **дефолт** (см. skill `plan`). Прямая правка — только truly-atomic и
багфиксы (→ `triage-failure`). Планы — файлы `.claude/plans/<slug>.md`, бэклог —
`.claude/plans/backlog.md`; закрытый план удаляется (история в git).

## Кто что enforce'ит

| Механизм | Где | Сила |
|---|---|---|
| git-хуки | `scripts/git-hooks/` (подключает `make setup` через `core.hooksPath`): commit-msg (Conventional Commits), pre-commit (fmt-check+vet+lint+validate-stack/plan), pre-push (`make check` на чистом дереве, только HEAD) | **детерминированно** |
| permissions.deny | `.claude/settings.json` | **детерминированно** (блок `--no-verify`, force-push, reset --hard, правка `*.pem`/`*.key`/generated) |
| Хук Claude Code | `.claude/hooks/guard-bash.sh` (PreToolUse, Bash) | **детерминированно** (блок `git commit -n`, `--no-veri…`, `SKIP_PREPUSH`, `core.hooksPath`; команда проверяется построчно) |
| Самопроверка обвязки | `internal/archtest/harness_test.go`, `cmd/api/main_test.go` (шаг тестов) | **детерминированно** (make-цели и пути репо в прозе существуют; модуль, HTTP- и NATS-входы и outbox-таблица каждого домена подключены в `cmd/api` и `cmd/migrate`; required env конфигов — в `.env.example`) |
| Самозащита гейта | `Makefile` (`make check`), pre-push | **детерминированно** (`make check` не запускается с `-i`/`-n`, подменой своих переменных и `GOFLAGS`; pre-push не передаёт гейту флаги make) |
| Path-scoped rules | `.claude/rules/*.md` | контекстная подсказка |
| CLAUDE.md | repo root | always-on advisory |

Хук Claude Code в шаблоне один — `guard-bash` против обходов git-хуков и гейта; остальные
критичные инварианты держат git-хуки + `permissions.deny`; soft-guidance — CLAUDE.md и rules.
Push в `main` разрешён (прямой или через PR); gate — `pre-push → make check`.

## Course correction

| Хочется | Как |
|---|---|
| Прервать работу | `Esc` |
| Спланировать до правок | `/plan` (план в `.claude/plans/`) |
| Заблокировать путь | `.claude/settings.local.json` → `permissions.deny` |
| После 2 неудачных попыток | `/clear` + переписать запрос |
| Разовый контекст | `@path/to/file` |
| Откатить | `git restore` (snapshots не нужны — git) |

## Что осознанно не делаем

Обоснования (trade-offs) — в `docs/reference/architecture.md § Non-goals`: без multi-agent
swarm, без иерархии агентов, без vector-DB retrieval, CLAUDE.md ≤200 строк, нативный
string-replace edit.

## Troubleshooting

| Симптом | Куда смотреть |
|---|---|
| Skill не активируется | `description` в frontmatter: ключевой сценарий первым, категории намерений вместо списка фраз; `description` + `when_to_use` ≤ 1536 символов (дальше листинг обрезает) |
| Path-scoped rule не грузится | frontmatter `paths:` (не `globs:`!) — проверить glob |
| CLAUDE.md игнорируется | длина >200 строк, либо конфликт инструкций между секциями |
| Коммит reject'нут | `scripts/git-hooks/commit-msg` — type ∈ {feat,fix,refactor,docs,test,chore,meta} |
| `make gen` / `gen-check` падает | queries ссылаются на несуществующие колонки / забыл `make gen` после правки интерфейса |
| `make lint` → `FAIL: golangci-lint exit N` | линтер не отработал, причина — в хвосте вывода: 3 — конфиг или неизвестный линтер, 4 — таймаут, 5 — нет Go-файлов, 7 — ошибка в логе (например, пакет не найден) |
| Замечания lint по файлам, которых нет | кеш анализа своего дерева — `.cache/golangci-lint`; удалить каталог |
| `validate-plan` / `validate-stack` падает | структура артефакта — см. соответствующий `README`/skill |
