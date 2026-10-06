---
name: init-service
description: Оркестратор онбординга форка go-backend-template — проектирование стека (stack-deep-dive) → de-template → код доменов из соседнего `<org>/strategy` → удаление reference-доменов, пока проект не перестанет помнить про шаблон. Используй на свежем форке (module path ещё go-backend-template, reference-домены auth/user/verification на месте) или когда юзер хочет развернуть свой сервис из шаблона. Сам не проектирует и не де-темплейтит — чейнит специализированные скиллы по порядку.
---

# Skill: init-service (онбординг-оркестратор)

Дирижёр пути «форк шаблона → свой проект». Не делает проектирование/генерацию сам, а **ведёт по
этапам и чейнит** специализированные скиллы, держа фокус: docs первичны, код вторичен и
разворачивается по ним. Один проход доводит до проекта, который ничего не помнит про шаблон.

Полное описание пути для человека — `docs/getting-started.md`; этот скилл его исполняет.

## Когда применять

- Юзер (или агент хаба `<org>/infra`) запускает онбординг форка: «развернул форк, что дальше»
- Свежий форк: module path ещё `…/go-backend-template`, reference-домены `auth`/`user`/`verification` на месте

## Когда НЕ применять

- Проект уже де-темплейтизирован (свой module path, reference-домены удалены) — это обычный
  день-в-день, иди в `/smartdev`.
- Юзер хочет добавить домен или ручку в уже свой проект — не онбординг: маршрут по таблице
  «Маршрут задачи» (CLAUDE.md).

## Pre-flight

- [ ] Форк создан, репозиторий склонирован (Gitea → **Use this template** → `<org>/<service>`).
- [ ] `make setup` прогнан (pinned dev-tools + git-хуки).
- [ ] Домены сервиса в соседнем `<org>/strategy` (`../strategy/docs/projects/<slug>/services/<svc>/domains/`)
      уже `stable` — это **вход** онбординга, не его шаг (см. этап 1).
- [ ] Пункт не закрыт — подскажи, как закрыть (шаг 0 `docs/getting-started.md` или доведение
      доменов в strategy-репо), дальше не иди.

## Этапы (чейнить по порядку, не делать самому)

### 1. Домены — вход (уже спроектированы в strategy-репо)

Домены проектируются **не в этом репо и не на этом шаге** — они уже готовы к моменту онбординга
бэка. Бизнес-аналитика и DDD продукта прорабатываются в соседнем `<org>/strategy` (форк
`business-strategy-template`) **до** форка бэкенда; домены живут там внутри сервиса-владельца —
`docs/projects/<slug>/services/<svc>/domains/<ctx>/`. init-service берёт их как **готовый вход**:

- **Предусловие**: нужные домены продукта в `<org>/strategy` (рядом, по конвенции `../strategy`) уже
  `status: stable` (собственный валидатор strategy-репо зелёный, state без `**blocker**`). Бэкенд это
  сам не проверяет — доверяет strategy-репо. Если домен ещё не доведён — это **не шаг онбординга бэка**:
  доводишь его в strategy-репо скиллом `/domain-deep-dive <slug> <ctx>` и возвращаешься.
- Reference-домены `auth`/`user`/`verification` в этом backend-репо — **code-only** учебные
  образцы (артефактов в docs у них нет): изучить как образец Go-раскладки, удаление — этап 5.
- Зафиксируй, какой `<slug>` и какой сервис `<svc>` из реестра `services/service.md` strategy-репо
  реализует этот бэкенд — они уйдут в `docs/stack/service.md` на этапе 2 (по ним `scaffold` потом
  найдёт домены).

### 2. Спроектировать стек → `stack-deep-dive`

- Скопировать 8 шаблонов `docs/stack/_templates/` в `docs/stack/` — либо `stack-deep-dive`
  в **режиме A** разложит недостающие сам.
