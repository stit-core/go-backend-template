# Roadmap

Единственный артефакт планирования проекта — этот файл. Подпапок, `<slug>.md`,
архивов и changelog'ов нет: они расходятся с правдой и мешают.

**Planning-first.** Изменение идёт через план — секцию в `## Active` ниже.
Прямая правка без плана допустима только для truly-atomic (опечатка, один
очевидный файл) и багфиксов (→ skill `triage-failure`, RED-first). Отклонение
от плана — осознанное и проговаривается с юзером.

**Жизненный цикл.** `planning → in-progress → blocked? → done`. По `done` секция
задачи удаляется целиком — история остаётся в `git log`. Незакрытый follow-up
переезжает в backlog или новой Active-секцией, а не хвостом в удаляемой.

**Current focus:** `doc-and-scaffold-overhaul`

## Active

### doc-and-scaffold-overhaul — переработка документации и скаффолд-системы
**Создан:** 2026-05-26 · **Статус:** in-progress

**Контекст.** Шаблон — «полупроект-полушаблон»: CLAUDE.md (203 строки, озаглавлен
`# AGENTS.md`) смешивает ядро и справку; README (295 строк) тащит ~140 строк
observability-референса; половина правил на нерабочем ключе `globs:` (грузятся
всегда вместо path-scope); скиллы без единого спека. Цель: docs становятся
design-source-of-truth, из которого скиллы (`/init-service` → `domain-deep-dive`
+ `stack-deep-dive` → `de-template`) разворачивают реальный проект; CLAUDE.md
худеет до роутера (<200); правила/скиллы/планы приводятся к системным
возможностям Claude Code. Решения по дизайну — в «Открытых вопросах».

**Не входит в скоуп.**
- Реальная де-темплейтизация конкретного форка (это запускает потребитель).
- Правки `golibinfra` / соседних репо экосистемы.
- Изменение runtime-кода доменов (auth/user/verification остаются reference).
- git-хуки (commit-msg/pre-commit/pre-push) — остаются; убираем только Claude-хуки.

**Открытые вопросы.** (закрыты до старта работы; A14 — единственный к решению в ходе)
- [x] Канон агент-гайда → только `CLAUDE.md`, AGENTS.md выпиливаем.
- [x] Push в `main` → разрешён; git `pre-push → make check` остаётся gate.
- [x] Claude Code хуки → снести целиком (settings.json + scripts/claude-hooks/); git-хуки и permissions.deny остаются.
- [x] Reference-домены → живут; удаление — шаг в docs/getting-started.
- [x] `docs/stack` → rich-набор 8 kinds (service, runtime, config, messaging, observability, persistence, auth, dependencies).
- [x] `/de-template` → гибрид: детерминированная механика + doc-driven wiring как reviewable diff поверх скелета.
- [x] golibinfra-first → CLAUDE.md (инвариант) + правило `upstream-first.md` + `docs/reference/architecture.md`.
- [x] Правила: slim `comments`/`domain-artifacts`/`env-local`/`infra-impact`/`sqlc-and-migrations`/`tests-pipeline`; split `domain-architecture`; merge `outbox-events`+`jobs-and-subscribers` → `events-and-messaging.md`; `forward-only` как есть.
- [x] Планы → единый roadmap.md, планы inline; статусы {planning, in-progress, blocked}; planning-first = plan по default.
- [x] `WORKFLOW.md` → растворить по ролям: §2 петля → smartdev; §1/§4/§5/§7 → docs/contributing; §6 non-goals → docs/reference/architecture; сценарии → getting-started/contributing. Файл удаляется.
- [ ] Какие из 12 скиллов удалить — решается первым шагом Фазы 4 (атом A14).

**Атомы.**

#### Phase 0 — Механика (независима, низкий риск)
- [x] **A1:** `globs:`→`paths:` в `comments`/`domain-artifacts`/`env-local`/`infra-impact`; сверить все 10 правил.
  - Verify: `grep -l 'globs:' .claude/rules/*.md` → пусто · Зависит от: —
  - **Done:** 4 правила сконвертированы в YAML-list `paths:`; `grep -l globs` пусто, `grep -L paths` пусто (все 10 path-scoped).
