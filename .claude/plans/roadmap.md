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

**Current focus:** `fix-audit-findings` (planning — ожидает approve)

## Active

### fix-audit-findings — устранение находок аудита (md + код + тулинг + флоу)
**Создан:** 2026-05-28 · **Статус:** planning

**Контекст.** Аудит репо (19-агентный workflow + ручная верификация) нашёл набор
дефектов на стыке доков/кода/тулинга. Гейты при этом зелёные (`make check` проходит:
build/vet/lint/test 120шт/gen-check), архитектура enforce'ится (depguard эмпирически
ловит cross-domain), MD-связность чистая. Чинимое — это **content drift и
рассинхрон слоёв**, а не сломанная сборка. План закрывает 6 quick-win'ов и 7
структурных правок. Решения по 4 форкам приняты юзером (см. Открытые вопросы).
Ключевые улики: `Makefile:64,69,133`, `scripts/new-domain.sh:336-344`,
`.claude/rules/tests-pipeline.md:38` + `.claude/agents/reviewer.md:64`,
`scripts/release.sh:70`, `.claude/skills/de-template/references/prune.md:13`,
`.claude/rules/forward-only.md:27,37`, `internal/domain/auth/module/adapters.go:65`,
`internal/domain/user/delivery/http/dto.go:43,49`,
`.claude/rules/sqlc-and-migrations.md:39`, `docs/domains/` (0 stable),
`docs/getting-started.md:36-50`, `docs/reference/{ecosystem,release}.md`.

**Не входит в скоуп.** Изменение поведения кода soft-delete (по решению — правим
ТОЛЬКО правило, код user-домена остаётся). Создание worked-примера в `docs/domains/`
(по решению — документируем code-only). Прочие medium/low из аудита уезжают в
`## Backlog` отдельными строками: scaffold-референсы зовут `scripts/tools/*.sh`
напрямую; gen-check не ловит untracked-моки; `make check` мутирует дерево
(`gofumpt -w`) на pre-push; `setup` глотает ошибку копии pre-commit; CI-floor 60%
без CI; `architecture.md` без карты platform/jobs/cmd + metrics.go; нет
Go-integration-тестов для auth/verification; `RequestPasswordReset` глотает все
ошибки; dead-код (`auth.LoggedIn`, `Set/ClearAuthCookies`, дубль `ForbiddenError`);
polish-нити (`dependencies.md:26` хвостовой `>`, latin-`a` в `comments.md:10`,
`Dockerfile:19` cmd/worker, `infra-impact.md:6` мёртвый glob, `MAPPING.md`-роутинг).

**Открытые вопросы.**
- [x] Q1: soft-delete — править код или правило? → **Обновить правило** (status-soft-delete оставить, код не трогаем). → A9
- [x] Q2: design-SoT — заполнить worked-пример или задокументировать? → **Задокументировать code-only**. → A10
- [x] Q3: имя образа — `<org>/backend` фикс или `<org>/<service>` из remote? → **`<org>/<service>`, release.sh выводит из remote**. → A12 (infra-impact)
- [x] Q4: reviewer-агент — manual-only или подключить? → **Подключить в smartdev §4 после GREEN**. → A13

**Атомы.**

#### Phase 1 — Quick wins (тулинг + doc-drift; независимы, низкий риск)
- [ ] **A1:** `Makefile` — убрать `mockery@latest`-fallback (строки 64,69 → падать с подсказкой `make setup`, как `lint.sh`); починить `test-fuzz` (строка 133 → `-fuzz=FuzzNewUser ... ./internal/domain/user/`).
  - Verify: `make test-fuzz` → фаззинг стартует (не «multiple packages»); `make gen-check` → green · Зависит от: —
- [ ] **A2:** `scripts/new-domain.sh:336-344` — переписать печатаемый depguard-сниппет под закоммиченный идиом (`list-mode: lax` + `!**/*_test.go` + `!**/internal/domain/<name>/module/**` + deny по конкретным sibling-доменам, не по umbrella-префиксу `internal/domain`).
  - Verify: сгенерить тестовый домен, вставить сниппет, `make lint` → 0 issues на собственных импортах домена · Зависит от: —
- [ ] **A3:** Фантом `pgseed.New(t)` → `.claude/rules/tests-pipeline.md:38` и `.claude/agents/reviewer.md:64` переписать на `golibinfra/testkit.StartPostgres(t, migrationsPath)`; уточнить роль `pgseed` (per-entity seed-хелперы поверх).
  - Verify: `grep -rn 'pgseed.New(t)' .claude` → пусто; ссылка на `repository_test.go:20` как канон · Зависит от: —
- [ ] **A4:** `scripts/release.sh:70` — выровнять агрегацию infra-impact под реальный формат: `**Infra impact.**` (bold inline в `### `-секциях) и единый `.claude/plans/roadmap.md` (не `*.md`-glob, не `## Infra impact`-H2).
  - Verify: временно добавить `**Infra impact.**`-строку в тест-секцию roadmap → агрегация ловит её (не печатает пустой fallback) · Зависит от: —
- [ ] **A5:** `.claude/skills/de-template/references/prune.md` — переписать инструкцию по миграциям под реальность: `0001_users.*` = user+verification (`users`/`password_resets`/`email_verifications`), `0002_auth.*` = `refresh_sessions`; частичный prune = EDIT файла, не delete; исправить неверные имена таблиц (`sessions`/`verification_*`).
  - Verify: ручная сверка с `migrations/*.sql` — все имена таблиц и связки совпадают · Зависит от: —
