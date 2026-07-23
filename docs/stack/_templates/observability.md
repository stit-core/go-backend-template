---
id: stack.observability
kind: observability
name: observability
status: draft
---

# Observability — логирование

Шаблон эмитит **структурные логи в stdout** через `golibinfra/logkit`. Сбор/хранение
логов (docker/host log-driver, внешний агрегатор) — зона деплой-инфры, не этого репо.
Корп-телеметрии (OTLP-экспорт, дашборды) в шаблоне нет.

## Log sink
<Куда идут логи: structured slog → stdout (`docker`/`kubectl logs`). Формат (json/text),
уровень по `APP_ENV`/`LOG_LEVEL`, `trace_id`/`span_id` в сообщении если включён контекст.>

## Log levels & content
<Уровни и что на каждом; какие события домена логируются (старт/шатдаун, ошибки usecase,
auth-события). Логи — только через `logkit.Logger`, не `fmt.Print*`/`slog.*`.>

## Generates
<Что de-template родит:
- logkit-wiring (logger в fx-графе, stdout-sink)
- уровни/формат из config-артефакта>
