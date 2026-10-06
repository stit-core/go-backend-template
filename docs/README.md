# Документация

Роутер по docs. Три аудитории + design source-of-truth.

## Потребителю шаблона
- `README.md` § Как начать — форк и `make setup`; дальше онбординг ведёт `/init-service`: стек → `/de-template` → свои домены → домены идентичности.

## Контрибьютору шаблона
- [contributing.md](contributing.md) — как устроена `.claude`-система (rules/skills/settings), что enforce'ится, версия обвязки и теги шаблона, как вмешаться, troubleshooting. **[META]**.

## Reference (переезжает в проект)
- [reference/architecture.md](reference/architecture.md) — слои домена, ports/adapters, fx, контракт ошибок API, outbox, non-goals, граница с golibinfra.
- [reference/domain-mapping.md](reference/domain-mapping.md) — как DDD-артефакт (из соседнего strategy-репо) ложится в Go-код этого репо; потребляет `scaffold`.
- [reference/release.md](reference/release.md) — релиз образов, handoff в infra, откат, версионирование.
- [reference/ecosystem.md](reference/ecosystem.md) — роль репо и соседи, как сюда приходят агенты (из хаба `<org>/infra` или прямым заходом), что репо отдаёт и берёт, контракт с `<org>/infra`.

## Design source-of-truth
- **Домены** — в **соседнем strategy-репо** орга (`../strategy`, форк `business-strategy-template`): `docs/projects/<slug>/services/<svc>/domains/`, внутри сервиса, который реализует этот репо. Проектирование — скиллом `/domain-deep-dive` там; маппинг в Go-код — [reference/domain-mapping.md](reference/domain-mapping.md).
- [stack/README.md](stack/README.md) — артефакты платформы/стека (8 kinds, вход для `/de-template`). Остаётся в этом репо.

Точка входа агента в репо — `CLAUDE.md` (ядро + роутер). Дневной флоу — skill `/smartdev`.
