---
id: stack.observability
kind: observability
name: observability
status: draft
---

# Observability — логирование

Шаблон эмитит **структурные логи в stdout** через `golibinfra/logkit`. Сбор/хранение
логов (docker/host log-driver, внешний агрегатор) — зона деплой-инфры, не этого репо.
Метрики, трейсы и копию логов `golibinfra/telemetry` (`cmd/api`) отдаёт по OTLP в коллектор
`OTEL_EXPORTER_OTLP_ENDPOINT`; без коллектора — `OTEL_SDK_DISABLED=true`, сигналы не уходят, а
trace-context соседних сервисов идёт дальше как пришёл.
Бизнес-метрики доменов (`biz_*`, `metrics.go`) — только под алерт.

## Log sink
<Куда идут логи: structured slog → stdout (`docker`/`kubectl logs`). Формат `LOG_FORMAT`
(json/text), уровень `LOG_LEVEL`, `LOG_SILENT` глушит stdout-sink; `trace_id`/`span_id` в
записи, если в ctx есть валидный span: при `OTEL_SDK_DISABLED=true` — только у работы с доверенным
входящим trace-context (сообщение шины; HTTP — при `HTTP_TRUST_TRACE_CONTEXT=true`), сквозной
идентификатор — `request_id`.>

## Log levels & content
<Уровни и что на каждом; какие события домена логируются (старт/шатдаун, ошибки usecase,
auth-события). Логи — только через `logkit.Logger`, не `fmt.Print*`/`slog.*`.>

## Generates
<Что de-template родит:
- logkit-wiring (logger в fx-графе, stdout-sink)
- уровни/формат из config-артефакта>
