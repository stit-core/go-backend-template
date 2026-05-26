---
name: domain-deep-dive
description: Спроектировать бизнес-домен — довести DDD-артефакты в docs/domains/<context>/ от минимального драфта до status=stable через волны вопросов inside-out (properties→methods→invariants→lifecycle→events→use-cases→policies). Эти артефакты — source of truth, из которого de-template/scaffold разворачивают код. Активируй когда юзер проектирует домен, приносит черновики и говорит "спроектируем домен", "разогнать/углубить/добить артефакты", "/domain-deep-dive <ctx>", или когда init-service дошёл до шага проектирования доменов. Бизнес-решения не выдумывай — интервьюируй.
---

# Skill: domain-deep-dive

Проектирование бизнес-домена: превращает минимальные драфты артефактов (frontmatter + properties + methods) в полные `status: stable` артефакты. Это **source of truth** — из stable-артефактов `de-template` (онбординг) и `scaffold` (вариант domain, день-в-день) разворачивают код; проектируем здесь, кодим потом. Работает через AskUserQuestion в режиме интервьюера, **не выдумывает** бизнес-решения сам.

В онбординг-флоу шаблона `init-service` чейнит сюда (шаг «спроектируй домены»), затем — `stack-deep-dive` → `de-template`.

## Когда применять

- "Спроектируем домен `order`" / "Разогнать артефакты в docs/domains/user/"
- "Принёс драфты, давай добивать"
- "/domain-deep-dive user"
- `init-service` дошёл до проектирования доменов
- Когда пользователь просит превратить минимальный артефакт в готовый

## Когда НЕ применять

- Артефактов ещё нет — сначала пусть пользователь скопирует шаблоны из `docs/domains/_templates/` и заполнит минимум.
- Артефакт уже `status: stable` — просто запусти валидатор, не интервьюй заново.
- Изменение существующего стабильного артефакта — это правка, не углубление.

## Шаги

### 1. Сканируй контекст

```bash
ls docs/domains/<ctx>/
scripts/tools/validate-domain-doc.sh docs/domains/<ctx>
```

Если валидатор зелёный и все артефакты stable — выходи: "уже всё stable, делать нечего".

Иначе — собери список:
- aggregates (`<ctx>/*.md` кроме README.md)
- events (`<ctx>/events/*.md`)
- use-cases (`<ctx>/use-cases/*.md`)
- policies (`<ctx>/policies/*.md`)

Если по ходу углубления обнаружишь VO, который структурно идентичен в ≥2 контекстах и не несёт доменной семантики (примеры: storage refs, media-метаданные) — предложи вынести его в `docs/domains/_shared/<Name>.md` (kind=vo). Внутри-контекстные VO остаются inline в aggregate-файле.

Для каждого прочитай frontmatter (id/kind/status) и H2-заголовки, вычисли заполненность по обязательному чеклисту (см. `docs/domains/README.md`).

### 2. Покажи карту состояния

Сжато, ≤20 строк:

```
## Состояние docs/domains/<ctx>

| артефакт | kind | status | заполнено | дыры |
|---|---|---|---|---|
| User.md | aggregate | draft | 6/12 | Invariants, Lifecycle, Events, Relations, VOs, Use Cases |
| events/UserRegistered.md | event | draft | 1/4 | Payload, Consumers, Delivery |
| use-cases/RegisterUser.md | use-case | draft | 3/8 | Dependencies, Steps, Errors, ... |
```

### 3. Подтверди план волн

Inside-out порядок (запоминай его, спрашивать у пользователя не надо):