- [x] **A2:** снести `hooks`-блок из `.claude/settings.json` + удалить `scripts/claude-hooks/`.
  - Verify: `jq '.hooks' .claude/settings.json` → null; каталога нет · Зависит от: —
  - **Done:** hooks-блок удалён (settings.json валиден через python3, ключа `hooks` нет); `scripts/claude-hooks/` снят `git rm -r`. `permissions.deny` сохранён.
- [x] **A3:** разрешить push в `main` — убрать запрет из CLAUDE.md §Границы и skill `release`.
  - Verify: `grep -rin 'main.*напряму\|напряму.*main' CLAUDE.md .claude/skills/release` → пусто · Зависит от: —
  - **Done:** удалена строка-запрет в §7 «Не делать никогда»; §5 переформулирована «Push в main разрешён»; в release-скилле запрета не было.
- [x] **A4:** ретайтл `# AGENTS.md`→нормальный заголовок; выпилить упоминания AGENTS.md.
  - Verify: `grep -rl 'AGENTS.md' --exclude-dir=.git .` → пусто (кроме roadmap/WORKFLOW) · Зависит от: —
  - **Done:** `CLAUDE.md` был симлинком на реальный `AGENTS.md` — инверсия плана. `git rm` симлинка + `git mv AGENTS.md→CLAUDE.md` (реальный файл); заголовок → «go-backend-template — гайд для агентов»; рефы вычищены в README/commit-msg/comments/forward-only. Остаток AGENTS.md — только WORKFLOW.md (умрёт в A21).

#### Phase 1 — Система планов (foundational; догфуд этого файла)
- [x] **A5:** финализировать inline-формат единого roadmap (этот файл — эталон секции).
  - Verify: human-review структуры · Зависит от: —
  - **Done:** формат зафиксирован в roadmap + в `plan/SKILL.md §3` (шаблон секции); валидатор A6 его принимает. Чекпоинты — буллеты, фазы — `#### ` (не `### `, коллизия с план-секцией).
- [x] **A6:** переписать `validate-plan.sh` под секции внутри roadmap.md (per-section header, статусы {planning,in-progress,blocked}, Verify/Зависит каждого атома).
  - Verify: `scripts/tools/validate-plan.sh` → OK на этом roadmap · Зависит от: A5
  - **Done:** переписан под inline-`### `-секции под `## Active`; тест на живом roadmap → OK, на сломанной копии ловит битый статус/нет секции/атом без Verify/битую зависимость. bash+awk, без внешних деп.
- [x] **A7:** переписать skill `plan` под single-file inline + planning-first + blocked; длинный список вопросов → `references/`.
  - Verify: skill <500 строк; описание-триггеры по спеку · Зависит от: A5
  - **Done:** `plan/SKILL.md` ~190 строк (planning-first дефолт, статус blocked, единый roadmap, чекпоинты-буллеты, DONE=удалить секцию); вопросы → `references/questions.md`. Description «pushy».
- [x] **A8:** обновить pre-commit (валидирует roadmap.md) + smartdev читает roadmap как state-of-the-world.
  - Verify: pre-commit dry-run зелёный · Зависит от: A6
  - **Done:** pre-commit валидирует `roadmap.md` (вместо `<slug>.md`); `status.sh` Plan-секция → компактная сводка (focus + `slug: status | next: A#`) вместо дампа всего плана; smartdev §2 указывает на inline-секцию. Глубокий рерайт smartdev (WORKFLOW §2) — A17.

