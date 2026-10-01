---
name: de-template
description: Развернуть форк go-backend-template в самостоятельный проект по stable-артефактам docs/stack (домены — в соседнем strategy-репо) — module path, README, .env.example, platform/cmd wiring, prune reference-доменов. Используй, когда init-service дошёл до развёртывания (стек спроектирован и stable) или юзер просит превратить форк в свой проект и убрать шаблонную мету. Движок skill-only: и механика, и wiring — reviewable edits под этой процедурой. На каноническом репозитории шаблона не запускать — это его уничтожит.
---

# Skill: de-template (движок развёртывания)

Превращает форк шаблона в проект, который ничего не помнит про `go-backend-template`. Читает
**stable** `docs/stack` и разворачивает фундамент по секциям `Generates` восьми
stack-артефактов. Домены живут в соседнем strategy-репо и разворачиваются отдельно (`scaffold`
domain), не здесь. Движок **skill-only**: нет committed-скрипта — и детерминированная механика
(rewrite/prune/env), и doc-driven wiring идут как **reviewable edits** под этой процедурой;
механика — по точным командам ниже, wiring — диффом поверх скелета, который юзер ревьюит.

Запускается **один раз** на форке. Финал — зелёный `make build` и проект без следов шаблона.

## Когда применять

- Юзер просит развернуть фундамент форка / сделать из шаблона свой проект
- `init-service` дошёл до развёртывания (стек спроектирован, валидатор зелёный; домены — в strategy-репо)

## Когда НЕ применять

- Стек ещё не `stable` — сначала `stack-deep-dive` (gate ниже). Домены — в соседнем strategy-репо.
- Проект уже де-темплейтизирован (свой module path, reference-домены удалены) — обычный
  день-в-день через `/smartdev`.
- **Канонический репозиторий шаблона** — никогда (guard ниже). Только форк/копия.

## Gate (проверить ДО любых правок)

1. **Не канонический шаблон.** `go.mod` module path всё ещё `git.stit.tech/stit-core/go-backend-template`
   И каталог — не временная копия/форк → СТОП. Запуск на самом шаблоне его уничтожит. Спроси
   юзера явно подтвердить, что это форк/тест-копия, прежде чем продолжать.
2. **Стек stable.** Валидатор зелёный, иначе СТОП → `stack-deep-dive`.
   Сначала проверь, что 8 stack-kind'ов существуют — на пустом `docs/stack` (только
   `_templates/`+README) валидатор даёт ложный `OK`, ему нечего проверять:
   ```bash
   for k in service runtime config persistence messaging auth observability dependencies; do
     test -f "docs/stack/$k.md" || echo "STOP: docs/stack/$k.md нет — стек не спроектирован (/stack-deep-dive)"
   done
   make validate-stack-docs
   ```
   Все 8 артефактов `status: stable`, секции `Generates` непусты — это вход движка. Домены
   (соседний strategy-репо) для de-template не нужны: движок разворачивает только платформу,
   спроектированные домены скаффолдятся отдельно (`scaffold` domain) до prune reference (Phase A.5).
3. **State stack-deep-dive чист.** Открытые `**blocker**`-вопросы у `stack-deep-dive`
   означают незакрытые платформенные решения — артефакт мог попасть в `stable` только если
   blocker'ов нет (§5/§6 его `references/draft-stable.md`), но защёлка: пробежаться по
   state-каталогу (коммитится вместе с репо) и убедиться.
   ```bash
   grep -RlE '\*\*blocker\*\*' .claude/skills/stack-deep-dive/state/ 2>/dev/null \
     && echo "STOP: есть открытые blocker'ы в state stack-deep-dive — добей их сначала" \
     || echo "OK: state чист"
   ```

## Phase A — детерминированная механика

Делается по точным командам; результат предсказуем. Каждый шаг — отдельный reviewable дифф.

### A.1 Module path

Новый путь — из `docs/stack/service.md § Generates` (поле module path). Перепиши **везде**
(go.mod + все импорты) одной заменой:

```bash
OLD=git.stit.tech/stit-core/go-backend-template
NEW=<module path из docs/stack/service.md>
git grep -lF "$OLD" | xargs sed -i "s#$OLD#$NEW#g"
go mod tidy
```

Проверь `head -1 go.mod` == `module $NEW` и `go build ./...` (импорты резолвятся).

### A.2 README-шапка

Из `docs/stack/service.md` (имя, назначение, бинари): замени шаблонную шапку `README.md` на
проектную. Убери упоминания «шаблон»/«template»/«форкни». Детали release/ecosystem/architecture
остаются ссылками на `docs/reference/*` (они общие).

### A.3 `.env.example`

Из `docs/stack/config.md § Generates` (required+optional env с дефолтами): приведи `.env.example`
к фактическому контракту проекта. Секреты — только имена ключей (значения не коммить;
`.claude/rules/env-local.md`).

### A.4 Strip шаблонной меты

- Удали `docs/getting-started.md` ([META] — гайд «как форкнуть шаблон», форку не нужен).
- `docs/contributing.md` ([META] про устройство `.claude`-системы) — **оставь** (система
  наследуется форком); при желании переформулируй «шаблон» → имя проекта.
- `.claude/plans/` — форк стартует без планов и handoff шаблона: только `backlog.md` и
  `handoff.md` с `_пусто_` (планы шаблона — не планы форка).

### A.5 Prune reference-доменов

`auth`/`user`/`verification` — учебные. Удаляются по полному чеклисту (7 точек) —
см. **`references/prune.md`**. Делай это **после** того как свои домены спроектированы (в
соседнем strategy-репо) и сгенерированы (иначе теряешь живой образец). Если свои домены ещё не
скаффолжены — сначала они, по таблице «Маршрут задачи» (CLAUDE.md), потом prune.

## Phase B — doc-driven wiring

Шесть оставшихся `Generates` (runtime/messaging/observability/auth/persistence/dependencies)
разворачиваются как **reviewable diff поверх скелета** — `cmd/api/main.go` композиция, fx-модули,
logging, subjects, authgate, pgkit, kit-prune. Процедура и маппинг `Generates`→код —
см. **`references/wiring.md`**. Не правь сгенерированное (`infra/postgres/db/*`, `mocks/*`) —
только `make gen`.

## Verify

```bash
make build      # фундамент компилируется
make check      # готово — по CLAUDE.md § Готово
```

Зелёный `make build` на развёрнутом форке — главный критерий движка.

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