- [ ] **A6:** `.claude/rules/forward-only.md:27,37` — переписать секцию планов под актуальную модель: единый `roadmap.md`, `### <slug>` под `## Active` (англ.), удаление секции по done; убрать упоминания per-slug `<slug>.md` и `## Активные`.
  - Verify: `grep -nE 'slug>\.md|Активные' .claude/rules/forward-only.md` → пусто; текст согласован с `plan/SKILL.md:9-10` · Зависит от: —

#### Phase 2 — Code reconcile (reference-домены под собственные правила)
- [ ] **A7:** Инъекция clock в адаптеры: `auth/module/adapters.go:65` (`tokensAdapter` — добавить `clockkit.Clock`-поле, прокинуть в `module.go:23`, `clock.Now()` вместо `time.Now()`); `verification/module/adapters.go:56,66` (`logMailerAdapter` — то же).
  - Verify: `make test PKG=./internal/domain/auth/... ./internal/domain/verification/...` → PASS; `grep -rn 'time.Now()' internal/domain/*/module/adapters.go` → пусто · Зависит от: —
- [ ] **A8:** Префикс HTTP DTO: `user/delivery/http/dto.go:43,49` `ListInput/ListOutput` → `UserListInput/UserListOutput` (+ обновить ссылки в хендлере/usecase); сверить остальные дженерик-имена на коллизии.
  - Verify: `make build` → OK; `make test PKG=./internal/domain/user/...` → PASS · Зависит от: —

#### Phase 3 — Structural (флоу + контракты)
- [ ] **A9:** soft-delete — обновить `.claude/rules/sqlc-and-migrations.md:39`: зафиксировать status-based soft-delete как принятый паттерн user-домена (WHY: partial unique index `WHERE status != 'deleted'`, аудит-история). Код НЕ трогаем.
  - Verify: текст правила согласован с `user/usecase.go:275` + `0001_users.up.sql:21` (нет противоречия) · Зависит от: —
- [ ] **A10:** Design-SoT — задокументировать code-only: в `docs/domains/README.md` и `docs/getting-started.md` явно «reference-домены auth/user/verification поставляются code-only by design (без `docs/domains/`-артефактов)»; в `prune.md` пункт 2 → «(reference-домены без docs/domains-артефактов — skip)».
  - Verify: delete-list в `prune.md`/`getting-started.md`/`init-service/SKILL.md` больше не ссылается на несуществующие `docs/domains/{auth,user,verification}/` · Зависит от: —
- [ ] **A11:** Lifecycle — вставить явный шаг «Scaffold спроектированных доменов → `/scaffold` (вариант domain) per stable artifact» МЕЖДУ de-template и удалением reference в `docs/getting-started.md` и `.claude/skills/init-service/SKILL.md`; зафиксировать инвариант «scaffold своих доменов ДО prune reference».
  - Verify: оба файла содержат scaffold-шаг перед шагом удаления reference; инвариант порядка явный · Зависит от: A10
- [ ] **A12:** Image naming — `scripts/release.sh` выводит repo-сегмент из `git remote` (вместо хардкода `/backend`): `<host>/<org>/<service>{,-migrate}`; правка `docs/reference/ecosystem.md`/`docs/reference/release.md`/`docs/getting-started.md`/`README.md` на единый `<org>/<service>`.
  - Verify: `bash scripts/release.sh` (preflight/dry) на тест-remote → image-path = `<host>/<org>/<service>`; semver-гейт цел · Зависит от: —
- [ ] **A13:** reviewer wiring — добавить `name: reviewer`/`name: explorer` во frontmatter `.claude/agents/*.md`; добавить шаг делегирования `reviewer` в `.claude/skills/smartdev/SKILL.md §4` (после GREEN) + кросс-ссылку из `scaffold`/`triage-failure`.
  - Verify: `grep -n '^name:' .claude/agents/reviewer.md .claude/agents/explorer.md` → есть; smartdev §4 содержит шаг reviewer · Зависит от: —

**Infra impact.**
- **A12 — image-path derivation (cross-service contract).** WHAT: `release.sh` перестаёт хардкодить repo-сегмент `backend`, выводит его из `git remote` → образ `<host>/<org>/<service>{,-migrate}`. WHY: `getting-started`/`README` обещают форк `<org>/<service>`, а образ собирался как `<org>/backend` — рассинхрон image-path между build (этот репо) и pull (deploy-репо `<org>/infra`). ACTION: владельцу `<org>/infra` — сверить deploy-конфиги/Vault-пути на новый image-path `<org>/<service>` вместо `<org>/backend`. WHO: infra-админ. DECIDE: до `make release` следующей версии. RISK: deploy тянет несуществующий тег, если image-path в build и infra разойдутся.

**Чекпоинты.**
- —

## Backlog (idea state)
- Description-оптимизация (run_loop из skill-creator) для новых/слитых скиллов (`init-service`, `domain-deep-dive`, `stack-deep-dive`, `scaffold`, `de-template`) — эмпирический тюнинг триггеров через `claude -p`. Опционально: descriptions уже канон-pushy; запускать отдельным заходом (тяжёлый), приоритетно `scaffold` (одно description на 5 интентов).
- MinIO-примитив (`test/infra/objstore.py` + minio-сервис в compose) — завести когда форк добавит реального потребителя объектного хранилища (сейчас потребителя нет).