- **Чейнь в `stack-deep-dive`**: волнами фундамент→роллап (`service` → kit'ы → `config`+`runtime`)
  до `status: stable` всех 8 артефактов с **непустой `Generates`** в каждом — это контракт
  входа для `de-template`. Полный чек-лист промоушна (включая cross-kind sync) —
  `.claude/skills/stack-deep-dive/references/draft-stable.md`.
- В `service.md § Identity` зафиксируй `project-slug`, `service-slug` и `strategy-path` (по ним
  `scaffold` найдёт домены в соседнем strategy-репо — этап 4).
- Gate: `make validate-stack-docs` зелёный + у всех 8 `status: stable`
  + state-каталог `.claude/skills/stack-deep-dive/state/` чист по `**blocker**`-вопросам.

### 3. Развернуть фундамент → `de-template`

- Gate перед запуском: стек `stable`, `make validate-stack-docs` зелёный, state `stack-deep-dive`
  чист по `**blocker**`-вопросам. Домены — в strategy-репо, для de-template не требуются (он
  разворачивает только платформу).
- **Чейнь в `de-template`**: читает stable-артефакты `docs/stack`, разворачивает по
  секциям `Generates` — module path, README, `internal/platform/*`, `cmd/api`-композиция,
  `go.mod`, `.env.example`, нужные kit'ы. Механика детерминирована (Phase A); platform/main
  wiring — reviewable diff (Phase B).

### 4. Свои домены — по таблице «Маршрут задачи»

Платформа развёрнута — теперь каждый **stable**-домен из strategy-репо идёт в Go-код по таблице
«Маршрут задачи» (CLAUDE.md):

- Слои домена разворачивает `scaffold` (вариант domain): читает
  `<strategy-path>/docs/projects/<slug>/services/<svc>/domains/<ctx>/` (slug, сервис и strategy-path —
  из `docs/stack/service.md`), разворачивает `internal/domain/<ctx>/`.
  Маппинг секций → код — `docs/reference/domain-mapping.md`.
- **Инвариант порядка**: свои домены — **до** удаления reference (этап 5), иначе потеряешь живой
  Go-образец.

### 5. Удалить reference-домены

Когда свои домены спроектированы (в strategy-репо) и сгенерированы (этап 4) — убрать учебные
`auth`/`user`/`verification` по чеклисту `.claude/skills/de-template/references/prune.md`
(пакеты, блоки `sqlc.yaml`/`.mockery.yml`, миграции, fx-wiring, e2e, `cmd/seed-admin`);
его выполняет `de-template` (Phase A.5).

### 6. Финал

`make check` зелёный → проект больше не помнит про шаблон. Сообщи юзеру: онбординг закрыт,
дальше обычный флоу через `/smartdev`; релиз образов — `/release`; обновления golibinfra и обвязки
шаблона (`.template/`) — `/upgrade`. `docs/getting-started.md` (META) можно удалить.

## Что init-service НЕ делает

- ❌ Не интервьюирует по доменам сам — это `/domain-deep-dive` в соседнем strategy-репо; по стеку —
  `stack-deep-dive` здесь (3 режима A/B/C, дисциплина опции-vs-Other, 6-пунктный draft→stable gate).
- ❌ Не лезет в `state/` чейненных скиллов — это их рабочая память, оркестратор её только
  проверяет на чистоту `**blocker**`-вопросов как gate-условие.
- ❌ Не правит фундамент руками и не де-темплейтит — это `de-template` (по stable docs/stack).
- ❌ Не запускает `de-template` до зелёного `make validate-stack-docs` (gate).
- ❌ Не пропускает проектирование ради «быстро сгенерить» — docs первичны, иначе движку не из чего разворачивать.

## Anti-patterns

- ❌ Кодить/править `internal/platform` до того как спроектированы артефакты.
- ❌ Удалять reference-домены до того как свои готовы (теряешь живой образец).
- ❌ Дублировать процедуру deep-dive внутри этого скилла — он только дирижёр.
