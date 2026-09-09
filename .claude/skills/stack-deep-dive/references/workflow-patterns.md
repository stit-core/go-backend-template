# Три режима ведения сеанса

`stack-deep-dive` поддерживает три явно различающихся сценария входа.
Скилл выбирает режим по виду args при запуске.

## A. Новая (заведение)

**Сигнал**: args = `<kind>` для которого `docs/stack/<kind>.md` ещё нет.

### Шаги

1. Прочитать `docs/stack/README.md` § Kinds — освежить контракт.
2. Прочитать `docs/stack/_templates/<kind>.md` — это входная форма.
3. Скопировать шаблон: `cp docs/stack/_templates/<kind>.md
   docs/stack/<kind>.md`.
4. Заполнить frontmatter (`id: stack.<kind>`, `kind`, `name: <kind>`,
   `status: draft`). Frontmatter тривиально из имени файла — без AskUserQuestion.
5. Раунд 1 `AskUserQuestion` — короткий фундамент:
   - для `service`: имя сервиса (Other), module path (Other), одно-фразовое
     назначение (Other);
   - для остальных kind'ов: одно-фразовое описание роли в стеке (Other).
6. Перейти в режим C на свежезаведённом артефакте.

### Когда заводить «недостающие» 8

Если в сводке видно `K из 8 файлов нет` — предложить через
`AskUserQuestion`: «завести все недостающие сразу (минимальный
frontmatter + draft) и потом копать по одному» / «завести только
выбранный kind». По «всё сразу» — последовательно копирует все
недостающие, потом C по первому из списка (фундамент → роллап,
см. SKILL.md § Inside-out волны).

## B. Сырьё