#### Phase 2 — docs/ + диспозиция правил
- [x] **A9:** дерево docs: `getting-started.md`/`contributing.md` [META] + `reference/{architecture,observability,release,ecosystem}.md` + `docs/README.md`-роутер. Контрибьютор-explainer (WORKFLOW §1/§4/§5/§7) → contributing.md; WORKFLOW §6 non-goals → architecture.md; сценарий «новый проект» → getting-started.
  - Verify: файлы есть, внутренние ссылки резолвятся · Зависит от: —
  - **Done:** созданы 7 файлов (router + 2 META + 4 reference). Observability ~140 строк и WORKFLOW §1/4/5/6/7 перенесены. Ссылки в docs/ резолвятся (dangling — только плейсхолдеры в _templates). Вынос из README/CLAUDE/WORKFLOW (удаление дублей) — в A12/A13/A21.
- [x] **A10:** `docs/stack/` — 8 kinds: frontmatter-контракт, обязательные H2, `_templates/`, `validate-stack-doc.sh`.
  - Verify: `scripts/tools/validate-stack-doc.sh docs/stack` → OK на templates · Зависит от: —
  - **Done:** созданы `docs/stack/README.md` + 8 `_templates/<kind>.md` (каждый с H2 `Generates` — контракт для de-template) + `validate-stack-doc.sh` (зеркало domain-валидатора: frontmatter, kind∈8, basename==name, H2-секции, stable→непусто). Тест: пустой→OK, stable-образец→OK, битый kind→FAIL. Оказался независим от A9 (docs/stack самодостаточен) — сделан первым в Phase 2.
