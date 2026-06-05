---
name: stack-deep-dive
description: Углублённая интерактивная проработка 8 платформенных артефактов в `docs/stack/` (service · runtime · config · persistence · messaging · auth · observability · dependencies) — единственная точка входа для **проектирования стека**. Ведёт сессию через серию структурированных раундов `AskUserQuestion` (каждый шаг — раунд вопросов; без монологов и догадок), фиксирует рабочее состояние в собственной памяти `state/`, не выдумывает платформенные решения. Активируй когда архитектор проектирует/добивает стек, говорит "/stack-deep-dive", "спроектируем стек", "добьём stack-артефакты", приносит сырьё (compose.yaml, k8s manifest, текущий main.go) под платформенный разбор, или когда `/init-service` дошёл до шага стека. Точечные правки **уже стабильного** артефакта (поменялся env / добавился subject) идут через прямой Edit, не сюда.
---

# Skill: stack-deep-dive

Проектирование платформы сервиса. Превращает минимальные драфты 8
артефактов `docs/stack/` (frontmatter + placeholder-H2) в полные
`status: stable` артефакты с непустой секцией **Generates** в каждом.

Где домены (соседний docs-репо, скилл `/domain-deep-dive` там) отвечают
«какая бизнес-логика», этот скилл — «на какой платформе она крутится»:
runtime, env, kit'ы `golibinfra`, subjects, как версионируется и деплоится образ.

