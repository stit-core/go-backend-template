# Дневной флоу разработки с Claude Code

Этот документ — для человека. Описывает как работать с этим репо через Claude Code: что грузится, что когда срабатывает, как действовать в типовых сценариях.

## 1. Что лежит где

| Слой | Где | Когда грузится | Сила |
|---|---|---|---|
| Корневые правила | `AGENTS.md` (CLAUDE.md → AGENTS.md) | Всегда, на каждый запуск | Advisory |
| Path-scoped правила | `.claude/rules/*.md` с `paths:` frontmatter | Когда matching-файлы попадают в контекст | Advisory |
| Skills (процедуры) | `.claude/skills/<name>/SKILL.md` | Когда description совпадает с задачей или ты вызываешь `/<name>` | Procedural |
| Subagent'ы | `.claude/agents/*.md` | Когда основной агент делегирует | Role-specific |
| Hooks | `.claude/settings.json` + `scripts/claude-hooks/*.sh` | На события (PreToolUse, PostToolUse, Stop, и т.д.) | **Детерминированно** — обойти нельзя |
| Memory | `~/.claude/projects/<proj>/memory/` | Auto-managed, грузится первые 200 строк | Persistent |

Принцип progressive disclosure: на холодном старте грузятся имена и описания (~3-5k токенов суммарно), тела skills/rules — только когда нужны.

## 2. Цикл одной задачи

Anthropic Agent SDK называет это явно: **gather → act → verify → repeat**.

```
[твой prompt]
     ↓
GATHER  ─── Claude читает файлы (path-scoped правила автогрузятся)
            может вызвать subagent `explorer` для fan-out
            может активировать skill если description match
     ↓
PLAN    ─── Plan Mode (Shift+Tab) для неоднозначных задач
            пропускается если "diff в одно предложение"
     ↓
ACT     ─── Edit/Write tool calls
            PostToolUse хук: gofumpt + go vet (детерминированно)
     ↓
VERIFY  ─── make test / make lint / make build
            или agent-tools wrappers для сжатого вывода
     ↓
REVIEW  ─── (опционально) subagent `reviewer` со свежим контекстом
     ↓
COMMIT  ─── conventional prefix per AGENTS.md
            Stop-хук: финальная проверка `go build ./...`
```

## 3. Сценарии

### А. Новый проект из шаблона

```
git clone <шаблон> my-service
cd my-service
make tools          # установить pinned dev-tools
make hooks-install  # поставить git-хуки
claude              # запустить Claude Code
```

В сессии:
```
> Удали референс-домены auth/user/verification и сделай новый домен `order`
  с CRUD и outbox-эвентами
```

Что произойдёт:
1. Claude читает структуру `internal/domain/`, видит auth/user/verification как референсы
2. Description match → активируется skill `add-domain`
3. Skill инструктирует: подтвердить удаление, скаффолдить новый домен через `make new-domain NAME=order`, добавить миграцию, queries, sqlc-генерацию, mocks
4. Каждый Edit запускает PostToolUse-хук → gofumpt + go vet
5. По завершении — Stop-хук гоняет `go build ./...`
6. Коммит: `feat(order): scaffold order domain with CRUD and outbox events`

### Б. Существующий сервис, новая фича

```
> Добавь 2FA в auth-домен
```

1. Claude читает `internal/domain/auth/`
2. Path-scoped `domain-architecture.md` автогрузится (paths: `internal/domain/**`)
3. Возможно вызовет `explorer` subagent: "найди все места где валидируется auth-токен" → возвращает 1-2k summary
4. План: использовать существующий `verification`-домен для 2FA-кодов
5. Реализация: правки в `auth/usecase.go`, `auth/dto.go`, новые тесты в `auth/usecase_test.go`
6. PostToolUse-хук на каждый `.go` edit → gofumpt + vet
7. Реген моков: `make mocks` (если поменялись интерфейсы)
8. `make test` → green
9. `reviewer` subagent — свежий контекст, читает diff, возвращает severity-graded findings
10. Фикс находок → коммит

### В. Багфикс

```
claude --permission-mode plan
> В логах вижу что refresh-токены принимаются после ротации.
  jti-проверка не работает.
```

Plan mode запрещает edits до approve.

1. Claude читает `internal/domain/auth/`, миграции `0005_refresh_sessions.*.sql`, sqlc-сгенерированный код
2. Path-scoped `sqlc-and-migrations.md` грузится
3. Может вызвать `explorer`: "все места валидации refresh-токена"
4. Выдаёт план в Plan Mode UI: какой файл, какая строка, какой тест писать первым
5. Approve → переход в ACT
6. **TDD**: сначала падающий тест воспроизводящий баг, потом фикс, тест green
7. Stop-хук → `go build`
8. Коммит: `fix(auth): block reused refresh tokens by validating jti in session lookup`

