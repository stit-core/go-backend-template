# Вклад в go-backend-template

Это форкаемый шаблон Go HTTP-бэкенда на стеке stit. Вклад полезен, когда он
делает шаблон чище, надёжнее или удобнее для того, кто разворачивает из него свой
сервис — не когда он зашивает доменную специфику конкретного проекта.

## С чего начать

- Прочитай `CLAUDE.md` (ядро + роутер) и `docs/README.md` (карта документации);
  архитектура слоёв — `docs/reference/architecture.md`.
- Поставь dev-tools и git-хуки: `make setup`. Прогон gate'а — `make check`.
- Reference-домены `auth`/`user`/`verification` — учебные примеры (изучить, потом
  удалить под свой проект через `/de-template`); правки в них уместны только если
  чинят сам референс.

## Принципы

- **Проектирование первично, код вторичен.** DDD-артефакты доменов живут в соседнем
  strategy-репо, stack-артефакты — в `docs/stack/`. Код пишется по ним, не наоборот.
- **Не городить велосипеды.** Переиспользуемый/инфра-примитив (generic util,
  HTTP-client, middleware, любой «kit») идёт в `golibinfra`, не в локальную
  полукалеку. Граница — `docs/reference/architecture.md § Граница с golibinfra`.
- **Cross-domain — только порты + адаптеры.** Бизнес-домены не импортят друг друга
  напрямую (depguard enforce'ит). Логи — только `logkit.Logger`, не `fmt.Print*`/`slog.*`.
- **Без вендор-лока.** Эндпоинты control-plane (registry, OTLP, Vault) конфигурируются
  через env/Vault, не зашиты — шаблон работает и на managed, и на self-hosted контуре.
- **Forward-only документация.** Текст описывает текущее состояние, без «было/стало» —
  история живёт в `git log`.

## Форк, issue, PR

- **Issue** — для багов и предложений: что наблюдаешь, как воспроизвести, ожидаемое
  поведение.
- **PR** — форкни репозиторий, заведи ветку, открой pull request с понятным описанием
  «зачем». Держи PR сфокусированным; не подмешивай несвязанные правки.
- Тронул инфра-поверхность (новый required env/Vault-секрет; destructive/lock/backfill
  миграция; cross-service NATS/outbox-subject; breaking API; новая runtime-dep в `go.mod`)
  — опиши это в PR отдельно (в проекте это оформляется INFRA-HANDOFF-блоком инфра-админу).

## Перед PR

```bash
make check    # канонический gate: fmt + vet + lint + test + build + gen-check
```

Зелёный `make check` — gate пройден (его же гоняет git-хук `pre-push`).

- После правки `queries/`, `migrations/` или интерфейсов из `.mockery.yml` →
  `make gen` (sqlc + mockery). Сгенерированное руками не правим.
- **Conventional Commits** (хук `commit-msg`): `<type>(<scope>)?: <subject>`,
  subject ≤72. Типы: `feat`, `fix`, `refactor`, `docs`, `test`, `chore`, `meta`.
  Пример: `feat(user): add email change endpoint`.

## Стиль

Спокойно и по делу, без хайпа. **gofumpt** + **golangci-lint** обязательны.
Ошибки — `%w` + `errors.Is/As`, sentinel в `errors.go` корня домена. Комментарии —
только когда WHY неочевидно, не пересказ кода.

## Лицензия

Контрибьютя, ты соглашаешься, что вклад выходит под лицензией **Apache-2.0** —
той же, под которой распространяется репозиторий (см. `LICENSE`).
