# Гайд для агентов

Go HTTP-бэкенд stit на шаблоне `go-backend-template`: huma/v2 + chi + pgx/v5 + sqlc + uber/fx + киты
`golibinfra`.

Этот файл — ядро (always-on) + роутер. Детали — в `docs/`, контекстные правила — в
`.claude/rules/`, процедуры — в `.claude/skills/`. Карта документации — `docs/README.md`.
Правило грузится, когда файл под его `paths:` прочитан инструментом Read; чтение через Bash и
запись через Write его не грузят — перед правкой открой файл или само правило через Read.

## Точка входа

- **Как сюда попадают.** Обычно — агентом из хаба `<org>/infra`: задача и общий план — оттуда,
  обратно — образ в registry (`/release`) и INFRA-HANDOFF-блок. Прямой заход разработчика — с той
  же обвязкой. Карта — `docs/reference/ecosystem.md`.
- **`/smartdev`** — cockpit сессии: сводка состояния (ветка, drift, активный план, TODO) +
  переключение в нужный режим. Запускай в начале / после `/clear`.
- **Онбординг форка** — `/init-service`, планом: стек → `/de-template` → свои домены → домены
  идентичности (остаются или prune — по `docs/stack/auth.md`).

## Маршрут задачи

Маршрут — первая подходящая строка сверху; `/smartdev` выбирает по этой же таблице.

| Задача | Маршрут | Пример |
|---|---|---|
| Упало или баг | `/triage-failure`: воспроизводящий тест → фикс, одним коммитом | красный `make check`, 500 вместо 409 |
| Больше одного коммита; breaking API или события; миграция с эффектом (§ Границы); вход, сессии, токены, права (`pkg/policy`, `authgate`); фаза из хаба | `/plan` — план в `.claude/plans/`, его шаги идут по этой же таблице | домен по артефакту strategy со своим API и событиями |
| Новый блок в один коммит: домен, ручка, событие, миграция, поле, env, порт | `/scaffold`, вариант по блоку | «добавь колонку `phone` в users и поле в ответ» — вариант field |
| Правка существующего в один коммит | напрямую; меняется поведение — сначала тест | переименование, правка запроса, текст доки |

## Готово

Claim «готово» и коммит — только когда:
- `make check` → `✓ check clean`: CI нет, этот гейт (его же гоняет pre-push) — единственная
  проверка изменения;
- код прошёл `/code-review` по диффу, а вход, сессии, токены, секреты и миграции — ещё
  `/security-review`; находки исправлены или отклонены с причиной;
- сменилась конвенция — правило, скилл, reference и доки о ней обновлены тем же коммитом;
- задета инфра-поверхность (§ Границы) — INFRA-HANDOFF оформлен.

## Проектирование первично, код вторичен

DDD-артефакты доменов — в соседнем strategy-репо (`../strategy`, форк
`business-strategy-template`): `docs/projects/<slug>/services/<svc>/domains/<ctx>/`; артефакты
стека — `docs/stack/`. Это source of truth: код пишется по ним, не наоборот.
`auth`/`user`/`verification` — домены идентичности (вход, сессии, учётные записи; механика — в китах):
у проекта со своей идентичностью остаются и растут по его артефактам, с внешней (`identity: external`
в `docs/stack/auth.md`) их удаляет prune. Проза ссылается на них как на образцы; у проекта без них —
`git show <версия>:<путь>`, версия — `.template/version`.

## Команды

Единый интерфейс `make <verb>` (`make help`; реализация — `scripts/tools/*`, напрямую не зови).
Конциз-by-default: `FORMAT=detailed`/`VERBOSE=1` — полно, `PKG=./x/...` — подмножество; `make check`
их не читает. `make check` = fmt-check + vet + lint + migrations-check + test-int (юнит +
integration, нужен Docker) + build + gen-check + cover-check (покрытие профиля test-int ≥95%, функций
без тестов нет). `make gen` — sqlc, mockery и `openapi.json` (контракт API из регистраций huma, без
запуска сервиса).

## Карта: где что

| Тема | Где |
|---|---|
| Архитектура, слои, ports/adapters, события, non-goals | `docs/reference/architecture.md` + rule `domain-architecture` |
| Логирование (`logkit.Logger`, без токенов и PII) | CLAUDE.md § Стиль кода + rule `domain-architecture`; артефакт стека — `docs/stack/_templates/observability.md` |
| Release (образы, handoff, откат) | `docs/reference/release.md` + skill `/release` |
| Обновления: golibinfra, обвязка шаблона (`.template/`) | skill `/upgrade` + `docs/contributing.md § Версия обвязки` |
| Экосистема, контракт с `<org>/infra` | `docs/reference/ecosystem.md` |
| Тесты, моки, TDD, e2e | rule `tests-pipeline` + `test/README.md` |
| Миграции, sqlc | rule `sqlc-and-migrations` + skill `/scaffold` (вариант migration) |
| Outbox / NATS / jobs | rule `events-and-messaging` |
| DDD-домены (проектирование) | соседний strategy-репо `../strategy` (skill `/domain-deep-dive` там); маппинг DDD→Go-код — `docs/reference/domain-mapping.md` |
| Stack-артефакты (8 kinds, вход de-template) | `docs/stack/README.md` + skill `/stack-deep-dive` |
| Устройство `.claude`-системы, troubleshooting | `docs/contributing.md` |

