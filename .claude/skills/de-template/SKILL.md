---
name: de-template
description: Развернуть форк go-backend-template в самостоятельный проект по stable-артефактам docs/{domains,stack} — module path, README, .env.example, platform/cmd wiring, prune reference-доменов. Активируй когда юзер форкнул шаблон и говорит "/de-template", "разверни фундамент", "де-темплейтизируй", "убери шаблонную мету", "сделай из шаблона свой проект", или когда init-service дошёл до развёртывания (домены+стек спроектированы и stable). Движок skill-only: и механика, и wiring — reviewable edits под этой процедурой. НЕ запускать на каноническом репозитории шаблона.
---

# Skill: de-template (движок развёртывания)

Превращает форк шаблона в проект, который ничего не помнит про `go-backend-template`. Читает
**stable** `docs/{domains,stack}` и разворачивает фундамент по секциям `Generates` восьми
stack-артефактов. Движок **skill-only**: нет committed-скрипта — и детерминированная механика
(rewrite/prune/env), и doc-driven wiring идут как **reviewable edits** под этой процедурой;
механика — по точным командам ниже, wiring — диффом поверх скелета, который юзер ревьюит.

Запускается **один раз** на форке. Финал — зелёный `make build` и проект без следов шаблона.

## Когда применять

- "/de-template" / "разверни фундамент" / "сделай из шаблона свой проект"
- `init-service` дошёл до шага 4 (домены и стек спроектированы, валидаторы зелёные)

## Когда НЕ применять

- Домены/стек ещё не `stable` — сначала `domain-deep-dive` + `stack-deep-dive` (gate ниже).
- Проект уже де-темплейтизирован (свой module path, reference-домены удалены) — обычный
  день-в-день через `/smartdev`.
- **Канонический репозиторий шаблона** — никогда (guard ниже). Только форк/копия.

## Gate (проверить ДО любых правок)

1. **Не канонический шаблон.** `go.mod` module path всё ещё `git.stit.tech/stit-infra/go-backend-template`
   И каталог — не временная копия/форк → СТОП. Запуск на самом шаблоне его уничтожит. Спроси
   юзера явно подтвердить, что это форк/тест-копия, прежде чем продолжать.
2. **Артефакты stable.** Оба зелёные, иначе СТОП → `domain-deep-dive`/`stack-deep-dive`:
   ```bash
   scripts/tools/validate-domain-doc.sh docs/domains
   scripts/tools/validate-stack-doc.sh  docs/stack
   ```
   Все артефакты `status: stable`, секции `Generates` непусты — это вход движка.

## Phase A — детерминированная механика

Делается по точным командам; результат предсказуем. Каждый шаг — отдельный reviewable дифф.

### A.1 Module path

Новый путь — из `docs/stack/service.md § Generates` (поле module path). Перепиши **везде**
(go.mod + все импорты) одной заменой:

```bash
OLD=git.stit.tech/stit-infra/go-backend-template
NEW=<module path из docs/stack/service.md>
git grep -lF "$OLD" | xargs sed -i "s#$OLD#$NEW#g"
go mod tidy
```

Проверь `head -1 go.mod` == `module $NEW` и `go build ./...` (импорты резолвятся).

### A.2 README-шапка

Из `docs/stack/service.md` (имя, назначение, бинари): замени шаблонную шапку `README.md` на
проектную. Убери упоминания «шаблон»/«template»/«форкни». Детали release/observability/ecosystem
остаются ссылками на `docs/reference/*` (они общие).

### A.3 `.env.example`

Из `docs/stack/config.md § Generates` (required+optional env с дефолтами): приведи `.env.example`
к фактическому контракту проекта. Секреты — только имена ключей (значения не коммить;
`.claude/rules/env-local.md`).

### A.4 Strip шаблонной меты

- Удали `docs/getting-started.md` ([META] — гайд «как форкнуть шаблон», форку не нужен).
- `docs/contributing.md` ([META] про устройство `.claude`-системы) — **оставь** (система
  наследуется форком); при желании переформулируй «шаблон» → имя проекта.
- Сбрось `.claude/plans/roadmap.md` к пустому fork-start (history шаблона — не история форка):
  оставь шапку + пустые `## Active` / `## Backlog`.

### A.5 Prune reference-доменов

`auth`/`user`/`verification` — учебные. Удаляются по полному чеклисту (7 точек) —
см. **`references/prune.md`**. Делай это **после** того как свои домены спроектированы и
сгенерированы (иначе теряешь живой образец). Если свои домены ещё не скаффолжены — сначала
`scaffold` (вариант domain) по stable-артефактам, потом prune.

## Phase B — doc-driven wiring

Шесть оставшихся `Generates` (runtime/messaging/observability/auth/persistence/dependencies)
разворачиваются как **reviewable diff поверх скелета** — `cmd/api/main.go` композиция, fx-модули,
telemetry, subjects, authgate, pgkit, kit-prune. Процедура и маппинг `Generates`→код —
см. **`references/wiring.md`**. Не правь сгенерированное (`infra/postgres/db/*`, `mocks/*`) —
только `make gen`.

## Verify

```bash
make build      # фундамент компилируется
make check      # перед claim «готово»: fmt+vet+lint+test+build+gen-check
```

Зелёный `make build` на развёрнутом форке — главный критерий движка. Тест-обвязка для
воспроизводимой проверки — `test/de-template/` (harness ставит tmp-копию + ассертит build).

## Anti-patterns

- ❌ Запуск на каноническом шаблоне (см. Gate) — уничтожает шаблон.
- ❌ Старт до `stable`-артефактов — движку не из чего разворачивать.
- ❌ Prune reference-доменов раньше, чем свои спроектированы/сгенерированы.
- ❌ Wiring «по памяти» вместо `Generates`-секций — артефакты source of truth.
- ❌ Правка сгенерированного руками вместо `make gen`.
- ❌ Коммит значений секретов в `.env.example` (только имена ключей).

## См. также

- `references/prune.md` — удаление reference-доменов (7 точек).
- `references/wiring.md` — doc-driven wiring 6 `Generates`-kinds.
- `docs/stack/README.md` — контракт 8 артефактов и их `Generates`.
- `.claude/skills/init-service/SKILL.md` — оркестратор, чейнит сюда последним шагом.
