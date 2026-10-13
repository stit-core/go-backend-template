---
name: tests-pipeline
description: Инварианты тестов, моков, фикстур, TDD. Грузится при работе с тестами или testkit. Процедура добавления моков/доменных тестов — skill scaffold; e2e-харнес — test/README.md.
paths:
  - "**/*_test.go"
  - "internal/testkit/**"
  - ".mockery.yml"
---

# Тесты

Процедура (добавить мок для интерфейса, тесты при скаффолде блока) — skill `scaffold`.
E2E/smoke-харнес (pytest, side-effect-ассерты) — `test/README.md`. Здесь — инварианты.

## Тест проверяет поведение

- Утверждай результат, ошибку (`require.ErrorIs`) и эффекты: вызовы портов с точными
  аргументами, запись в БД, событие (`outboxtest.Recorder`), исход метрики (провайдер теста
  `metrictest.NewProvider`, значение — `require.InDelta`). `NotNil` ради покрытия — не тест:
  тест без утверждений в любом пакете модуля ловит archtest `TestTestsAssert`, пустышку — `/code-review`.
- Рядом с happy path — error path и граница (пусто, максимум, чужой id).
- Красный тест не подгоняй под код: сначала реши, кто прав; меняешь ожидание — причина в теле
  коммита.

## Структура и стиль

- `<file>_test.go` рядом с `<file>.go`. Package `<pkg>_test` (black-box); `<pkg>`
  (white-box) — только для приватных функций, редко.
- **Ассерты — `require`** (`stretchr/testify/require`) + `handlertest.Assert*` в
  handler-тестах. `assert` и голые `t.Fatalf`-сравнения не используем.
- **Таблица — slice анонимных struct + `t.Run(tc.name)`**. Сценарии одной функции, различающиеся
  данными → table-driven `Test<Method>`; существенно разный arrange (другие моки/фикстуры) или
  одиночный сценарий → `Test<Method>_<Scenario>`. Переиспользуемая таблица семейства методов —
  named type + shared var допустимы.
- **`t.Parallel()`** — в каждом юнит-тесте и сабтесте; в integration — в каждом тесте (у теста свой
  контейнер), его сабтесты делят БД и идут подряд. Fuzz — без.
- Сообщения ошибок в тестах — английский.
- Часы: `clockkit.NewFixed(time.Date(...))` с литеральной датой — не `NewFixed(time.Now())`.

## Что обязано иметь тест (минимум слоя)

