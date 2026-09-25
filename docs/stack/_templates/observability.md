---
id: stack.observability
kind: observability
name: observability
status: draft
---

# Observability — логирование

Шаблон эмитит **структурные логи в stdout** через `golibinfra/logkit`. Сбор/хранение
логов (docker/host log-driver, внешний агрегатор) — зона деплой-инфры, не этого репо.
Бизнес-счётчики доменов (`biz_*`, `metrics.go`) пишутся в глобальный no-op `metric.Meter`:
MeterProvider в `cmd/api` не сконфигурирован, экспорта метрик нет.

## Log sink
<Куда идут логи: structured slog → stdout (`docker`/`kubectl logs`). Формат `LOG_FORMAT`
(json/text), уровень `LOG_LEVEL`, `LOG_SILENT` глушит stdout-sink; `trace_id`/`span_id` в
записи, если в ctx есть валидный span.>

## Log levels & content
<Уровни и что на каждом; какие события домена логируются (старт/шатдаун, ошибки usecase,
auth-события). Логи — только через `logkit.Logger`, не `fmt.Print*`/`slog.*`.>

## Generates
<Что de-template родит:
- logkit-wiring (logger в fx-графе, stdout-sink)
- уровни/формат из config-артефакта>
