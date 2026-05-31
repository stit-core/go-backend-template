# Контракт `draft → stable`

Артефакт становится `stable`, когда **все 5 пунктов одновременно** ✅.
Скилл проверяет, юзер подтверждает, скилл применяет правку `status:`.

## Чек-лист

### 1. Обязательные H2 заполнены

Опциональные могут быть `_N/A_` / `_нет_`. Обязательные — с непустым
осмысленным содержимым (не placeholder из шаблона типа `<…>` /
`<смысл>`). См. `references/field-checklists.md` — там по каждому
`kind` перечислены обязательные H2 и какой шаблон placeholder'а они
вынесли.

Программная проверка: пройтись по обязательным H2 (из field-checklist),
для каждой убедиться что:
- секция существует (`## <Name>` найден);
- в ней есть непустой текст / таблица с непустыми ячейками;
- placeholder'ы вида `<...>` и `_TBD_` не остались.

### 2. Все cross-ref'ы валидны

Каждая ссылка во frontmatter (`relations`, `emits`, `consumes`) и в
теле артефакта (`[<X>](path)`) указывает на существующий
`<context>.<Name>` (т.е. реальный артефакт).

- `relations`/`emits`/`consumes` парсятся как `<ctx>.<Name>`;
  соответствующий файл существует.
- Markdown-ссылки на локальные `.md` резолвятся (путь существует).

`stable` может ссылаться на `draft` — это допустимо. Валидатор
подсветит hint, но не блокирует переход.

### 3. Валидатор зелёный

```bash
scripts/tools/validate-domain-doc.sh docs/domains/<ctx>
```

Проверяет:
- frontmatter (`id`, `kind`, `context`, `name`, `status` обязательны;
  `kind ∈ {aggregate, entity, event, use-case, policy, vo}`);
- `id` имеет ровно одну точку (форма `<context>.<Name>`);
- basename файла совпадает с `name`;
- все обязательные H2 для kind присутствуют;
- при `status: stable` — секции непусты, и `relations`/`emits`/`consumes`
  указывают на существующие id;
- для `<ctx>/README.md` — каждый aggregate-файл слинкован, и каждая
  ссылка на локальный `*.md` резолвится.

Если красный — вернись к проблемным секциям, не маркируй `stable`.

### 4. Тест-критерий kind'а явно подтверждён

В этой сессии. Скилл предъявляет через `AskUserQuestion` примеры
✅/❌, юзер соотносит свою запись и выбирает «соответствует ✅» / «нет,
ближе к ❌» (через Other — что именно).

См. **§ Тест-критерий по kind** ниже.

### 5. State чист по `**blocker**`-вопросам

`state/<ctx>/<kind>/<slug>.md`:
- Не содержит секцию `## Открытые вопросы` ИЛИ
- Содержит, но в ней нет вопросов с префиксом/суффиксом `**blocker**`.

Не-blocker открытые вопросы при stable допустимы — они описывают
«хорошо бы узнать», не «здесь смысл сломан».

Секция `## Доменные решения` к stable не относится — она может быть и
после stable, как накопленный контекст.
Секция `## Что копать дальше` — обычно пуста к stable.

## Тест-критерий по kind

Скилл готовит ✅/❌-варианты под конкретный артефакт. Шаблон вопросов
ниже; конкретные примеры скилл подставляет из самого артефакта.

### aggregate

> Каждый метод агрегата либо чистый (computation + state-mutation, без
> I/O / БД / сети), либо порождает событие/sentinel-ошибку?

- ✅ `User.ChangeEmail(newEmail Email) error` — валидирует, мутирует
  поле, ставит `UpdatedAt`, эмитит `UserEmailChanged`. Без I/O.
- ❌ `User.SendVerification() error` — вызывает email-провайдер.
  Это use-case (`SendVerification`), не метод агрегата.

### entity

> Child entity недоступна снаружи aggregate'а напрямую? Её жизненный
> цикл управляется через методы parent aggregate'а?

- ✅ `OrderLineItem` мутируется только через `Order.AddLine` /
  `Order.RemoveLine`. Прямого репозитория `LineItemRepo` нет.