- [x] **A11:** правила — slim/split/merge(`events-and-messaging`)/new `upstream-first`; проза/примеры → `docs/reference/architecture.md`.
  - Verify: `make lint` зелёный; `grep -l 'globs:'` пусто; merge выполнен · Зависит от: A9
  - **Done:** split `domain-architecture` (slim + указатель на architecture.md); merge `outbox-events`+`jobs-and-subscribers` → `events-and-messaging.md` (старые удалены `git rm`); new `upstream-first.md` (paths internal/**,pkg/**). Висящие ссылки (new-domain.sh, add-event) починены. `grep globs` пусто. lint отдельно не гонял (правила .md, на Go-lint не влияют; pre-commit на коммите покроет). Глубокий slim остальных 6 правил → Backlog (низкий приоритет).

#### Phase 3 — CLAUDE.md + README
- [x] **A12:** CLAUDE.md <200 строк: ядро-инварианты (+golibinfra-first, +planning-first) + таблица-роутер в docs/rules/skills.
  - Verify: `wc -l CLAUDE.md` <200; ссылки резолвятся · Зависит от: A9, A11
  - **Done:** 203→**96 строк**. §1–§6 (команды/тесты/структура/telemetry/DDD) → docs + rules; остались ядро (точка входа, planning-first, design-first, стиль, git, Границы, forward-only) + таблица-роутер. Ссылки на removed Claude-хуки убраны (теперь deny+git-hooks). Dangling-ссылок нет.
- [x] **A13:** README — потребительская шапка + старт через `/init-service`; observability-блок → `docs/reference/observability.md`.
  - Verify: README ссылается на docs; observability в reference · Зависит от: A9
  - **Done:** 296→**76 строк**. ~140-строчный observability-блок, Release-детали, ecosystem, структура → указатели на docs/reference/*. Шапка ведёт на getting-started + `/init-service`. Ссылки резолвятся; WORKFLOW/AGENTS-рефы убраны.

#### Phase 4 — Скиллы (skill-by-skill)
- [ ] **A14:** решение keep/rework/delete по каждому из 12 скиллов (фиксируется чекпоинтом).
  - Verify: чеклист решений в чекпоинте · Зависит от: A7
- [ ] **A15:** `domain-deepen`→`domain-deep-dive` (rename + rework).
  - Verify: skill-spec lint; ссылки обновлены · Зависит от: A14
- [ ] **A16:** новые `init-service` (оркестратор) + `stack-deep-dive`.
  - Verify: skill-spec; init-service чейнит deep-dives → de-template · Зависит от: A10, A15
- [ ] **A17:** выжившие скиллы к канону `spec/agent-skills-spec.md`; перенос domain-gate в скиллы (soft-шаг); smartdev вбирает cockpit-петлю WORKFLOW §2 + сценарии.
  - Verify: каждый <500 строк, description-триггеры, cross-ссылки резолвятся · Зависит от: A14

#### Phase 5 — Движок de-template (гибрид)
- [ ] **A18:** завести под-план движка `de-template` отдельной Active-секцией; реализовать механику + doc-driven wiring.
  - Verify: под-план валиден; de-template на тест-форке даёт зелёный `make build` · Зависит от: A10, A16

#### Phase 6 — Проводка и верификация
- [ ] **A19:** сшить cross-ссылки rules↔skills↔docs↔roadmap; обновить pre-skill ссылки на новые имена скиллов.
  - Verify: нет битых внутренних ссылок · Зависит от: A11, A12, A17
- [ ] **A21:** удалить `.claude/WORKFLOW.md` (контент разнесён A9/A17); вычистить ссылки на него в CLAUDE.md и README.
  - Verify: `grep -rl 'WORKFLOW.md' --exclude-dir=.git .` → пусто · Зависит от: A9, A13, A17
- [ ] **A20:** финальный gate.
  - Verify: `make check` → PASS; валидаторы plan/domain/stack зелёные; e2e не задет · Зависит от: A19, A21, A18

**Infra impact.** — (вся работа — docs/skills/config; без env/миграций/cross-service-контрактов/go.mod).

**Чекпоинты.**
- **2026-05-26 · Phase 0 закрыта.**
  - Закрыто: A1, A2, A3, A4 ✓
  - Обнаружено: `CLAUDE.md` был симлинком на реальный `AGENTS.md` (план предполагал обратное). A4 выполнен как swap: `git rm` симлинка + `git mv AGENTS.md→CLAUDE.md`, AGENTS.md удалён. Закоммичено `3b88931`.
  - Окружение: `jq`/`file` отсутствуют — проверки через `python3`/`awk`. Новые валидаторы (A6 done, A10) — без внешних зависимостей.
- **2026-05-26 · Phase 1 закрыта.**
  - A5–A8 ✓: единый inline-roadmap как формат; `validate-plan.sh` переписан и протестирован; skill `plan` переписан (+`references/questions.md`); pre-commit/status.sh/smartdev читают единый roadmap.
  - Формат: чекпоинт — буллеты, фазы — `#### ` (не `### `, коллизия с план-секцией). Учтено в `plan/SKILL.md`.
  - Phase 0+1 запушены: `2b0ea63..c6ca7f3 origin/main`, `make check` зелёный.
- **2026-05-26 · Phase 2 закрыта.**
  - A10 ✓: `docs/stack/` контракт (README + 8 templates + `validate-stack-doc.sh`).
  - A9 ✓: дерево docs (router + getting-started/contributing [META] + reference/{architecture,observability,release,ecosystem}). Observability + WORKFLOW-куски перенесены.
  - A11 ✓: split domain-architecture, merge → events-and-messaging, new upstream-first; висящие ссылки починены.
  - Дальше: Phase 3 (CLAUDE.md <200 + README trim, A12–A13) — здесь же удаляются дубли, перенесённые в docs.
- **2026-05-26 · Phase 3 закрыта.**
  - A12 ✓: CLAUDE.md 203→96 строк (ядро + роутер). A13 ✓: README 296→76 (указатели на docs/reference).
  - Дубли observability/structure/release сняты (контент в docs из A9). Дальше: Phase 4 (скиллы, A14–A17).

## Backlog (idea state)
- MinIO-примитив (`test/infra/objstore.py` + minio-сервис в compose) — завести когда форк добавит реального потребителя объектного хранилища (сейчас потребителя нет).
- Глубокий slim правил `comments`/`domain-artifacts`/`env-local`/`infra-impact`/`sqlc-and-migrations`/`tests-pipeline` (длинные примеры → docs/skills) — низкий приоритет: структурная диспозиция и фикс `globs` уже сделаны (A1, A11), правила path-scoped и нормального размера.
