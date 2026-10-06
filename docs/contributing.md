# Contributing — работа с шаблоном через Claude Code

**[META]** Для контрибьютора *самого шаблона*: как устроена `.claude`-система, что
enforce'ится детерминированно, как вмешаться. (Потребителю шаблона — `README.md` § Как начать и `/init-service`.)

## Что где лежит и когда грузится

| Слой | Где | Когда грузится | Сила |
|---|---|---|---|
| Ядро + роутер | `CLAUDE.md` (реальный файл) | всегда, каждый запуск | advisory |
| Path-scoped правила | `.claude/rules/*.md` с `paths:` | когда matching-файл прочитан инструментом Read (не Bash, не Write) | advisory |
| Skills (процедуры) | `.claude/skills/<name>/SKILL.md` | по `description` или `/<name>` | procedural |
| Settings / permissions | `.claude/settings.json` | старт сессии | детерминированно (deny) |

Progressive disclosure: на холодном старте — только имена+описания; тела skills/rules и
`references/` грузятся по необходимости.

**Важно про `paths:`** — нативный ключ path-scoping в Claude Code именно `paths:` (не
`globs:`). Правило без `paths:` грузится всегда. Проверь, что новое правило path-scoped.

## Цикл одной задачи

```
prompt → /smartdev (cockpit: make status, читает активные планы, чейнит режим)
       → маршрут (CLAUDE.md § Маршрут задачи: triage-failure / plan / scaffold / напрямую)
       → act (меняется поведение — тест первым; инфра-поверхность задета → infra-handoff)
       → готово (CLAUDE.md § Готово)
       → commit (Conventional Commits)
```

Планы — файлы `.claude/plans/<slug>.md`, бэклог — `.claude/plans/backlog.md`; закрытый план
удаляется (история в git).

## Кто что enforce'ит