| Слой | Тест |
|---|---|
| `policy.go` | unit table-driven |
| `entity.go` (конструкторы/методы) | unit; +fuzz (`entity_fuzz_test.go`) при строковых валидациях |
| `usecase.go` | unit через строгие моки портов и настоящую policy, fixture `newUC(t, …)` |
| `delivery/http` | handler-тест через `golibinfra/testkit/handlertest` (операции под `Security` — `handlertest.WithSecuritySchemes(authgate.CookieScheme(identitytest.Extract))`, личность — ctx запроса): happy path каждой операции — точный вход usecase и тело ответа; `errors_test.go` (white-box): таблица операции объявляет ошибки её usecase (список — по коду usecase); без 500 и без таблицы операции вне `Declare` — archtest `TestDomainErrorTables` |
| `delivery/nats` | unit handler-функции (мок Usecase-порта) — включая дубль-доставку; контракт с поставщиком — `contract_test.go` (rule `events-and-messaging`) |
| `module/adapters.go` | unit (white-box) поверх `Trusted` поставщика: маппинг полей, ctx вызывающего до `Trusted` (запись и чтение под блокировкой — в его транзакции) и каждая ошибка `TrustedErrors` → sentinel потребителя |
| `infra/<kit>/` | unit на настоящем ките или фейке из `testkit` |
| `trusted.go` | unit через моки портов; методы отдают только ошибки `TrustedErrors` (`return` в `trusted.go` — archtest) |
| `infra/postgres/repository.go` | integration (build-tag) на каждый метод: запись видна чтением, условия SQL (версия, удалённые, регистр, фильтр, страница); битый id — sentinel «нет такого» (id не от клиента — ошибка), сбой БД (отменённый ctx) — ошибка с причиной, не sentinel домена |
| fx-граф | `cmd/api/main_test.go` `fx.ValidateApp`, корню графа `context.Context` не доступен; `fx.As` нет — archtest |
| HTTP API | `cmd/api/api_test.go`: API проходит проверку старта httpkit (`Security` или `httpkit.Public`, уникальный метод+путь), операция без пометки — ошибка старта, пути операций доменов — под `/api/`, открыты ровно `publicOperations` (открыть операцию — правка списка), под `Security` — коды 401 сессии; `openapi.json` совпадает с кодом |
| подкоманды `main` | `cmd/api/healthcheck_test.go`: тестовый бинарь, перезапущенный как `api healthcheck` (`TestMain` зовёт `main`), — код выхода по `/health` на `HTTP_ADDR`, сервис не стартует; `cmd/api/main_test.go`: тот же бинарь как `api` без окружения — отказ старта в логе сервиса (JSON, ERROR с причиной), а не в консоли fx |
| запуск сервиса | `cmd/api/boot_test.go` (integration): граф `main` на контейнерах с env из `.env.example` — старт, проверка здоровья (`httpkit.CheckHealth`) получает 200 от `/health` сервиса, `/ready` называет каждую зависимость сервиса (пробы Postgres и NATS), с остановкой сверх бюджета старт падает, 401 у каждой операции под `Security`, `/openapi.json` = `openapi.json`, сырых маршрутов нет, kit-таблицы графа = источники `cmd/migrate`, HTTP стартует последним и гасится первым, глобальный провайдер метрик — провайдер telemetry |
| команда миграций | `cmd/migrate/run_test.go` (integration): `up` накатывает источники китов и каталог приложения, блокировку таблицы ждёт не дольше `PG_MIGRATE_LOCK_TIMEOUT`; отказ оставляет источник dirty на версии, до которой шёл, — `version` отвечает кодом 1 с его именем, `up` дальше не идёт; со снятым флагом `up` считает миграцию применённой, ремонт — прежняя версия без флага; `down` — одна миграция приложения, источники китов не трогает |
| домены идентичности | `cmd/api/identity_test.go` (integration): сквозь HTTP — регистрация, вход, 429 антибрутфорса при любом X-Forwarded-For, событие до подписчика, outbox verification — до стрима, без стрима публикатора старт падает, 5xx без причины (она — в логе под тем же `request_id`), обе ссылки из писем гасятся, адрес, пароли, cookie сессии и токены писем — ни в логе сервиса, ни в `audit_log` и outbox, метрики доменов — в провайдер telemetry; сессии: ротация, повтор токена и выход закрывают сессию (выход — с одной записью аудита, её владельца), смена и сброс пароля, блокировка и удаление — все сессии пользователя, вход старым паролем в гонке со сменой сессии не получает; `cmd/seed-admin/main_test.go` — повышение закрывает прежние сессии той же транзакцией (сбой закрытия откатывает повышение) |

E2E (`test/`, `make e2e`) — сквозные потоки на собранном стеке: вход и сессии, событие между
доменами. Новой ручке хватает handler-теста, сценарий e2e — когда она меняет такой поток. `make e2e`
в гейт не входит, `make e2e-smoke` гоняет `make release`.

## TDD для багфиксов — обязательно

RED→GREEN: падающий тест, воспроизводящий баг → фикс → зелёный → оба файла одним коммитом.
Тест должен падать по той причине, что и баг. Ведёт это skill `triage-failure`.

## Моки и фикстуры (mockery v3)

- Конфиг — `.mockery.yml`: интерфейсы из `usecase.go` и `delivery/*` (Usecase-порты), кроме
  policy-порта `Authorizer` — у него мока нет, тесты берут настоящую policy, — и портов с
  рукописным фейком (ниже).
  Файлы: `mocks/<interface_snake>.go` (пакет `mocks`, тип `Mock<Interface>`). Регенерация —
  `make gen`; `make gen-check` ловит дрейф и untracked-моки.
- **Изоляция (archtest)**: пакет `mocks` любого домена импортируется только из `*_test.go`.
- Template `testify` даёт `EXPECT()`-API; `t` в конструкторе — авто-`AssertExpectations`.
- **Fixture usecase-теста** — `newUC(t, …)`: порты — строгие моки без ожиданий (вызов, которого
  кейс не ждёт, — провал), policy — настоящая (`NewPolicy()`): право решает identity кейса.
  Ожидание побочного эффекта — с точными аргументами и `.Once()`, без `.Maybe()`; запись в
  транзакции — `mock.MatchedBy(pgkit.InTx)`; событие — `Recorder.Last(subject)`, лишних нет —
  `Count()`. Порт кейс передаёт параметром, несколько портов — functional options `withX(...)`.
