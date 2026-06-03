---
name: init-service
description: Оркестратор онбординга потребителя шаблона — ведёт путь форк → проектирование доменов (в соседнем docs-репо орга) → проектирование стека (stack-deep-dive) → de-template → scaffold доменов → удаление reference-доменов, пока проект не перестанет помнить про шаблон. Активируй когда юзер форкнул go-backend-template и говорит "/init-service", "онбординг", "развернуть свой сервис из шаблона", "с чего начать новый проект из шаблона", или когда видишь свежий форк (module path всё ещё go-backend-template, reference-домены auth/user/verification на месте). Сам не проектирует и не де-темплейтит — чейнит специализированные скиллы по порядку.
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

### 1. Спроектировать домены → в соседнем docs-репо орга

Домены проектируются **не в этом репо**. Под организацию рядом с `<org>/backend` форкается
`<org>/docs` (из `project-docs-template`); DDD-артефакты продукта живут там —
`docs/projects/<slug>/domains/<ctx>/`. Этот оркестратор живёт в backend-репо и **не может
чейнить** скилл из другого репо — он направляет архитектора:

- Открой форк `<org>/docs` (рядом, по конвенции `../docs`). Там его скилл
  `/domain-deep-dive <slug> <ctx>` ведёт волнами inside-out до `status: stable` (и заводит
  сам продукт через `/project-deep-dive`, если его ещё нет).
- Reference-домены `auth`/`user`/`verification` в этом backend-репо — **code-only** учебные
  образцы (артефактов в docs у них нет): изучить как образец Go-раскладки, удаление — этап 5.
- Gate этапа: у нужных доменов в docs-репо `status: stable` (его собственный валидатор зелёный,
  state без `**blocker**`-вопросов). Бэкенд это сам не проверяет — доверяет docs-репо.
- Зафиксируй, какой `<slug>` обслуживает этот бэкенд — он уйдёт в `docs/stack/service.md` на
  этапе 2 (по нему `scaffold` потом найдёт домены).

### 2. Спроектировать стек → `stack-deep-dive`

- Скопировать 8 шаблонов `docs/stack/_templates/` в `docs/stack/` — либо `stack-deep-dive`
  в **режиме A** разложит недостающие сам.
- **Чейнь в `stack-deep-dive`**: волнами фундамент→роллап (`service` → kit'ы → `config`+`runtime`)
  до `status: stable` всех 8 артефактов с **непустой `Generates`** в каждом — это контракт
  входа для `de-template`. Полный чек-лист промоушна (включая cross-kind sync) —
  `.claude/skills/stack-deep-dive/references/draft-stable.md`.
- В `service.md § Identity` зафиксируй `project-slug` + `docs-path` (по ним `scaffold` найдёт
  домены в соседнем docs-репо — этап 4).
- Gate: `scripts/tools/validate-stack-doc.sh docs/stack` зелёный + у всех 8 `status: stable`
  + state-каталог `.claude/skills/stack-deep-dive/state/` чист по `**blocker**`-вопросам.

### 3. Развернуть фундамент → `de-template`

- Gate перед запуском: стек `stable`, `validate-stack-doc.sh` зелёный, state `stack-deep-dive`
  чист по `**blocker**`-вопросам. Домены — в docs-репо, для de-template не требуются (он
  разворачивает только платформу).
- **Чейнь в `de-template`**: читает stable-артефакты `docs/stack`, разворачивает по
  секциям `Generates` — module path, README, `internal/platform/*`, `cmd/api`-композиция,
  `go.mod`, `.env.example`, нужные kit'ы. Механика детерминирована (Phase A); platform/main
  wiring — reviewable diff (Phase B).

### 4. Scaffold спроектированных доменов → `scaffold` (вариант domain)

Платформа развёрнута — теперь по каждому **stable**-домену из docs-репо разверни Go-код:

- **Чейнь в `scaffold` (domain)** per домен: читает `<docs-path>/projects/<slug>/domains/<ctx>/`
  (slug/docs-path из `docs/stack/service.md`), разворачивает слои `internal/domain/<ctx>/`.
  Маппинг секций → код — `docs/reference/domain-mapping.md`.
- **Инвариант порядка**: scaffold своих доменов идёт **до** удаления reference (этап 5) — иначе
  потеряешь живой Go-образец.

### 5. Удалить reference-домены

Когда свои домены спроектированы (в docs-репо) и сгенерированы (этап 4) — убрать учебные
`auth`/`user`/`verification` (чеклист подсказывает/выполняет `de-template`):
`internal/domain/{auth,user,verification}/`, блоки в `sqlc.yaml`, их `migrations/`, fx-wiring в
`cmd/api`, depguard-правила в `.golangci.yml`, их e2e в `test/`. DDD-артефактов в docs у них нет
(code-only) — удалять там нечего.

### 6. Финал

`make check` зелёный → проект больше не помнит про шаблон. Сообщи юзеру: онбординг закрыт,
дальше обычный флоу через `/smartdev`; релиз образов — `/release`. `docs/getting-started.md`
(META) можно удалить.

## Что init-service НЕ делает

- ❌ Не интервьюирует по доменам сам — это `/domain-deep-dive` в соседнем docs-репо; по стеку —
  `stack-deep-dive` здесь (3 режима A/B/C, дисциплина опции-vs-Other, 5-пунктный draft→stable gate).
- ❌ Не лезет в `state/` чейненных скиллов — это их рабочая память, оркестратор её только
  проверяет на чистоту `**blocker**`-вопросов как gate-условие.
- ❌ Не правит фундамент руками и не де-темплейтит — это `de-template` (по stable docs/stack).
- ❌ Не запускает `de-template` до зелёного `validate-stack-doc.sh` (gate).
- ❌ Не пропускает проектирование ради «быстро сгенерить» — docs первичны, иначе движку не из чего разворачивать.

## Anti-patterns

- ❌ Кодить/править `internal/platform` до того как спроектированы артефакты.
- ❌ Удалять reference-домены до того как свои готовы (теряешь живой образец).
- ❌ Дублировать процедуру deep-dive внутри этого скилла — он только дирижёр.