| Механизм | Где | Сила |
|---|---|---|
| git-хуки | `scripts/git-hooks/` (подключает `make setup` через `core.hooksPath`): commit-msg (Conventional Commits), pre-commit (fmt-check+vet+lint+validate-stack/plan), pre-push (`make check` каждого пушимого ref'а — его верхнего коммита — в свежем worktree: незакоммиченное и игнорируемое на вердикт не влияет) | **детерминированно** |
| permissions.deny | `.claude/settings.json` | **детерминированно** (блок `--no-veri…` где угодно в команде — и сокращение, и в кавычках, force-push, reset --hard, правка `*.pem`/`*.key`/generated); чтение `.env`/`.env.local` — только для инструмента Read: `cat` и другие команды Bash deny не видит, их держит правило (CLAUDE.md, rule `env-local`) |
| Хук Claude Code | `.claude/hooks/guard-bash.sh` (PreToolUse, Bash) | барьер от случайного обхода, не песочница (блок `git commit -n`, `--no-veri…`, `send-pack` — и со словом в кавычках или через `bash -c`, `eval`, `\| bash`, heredoc интерпретатору; `SKIP_PREPUSH` и `hooksPath` в любой команде; алиасы и include конфига git; запись по `scripts/git-hooks` и `.git/config`; `TestGuardBash`). Не видит: подстановки оболочки (`$G`, `$(…)`), glob, `chmod -R` каталога выше, правку инструментами Edit и Write — такая правка хуков — правка гейта (CLAUDE.md § Сначала спросить) |
| Самопроверка обвязки | `internal/archtest/harness_test.go`, `internal/archtest/template_test.go`, `internal/archtest/onboarding_test.go`, `cmd/api/main_test.go`, `cmd/api/boot_test.go` (шаг тестов) | **детерминированно** (make-цели, пути — от корня репо или каталога документа — и файлы, пути API, скиллы и правила (каждое имя перечня) в прозе существуют — кроме опечатки в первом сегменте пути и ссылки простым текстом; модуль, HTTP- и NATS-входы каждого домена подключены в `cmd/api`; kit-таблицы графа, в том числе outbox доменов, — источники `cmd/migrate`; required env конфигов — в `.env.example`; пути `.template/paths` есть, тег обвязки на HEAD = `.template/version`, `make template-update` переносит обвязку в форк, маркеров конфликта в репо нет; пути `.template/identity` есть, prune называет каждый файл, который знает их пакеты; канонический репо шаблона (origin) — с module path шаблона; валидатор stack отвергает шаблоны как есть) |
| Самозащита гейта | `Makefile` (`make check`), pre-push | **детерминированно** (`make check` не запускается с `-i`/`-k`/`-n`/`-p`/`-q`/`-t`/`-j`, `--eval`, подменой своих переменных (и `override`), вторым makefile (`-f`, `MAKEFILES`: рецепт `check::` первой строкой сверяет состав makefile'ов при любых флагах), `GOFLAGS` и `API_TEST_MAIN_ARGS` — `TestMakeCheckCannotBeForged`; pre-push не передаёт гейту флаги make) |
| Path-scoped rules | `.claude/rules/*.md` | контекстная подсказка |
| CLAUDE.md | repo root | always-on advisory |

Хук Claude Code в шаблоне один — `guard-bash` против обходов git-хуков и гейта; остальные
критичные инварианты держат git-хуки + `permissions.deny`; soft-guidance — CLAUDE.md и rules.
Push в `main` разрешён (прямой или через PR); gate — `pre-push → make check`.

## Версия обвязки

Форк получает обвязку шаблона git-способом: `make template-update`, процедура — skill `/upgrade`.

- `.template/paths` — пути шаблона, строка — путь или `:(exclude)<путь>`: обвязка агента,
  инструменты и гейт, самопроверки, доки шаблона. Остальное — код форка: правки шаблона там тег
  называет в разделе `Платформа`.
- `.template/version` — версия обвязки, имя тега `template/vX.Y.Z`: коммит выпуска несёт этот тег,
  следующие коммиты — ту же версию до нового выпуска. Форк получает её при создании,
  `make template-update` сдвигает её вместе с обвязкой. Первый перенос форка идёт от коммита
  шаблона, из которого форк создан (по дереву его первого коммита): кончик `main` обычно новее тега.
- Тег обвязки — аннотированный `template/vX.Y.Z` на коммите `main`, где `.template/version` — имя
  этого тега (иначе archtest `TestTemplatePaths` не пустит push тега). Релизы образа — теги
  `vX.Y.Z`; теги обвязки `make release` не видит.
- Версия: minor — новое и breaking для форка, каждое breaking — со шагом в аннотации; patch —
  исправления без новых требований к форку.
- Выпуск — с OK владельца (тег и push — CLAUDE.md § Границы): новая `.template/version` коммитом
  `meta(template): template/vX.Y.Z` → `git tag -a template/vX.Y.Z -F <аннотация>` →
  `git push origin main template/vX.Y.Z` (pre-push гонит `make check` на пушимом коммите с тегом). `main`,
  где `.template/version` называет ещё не выпущенный тег, уходит в origin только с этим тегом на
  HEAD: иначе форк с такого `main` создан раньше тега своей версии, и правки до тега придут к нему
  только переносом до следующего тега.

Аннотация — как у golibinfra, по разделам; пустой раздел не пишется:

```
template/vX.Y.Z

Breaking:
- <что изменилось> → <что форк делает руками>

Гейт:
- <новая или более строгая проверка> → <как привести к ней код форка>

Обвязка:
- <правила, скиллы, доки, скрипты — приходят переносом>

Платформа:
- <правка вне путей шаблона: internal/platform, cmd, pkg, конфиги> → <перенести руками, если нужна>
```

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
| Path-scoped rule не грузится | файл читали через Bash или создали через Write — открой его через Read; frontmatter `paths:` (не `globs:`!) — проверить glob |
| CLAUDE.md игнорируется | длина >200 строк, либо конфликт инструкций между секциями |
| Коммит reject'нут | `scripts/git-hooks/commit-msg` — type ∈ {feat,fix,refactor,docs,test,chore,meta} |
| `make gen` / `gen-check` падает | queries ссылаются на несуществующие колонки / забыл `make gen` после правки интерфейса или HTTP-операции (`openapi.json`) / `untracked mocks` — новый мок не в индексе: `git add` |
| archtest: `golangci-lint linters --json` не запустился | golangci-lint нет в PATH — `make setup` (pinned-версия) |
| `make lint` → `FAIL: golangci-lint exit N` | линтер не отработал, причина — в хвосте вывода: 3 — конфиг или неизвестный линтер, 4 — таймаут, 5 — нет Go-файлов, 7 — ошибка в логе (например, пакет не найден) |
| Замечания lint по файлам, которых нет | кеш анализа своего дерева — `.cache/golangci-lint`; удалить каталог |
| `validate-plan` / `validate-stack` падает | структура артефакта — см. соответствующий `README`/skill |