## Стиль кода (инварианты)

- **gofumpt** обязателен (`make fmt`).
- **golangci-lint** (`.golangci.yml`): логи — только `logkit.Logger`, не `fmt.Print*`/`log.*`/
  `print(ln)`/`slog.*`/`os.Stdout`/`os.Stderr` (forbidigo); errcheck (и `_ = f()`), gosec и
  остальные — по конфигу. `//nolint` — ослабление гейта (§ Границы): с именем включённого линтера
  и объяснением (nolintlint и archtest).
- **Раскладка.** Бизнес-код — только `internal/domain/<d>/` (слои — rule `domain-architecture`);
  клей платформы — `internal/platform/`; `pkg/` — правила и типы этого сервиса (`policy`, `paging`,
  `outcome`) и сгенерированные клиенты (gRPC, OpenAPI: зовёт `infra/` домена, не корень); входы —
  `cmd/`; тестовая обвязка — `internal/testkit/` и `internal/archtest/`. Других каталогов в
  `internal/` нет; generic-примитив — golibinfra (ниже). Хартия — `docs/reference/architecture.md`
  § Каталоги.
- **archtest** (`internal/archtest`, шаг тестов; `//nolint` на него не действует) — раскладка и
  границы любого домена: чужой домен — только через порт и адаптер в `<domain>/module/adapters.go`,
  моки — только из тестов, `platform` и `pkg` доменов не знают. Перечень — комментарий пакета в
  `internal/archtest/archtest_test.go`, в правилах — пометка archtest у инварианта.
- Ошибки — `%w` + `errors.Is/As`; sentinel в `errors.go` корня домена.
- Комментарии — только WHY (rule `comments`).
- **Не городить велосипеды**: нужен примитив — сначала `README.md` golibinfra § Пакеты версии из
  `go.mod` (каталог — `go list -m -f '{{.Dir}}' git.stit.tech/stit-core/golibinfra`), затем README
  кита. Generic util, HTTP-клиент, middleware, любой «kit» — в `golibinfra`; нет нужного — план в
  golibinfra → тег → bump (`docs/reference/architecture.md § Граница с golibinfra`).

## Git workflow

- **Conventional Commits** (`scripts/git-hooks/commit-msg`): типы `feat|fix|refactor|docs|test|chore|meta`,
  формат `<type>(<scope>)?: <subject>`, subject ≤72; сообщения самого git (`Revert "…"`,
  `fixup!`/`squash!`) тоже проходят.
- **Push в `main` разрешён** (прямой или через PR); линейная история; pre-push пускает только
  чистый HEAD, обходы хуков из сессии блокирует `.claude/hooks/guard-bash.sh`.
- git-хуки подключает `make setup` (`core.hooksPath=scripts/git-hooks`). Релиз — `/release`.

## Границы

### Делать всегда
- **Тронул инфра-поверхность** (required env или секрет; миграция с эффектом — destructive, lock,
  backfill, extension; cross-service subject; breaking API; новый сетевой порт; runtime-dep в
  `go.mod`) → до коммита INFRA-HANDOFF-блок: в отчёт хабу (прямой заход — юзеру, чтобы
  передал) и тем же коммитом в накопитель `.claude/plans/handoff.md` (rule `infra-impact`, skill
  `/infra-handoff`).
- `make gen` после правки `queries/`, `migrations/`, интерфейсов из `.mockery.yml` или HTTP-операций.
- Conventional Commits. Дёргай `make <verb>`, не скрипты напрямую.

### Сначала спросить
- `migrations/` (skill `/scaffold`, вариант migration), `pkg/policy`, `internal/platform/authgate/`,
  `go.mod`/`go.sum`, `git tag -a` / `make release` (артефакт вне репо).
- Ослабить гейт (`//nolint`; правка `Makefile`, `scripts/`, `.golangci.yml`, `internal/archtest`,
  `.claude/settings.json`, `.claude/hooks/`, после которой он проверяет меньше) — только по явной
  просьбе юзера.
- Спросить некого (агент из хаба без собеседника): пункт этого списка, которого нет в задаче или
  плане хаба, не делай — вопрос в отчёт хабу и в «Открытые вопросы» плана, остальное продолжай.

### Не делать никогда
- Обходить хуки и гейт: `--no-verify`, `git commit -n`, `SKIP_PREPUSH`, `core.hooksPath` (блокируют
  `permissions.deny` и хук `guard-bash`; `make check` не запускается с `-i` и `GOFLAGS`).
  Коммитить/цитировать `.env`, `.env.local`, `*.pem`, `*.key` (в репо живут только `*.example`-шаблоны).
- Править сгенерированное руками (`internal/domain/*/infra/postgres/db/*.go`, `**/mocks/*.go`) — только `make gen`.
- Image-теги `:latest`/`:edge`/lightweight. Deploy-команды (`make bump`, `make ship`, `ssh`,
  env-прокатка) — выкатка идёт только из `<org>/infra`; после `make release` STOP.
- Молча отступать от маршрута, правил, скиллов или плана: отступление — с OK юзера (спросить
  некого — как в «Сначала спросить») и строкой в «Решениях» плана или в теле коммита.

## Forward-only

Любой текст в репо описывает только текущее состояние — не «было/стало». История — в
`git log` + Conventional Commits. Полное правило — `.claude/rules/forward-only.md`.