- ❌ Есть `LineItemRepo.Update(item LineItem) error`, доступный извне
  агрегата `Order` — нарушает границу.

### event

> Событие именовано в прошедшем времени, payload минимален и
> воспроизводим через `EventID` + `<Aggregate>ID`, без UI-state и без
> sensitive-данных (паролей, сырых токенов)?

- ✅ `UserRegistered { EventID, OccurredAt, UserID, Email }`.
- ❌ `SubmitForm { FormState, ScrollPosition, JWT }` — UI-состояние +
  чувствительные данные.

### use-case

> Один use-case = одна бизнес-операция актора с одной целью; авторизация
> — middleware (`authgate`) или явная policy в `Steps`, не молчаливо
> размазана; шаги observable; ошибки — sentinel'ы с HTTP-статусом?

- ✅ `RegisterUser`: actor=anonymous HTTP, Inputs/Outputs описаны, Steps
  = Validate→Construct→Persist→Emit→Return, Errors c HTTP-статусами.
- ❌ `HandleUserStuff`: actor неясен, Steps = «обрабатывает запрос»,
  Errors отсутствуют → размазанный CRUD-эндпоинт.

### policy

> Pure-решение: без I/O, без скрытого состояния; Inputs+Rules дают
> детерминированный Outcome; используется ≥1 use-case'ом?

- ✅ `CancelWindow`: Inputs=`Order`, Rules=«можно отменить если
  `now - Order.PlacedAt ≤ 3d`», Outcomes=`allow|deny`. Used by:
  `CancelOrder`.
- ❌ `OrderValidation` дёргает БД через `OrderRepo.HasDuplicate(...)` —
  это не policy, это use-case или метод репо.

### vo (cross-context, `_shared/`)

> Чисто платформенный тип без доменной семантики? Реально нужен ≥2
> контекстам?

- ✅ `PhotoRef { Bucket, Key }` — storage-ссылка, нейтральна.
- ❌ `Phone` / `Email` — доменная семантика; должно быть inline-VO
  в соответствующем aggregate-файле, не в `_shared/`.

### context-README

> Описывает только этот bounded context: aggregates/use-cases/events
> перечислены, cross-context dependencies указаны в обе стороны (in/out),
> ubiquitous language заполнен с явными «не путать с»?

- ✅ `user/README.md`: Purpose, Aggregates=[User], Events out=[UserRegistered],
  In=[EmailVerified from verification], Glossary=`Email≠email-сообщение`.
- ❌ README перечисляет половину артефактов и забывает cross-context
  inbound события.

## Алгоритм проверки скиллом

После любой правки артефакта:

1. `Read` файл артефакта.
2. `Read` `state/<ctx>/<kind>/<slug>.md`, если есть.
3. Прогнать пункты 1, 2, 3, 5 (программно проверяемые).
4. Если все 4 ✅ — раунд `AskUserQuestion` по тест-критерию kind'а
   (пункт 4) с двумя ✅/❌-вариантами + Other.
5. Если и пункт 4 ✅ — раунд `AskUserQuestion`: «Готов к stable,
   повышаем?» с опциями «Да, повысить» / «Нет, ещё не уверен» (Other —
   что именно смущает).
6. При «Да» — `Edit`'ом `status: draft → stable`. Запустить валидатор
   ещё раз для верификации (теперь он проверит stable-инварианты).
   Красный → откатить status, вернуться в C.

## Что НЕ делать

- **Не повышать сам**, без подтверждения юзера. Даже если все 5 ✅ —
  спросить.
- **Не понижать `stable → draft` сам** при обнаружении проблемы — это
  смысловое решение юзера. Можно подсветить и предложить через
  AskUserQuestion.
- **Не считать пункт 4 пройденным «по умолчанию»**, потому что секции
  заполнены. Тест-критерий — отдельное смысловое подтверждение.
- **Не игнорировать `**blocker**`-вопросы** — они должны быть закрыты
  или удалены (если стали неактуальны) до stable.
- **Не маркировать stable при красном валидаторе.** Пункт 3 — gate.
