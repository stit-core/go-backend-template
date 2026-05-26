---
name: stack-deep-dive
description: Спроектировать платформу/стек сервиса — довести 8 stack-артефактов в docs/stack/ (service/runtime/config/persistence/messaging/auth/observability/dependencies) от драфта до status=stable через волны вопросов. Каждый артефакт несёт секцию Generates — вход для de-template (что родится в коде). Активируй когда юзер проектирует стек, говорит "спроектируем стек", "добьём stack-артефакты", "/stack-deep-dive", или когда init-service дошёл до шага стека. Зеркало domain-deep-dive для платформенного слоя; решения не выдумывай — интервьюируй.
---

# Skill: stack-deep-dive

Проектирование платформы сервиса: доводит 8 singleton-артефактов `docs/stack/` до `status: stable`.
Где `domain-deep-dive` отвечает «какая бизнес-логика», этот скилл — «на какой платформе она
крутится»: runtime, env, kit'ы golibinfra, subjects, как версионируется и деплоится образ.

Это **source of truth и вход для `de-template`**: секция `Generates` каждого артефакта описывает,
что из него родится в коде (module path, `internal/platform/*`, `cmd/api`-композиция, `.env.example`,
нужные kit'ы). Проектируем здесь, разворачиваем потом. Работает через AskUserQuestion в режиме
интервьюера, **не выдумывает** платформенные решения сам.

В онбординг-флоу `init-service` чейнит сюда после `domain-deep-dive`, затем — `de-template`.

## Когда применять

- "Спроектируем стек сервиса" / "Добьём stack-артефакты"
- "/stack-deep-dive"
- `init-service` дошёл до шага проектирования стека
- Артефакты `docs/stack/` в `draft` — нужно довести до `stable`

## Когда НЕ применять

- Артефактов ещё нет — сначала скопируй 8 шаблонов из `docs/stack/_templates/` в `docs/stack/`.
- Всё уже `stable` — просто запусти валидатор, не интервьюй заново.
- Правка одного готового артефакта (поменялся env) — это точечная правка, не deep-dive.

## Шаги

### 1. Сканируй контекст

```bash
ls docs/stack/
scripts/tools/validate-stack-doc.sh docs/stack
```

Если валидатор зелёный и все 8 — `stable`: выходи, «стек спроектирован, делать нечего».
Иначе для каждого артефакта прочитай frontmatter (`id`/`kind`/`status`) и H2-заголовки,
вычисли заполненность по обязательным секциям (см. `docs/stack/README.md`).

### 2. Покажи карту состояния

Сжато, ≤12 строк — по одному на kind:

```
## Состояние docs/stack

| kind | status | заполнено | дыры |
|---|---|---|---|
| service | draft | 2/4 | Versioning, Generates |
| config  | draft | 1/5 | Optional env, Secrets, Validation, Generates |
| …       |        |     |      |
```

### 3. Веди волны

Порядок — фундамент → производное (запоминай, у пользователя не спрашивай):

1. **service** — имя, module path, версионирование, бинари.
2. **dependencies** — какие kit'ы golibinfra, внешние runtime-deps, ёмкость.
3. **persistence** — Postgres: driver, миграции, kit-таблицы, пул.
4. **messaging** — published/consumed subjects, delivery semantics.
5. **auth** — authn/authz, authgate, роли, seed-admin, brute-force.
6. **observability** — telemetry-режим, resource attrs, biz-метрики, trace-points.
7. **config** — required/optional env, секреты (Vault-класс), валидация (роллап того,
   что запросили предыдущие kit'ы/subsystem'ы).
8. **runtime** — entrypoints в `cmd/api` (HTTP / outbox-drain / NATS-subs / jobs), lifecycle,
   shutdown (роллап: что из messaging/jobs реально поднимается).

Для текущего kind:

1. Собери пустые/неполные H2-секции (обязательные — в `required_sections` валидатора и
   `docs/stack/README.md`).
2. Сформулируй 3–5 предметных вопросов через `AskUserQuestion` (≤4 за tool-call — лимит).
3. После ответов **сразу** пиши в md через Edit/Write — не копи «запишу в конце» (теряется при /clear).
4. **Секция `Generates`** — заполняй явно: что из этого артефакта родится в коде (это контракт
   для `de-template`). Без `Generates` артефакт бесполезен движку.
5. Кратко покажи, что добавилось, переходи к следующему kind.

### 4. Правила вопросов

- **Конкретные, falsifiable.** Плохо: «как с конфигом?». Хорошо: «`DATABASE_URL` required без
  дефолта, или есть fallback на localhost для dev?».
- **Bounded.** Опции AskUserQuestion взаимоисключающие (или `multiSelect=true`).
- **Не выдумывай.** «Не знаю» → пиши `TBD` явно, вернись позже. Артефакт = слепок решения юзера.
- **Опирайся на reference.** Шаблонные `auth`/`user`/`verification` + `golibinfra` kit'ы — живой
  пример: «telemetry сейчас через `otelkit` в stdout-режиме — оставляем или меняем?».

### 5. Промоушн status

После всех волн:

```bash
scripts/tools/validate-stack-doc.sh docs/stack
```

Зелёный → отредактируй frontmatter каждого `status: draft → stable`. Запусти валидатор ещё раз
(теперь проверит non-empty bodies). Красный → вернись к проблемным секциям, не маркируй stable.

### 6. Финал

Покажи сводку: 8/8 stable. Предложи следующий шаг по контексту:
- **Онбординг** (пришёл из `init-service`): стек спроектирован → дальше `de-template` развернёт
  фундамент по секциям `Generates`.
- **Отдельно**: артефакты — актуальный source of truth платформы; правь их при изменениях стека.

## Anti-patterns

- ❌ >4 вопросов одним AskUserQuestion (инструмент режет до 4).
- ❌ Копить ответы в RAM и писать в файлы в конце волны — теряется прогресс при /clear.
- ❌ Оставить `Generates` пустым — движок `de-template` именно по ней разворачивает код.
- ❌ Самому додумывать платформенное решение (какой env, какой subject). Интервьюируй.
- ❌ Промоутить `stable` при красном валидаторе.
- ❌ Прыгать через волны: config/runtime — роллапы, их заполняют после concrete-подсистем.

## Что делает валидатор (для справки)

- Frontmatter: `id`, `kind`, `name`, `status` обязательны; `kind` ∈ 8 stack-kinds.
- `basename == name`; `id == "stack.<name>"`.
- Все обязательные H2 для kind присутствуют; при `status: stable` — непусты.

См. `scripts/tools/validate-stack-doc.sh` и `docs/stack/README.md`.