- Рукописный фейк вместо мока — у протокольного порта (stateful: цепочки insert/markUsed) и у
  чистого порта без эффектов (выпуск access-токена `auth.Tokens`: ответ выводится из входа, тест сверяет
  его в результате — строгий мок закреплял бы факт и число вызовов на каждом пути, а не результат);
  тогда интерфейс из `.mockery.yml` убирается — мок и фейк вместе не живут.
- Identity-контексты — `internal/testkit/identitytest` (`Admin`/`Client`/`Anon`), не
  inline `identity.With(...)`: это ctx запроса с IP клиента и ID (`identitytest.IP`, `RequestID`),
  с ними тест сверяет аудит, guard входа и лимитер.

## Интеграционные

- Build-tag `//go:build integration` сверху файла; гоняются в `make check` (шаг `make test-int`).
  Docker обязателен: вне `-short` testkit без Docker роняет тест, а не пропускает его.
- Другого ограничения сборки в репо нет: только `//go:build integration` и только у `*_test.go`, без
  суффиксов ОС и архитектуры в имени (archtest `TestBuildConstraints`) — иначе файл выпал бы из vet,
  lint, тестов и покрытия гейта или из образа; шаг build собирает `./...` без тегов, как образ.
- Контейнер+миграции+пул — `golibinfra/testkit.StartPostgres(t, testkit.WithMigrationsDir(path))`
  (нужна kit-таблица → ещё `testkit.WithMigrations(<kit>pg.Migrations)`);
  канон — `internal/domain/user/infra/postgres/repository_test.go`. Seed-хелперы строк
  поверх — `internal/testkit/pgseed`.

## Coverage

`make cover-check` (шаг `make check`, pre-push) — профиль одного прогона `make test-int` (юнит +
integration, `-coverpkg`) по `internal/` и `pkg/`: сгенерированное (заголовок `Code generated … DO NOT
EDIT.` — вывод sqlc, mockery и клиенты `pkg/`; в другом месте заголовок не пускает archtest) и тестовая
обвязка `internal/testkit/` вне scope, fx-провайдеры засчитывает boot-тест `cmd/api`. Порог — факт, 100%:
красный, если хоть один блок не исполнен или функцию тесты не исполнили (пустое тело — исполнено ли оно);
отказ называет блоки и функции (все блоки — `make cover-check FORMAT=detailed`). Блок, который тесты не
исполнят по построению, — запись списка непокрываемого `scripts/tools/cover.sh` с причиной: пополнить его —
ослабить гейт (CLAUDE.md § Сначала спросить), запись без неисполненного блока — отказ. Покрытие —
осмысленными тестами: силу держат мутации (ниже), а не процент.

## Мутации

`make mutate` (вне гейта, нужен Docker): правки «было → стало» из `scripts/tools/mutants.txt` — по тексту
кода, без номеров строк, — по одной на чистом дереве, затем сборка, юнит- и integration-тесты; итог —
убитые и выжившие списком, код 1 при новых выживших. Выжившая — пробел теста: тест, который её убивает,
или, если поломку тесты не увидят по построению, строка «=» с причиной в её блоке (известная
эквивалентная); строка «=» и снятый блок ослабляют проверку (CLAUDE.md § Сначала спросить). Мутация, чей
текст «было» правка кода убрала, — в отчёте: её блок переписывают под код (`make mutate A=--check` —
только якоря). Оборванный прогон на том же коммите продолжается с места.

## Anti-patterns

- ❌ Мокать `*sql.DB`/`*pgxpool.Pool` напрямую — мокай интерфейсы домена (Repository).
- ❌ Тесты на сгенерированный sqlc-код — тестируй методы Repository.
- ❌ `time.Now()` в коде — инжекть `clockkit.Clock` (детерминизм). `context.Background()` в проде — только корень процесса в `cmd/*`, не `internal/`.
- ❌ Закомментированные тесты — удаляй или фикси.
- ❌ Мок и рукописный фейк одного интерфейса одновременно.
