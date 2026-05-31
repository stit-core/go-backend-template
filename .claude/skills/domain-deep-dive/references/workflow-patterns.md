# Три режима ведения сеанса

`domain-deep-dive` поддерживает три явно различающихся сценария входа.
Скилл выбирает режим по виду args при запуске и далее ведёт сеанс,
комбинируя точечные шаги.

## A. Новая сущность

**Сигнал**: args = `<ctx>:<kind>` или `<ctx>:<kind>:<новый-Name>` (когда
артефакта с таким `Name` ещё нет на нужном пути).

### Шаги

1. Прочитать `docs/domains/README.md` § Kinds и обязательные H2 для
   указанного `kind`. Прочитать `docs/domains/_templates/<kind>.md`
   (шаблон) — это входная форма.
2. Раунд 1 `AskUserQuestion` — фундамент:
   - `Name` (через Other) — если ещё не дан;
   - `description` (Other) — одна-две фразы что это в бизнесе;
   - для `aggregate`/`entity`: `Identity` (ID type, mutable yes/no,
     visibility in-context|published — три опции в одном раунде если
     влезают, иначе двумя).
3. Скопировать шаблон по правильному пути:
   - `aggregate` → `docs/domains/<ctx>/<Name>.md`
   - `entity` → `docs/domains/<ctx>/<Name>.md`
   - `event` → `docs/domains/<ctx>/events/<Name>.md`
   - `use-case` → `docs/domains/<ctx>/use-cases/<Name>.md`
   - `policy` → `docs/domains/<ctx>/policies/<Name>.md`
   - `vo` (cross-context) → `docs/domains/_shared/<Name>.md`

   Заполнить frontmatter (`id: <ctx>.<Name>`, `kind`, `context`, `name`,
   `status: draft`, `relations: []`, `emits: []`, `consumes: []`) и
   `description`-параграф; остальные H2-секции оставить как в шаблоне
   (с placeholder'ами).
4. Если контекст новый (`docs/domains/<ctx>/` создан впервые) — скопировать
   также `docs/domains/_templates/context-readme.md` →
   `docs/domains/<ctx>/README.md`, занести в него ссылку на свежезаведённый
   артефакт.
5. Раунд 2+ `AskUserQuestion` — батч по доменным слотам kind в порядке
   `references/field-checklists.md`. До 4 вопросов в раунде; перемежать
   опции (anti-patterns, enum, выбор соседа) и Other (значения).
6. По мере заполнения — если в поле нужна ссылка на ещё не существующий
   `<ctx>.<Name>`, отработать **stub** (см. SKILL.md § Stub-сущности):
   создать stub-артефакт минимальной формы, завести state-заметку с
   `**blocker**`-вопросом, продолжить заполнение исходного.
7. После всех обязательных H2 — раунд `AskUserQuestion` по
   **тест-критерию** kind'а (см. `references/draft-stable.md § Тест-критерий
   по kind`): два-три ✅/❌-варианта как опции, архитектор соотносит свою запись.
8. Применить чек-лист `references/draft-stable.md`. Если все 5 ✅ —
   предложить через AskUserQuestion промоушн `draft → stable`.
9. Записать `state/<ctx>/<kind>/<Name>.md`, если есть что (открытые
   вопросы, неочевидные доменные решения, next-step'ы).

### Пример раунда (use-case, после фундамента)

Скилл предлагает четыре вопроса разом для `RegisterUser`:

- **Actor → Role**: опции `anonymous` / `admin` / `client` / `system`.
- **Actor → Channel**: опции `HTTP API` / `NATS subscriber` /
  `scheduled job` / `internal`.
- **Called aggregate methods**: множественный выбор из существующих
  методов `User.md`; «нужен новый» через Other (триггерит правку
  `User.md` методом + stub события).
- **Events emitted**: выбор из существующих `events/*.md` контекста +
  «нужно новое событие» через Other (stub event).

`Inputs`, `Outputs`, `Steps`, `Errors` — следующие раунды; их значения
**только** через Other, не предлагать тексты как опции.

## B. Сырьё

**Сигнал**: args = свободный текст, либо архитектор приложил транскрипт
интервью / заметки / набросок кода / ER-диаграмму.

### Шаги

1. Прочитать материал внимательно.
2. Извлечь кандидатов на DDD-сущности — что похоже на агрегат, методы,
   события, use-case'ы, инварианты, value objects. Фильтр —
   `docs/domains/README.md § Anti-patterns` (отбросить заведомо
   неподходящее: I/O-операция как метод агрегата → use-case;
   внутри-контекстный VO как отдельный файл → inline; spanning invariant
   между двумя aggregate'ами → переразбить).
3. Через чат показать план:

   ```
   Из материала вижу:
   - 2 aggregate-кандидата: `Order`, `LineItem` (child entity Order);
   - 3 use-case: `PlaceOrder`, `CancelOrder`, `ListOrders`;
   - 2 события: `OrderPlaced`, `OrderCancelled`;
   - 1 policy: `CancelWindow` (3 дня на отмену);
   - 4 inline-VO в Order: `OrderId`, `OrderStatus`, `LineItemSlice`, `Money`
     (последний — кандидат в `_shared/`).
   Так?
   ```
4. Раунд `AskUserQuestion` — подтвердить план / попросить корректировку
   через Other по каждой группе.
5. По подтверждению — завести **stub'ы** в соответствующих файлах
   (минимум `frontmatter + description + status: draft`); создать
   state-заметки с открытыми вопросами «извлечено из сырья —
   уточнить <что>», помечая `**blocker**` там, где материал прямо
   противоречил `docs/domains/README.md § Anti-patterns`.
6. Через `AskUserQuestion` — выбрать, какую сущность углубить первой
   (по умолчанию — самые fundamental: aggregate перед events перед
   use-case'ами).
7. Перейти в режим C на выбранной. Повторить для остальных.

### Что НЕ делать в режиме B

- Не объявлять извлечённое «зафиксированным». Это кандидаты до
  подтверждения архитектора.
- Не угадывать `invariants`, `preconditions`, тексты `Rules` по сырью —
  только `name` / краткое `description`, максимально близко к языку
  архитектора.
- Не выдумывать кандидатов, которых в материале нет, «потому что обычно
  бывают в таком домене».

## C. Углубление

**Сигнал**: args = `<ctx>:<kind>:<Name>` существующего артефакта, либо
архитектор выбрал из сводки.

### Шаги

1. Прочитать файл артефакта целиком.
2. Прочитать `state/<ctx>/<kind>/<Name>.md`, если есть.
3. Прочитать соседей по cross-ref'ам: для `aggregate` — связанные
   events/, use-cases/, policies/ из своего контекста + чужие
   контексты, на которые ссылается; для `event` — source aggregate и
   consumers; для `use-case` — aggregate (called methods) + emitted events.
4. Прогнать обязательные H2 по `references/field-checklists.md` для
   этого kind'а. Найти:
   - пустые / placeholder-only H2-секции;
   - подозрения по anti-patterns (метод агрегата с I/O → use-case;
     `goal` use-case = «чтобы продолжить работу» → подмена предусловием;
     event payload содержит UI-state; spanning invariant между
     aggregate'ами; cross-context relation типа `composition`);
   - битые cross-ref'ы (`relations`, `emits`, `consumes` указывают на
     несуществующее);
   - frontmatter-дрифт (basename не совпадает с `name`, `id` не равен
     `<context>.<name>`).
5. Раунд `AskUserQuestion` (до 4 вопросов) — точечно по найденному:
   - дыры — заполняем через Other;
   - anti-pattern-подозрения — опции «да, anti-pattern, переразбиваю»
     / «нет, вот почему» (через Other);
   - битые ссылки — опции «исправить на <варианты>» / «удалить ссылку»
     / «оставить stub-сосед (создать)»;
   - frontmatter-дрифт — авто-предложение исправить через AskUserQuestion
     «исправляю?» (опции Да/Нет).
6. Применить ответы: правка артефакта через `Edit`, обновление
   state-заметки, mirror-обновление соседей если нужно (`emits` в use-case
   → `events/<X>.md § Triggered by`; `relations` aggregate → симметричная
   запись в соседе если двунаправленная).
7. После раунда — повторно прогнать чек-лист `references/draft-stable.md`.
   Если все 5 ✅ — предложить промоушн.
8. Если осталась хотя бы одна дыра или подозрение — следующий раунд.

### Когда переходить от C к завершению

- Все 5 пунктов чек-листа ✅ → AskUserQuestion «Готов к stable, повышаем?»,
  по «Да» — `Edit` `status: draft → stable`.
- Остался `**blocker**`-вопрос, на который архитектор сейчас ответить не может
  → зафиксировать в state-заметке, оставить `status: draft`, выйти из
  углубления этой сущности; предложить другую из сводки.

## Стиль вопросов: правила

### Когда давать опции

| ситуация | формат |
|---|---|
| anti-pattern из `docs/domains/README.md § Anti-patterns` | две опции: «это anti-pattern, переразбиваю» / «нет, вот почему» (Other) |
| enum-поле (`kind`, `Visibility`, `Mutable`, `Source`, `Type of decision`, `Channel`, `Delivery`) | по опции на каждое значение enum |
| тест-критерий из `references/draft-stable.md` | опции из примеров ✅/❌, архитектор соотносит свой случай |
| выбор существующего соседа (relations/emits/consumes/called methods) | по опции на каждый существующий `<ctx>.<Name>` + «нужен новый» (Other → A-режим / stub) |
| структурный bool (`entrypoint`, `mutable`, `pure`) | две опции `yes` / `no` |
| «promote draft → stable?» | две опции `Да, повысить` / `Нет, ещё не уверен` (Other) |

### Когда не давать опции, только Other

| поле | почему |
|---|---|
| `name` | человекочитаемое имя — формулировка архитектора |
| `description` | смысл артефакта — слова архитектора |
| `Constraints`, `Default`, `preconditions`, `postconditions` | доменная семантика, не угадывать |
| текст инварианта | falsifiable-формулировка — выбор архитектора |
| текст `Rule` в policy | бизнес-правило — формулировка архитектора |
| текст `Step` в use-case | observable действие — формулировка архитектора |
| текст `Errors` в use-case | sentinel-имя + when — выбор архитектора |
| имена `Field` / `Method` / `Event` | архитектор называет, не скилл |
| любой `Name`, который ещё не существует | архитектор называет, не скилл |

### Чего не делать

- Не предлагать «образцы» вроде «может быть, инвариант такой-то?» — это
  выдумывание домена.
- Не упоминать «правильные» формулировки из шаблонов как опции в виде
  готовых текстов значений. Примеры в шаблонах — это иллюстрация формы,
  не варианты для текущего проекта.
- Не задавать больше 4 вопросов в одном `AskUserQuestion`. Если нужно
  больше — следующий раунд.
- Не закрывать сессию без чек-листа `references/draft-stable.md` хотя
  бы по одной правленой сущности.
- Не делать AskUserQuestion ради подтверждения тривиального ввода архитектора
  («ты сказал X, точно X?») — пиши и иди дальше.
