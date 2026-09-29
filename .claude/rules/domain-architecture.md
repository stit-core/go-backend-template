---
name: domain-architecture
description: Слои и границы бизнес-доменов internal/domain/. Terse-инварианты; «как и почему» — docs/reference/architecture.md. Грузится при работе с internal/domain/.
paths:
  - "internal/domain/**"
---

# Архитектура домена

Полное описание слоёв, обязательности файлов, naming-канонов, ports/adapters, событий —
`docs/reference/architecture.md`. Здесь — что обязан соблюдать при правке `internal/domain/`.

## Инварианты

- **Раскладка по слоям.** `entity.go` (типы, без I/O) · `dto.go` (`<Op>Input/Output`) ·
  `errors.go` (все sentinel и error-типы домена — только здесь) · `policy.go` (authorization
  + чистые правила, без I/O) · `usecase.go` (логика, зависимости через порты) · `events.go`
  (payload + unexported subject-const) · `metrics.go` (`biz_<domain>_*`, nil-safe) ·
  `delivery/{http,nats}/` · `infra/postgres/` · `module/`. Файл существует только вместе со
  своим содержимым (таблица обязательности — architecture.md); мёртвые типы/файлы удаляются.
- **Cross-domain — только порты + адаптеры.** depguard (`<domain>-domain-isolation`) запрещает
  импортить что-либо вне своего поддерева `internal/domain/<domain>/`: deny префикса
  `internal/domain` + allow только своего поддерева → любой сосед, включая будущие, блокируется
  автоматически. Исключения — `module/` и `*_test.go`. Порт объявляешь в домене-потребителе
  (`usecase.go`), адаптер — в `<consumer>/module/adapters.go`. DTO в порте — локальные; порт
  возвращает **sentinel'ы потребителя** (адаптер мапит чужие) и **вычисленные свойства**
  (`Active bool`), не сырые чужие enum'ы. Identity-closure (`return uc`) — только без логики;
  любой маппинг → типовой адаптер, тип и конструктор в `adapters.go`. Clock в адаптерах
  инжектится (`clockkit.Clock`), `time.Now()` запрещён.
- **HTTP DTO — domain-specific имена.** huma собирает все DTO в одну OpenAPI registry по
  имени типа; `ListOutput` в двух доменах → panic. Префиксуй: `UserListOutput`, не `ListOutput`.
  Envelope — `data`. OperationID — lowerCamel `<verb><Noun>`, глобально уникален.
- **I/O — это use-case, не метод агрегата.** Нужен repo/сеть/время → метод `*Usecase`.
  Чистое правило/константа правила (password strength и т.п.) → `policy.go`, не inline в usecase.
- **Usecase не знает транспорта.** `httpkit`/`natskit`/huma в usecase запрещены; ctx-геттеры
  (`IPFrom`, `RequestIDFrom`) — только `internal/platform/identity`. Named returns не используем.
  Мутирующий метод инструментирован `metrics.Record*` на всех исходах; security-операции
  пишут audit из usecase.
- **Логи — `logkit.Logger`.** Не `fmt.Print*` / `log.*` / `print(ln)` / `slog.*` и не запись в
  `os.Stdout`/`os.Stderr` напрямую (forbidigo).
- Файл > ~400 строк → разбей по потокам (`usecase_<flow>.go`, `handler_<op>.go`), порты и
  конструкторы остаются в `usecase.go`.
- Pure-методы агрегата покрыты `*_test.go`; интерфейсы из `usecase.go` и `delivery/*` →
  `.mockery.yml` → `make gen`.

Outbox / subscribers / jobs — `.claude/rules/events-and-messaging.md`. Миграции / sqlc —
`.claude/rules/sqlc-and-migrations.md`. Переиспользуемые примитивы → golibinfra (CLAUDE.md § Стиль кода).