Это **source of truth и вход для `de-template`**. Секция `Generates`
каждого артефакта описывает, что из него родится в коде (module path,
`internal/platform/*`, `cmd/api`-композиция, `.env.example`, нужные
kit'ы). Канон артефактов и обязательных H2 — `docs/stack/README.md`;
маппинг `Generates → код` — `.claude/skills/de-template/references/wiring.md`.
Скилл их не дублирует, а исполняет.

В онбординг-флоу `init-service` чейнит сюда после проектирования доменов
(в соседнем docs-репо), затем — `de-template`.

## Главный ритм: AskUserQuestion

Скилл — **интервьюер архитектора**, не редактор. Партнёр по диалогу —
архитектор сервиса (тот, кто принимает платформенные решения: runtime,
kit'ы, env, subjects, capacity); его решения дословно ложатся в
артефакт, скилл их **не сочиняет**. Каждый шаг сеанса — раунд
`AskUserQuestion` (до 4 вопросов за раунд, иначе следующий раунд) →
получаем ответ → пишем в артефакт через `Edit` → следующий раунд.

**Никаких альтернатив этому ритму нет.** В частности:

- ❌ Длинный prose-ответ «вот как я бы спроектировал X» вместо вопросов.
- ❌ «По умолчанию» применить решение, потому что «в других сервисах
  так».
- ❌ Молча выставить «обычный» env (`HTTP_PORT=8080`,
  `OTEL_STDOUT_ONLY=false`) — это решение архитектора, спрашиваем.
- ❌ Один большой `AskUserQuestion` с попыткой влезть >4 вопросов —
  лимит инструмента, инструмент режет молча.
- ❌ Скопить ответы «запишу в конце волны» — пиши после **каждого**
  раунда, иначе теряется при `/clear`.
- ✅ Сомневаешься, что архитектор хочет именно `JWT RS256`, а не `EdDSA` →
  лишний раунд вопросов, а не догадка.

Платформенные решения скилл **не выдумывает**. Он формулирует
falsifiable-вопрос и пишет ответ архитектора дословно. Anti-pattern'ы и
enum'ы из канона — даёт как опции; имена env, subject'ы, module path,
числа capacity — только через свободную форму (Other).

## Точки входа

| вызов | поведение |
|---|---|
| `/stack-deep-dive` | сводка состояния 8 артефактов + AskUserQuestion: что углубить / что заводить (если каких-то нет) |
| `/stack-deep-dive <kind>` | завести/углубить артефакт указанного kind: режим A (если файла нет) или режим C (если есть) |
| `/stack-deep-dive <свободный текст / приложение>` | сырьё → разбор (режим **B**) |

`kind` ∈ {`service`, `runtime`, `config`, `persistence`, `messaging`,
`auth`, `observability`, `dependencies`} — фиксированные 8 singleton'ов.

## Что делать сразу при запуске

1. Прочитать `docs/stack/README.md` — освежить канон (8 kinds,
   обязательные H2, контракт `Generates`).
2. Если args пустые — сделать сводку и через `AskUserQuestion`
   предложить выбор.
3. Если args указывают на kind — определить режим (A — нет файла, C —
   есть), применить маршрут из `references/workflow-patterns.md`.
4. Если args = сырьё (свободный текст / приложение) — режим B.

## Сводка состояния

1. `ls docs/stack/` — какие из 8 файлов уже есть.
2. Для каждого присутствующего файла: прочитать frontmatter (`status`,
   `kind`) и H2-заголовки; считать заполненность обязательных секций
   (см. `references/field-checklists.md`).
3. Прогнать `make validate-stack-docs`. Если
   зелёный + все 8 stable — выйти: «стек спроектирован, делать нечего».

   ⚠️ Перед валидатором проверить, что 8 kind-файлов **существуют**:
   на пустом `docs/stack/` (только `_templates/` + README) валидатор
   даёт ложный OK, ему нечего проверять. Если файлов меньше 8 —
   зафиксировать «нужно завести K из 8» как первый шаг.
4. Прочитать `state/` (если есть) — `## Открытые вопросы`; пометки
   `**blocker**` приоритетны.
5. Сжатая карта в чат:

   ```
   ## Состояние docs/stack

   | kind | файл | status | H2 заполнено | дыры |
   |---|---|---|---|---|
   | service | ✅ | draft | 2/4 | Versioning, Generates |
   | config | ✅ | draft | 1/5 | Optional, Secrets, Validation, Generates |
   | runtime | ❌ нет | — | — | — |
   …
   ```

6. Через `AskUserQuestion` — что копать дальше (опции = существующие
   draft-артефакты + «завести недостающие»).

## Три режима

Детали — `references/workflow-patterns.md`. Краткая семантика:

- **A. Новая (заведение).** Args = `<kind>` для которого файла ещё нет.
  Скилл копирует `docs/stack/_templates/<kind>.md` → `docs/stack/<kind>.md`,
  заполняет frontmatter, заводит минимум (одна фраза идентичности /
  назначения), переходит в C.
- **B. Сырьё.** Args = свободный текст / приложение (compose.yaml, k8s
  manifest, текущий main.go, заметка про env'ы, описание интеграций).
  Извлекает платформенные решения, привязывает к 8 kind'ам, показывает
  план, по подтверждению — заводит/правит артефакты и переходит в C
  по каждому затронутому.
- **C. Углубление.** Args = `<kind>` существующего артефакта либо
  выбран из сводки. Точечные вопросы по дырам и подозрениям; волны
  по фундамент→роллап; чек-лист `draft → stable` после каждой правки.

## Inside-out волны (порядок копания)

Запоминается в скилле, у архитектора не спрашивается. Фундамент → производное;
два роллап-артефакта (`config`, `runtime`) идут последними, т.к. собирают
решения предыдущих:

1. **service** — идентичность: имя, module path, версионирование, бинари.
2. **dependencies** — какие kit'ы golibinfra, внешние runtime-deps,
   ёмкость.
3. **persistence** — Postgres: driver, миграции, kit-таблицы, пул.
4. **messaging** — published/consumed subjects, delivery semantics.
5. **auth** — authn/authz, authgate, роли, seed-admin, brute-force.
6. **observability** — telemetry-режим, resource attrs, biz-метрики,
   trace-points.
7. **config** (роллап) — required/optional env, секреты, валидация —
   собирает все envconfig-теги из подключённых kit'ов и решений 1–6.
8. **runtime** (роллап) — entrypoints в `cmd/api` (HTTP / outbox-drain /
   NATS-subs / jobs), lifecycle, shutdown — собирает реально
   поднимаемые подсистемы из 1–6.

Юзеру в начале волн — короткое: «иду фундамент → роллап, начну с
service; скажи если хочешь прыгнуть в конкретный kind».

## Стиль вопросов

См. `references/workflow-patterns.md § Стиль вопросов`. Дисциплина:

- **Всегда через `AskUserQuestion`**, до 4 вопросов в одном раунде.
- **Опции** только для:
  - anti-patterns (см. шаблоны + `references/field-checklists.md`);
  - enum-поля (`Telemetry mode`: `local stdout` / `OTLP prod`;
    `Delivery`: `at-least-once` / `at-most-once`;
    `Authn type`: `JWT` / `session` / `none`; и т.п.);
  - тест-критерий kind'а (✅/❌-примеры через AskUserQuestion);
  - выбор из известного множества (какой kit golibinfra, какой subject
    из уже-объявленных).
- **Свободная форма** (через Other) — только для **значения**: имя
  сервиса, module path, имена env, имена subjects, человекочитаемые
  описания, конкретные числа (timeout'ы, размер пула).
- **Никогда** не предлагать конкретные env-имена / module path / subject'ы
  как «правильный вариант» от себя. Anti-patterns как опции — да; имя
  как значение — нет.

## Память `state/<kind>.md`

См. `references/state-format.md`. Кратко:

- Один файл на kind (стек — singleton); `state/<kind>.md` без вложений.
- Файл существует, пока есть хоть одна непустая секция.
- Три возможные секции (любая может отсутствовать):
  - `## Открытые вопросы` (опц. `**blocker**` — блокирует stable);
  - `## Платформенные решения` (неочевидные решения, не лезущие в
    артефакт по форме);
  - `## Что копать дальше`.
- Если все три пусты — заметку удалить.
- **Forward-only**: не аккумулировать «было/стало», даты сессий.

State лежит рядом со скиллом
(`.claude/skills/stack-deep-dive/state/`) и **коммитится** — это
общая проектная память между сессиями и архитекторами. Forward-only
держит `git log` чистым. Не часть канона `docs/stack/` (туда уходят
зафиксированные платформенные решения), но часть проектного знания
о стеке.

## Контракт `draft → stable`

См. `references/draft-stable.md`. Чек-лист после каждой правки (все 5
✅ → предложить промоушн):

1. **Обязательные H2 заполнены** (см. `references/field-checklists.md`
   по kind) непустым осмысленным содержимым, без placeholder'ов из
   шаблона.
2. **Секция `Generates` непуста и конкретна.** Это контракт для
   `de-template`: пустая / `TBD`-`Generates` = артефакт бесполезен
   движку. Generates перечисляет конкретные пути в коде и/или env-имена.
3. **Cross-kind sync согласован.** Решения, объявленные в kit-артефактах
   (`auth`, `persistence`, `messaging`, `observability`, `dependencies`),
   роллапятся в `config` (env + секреты) и `runtime` (subscribers, jobs,
   shutdown-хуки). Если auth добавил `JWT_PUBLIC_KEY` — он есть в
   `config.md § Required env`. См. `references/draft-stable.md § Cross-kind
   sync`.
4. **Валидатор зелёный** — `make validate-stack-docs
   docs/stack` без ошибок.
5. **Тест-критерий kind'а явно подтверждён** в этой сессии. См.
   `references/draft-stable.md § Тест-критерий по kind`.
6. **State чист по `**blocker**`-вопросам.**

(Шесть пунктов: kind-специфичный `Generates` оформлен отдельным пунктом,
т.к. это контракт de-template; cross-kind sync — отдельным, т.к. без него
config/runtime роллапаются с пробелами.)

Все ✅ → AskUserQuestion «Готов к stable, повышаем?» с опциями
«Да» / «Нет, ещё не уверен» (Other). По «Да» — `Edit` `status: draft →
stable`. **Скилл не повышает сам.**

## Cross-kind sync

В отличие от доменов (где есть stub'ы и каскад между
артефактами), стек — 8 фиксированных kind'ов, и связи между ними
выражены через **`Generates`-секции** и через **роллапы в `config`/`runtime`**.

Правило: после правки kit-артефакта (`auth`, `persistence`, `messaging`,
`observability`, `dependencies`) — скилл предлагает через
`AskUserQuestion` пройтись по `config` и `runtime`, чтобы обновить
роллап (новый env / новый subscriber / новый job).

- `auth` → `config.md § Required/Secrets` (JWT keys, seed-admin),
  `runtime.md § Lifecycle` (если auth-cleanup-job).
- `persistence` → `config.md § Required` (`DATABASE_URL`, pool tuning),
  `runtime.md § Lifecycle` (миграция как init-контейнер? отдельный
  бинарь?), `dependencies.md § External` (Postgres как зависимость).
- `messaging` → `config.md § Required/Optional` (NATS URL, subjects),
  `runtime.md § Entrypoints` (поднимаем ли NATS-subs, поднимаем ли
  outbox-drain), `dependencies.md § External` (NATS).
- `observability` → `config.md § Optional` (`OTEL_*`), `dependencies.md
  § kit'ы` (telemetry/bizmetric).
- `dependencies` → `config.md § Optional` (по подключённым kit'ам — их
  envconfig-теги), `runtime.md § Entrypoints` (что из объявленного
  действительно поднимается).

Скилл держит sync **автоматически после каждой правки** kit-артефакта —
без подтверждения добавляет роллап в `config` / `runtime` черновиком, и
через `AskUserQuestion` показывает дифф «вот что добавилось — окей?».

## Завершение сессии

1. Перечислить затронутые артефакты (заведены / углублены / промоутнуты).
2. Перечислить нерешённые `**blocker**`-вопросы (читая `state/`).
3. Прогнать `make validate-stack-docs` —
   подтвердить зелёный.
4. Предложить следующий шаг:
   - **Онбординг** (пришёл из `/init-service`): 8/8 stable → дальше
     `/de-template`.
   - **Отдельно** (стек уже жил, поменялся): артефакты теперь актуальный
     source of truth; если правка задевает инфра-поверхность (env /
     секрет / NATS subject / runtime-dep) — оформи через `/infra-handoff`
     для админа.

## Запреты

- Не редактировать артефакты вслепую — всегда сначала `Read`.
- Не выдумывать платформенные решения. Anti-patterns как опции — да;
  имя env, subject, путь, конкретное число — нет.
- Не оставлять `Generates` пустым / `TBD` — это вход `de-template`.
  Артефакт без конкретной Generates `stable` не становится.
- Не повышать `draft → stable` без явного подтверждения архитектора, даже
  при всех ✅.
- Не понижать `stable → draft` сам.
- Не игнорировать cross-kind sync: правка kit-артефакта без обновления
  `config`/`runtime` оставляет роллапы с пробелами → de-template
  разворачивает неполный скелет.
- Не аккумулировать историю в `state/` — forward-only.
- Не задавать >4 вопросов одним AskUserQuestion.
- Не копить ответы «запишу в конце волны» — пиши сразу через `Edit`,
  иначе теряется при `/clear`.
- Не дублировать здесь правила `docs/stack/README.md` и шаблонов — они
  source of truth канона, скилл их **исполняет**.
