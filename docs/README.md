# Документация

Роутер по docs. Три аудитории + design source-of-truth.

## Потребителю шаблона
- [getting-started.md](getting-started.md) — пошаговый путь: форк → проектирование → `/de-template` → свой проект. **[META]** — удаляется при де-темплейтизации.

## Контрибьютору шаблона
- [contributing.md](contributing.md) — как устроена `.claude`-система (rules/skills/agents/settings), что enforce'ится, как вмешаться, troubleshooting. **[META]**.

## Reference (переезжает в проект)
- [reference/architecture.md](reference/architecture.md) — слои домена, ports/adapters, fx, outbox, non-goals, граница с golibinfra.
- [reference/domain-mapping.md](reference/domain-mapping.md) — как DDD-артефакт (из соседнего strategy-репо) ложится в Go-код этого репо; потребляет `scaffold`.
- [reference/observability.md](reference/observability.md) — метрики/логи/трейсы, что эмитится автоматически, Grafana.
- [reference/release.md](reference/release.md) — релиз образов, handoff в infra, откат, версионирование.
- [reference/ecosystem.md](reference/ecosystem.md) — карта экосистемы (шаблоны `stit-core` + корп control-plane + дизайн-система), tenant/handoff, контракт со stit-инфрой.

## Design source-of-truth
- **Домены** — опциональный продуктовый слой **соседнего strategy-репо** орга (`../strategy`, форк `business-strategy-template`): `docs/projects/<slug>/domains/`. Проектирование — скиллом `/domain-deep-dive` там; маппинг в Go-код — [reference/domain-mapping.md](reference/domain-mapping.md).
- [stack/README.md](stack/README.md) — артефакты платформы/стека (8 kinds, вход для `/de-template`). Остаётся в этом репо.

Точка входа агента в репо — `CLAUDE.md` (ядро + роутер). Дневной флоу — skill `/smartdev`.