### Г. Миграция

```
> Добавь FK от sessions к users c cascade delete
```

1. Description "migration" + paths `migrations/` → main agent делегирует `migration-author` subagent
2. Subagent в **отдельном context window**: читает `migrations/`, `sqlc.yaml`, текущие queries
3. Создаёт `migrations/00NN_sessions_users_fk.{up,down}.sql`
4. Регенерит sqlc, проверяет компиляцию
5. Возвращает в main agent ~1k summary: "added migration NN, regenerated sqlc, all queries compile, no test changes"
6. Main agent коммитит

Зачем subagent: его контекст не загрязняется migration-specific деталями, основной агент остаётся чистым.

### Д. Ревью своего коммита

```
> Сделай ревью последнего коммита
```

1. Claude вызывает `reviewer` subagent (фрэш-контекст критично — иначе bias toward own code)
2. Subagent читает `git diff HEAD~1`, применяет path-scoped правила
3. Возвращает severity-graded findings: critical / important / nit
4. Main agent показывает тебе, ты выбираешь что фиксить

`reviewer` **не имеет write-инструментов** — только observation.

## 4. Кто что enforces

| Механизм | Где | Сила | Когда применяется |
|---|---|---|---|
| Hooks в settings.json | `.claude/settings.json` | **Детерминированно** | gofumpt после Edit; `--no-verify` блокируется; `go build` на Stop |
| Skills | `.claude/skills/<name>/SKILL.md` | Procedure | Когда description совпадает |
| Path-scoped rules | `.claude/rules/*.md` с `paths:` | Контекстная подсказка | Когда matching-файлы читаются |
| AGENTS.md | repo root | Always-on advisory | Каждый запуск |
| Subagent system prompts | `.claude/agents/*.md` | Role-specific | Когда subagent вызван |

Цитата Anthropic: *"CLAUDE.md instructions are advisory; hooks are deterministic and guarantee the action happens."* Поэтому критичные инварианты (gofumpt, no `--no-verify`, no commits with `.env`) — в хуках. Soft-guidance (стиль, conventions) — в AGENTS.md и rules.

## 5. Course correction — как вмешаться

| Что хочется | Как делать |
|---|---|
| Прервать текущую работу | `Esc` |
| Переключить в Plan Mode на одно сообщение | `Shift+Tab` или начать с `/plan` |
| Заблокировать конкретный путь | `.claude/settings.local.json` → `permissions.deny` |
| После 2 неудачных попыток | `/clear` и переписать запрос (Anthropic best practice) |
| Добавить разовый контекст | drag-and-drop файла или `@path/to/file` |
| Откатить изменения | `git restore` (snapshots не нужны — у нас git) |

## 6. Что осознанно не делаем

- ❌ Multi-agent swarm. Cognition + Anthropic Multi-Agent Research caveat про код.
- ❌ Hierarchy team-lead → task-lead → workers (предыдущая попытка). Cognition Principle 1 ("share full traces") сложнее в иерархии чем в едином агенте с subagent-изоляцией.
- ❌ 22 специализированных агента. GitHub 2,500-repo study: успех = чёткие границы в *правилах*, не множественность *агентов*.
- ❌ Vector DB для retrieval. Sourcegraph дропнул, Aider использует AST. ripgrep + `gopls` достаточно для Go-репо.
- ❌ Bloated CLAUDE.md. Anthropic: bloat снижает adherence. Лимит 200 строк.
- ❌ JSON-wrapped edit-формат. Aider измерил ~3× harm. Нативный string-replace через Edit-инструмент.

## 7. Куда смотреть когда что-то не так

| Симптом | Куда смотреть |
|---|---|
| Хук блокирует то что не должен | `scripts/claude-hooks/*.sh` + `.claude/settings.json` `hooks` секция |
| Skill не активируется на нужных задачах | `description` в frontmatter — переписать конкретнее |
| Path-scoped rule не подгружается | Frontmatter `paths:` glob — проверить что matching работает |
| Subagent ушёл слишком надолго | `tools` в frontmatter — урезать до read-only набора |
| AGENTS.md игнорируется | Длина файла > 200 строк, либо instructions конфликтуют между секциями |
| Коммит-сообщение reject'нуто | `scripts/git-hooks/commit-msg` — проверь что type ∈ {feat,fix,refactor,docs,test,chore,meta} |
| `make sqlc` падает | Запросы в `internal/domain/*/infra/postgres/queries/` ссылаются на колонки которых нет в `migrations/` |
| `make mocks-check` падает | Изменили интерфейс из `.mockery.yml` без `make mocks` |
