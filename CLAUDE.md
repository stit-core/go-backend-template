# go-backend-template — гайд для агентов

Шаблон Go HTTP-бэкенда stit: huma/v2 + chi + pgx/v5 + sqlc + uber/fx + `golibinfra`
(logging, authn, outbox, brute-force, NATS, audit, clock).

Этот файл — **ядро (always-on) + роутер**. Детали — в `docs/`, контекстные правила — в
`.claude/rules/`, процедуры — в `.claude/skills/`. Карта документации — `docs/README.md`.
Правило грузится, когда файл под его `paths:` прочитан инструментом Read; чтение через Bash и
запись через Write его не грузят — перед правкой открой файл или само правило через Read.

## Точка входа

- **Как сюда попадают.** Обычно — агентом из операционного хаба `<org>/infra`: общий план
  проекта ведёт хаб, задача приходит оттуда, а работаешь ты обвязкой этого репо (этот файл,
  скиллы, гейт `make check`). Результат уходит обратно в хаб: образ в registry (`/release`) и
  INFRA-HANDOFF-блок. Прямой заход разработчика — тоже нормальный режим с той же обвязкой;
  последствия для соседей оформляются INFRA-HANDOFF-блоком для хаба (`/infra-handoff`). Карта —
  `docs/reference/ecosystem.md`.
- **`/smartdev`** — cockpit сессии: сводка состояния (ветка, drift, активный план, TODO) +
  переключение в нужный режим. Запускай в начале / после `/clear`.
- **Потребителю шаблона** — `docs/getting-started.md`: форк → `/init-service` → проектирование
  стека (домены уже готовы в соседнем strategy-репо) → `/de-template` → свои домены → свой проект.

## Маршрут задачи

Маршрут — первая подходящая строка сверху; `/smartdev` выбирает по этой же таблице. Отступил от
неё — скажи почему: юзеру, а пришёл из хаба — в отчёте.

| Задача | Маршрут | Пример |
|---|---|---|
| Упало или баг | `/triage-failure`: воспроизводящий тест → фикс, одним коммитом | красный `make check`, 500 вместо 409 |
| Больше одного коммита; breaking API или события; миграция с эффектом (§ Границы); вход, сессии, токены, права (`pkg/policy`, `authgate`); фаза из хаба | `/plan` — план в `.claude/plans/`, его шаги идут по этой же таблице | домен по артефакту strategy со своим API и событиями |
| Новый блок в один коммит: домен, ручка, событие, миграция, порт | `/scaffold`, вариант по блоку | «добавь колонку `phone` в users и поле в ответ» — вариант migration и правка слоёв |
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

DDD-артефакты доменов живут в **соседнем strategy-репо** орга (`../strategy`, форк
`business-strategy-template`): `docs/projects/<slug>/services/<svc>/domains/<ctx>/` — домены
сервиса `<svc>`, который реализует этот репо; артефакты платформы/стека — локально `docs/stack/`.
Это **source of truth**; код пишется/генерируется по ним, не наоборот. `auth`/`user`/`verification`
— code-only референс-примеры (изучить, потом удалить под свой проект). Роль репо в экосистеме
(образ в registry; выкатка — `make bump` в `<org>/infra`) — `docs/reference/ecosystem.md`.

## Команды

Единый интерфейс `make <verb>`; `make help` — полный список. Конциз-by-default
(`FORMAT=detailed`/`VERBOSE=1` — полно; `PKG=./x/...` — подмножество; `make check` их не читает).
Канонический gate перед claim «готово» / push — **`make check`** (fmt-check + vet + lint +
migrations-check + test-int (юнит + integration, нужен Docker) + build + gen-check + cover-check
≥75%). `make gen` — регенерация sqlc, mockery и `openapi.json` (контракт API из регистраций huma, без
запуска сервиса). Реализация за таргетами — `scripts/tools/*`
(напрямую не зови).

## Карта: где что

| Тема | Где |
|---|---|
| Архитектура, слои, ports/adapters, события, non-goals | `docs/reference/architecture.md` + rule `domain-architecture` |
| Логирование (структурные логи в stdout, `logkit.Logger`) | CLAUDE.md § Стиль кода + артефакт `docs/stack/observability.md` (форма — `docs/stack/_templates/observability.md`) |
| Release (образы, handoff, откат) | `docs/reference/release.md` + skill `/release` |
| Экосистема, контракт с `<org>/infra` | `docs/reference/ecosystem.md` |
| Тесты, моки, TDD, e2e | rule `tests-pipeline` + `test/README.md` |
| Миграции, sqlc | rule `sqlc-and-migrations` + skill `/scaffold` (вариант migration) |
| Outbox / NATS / jobs | rule `events-and-messaging` |
| DDD-домены (проектирование) | соседний strategy-репо `../strategy` (skill `/domain-deep-dive` там); маппинг DDD→Go-код — `docs/reference/domain-mapping.md` |
| Stack-артефакты (8 kinds, вход de-template) | `docs/stack/README.md` + skill `/stack-deep-dive` |
| Устройство `.claude`-системы, troubleshooting | `docs/contributing.md` |