**Сигнал**: args = свободный текст / приложение (compose.yaml, k8s
manifest, заметки про env'ы, описание интеграций, текущий main.go).

### Шаги

1. Прочитать материал внимательно.
2. Извлечь платформенные решения и привязать каждое к одному из 8 kind'ов:
   - имя сервиса / module path → `service`;
   - что крутится в одном бинаре (HTTP / outbox / subs / jobs) → `runtime`;
   - env'ы и секреты → `config`;
   - БД, миграции, пул → `persistence`;
   - subjects published/consumed, delivery → `messaging`;
   - механизм authn, роли, seed-admin, bruteforce → `auth`;
   - логирование (формат, уровни, структурные поля) → `observability`;
   - kit'ы golibinfra, внешние deps, ёмкость → `dependencies`.
3. Показать план через чат:

   ```
   Из материала вижу:
   - service: имя `orders`, module `git.stit.tech/foo/orders`,
     версионирование semver через ldflags;
   - persistence: Postgres 16, миграции `migrate up`, пул 50;
   - messaging: пока без NATS, только outbox-drain;
   - auth: JWT (HS256), seed-admin через cmd/seed-admin;
   - observability: структурные логи в stdout (json);
   - dependencies: golibinfra v1.x, kit'ы logkit/pgkit/outboxkit/authnkit;
   - config: 4 required env, 2 секрета (env, значение не коммитим);
   - runtime: HTTP+outbox-drain в одном cmd/api, jobs нет.
   Так?
   ```
4. Раунд `AskUserQuestion` — подтвердить план / корректировка через
   Other по каждой группе.
5. По подтверждению — завести/обновить артефакты (минимум frontmatter
   + извлечённые ответы как драфты в нужных H2). Открытые места —
   state-заметки с `**blocker**`, где материал ничего не сказал по
   обязательной секции.
6. Через `AskUserQuestion` — выбрать, какой kind углубить первым (по
   умолчанию `service` → дальше волны inside-out).
7. Перейти в режим C.

### Что НЕ делать в режиме B

- Не объявлять извлечённое «зафиксированным». Это кандидаты до
  подтверждения.
- Не угадывать `Generates`-секции по сырью — это контракт de-template,
  его архитектор обдумывает явно в режиме C, не из compose.yaml.
- Не выдумывать env-имена, subject'ы, числа capacity — только то, что
  явно в материале.

## C. Углубление

**Сигнал**: args = `<kind>` существующего артефакта, либо выбран из
сводки.

### Шаги

1. Прочитать файл артефакта целиком.
2. Прочитать `state/<kind>.md`, если есть.
3. Прочитать соседей по cross-kind sync (см. SKILL.md):
   - правишь `auth` → читай `config` (роллап секретов/env), `runtime`
     (cleanup-job?), `dependencies` (authnkit/bruteforcekit);
   - правишь `persistence` → читай `config`, `runtime`, `dependencies`;
   - правишь `messaging` → читай `config`, `runtime`, `dependencies`;
   - правишь `observability` → читай `config`, `dependencies`;
   - правишь `dependencies` → читай `config` (все envconfig-теги),
     `runtime` (что из объявленного поднимается);
   - правишь `service` → читай `observability` (log fields);
   - правишь `config` / `runtime` — они роллапы, читай все 6 kit'ов.
4. Прогнать обязательные H2 по `references/field-checklists.md` для
   этого kind'а. Найти:
   - пустые / placeholder-only H2-секции;
   - подозрения по anti-patterns (`Engine = Postgres` без
     `DATABASE_URL` в `config § Required`; `Authn = JWT` без
     `JWT_PUBLIC_KEY` в `config`);
   - **пустую / TBD-`Generates`** (это **главная** дыра — без неё
     артефакт бесполезен de-template);
   - cross-kind drift (kit-артефакт объявил env, который не появился в
     `config`).
5. Раунд `AskUserQuestion` (до 4 вопросов) — точечно:
   - дыры → Other;
   - anti-pattern-подозрения → бинарные опции «согласен / нет» (Other);
   - cross-kind drift → опции «добавить недостающие env в config» /
     «убрать решение из этого kind'а» / «оставить и пометить blocker
     для ручного развода».
6. Применить ответы: правка через `Edit`. **Mirror-обновление
   `config`/`runtime` — автоматически черновиком**, потом
   `AskUserQuestion` «вот что добавил в config — окей?».
7. Прогнать чек-лист `references/draft-stable.md`. Все ✅ → промоушн.

### Когда переходить от C к завершению

- Все 6 пунктов чек-листа ✅ → AskUserQuestion «Готов к stable,
  повышаем?», по «Да» — `Edit` `status: draft → stable`.
- Остался `**blocker**` (например архитектор ещё не определился с провайдером
  внешней интеграции) → зафиксировать в state, оставить `draft`, выйти
  из углубления; предложить следующий kind из волн.

## Стиль вопросов: правила

### Когда давать опции

| ситуация | формат |
|---|---|
| anti-pattern (`config` объявил secret в plain-env, `messaging` оставил `Delivery: none` при at-least-once-требовании) | бинарные опции «соответствует anti-pattern» / «нет, вот почему» (Other) |
| enum-поле (`Log format`, `Delivery`, `Ordering`, `Authn type`, `Migration strategy`, `Source` для env) | по опции на каждое значение enum |
| тест-критерий из `references/draft-stable.md` | опции из ✅/❌-примеров, архитектор соотносит |
| выбор kit'а golibinfra (`auth` → какой authnkit-примитив) | по опции на kit + «не нужен» |
| выбор существующего subject'а (cross-kind) | по опции на каждый из объявленных в messaging |
| «promote draft → stable?» | две опции `Да` / `Нет, ещё не уверен` (Other) |

### Когда не давать опции, только Other

| поле | почему |
|---|---|
| имя сервиса, module path | архитектор называет, не скилл |
| имена env, subject'ы | архитектор именует под свой проект |
| `Connection pool` numbers, timeout'ы | конкретика — выбор архитектора |
| текст `Generates` — пути и имена в коде | архитектор пишет точный контракт для de-template |
| описание интеграции (`External runtime deps`) | бизнес-семантика провайдера |
| capacity (CPU/RAM/replicas) | оценка архитектора |
| `Architecture notes` стека | формулировка архитектора |

### Чего не делать

- Не предлагать «может быть, env называется `XXX_YYY`?» — это
  выдумывание стека.
- Не упоминать «правильные» env-имена из шаблонов как опции в виде
  готовых значений. Примеры в шаблонах — иллюстрация формы.
- Не задавать >4 вопросов в AskUserQuestion.
- Не закрывать сессию без чек-листа хотя бы по одному правленому артефакту.
- Не пропускать sync `config`/`runtime` при правке kit-артефакта — даже
  если архитектор сказал «потом», помечать blocker в `state/config.md` или
  `state/runtime.md`, чтобы не забылось.
