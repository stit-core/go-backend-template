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
  аргументами, запись в БД, событие (`outboxtest.Recorder`). `NotNil` ради покрытия — не тест:
  тест без утверждений ловит archtest `TestTestsAssert`, пустышку — `/code-review`.
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
| `usecase.go` | unit через моки портов, fixture `newUC(t, opts...)` |
| `delivery/http` | handler-тест через `golibinfra/testkit/handlertest`; `errors_test.go` (white-box): таблица операции объявляет ошибки её usecase (список — по коду usecase); без 500 и без таблицы операции вне `Declare` — archtest `TestDomainErrorTables` |
| `delivery/nats` | unit handler-функции (мок Usecase-порта) — включая дубль-доставку |
| `module/adapters.go` | unit (white-box) поверх `Trusted` поставщика: маппинг полей и каждая ошибка `TrustedErrors` → sentinel потребителя |
| `infra/<kit>/` | unit на настоящем ките или фейке из `testkit` |
| `trusted.go` | unit через моки портов; методы отдают только ошибки `TrustedErrors` (`return` в `trusted.go` — archtest) |
| `infra/postgres/repository.go` | integration per-домен (build-tag) |
| fx-граф | `cmd/api/main_test.go` `fx.ValidateApp`, корню графа `context.Context` не доступен; `fx.As` нет — archtest |
| HTTP API | `cmd/api/api_test.go`: `Security` или `publicOperations`, уникальный метод+путь; `openapi.json` совпадает с кодом |
| запуск сервиса | `cmd/api/boot_test.go` (integration): граф `main` на контейнерах — старт, `api healthcheck` получает 200 от `/health` сервиса, 401 у каждой операции под `Security`, 429, 5xx без причины (она — в логе под тем же `request_id`), событие до подписчика, outbox verification — до стрима, `/openapi.json` = `openapi.json`, kit-таблицы графа = источники `cmd/migrate`, события fx — в лог сервиса, HTTP стартует последним и гасится первым; пароли, cookie сессии и токены писем в лог сервиса не попадают |

## TDD для багфиксов — обязательно

RED→GREEN: падающий тест, воспроизводящий баг → фикс → зелёный → оба файла одним коммитом.
Тест должен падать **по той причине**, что и баг. Ведёт это skill `triage-failure`.

## Моки и фикстуры (mockery v3)

- Конфиг — `.mockery.yml`: интерфейсы из `usecase.go` **и** `delivery/*` (Usecase-порты).
  Файлы: `mocks/<interface_snake>.go` (пакет `mocks`, тип `Mock<Interface>`). Регенерация —
  `make gen`; `make gen-check` ловит дрейф и untracked-моки.
- **Изоляция (archtest)**: пакет `mocks` любого домена импортируется только из `*_test.go`.
- Template `testify` даёт `EXPECT()`-API; `t` в конструкторе — авто-`AssertExpectations`.
- **Fixture usecase-теста — functional options**: `newUC(t, opts...)` + `withX(...)`;
  дефолтные моки — именованные хелперы (`defaultPolicy(t, ...)`). Policy-дефолт —
  **deny** (fail-closed); разрешение кейс включает явно.
- Рукописный stateful-фейк допустим для протокольных портов (цепочки insert/markUsed);
  тогда интерфейс из `.mockery.yml` убирается — мок и фейк вместе не живут.
- Identity-контексты — `internal/testkit/identitytest` (`Admin`/`Client`/`Anon`), не
  inline `identity.With(...)`.

## Интеграционные

- Build-tag `//go:build integration` сверху файла; гоняются в `make check` (шаг `make test-int`).
  Docker обязателен: без него `TestDockerRequired` роняет гейт, а не пропускает integration.
- Контейнер+миграции+пул — `golibinfra/testkit.StartPostgres(t, testkit.WithMigrationsDir(path))`
  (нужна kit-таблица → ещё `testkit.WithMigrations(<kit>pg.Migrations)`);
  канон — `internal/domain/user/infra/postgres/repository_test.go`. Seed-хелперы строк
  поверх — `internal/testkit/pgseed`.

## Coverage

`make cover-check` — юнит-профиль по `internal/{domain,platform}/...` без `/mocks/`, `infra/`,
`module/` и `platform/events`; **floor 75%**, ниже — красный. Входит в `make check` (pre-push).
Полный отчёт — `make test-cover`.

## Anti-patterns

- ❌ Мокать `*sql.DB`/`*pgxpool.Pool` напрямую — мокай интерфейсы домена (Repository).
- ❌ Тесты на сгенерированный sqlc-код — тестируй методы Repository.
- ❌ `time.Now()` в коде — инжекть `clockkit.Clock` (детерминизм). `context.Background()` в проде — только корень процесса в `cmd/*`, не `internal/`.
- ❌ Закомментированные тесты — удаляй или фикси.
- ❌ Мок и рукописный фейк одного интерфейса одновременно.
