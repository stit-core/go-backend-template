# Профиль репо для /plan — go-backend-template

- **Роль:** сервис — Go-бэкенд (`<org>/backend`). Работа приходит фазой программы из хаба
  `<org>/infra` (`parent: infra:<slug>`) или прямым заходом. Итог — образы `backend` и
  `backend-migrate` в registry (`/release`); выкатка тега — только из хаба (`make bump SVC=backend`).
- **Гейт перед «готово»:** `make check` (fmt + vet + lint + test + build + gen-check).
- **Проверка планов:** `make plan-check`; pre-commit проверяет, когда меняется `.claude/plans/`.
- **Сводка планов:** `make status` → `== Plan ==` (или `make plan-check A=--summary`).
- **Типовые проверки шагов:** `make test PKG=./internal/domain/<d>/...` → PASS · `make gen-check` →
  без дрейфа · `make migrate-up` и `make migrate-down` → применяется и откатывается · `make e2e` →
  PASS · `curl` к ручке → ожидаемый код · `make check` → зелёный.
- **Handoff:** в хаб `<org>/infra` — INFRA-HANDOFF (skill `/infra-handoff`, правило
  `infra-impact`): required env и секреты, миграции с эффектом (destructive, lock, backfill,
  extension), cross-service NATS/outbox-subject, новый порт или breaking API, новая runtime-зависимость
  в `go.mod`. Пишется в раздел `## Handoff` плана; `make release` собирает его из активных планов.
- **Разбор падений:** skill `/triage-failure` — сначала падающий тест.
- **До плана выясни** — только то, от чего зависят шаги; дизайн-решения — через `AskUserQuestion`:
  - скоуп и что не входит; связь с другими планами и программой хаба;
  - записи домена в `<org>/strategy` (`docs/projects/<slug>/services/<svc>/domains/<ctx>/`): stable
    ли то, от чего зависит задача;
  - API: меняется ли существующий контракт и не breaking ли он; форма ответа и коды ошибок;
    идемпотентность; пагинация, фильтры, сортировка;
  - схема БД: миграция (skill `/scaffold`, вариант migration), совместимость со старым форматом,
    backfill, индексы, FK, cascade;
  - cross-domain: какой домен владеет логикой, нужны ли порты и адаптеры (пропустит ли depguard);
  - безопасность: кто вправе вызывать (policy), чувствительные данные в логах и outbox,
    audit-событие;
  - интеграции: outbox-событие и его потребитель, внешние API и ключ идемпотентности;
  - тесты: на каком слое TDD, нужен ли интеграционный тест, fuzz, bench;
  - выкатка и откат: feature-flag, поэтапно или сразу, откатывает ли down-миграция;
  - инфра-поверхность (→ `## Handoff`): env, миграции с эффектом, subjects, порты, зависимости
    в `go.mod`.
