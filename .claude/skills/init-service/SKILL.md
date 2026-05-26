---
name: init-service
description: Оркестратор онбординга потребителя шаблона — ведёт путь форк → проектирование доменов (domain-deep-dive) → проектирование стека (stack-deep-dive) → de-template → удаление reference-доменов, пока проект не перестанет помнить про шаблон. Активируй когда юзер форкнул go-backend-template и говорит "/init-service", "онбординг", "развернуть свой сервис из шаблона", "с чего начать новый проект из шаблона", или когда видишь свежий форк (module path всё ещё go-backend-template, reference-домены auth/user/verification на месте). Сам не проектирует и не де-темплейтит — чейнит специализированные скиллы по порядку.
---

# Skill: init-service (онбординг-оркестратор)

Дирижёр пути «форк шаблона → свой проект». Не делает проектирование/генерацию сам, а **ведёт по
этапам и чейнит** специализированные скиллы, держа фокус: docs первичны, код вторичен и
разворачивается по ним. Один проход доводит до проекта, который ничего не помнит про шаблон.

Полное описание пути для человека — `docs/getting-started.md`; этот скилл его исполняет.

## Когда применять

- "/init-service" / "онбординг" / "развернул форк, что дальше"
- Свежий форк: module path ещё `…/go-backend-template`, reference-домены `auth`/`user`/`verification` на месте
- Юзер: «хочу развернуть свой сервис из шаблона»

## Когда НЕ применять

- Проект уже де-темплейтизирован (свой module path, reference-домены удалены) — это обычный
  день-в-день, иди в `/smartdev`.
- Юзер хочет добавить домен/endpoint в уже-свой проект — это `scaffold`, не онбординг.

## Pre-flight

- [ ] Форк создан, репозиторий склонирован (Gitea → **Use this template** → `<org>/<service>`).
- [ ] `make setup` прогнан (pinned dev-tools + git-хуки).
- [ ] Если нет — подскажи шаги 0 из `docs/getting-started.md`, дальше не иди.

## Этапы (чейнить по порядку, не делать самому)

### 1. Спроектировать домены → `domain-deep-dive`

- Скопировать шаблоны `docs/domains/_templates/` в `docs/domains/<context>/`, заполнить минимум
  (frontmatter + properties + methods).
- **Чейнь в `domain-deep-dive`** на каждый контекст: волны inside-out до `status: stable`.
- Reference-домены `auth`/`user`/`verification` — учебные образцы: изучить, свои проектировать
  рядом; удаление — этап 4.
- Gate: `scripts/tools/validate-domain-doc.sh docs/domains` зелёный.

### 2. Спроектировать стек → `stack-deep-dive`

- Скопировать 8 шаблонов `docs/stack/_templates/` в `docs/stack/`.
- **Чейнь в `stack-deep-dive`**: волнами до `status: stable` всех 8 артефактов (с непустой `Generates`).
- Gate: `scripts/tools/validate-stack-doc.sh docs/stack` зелёный.

### 3. Развернуть фундамент → `de-template`

- Gate перед запуском: домены и стек `stable`, оба валидатора зелёные (зеркалит domain-gate).
- **Чейнь в `de-template`**: читает stable-артефакты `docs/{domains,stack}`, разворачивает по
  секциям `Generates` — module path, README, `internal/platform/*`, `cmd/api`-композиция,
  `go.mod`, `.env.example`, нужные kit'ы. Механика детерминирована; platform/main wiring —
  reviewable diff.

### 4. Удалить reference-домены

Когда свои домены спроектированы и сгенерированы — убрать учебные `auth`/`user`/`verification`
(чеклист подсказывает/выполняет `de-template`):
`internal/domain/{auth,user,verification}/` + их `docs/domains/*`, блоки в `sqlc.yaml`, их
`migrations/`, fx-wiring в `cmd/api`, depguard-правила в `.golangci.yml`, их e2e в `test/`.

### 5. Финал

`make check` зелёный → проект больше не помнит про шаблон. Сообщи юзеру: онбординг закрыт,
дальше обычный флоу через `/smartdev`; релиз образов — `/release`. `docs/getting-started.md`
(META) можно удалить.

## Что init-service НЕ делает

- ❌ Не интервьюирует по доменам/стеку сам — это `domain-deep-dive`/`stack-deep-dive`.
- ❌ Не правит фундамент руками и не де-темплейтит — это `de-template` (по stable-артефактам).
- ❌ Не запускает `de-template` до зелёных валидаторов (gate).
- ❌ Не пропускает проектирование ради «быстро сгенерить» — docs первичны, иначе движку не из чего разворачивать.

## Anti-patterns

- ❌ Кодить/править `internal/platform` до того как спроектированы артефакты.
- ❌ Удалять reference-домены до того как свои готовы (теряешь живой образец).
- ❌ Дублировать процедуру deep-dive внутри этого скилла — он только дирижёр.
