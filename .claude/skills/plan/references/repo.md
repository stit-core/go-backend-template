# Профиль репо для /plan — go-backend-template

- **Роль:** сервис — Go-бэкенд (`<org>/<репо>`, первый сервис орги — `backend`). Работа приходит фазой
  программы из хаба `<org>/infra` (`parent: infra:<slug>`) или прямым заходом. Итог — образы `<репо>` и
  `<репо>-migrate` в registry (путь из origin, `/release`); выкатка тега — только из хаба
  (`make bump SVC=<репо>`).
- **Гейт перед «готово»:** CLAUDE.md § Готово (`make check` и ревью).
- **Проверка планов:** `make plan-check`; pre-commit проверяет, когда меняется `.claude/plans/`.
- **Сводка планов:** `make status` → `== Plan ==` (или `make plan-check A=--summary`).
- **Коммит плана без кода:** `meta(plan): …` — типы коммитов держит хук `commit-msg`
  (CLAUDE.md § Git workflow).
- **Типовые проверки шагов:** `make test PKG=./internal/domain/<d>/...` → PASS · `make gen-check` →
  без дрейфа · `make migrate-up` и `make migrate-down` → применяется и откатывается · `make e2e` →
  PASS (ручку сквозь HTTP держат handler-тест и boot-тест `cmd/api`) · `make check` → зелёный.
- **Handoff:** в хаб `<org>/infra` — INFRA-HANDOFF (skill `/infra-handoff`; что считается инфра-поверхностью —
  таблица rule `infra-impact`). Готовый блок — в накопитель `.claude/plans/handoff.md`: его
  печатает и очищает `make release`; раздел `## Handoff` плана — заметки до блока, при закрытии
  плана недоставленное уходит в накопитель.
- **Разбор падений:** skill `/triage-failure` — сначала падающий тест.
- **До плана выясни** — только то, от чего зависят шаги; дизайн-решения — через `AskUserQuestion`:
  - скоуп и что не входит; связь с другими планами и программой хаба;
  - записи домена в `<org>/strategy` (`docs/projects/<slug>/services/<svc>/domains/<ctx>/`): stable
    ли то, от чего зависит задача;
  - API: меняется ли существующий контракт и не breaking ли он; форма ответа и коды ошибок;
    идемпотентность; пагинация, фильтры, сортировка;
  - схема БД: миграция (skill `/scaffold`, вариант migration), совместимость со старым форматом,
    backfill, индексы, FK, cascade;
  - cross-domain: какой домен владеет логикой, нужны ли порты и адаптеры (пропустит ли archtest);
  - безопасность: кто вправе вызывать (policy), чувствительные данные в логах и outbox,
    audit-событие;
  - интеграции: outbox-событие и его потребитель, внешние API и ключ идемпотентности;
  - тесты: на каком слое TDD, нужен ли интеграционный тест, fuzz, bench;
  - выкатка и откат: feature-flag, поэтапно или сразу, откатывает ли down-миграция;
  - инфра-поверхность (→ `## Handoff`) — по таблице rule `infra-impact`.
