---
name: de-template
description: Развернуть платформу форка go-backend-template по stable-артефактам docs/stack — module path, README, .env.example, план и handoff с чистого листа, wiring platform и cmd. Используй, когда init-service дошёл до развёртывания (стек спроектирован и stable) или юзер просит превратить форк в свой проект и убрать шаблонную мету. Домены не трогает: свои — scaffold, домены идентичности — prune init-service. Движок skill-only: и механика, и wiring — reviewable edits под этой процедурой. На каноническом репозитории шаблона не запускать — это его уничтожит.
---

# Skill: de-template (движок развёртывания)

Превращает форк шаблона в проект, который ничего не помнит про `go-backend-template`, кроме версии
обвязки `.template/` — по ней приходят обновления (`/upgrade`). Читает **stable** `docs/stack` и
разворачивает платформу по секциям `Generates` восьми stack-артефактов. Домены — не здесь: свои
разворачивает `scaffold` (вариант domain), домены идентичности остаются или уходят по решению
`docs/stack/auth.md` (prune — этап init-service после своих доменов). Движок **skill-only**: механика —
по точным командам ниже, wiring — диффом поверх скелета, который юзер ревьюит.

Запускается **один раз** на форке, этапом онбординга (`/init-service`). Финал — зелёный `make check`.

## Когда применять

- Юзер просит развернуть фундамент форка / сделать из шаблона свой проект
- `init-service` дошёл до развёртывания (стек спроектирован, валидатор зелёный)

## Когда НЕ применять

- Стек ещё не `stable` — сначала `stack-deep-dive` (gate ниже).
- Проект уже де-темплейтизирован (свой module path) — обычный день-в-день через `/smartdev`.
- **Канонический репозиторий шаблона** — никогда (gate ниже). Только форк.

## Gate (проверить ДО любых правок)

1. **Не канонический шаблон.** Origin — репо шаблона (без учёта регистра) → СТОП: процедура его
   уничтожит.
   ```bash
   git config --get remote.origin.url | grep -qiE '[:/]stit-core/go-backend-template(\.git)?/?$' \
     && echo "STOP: origin — канонический шаблон; de-template — только в форке"
   ```
   Гейт держит то же: в каноническом репо `make check` красный на чужом module path и артефактах стека
   `docs/stack/<kind>.md` (archtest `TestCanonicalTemplate`), pre-push такое не отправит.
2. **Стек stable** — все 8 артефактов на месте, `status: stable`, валидатор зелёный; иначе СТОП →
   `stack-deep-dive`:
   ```bash
   make validate-stack-docs A=--stable
   ```
3. **State stack-deep-dive чист** по `**blocker**`-вопросам: артефакт мог попасть в `stable` только без
   них (§5/§6 `.claude/skills/stack-deep-dive/references/draft-stable.md`), но защёлка — state коммитится
   вместе с репо.
   ```bash
   grep -RlE '\*\*blocker\*\*' .claude/skills/stack-deep-dive/state/ 2>/dev/null \
     && echo "STOP: есть открытые blocker'ы в state stack-deep-dive — добей их сначала" \
     || echo "OK: state чист"
   ```

## Phase A — детерминированная механика

Делается по точным командам; результат предсказуем. Каждый шаг — отдельный коммит: gen-check сверяет
моки с индексом, и правка A.1 в моках без коммита — ложное «моки устарели».

### A.1 Module path

Новый путь — из `docs/stack/service.md § Generates`. Меняется в коде и конфигах генерации — `go.mod`,
`*.go`, `.mockery.yml`; проза его не меняет: там это путь шаблона (откуда форк, что не так у
неперенастроенного форка), и перенос обвязки (`make template-update`) правит module path тоже только в
Go-файлах.

```bash
OLD=git.stit.tech/stit-core/go-backend-template
NEW=<module path из docs/stack/service.md>
git grep -lF "$OLD" -- go.mod '*.go' .mockery.yml | xargs sed -i "s#$OLD#$NEW#g"
go mod tidy
make fmt
```

`make fmt` — порядок импортов: путь, который сортируется после `git.stit.tech/stit-core/golibinfra`
(`git.stit.tech/zeta/shop`, `github.com/…`), встаёт в группе после него. Проверь `head -1 go.mod` ==
`module $NEW` и `go build ./...` (импорты резолвятся).

### A.2 README и CONTRIBUTING

Из `docs/stack/service.md` (имя, назначение, бинари): шапка `README.md` — проектная, раздел «Как начать»
шаблона (форк → `/init-service`) — вон; заголовок `CONTRIBUTING.md` — имя проекта; `SECURITY.md` — политика
и адрес для отчётов об уязвимостях проекта, а не шаблона. Убери «шаблон»/«template»/«форкни». Детали release/ecosystem/architecture остаются ссылками на
`docs/reference/*` (они общие). `CLAUDE.md` и доки шаблона (`.template/paths`) не правь: они верны и для
форка, а правку форка перенос обвязки показал бы конфликтом.

### A.3 `.env.example`

Из `docs/stack/config.md § Generates` (required+optional env с дефолтами): приведи `.env.example`
к фактическому контракту проекта. Секреты — только имена ключей (значения не коммить;
`.claude/rules/env-local.md`). Boot-тест и e2e-стенд берут env из него.

При `identity: external` домены идентичности до prune (этап 4 `/init-service`) ещё в графе, и их
строки остаются: стримы `user` и `verification` в `NATS_JS_STREAM_NAMES` (без них старт падает: «no
stream covers subjects: user.>»), `SESSION_*`, `BRUTEFORCE_SUBJECT_*`. Снимает их шаг «Окружение»
`.claude/skills/init-service/references/prune.md`.

### A.4 Планы и handoff

`.claude/plans/` — форк стартует без планов и handoff шаблона: только `backlog.md` и `handoff.md` с
`_пусто_` (init-service делает это на этапе 0, до своего плана); план онбординга (`onboarding`,
`/init-service`) остаётся. `.template/` — **оставь**: версия
обвязки, пути шаблона и доменов идентичности.

## Phase B — doc-driven wiring

Шесть оставшихся `Generates` (runtime/messaging/observability/auth/persistence/dependencies)
разворачиваются как **reviewable diff поверх скелета** — `cmd/api/main.go` композиция, fx-модули,
logging, subjects, authgate, pgkit, киты. Процедура и маппинг `Generates`→код — `references/wiring.md`.
Не правь сгенерированное (`infra/postgres/db/*`, `mocks/*`) — только `make gen`.

## Verify

```bash
make check      # готово — по CLAUDE.md § Готово
```

## Anti-patterns

- ❌ Запуск на каноническом шаблоне (см. Gate) — уничтожает шаблон.
- ❌ Старт до `stable`-артефактов — движку не из чего разворачивать.
- ❌ Module path в прозе: там он — путь шаблона.
- ❌ Удаление доменов здесь — свои растут через `scaffold`, домены идентичности — решение `auth.md`, prune
  после своих доменов (`/init-service`).
- ❌ Wiring «по памяти» вместо `Generates`-секций — артефакты source of truth.
- ❌ Правка сгенерированного руками вместо `make gen`.
- ❌ Коммит значений секретов в `.env.example` (только имена ключей).

## См. также

- `references/wiring.md` — doc-driven wiring 6 `Generates`-kinds.
- `docs/stack/README.md` — контракт 8 артефактов и их `Generates`.
- `.claude/skills/init-service/SKILL.md` — оркестратор онбординга; prune доменов идентичности —
  `.claude/skills/init-service/references/prune.md`.
