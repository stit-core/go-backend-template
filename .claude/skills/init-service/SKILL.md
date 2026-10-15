---
name: init-service
description: Оркестратор онбординга форка go-backend-template — планом с этапами: проектирование стека (stack-deep-dive) → de-template → свои домены из соседнего `<org>/strategy` (без него — по описанию юзера) → домены идентичности (остаются или prune по `docs/stack/auth.md`), пока проект не перестанет помнить про шаблон. Используй на свежем форке (module path ещё go-backend-template) или когда юзер хочет развернуть свой сервис из шаблона. Сам не проектирует и не де-темплейтит — чейнит специализированные скиллы по порядку.
---

# Skill: init-service (онбординг-оркестратор)

Дирижёр пути «форк шаблона → свой проект». Не проектирует и не генерирует сам — **ведёт по этапам и
чейнит** специализированные скиллы: docs первичны, код разворачивается по ним. Этапы идут один раз и
по порядку, каждый — вход следующего; цикла нет.

## Когда применять

- Юзер (или агент хаба `<org>/infra`) запускает онбординг форка: «развернул форк, что дальше»
- Свежий форк: module path ещё `…/go-backend-template`

## Когда НЕ применять

- Проект уже де-темплейтизирован (свой module path) и плана онбординга нет — это обычный день-в-день,
  `/smartdev`.
- Домен или ручка в уже свой проект — не онбординг: маршрут по таблице «Маршрут задачи» (CLAUDE.md).

## План онбординга

Онбординг — несколько сессий и коммитов, поэтому он идёт планом `onboarding` в `.claude/plans/` (skill
`/plan`): шаги — этапы ниже с их гейтами, точка возобновления — «Где мы сейчас» (`/smartdev` после
`/clear` поднимет его сам). Есть план — продолжай с его «Где мы сейчас»; нет — заведи, этап 0 первым
шагом.

## Этапы

### 0. Pre-flight

- Не канонический шаблон: `git config --get remote.origin.url` — не `stit-core/go-backend-template` (иначе СТОП:
  онбординг разрушит шаблон; гейт держит то же — archtest `TestCanonicalTemplate`).
- `make setup` прогнан (pinned dev-tools + git-хуки), `make check` зелёный.
- Планы шаблона — не форка: до плана `onboarding` убери их (de-template A.4), иначе `/smartdev`
  предложит продолжить работу шаблона.
- Strategy-репо: `../strategy` (форк `business-strategy-template`) — откуда домены сервиса
  (`docs/projects/<slug>/services/<svc>/domains/`, нужные — `stable`). Нет его — режим без strategy:
  домены этапа 3 — по описанию юзера, в «Решения» плана — строка об этом.

### 1. Стек → `stack-deep-dive`

- **Чейнь в `stack-deep-dive`**: 8 артефактов `docs/stack/` волнами фундамент → роллап до `status:
  stable` с непустой `Generates` в каждом — вход `de-template`. В `service.md § Identity` —
  `project-slug`, `service-slug`, `strategy-path` (без strategy — `—`); в `auth.md` — `identity`: `own`
  (учётные записи в этом сервисе) или `external` (токены выпускает другой сервис или IdP).
- Gate: `make validate-stack-docs A=--stable` зелёный, state `stack-deep-dive` чист по
  `**blocker**`-вопросам.

### 2. Платформа → `de-template`

- **Чейнь в `de-template`**: module path, README, `.env.example`, планы с чистого листа, wiring
  `internal/platform/*` и `cmd/api` по секциям `Generates`. Домены он не трогает.
- Gate: `make check` зелёный.

### 3. Свои домены — по таблице «Маршрут задачи»

Каждый домен сервиса — в Go-код по таблице «Маршрут задачи» (CLAUDE.md): `scaffold` (вариант domain)
читает stable-артефакт `<strategy-path>/docs/projects/<slug>/services/<svc>/domains/<ctx>/` (маппинг
секций → код — `docs/reference/domain-mapping.md`); без strategy — контракт домена (сущность, операции,
права, события) от юзера одним блоком, юзер подтверждает. Gate: свой домен подключён, `make check`
зелёный.

### 4. Домены идентичности

Решение — `identity` в `docs/stack/auth.md`:
- `own` — `auth`, `user`, `verification` остаются доменами проекта и растут по его артефактам, как свои;
- `external` — prune по `references/prune.md`: пути `.template/identity` и подключение их в `cmd/`.
  Только после этапа 3 — без своих доменов сервису нечего регистрировать.

Gate: `make check` зелёный.

### 5. Финал

Канал обвязки — remote шаблона без его тегов: о новом теге `template/vX.Y.Z` скажет `make doctor`
(`git ls-remote`, сеть — не ошибка), перенос — `/upgrade`.

```bash
git remote add --no-tags template git@git.stit.tech:stit-core/go-backend-template.git
```

Gate: `make doctor` → строка `template:` с версией обвязки форка, без «remote template не заведён».

Проект больше не помнит про шаблон: сообщи юзеру — онбординг закрыт, план удалён (skill `/plan`),
дальше — `/smartdev`; релиз образов — `/release`; обновления golibinfra и обвязки шаблона
(`.template/`) — `/upgrade`.

## Что init-service НЕ делает

- Не интервьюирует по доменам сам — это `/domain-deep-dive` в соседнем strategy-репо; по стеку —
  `stack-deep-dive`.
- Не лезет в `state/` чейненных скиллов — только проверяет чистоту `**blocker**`-вопросов как гейт.
- Не правит фундамент руками и не де-темплейтит — это `de-template` по stable `docs/stack`.
- Не меняет порядок этапов: de-template до стабильного стека, prune до своих доменов.