## Стиль кода (инварианты)

- **gofumpt** обязателен (`make fmt`).
- **golangci-lint** (`.golangci.yml`): **forbidigo** — нельзя `fmt.Print*`/`log.*`/`print(ln)`/
  `slog.*` и запись в `os.Stdout`/`os.Stderr`, только `logkit.Logger`.
  errcheck (и `_ = f()`)/errorlint/gosec/govet/contextcheck/noctx/rowserrcheck/bodyclose
  /sqlclosecheck/testifylint — на. `//nolint` — ослабление гейта (§ Границы): с именем включённого
  линтера и объяснением (nolintlint, `make lint`).
- **Раскладка.** Бизнес-код — только `internal/domain/<d>/` (слои — rule `domain-architecture`);
  клей платформы — `internal/platform/`; общее для доменов — `pkg/`: чистые примитивы и
  сгенерированный код (gRPC-, OpenAPI-клиент; зовёт его `infra/` домена); входы — `cmd/`; тестовая
  обвязка — `internal/testkit/` и `internal/archtest/`. Других каталогов в `internal/` нет.
- **archtest** (`internal/archtest`, шаг тестов; `//nolint` на него не действует) — раскладка и
  границы любого домена: чужой домен — только через порт и адаптер в `<domain>/module/adapters.go`,
  моки — только из тестов, `platform` и `pkg` доменов не знают. Перечень — комментарий пакета в
  `internal/archtest/archtest_test.go`, в правилах — пометка archtest у инварианта.
- Ошибки — `%w` + `errors.Is/As`; sentinel в `errors.go` корня домена.
- Комментарии — только WHY (rule `comments`).
- **Не городить велосипеды**: нужен примитив — сначала `README.md` golibinfra § Пакеты той версии,
  что в `go.mod` (каталог — `go list -m -f '{{.Dir}}' git.stit.tech/stit-core/golibinfra`), затем
  README кита. Переиспользуемый/инфра-примитив (generic util, HTTP-client,
  middleware, любой «kit») → `golibinfra`, не локальная полукалека. Нет нужного — PR в
  `golibinfra` + строка в `.claude/plans/backlog.md`; временный shim только с явным OK юзера. Граница и
  обоснование — `docs/reference/architecture.md § Граница с golibinfra`.

## Git workflow

- **Conventional Commits** (`scripts/git-hooks/commit-msg`): типы `feat|fix|refactor|docs|test|chore|meta`,
  формат `<type>(<scope>)?: <subject>`, subject ≤72; сообщения самого git (`Revert "…"`,
  `fixup!`/`squash!`) тоже проходят.
- **Push в `main` разрешён** (прямой или через PR); линейная история. Gate — git
  `pre-push → make check` на чистом дереве, пушится только HEAD; обходы хуков из сессии блокирует
  хук `.claude/hooks/guard-bash.sh`.
- git-хуки подключает `make setup` (`core.hooksPath=scripts/git-hooks`). Релиз — `/release`.

## Границы

### Делать всегда
- **Тронул инфра-поверхность** (required env / sensitive-env-секрет; миграция destructive/lock/backfill/extension;
  cross-service NATS/outbox-subject; breaking API; новый сетевой порт; новая runtime-dep в `go.mod`) →
  до claim/коммита оформи **INFRA-HANDOFF**-блок для хаба `<org>/infra`: пришёл из хаба — блок
  входит в твой отчёт хабу, прямой заход — попроси юзера передать его туда; тем же коммитом — в
  накопитель `.claude/plans/handoff.md`, его печатает `make release` (rule `infra-impact` + skill
  `/infra-handoff`).
- Cross-domain — только порты + адаптеры. Логи — `logkit.Logger`. Generic-примитив → `golibinfra`.
- `make gen` после правки `queries/`, `migrations/`, интерфейсов из `.mockery.yml` или HTTP-операций.
- Conventional Commits. Дёргай `make <verb>`, не скрипты напрямую.

### Сначала спросить
- `migrations/` (skill `/scaffold`, вариант migration), `pkg/policy` + `pkg/tokens`, `internal/platform/authgate/`,
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
- `fmt.Print*`/`log.*`/`print(ln)`/`slog.*`/`os.Stdout`/`os.Stderr` напрямую. Image-теги `:latest`/`:edge`/lightweight.
- Deploy-команды (`make bump`, `make ship`, `ssh`, env-прокатка) — выкатка идёт только из
  `<org>/infra`; после `make release` STOP.
- Молча отступать от канона (правило, скилл, план): отступление — с OK юзера (спросить некого — как
  в «Сначала спросить») и строкой в «Решениях» плана или в теле коммита.

## Forward-only

Любой текст в репо описывает **только текущее состояние** — не «было/стало». История — в
`git log` + Conventional Commits. Полное правило — `.claude/rules/forward-only.md`.