1. **Properties (детали)** — constraints/default/source для каждого property всех aggregate'ов
2. **Methods (детали)** — params/returns/preconditions/postconditions/events для всех методов
3. **Invariants** — собрать список инвариантов и привязать каждый к owning method'ам
4. **Lifecycle** — таблица переходов (если stateful)
5. **Identity** — ID type, mutability, visibility
6. **Boundary / VOs / Contained** — для aggregate'ов
7. **Events** — добить payload, consumers, delivery semantics в `events/*.md`
8. **Use Cases** — actor, deps, steps, errors, called methods, errors
9. **Policies** — rules, outcomes (если есть)
10. **Relations** — связи между артефактами
11. **Context README / Ubiquitous Language** — финальный роллап на контекст

Спроси пользователя:
> Прохожу волны inside-out. Начну с Properties для всех aggregate'ов в этом контексте. Подтверди или скажи если хочешь сразу прыгнуть в конкретную волну.

### 4. Веди волну

Для текущей волны:

1. Собери все артефакты, у которых эта секция пуста или неполна.
2. На каждый артефакт сформулируй 3-5 предметных вопросов через `AskUserQuestion`. Не больше 4 вопросов за один tool-call (лимит инструмента) — если артефактов много, делай несколько раундов.
3. После ответов **сразу** записывай в md-файлы через Edit/Write. Не копи "запишу в конце".
4. После записи — кратко покажи что добавилось ("User.md: Properties детализированы для Email, PasswordHash, Status; обновил frontmatter не требуется").
5. Переходи к следующему артефакту в этой же волне или к следующей волне.

### 5. Правила вопросов

- **Конкретные, falsifiable.** Плохо: "какие у User инварианты?". Хорошо: "Может ли существовать User с Status=blocked и EmailVerified=false?".
- **Bounded.** Опции в AskUserQuestion должны быть взаимоисключающими (или multiSelect=true).
- **Не выдумывай бизнес-правила.** Если пользователь говорит "не знаю" — записывай `TBD` явно, не придумывай. Возвращайся к этому вопросу позже.
- **Используй contextual examples.** Если у пользователя в коде есть похожий паттерн — приводи как референс ("в auth.Session lifecycle это сделано так-то — следуем тому же или отличается?").

### 6. Промоушн status

После всех волн:

```bash
scripts/tools/validate-domain-doc.sh docs/domains/<ctx>
```

Если зелёный — отредактируй frontmatter каждого артефакта `status: draft → stable`.

Запусти валидатор ещё раз (теперь он проверит non-empty bodies + ref-целостность). Если красный — вернись к проблемным секциям, не маркируй stable.

### 7. Финал

Покажи короткую сводку: сколько артефактов теперь stable. Предложи следующий шаг по контексту:
- **Онбординг** (пришёл из `init-service`): домены готовы → дальше `stack-deep-dive`, затем `de-template`.
- **День-в-день** (домен в существующем проекте): `/smartdev` → `/plan <task>` для имплементации, или скаффолд через `scaffold` (вариант domain).

## Anti-patterns

- ❌ Задавать >4 вопросов одним AskUserQuestion (инструмент режет до 4).
- ❌ Накапливать ответы в памяти и писать в файлы только в конце волны — теряется прогресс при /clear.
- ❌ Самому додумывать бизнес-логику. Артефакт = слепок понимания пользователя, а не твоего.
- ❌ Прыгать через волны без причины. Inside-out даёт самый плотный feedback loop: пользователь думает о ближайшем (поля, методы), потом о правилах над ними.
- ❌ Промоутить status=stable если валидатор красный.

## Что делает валидатор (для справки)

- Frontmatter: id, kind, context, name, status — обязательны; kind ∈ {aggregate, entity, event, use-case, policy, vo}.
- `id` имеет ровно одну точку (форма `<context>.<Name>`); basename файла совпадает с `name`.
- Все обязательные H2-секции для kind присутствуют.
- При status=stable — секции непусты, и relations/emits/consumes указывают на существующие id.
- Для `<ctx>/README.md` — каждый aggregate-файл в директории должен быть слинкован, и каждая ссылка из README на локальный `*.md` должна резолвиться.

См. `scripts/tools/validate-domain-doc.sh` и `docs/domains/README.md`.
