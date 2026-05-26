---
id: stack.runtime
kind: runtime
name: runtime
status: draft
---

# Runtime — что крутится в cmd/api

## Entrypoints
<Какие «входы» поднимает один бинарь `cmd/api`: HTTP-сервер (huma+chi)? outbox-drain?
NATS-subscribers? background jobs? Отдельного worker-процесса нет — всё в одной
fx.Lifecycle.>

## Lifecycle
<fx-композиция: какие модули провайдятся, порядок старта, что регистрируется через
fx.Invoke. Какие jobs (`internal/jobs/`) и на каком интервале.>

## Shutdown
<Graceful shutdown: что закрывается на OnStop, таймауты, дренаж in-flight.>

## Generates
<Что de-template родит:
- композиция `cmd/api/main.go` (fx.New с нужными модулями)
- регистрация subscribers/jobs
- shutdown-хуки>
