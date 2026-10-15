# scaffold › job — периодическая задача домена

**Шаг 0** — прочитай инструментом Read `.claude/rules/events-and-messaging.md`,
`.claude/rules/domain-architecture.md` и `.claude/rules/tests-pipeline.md`.

Периодика без события-триггера (очистка своих строк, сверка, напоминание по расписанию) — `timerkit.Job`
домена: отдельного worker-бинаря нет, job гоняет `timerkit.Module` в `cmd/api`. Реакция на событие — вариант
`event`; обслуживание kit-таблиц (сессии, антибрутфорс, outbox) уже в китах, свой job над ними не пишется.

## Шаги

1. **Работа — метод usecase** (`Purge<Что>(ctx) error`): вызывающего нет, policy его не спрашивает; время —
   `clockkit.Clock`, пачка — с пределом, повтор после сбоя безопасен (условие запроса, а не память процесса).
   Тест — unit через строгие моки портов: эффект, пустая пачка, ошибка порта.
2. **Запрос пачки** — `queries/<name>.sql` и метод `Repository`, integration-тест: условие, предел, порядок.
3. **Job — в модуле домена** `module/module.go`: конструктор `new<Что>Job(uc *<name>.Usecase) timerkit.Job`
   (`Name` — `<name>-<что>`, `Interval`, `Timeout` короче интервала, `Run: uc.Purge<Что>`) и
   `timerkit.ProvideJob(new<Что>Job)` в `fx.Module`; в корне домена и в `delivery/` job не живёт (`TestLayout`).
   Конструктор исполняет boot-тест `cmd/api` (граф сервиса стартует с job). Интервал и предел пачки, которые
   меняет окружение, — вариант `env`.

Готово и коммит — по CLAUDE.md § Готово. Коммит — `feat(<name>): <что> по расписанию`, в теле — строка `Review:`.
