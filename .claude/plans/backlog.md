# Backlog

Кандидаты в работу, ещё не взятые в план: `- **<id>** — что и зачем. Триггер: когда браться.` Форма и жизненный цикл — skill `/plan`.

- **golibinfra-httpkit-fx** — httpkit: хук HTTP-сервера регистрировать в `fx.Invoke` модуля
  (HTTP стартует последним, останавливается первым — FX-01); без `AuthExtractor` — ошибка
  старта, а не пропуск авторизации (FX-03); порядок middleware из группы детерминирован или
  описан в доке (FX-12). Триггер: следующий релиз golibinfra; после — снять allowlist FX-01.
- **golibinfra-timerkit-dep** — outboxkit явно требует timerkit, чтобы без `timerkit.Module` граф
  не собирался, а не outbox молча стоял (FX-04). Триггер: следующий релиз golibinfra.
- **golibinfra-fxlogger** — `logkit.FxLogger`: события fx в структурный лог, причина падения
  старта видна в JSON (FX-05). Триггер: следующий релиз golibinfra; после — убрать адаптер
  шаблона.
- **golibinfra-outbox-migrations** — `outboxkit.Instance` кладёт миграции таблицы в группу
  `pgkit.migrations`, как `pgstore.Module`; I/O конструкторов пула и NATS — под
  `fx.StartTimeout` (FX-12). Триггер: следующий релиз golibinfra; после — `cmd/migrate` берёт
  источники из графа.
