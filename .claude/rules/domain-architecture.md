---
name: domain-architecture
description: Слои и границы бизнес-доменов internal/domain/. Terse-инварианты; «как и почему» — docs/reference/architecture.md. Грузится при работе с internal/domain/.
paths:
  - "internal/domain/**"
---

# Архитектура домена

Полное описание слоёв, ports/adapters, HTTP DTO naming, событий — `docs/reference/architecture.md`.
Здесь — что обязан соблюдать при правке `internal/domain/`.

## Инварианты

- **Раскладка по слоям.** `entity.go` (типы, без I/O) · `dto.go` · `errors.go` (sentinel) ·
  `policy.go` (решения без I/O) · `usecase.go` (логика, зависимости через порты-интерфейсы) ·
  `events.go` (outbox payload) · `metrics.go` (bizmetric-счётчики домена, nil-safe `Record*`) ·
  `delivery/{http,nats}/` · `infra/postgres/` · `module/`.
- **Cross-domain — только порты + адаптеры.** depguard блокирует прямые импорты между
  доменами. Порт объявляешь в домене-потребителе (`usecase.go`), адаптер — в
  `<consumer>/module/adapters.go` (он вправе импортить домен-провайдер). DTO в порте — локальные.
- **HTTP DTO — domain-specific имена.** huma собирает все DTO в одну OpenAPI registry по
  имени типа; `ListOutput` в двух доменах → panic. Префиксуй: `EventListOutput`, не `ListOutput`.
- **I/O — это use-case, не метод агрегата.** Нужен repo/сеть/время → метод `*Usecase`.
  Чистое правило → `policy.go`.
- **Логи — `logkit.Logger`.** Не `fmt.Print*` / `slog.*` напрямую (forbidigo).
- Pure-методы агрегата покрыты `*_test.go`; интерфейсы из `usecase.go` → `.mockery.yml` → `make gen`.

Outbox / subscribers / jobs — `.claude/rules/events-and-messaging.md`. Миграции / sqlc —
`.claude/rules/sqlc-and-migrations.md`. Переиспользуемые примитивы → golibinfra (CLAUDE.md § Стиль кода).
