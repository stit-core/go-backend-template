# scaffold › env — env-переменная

**Шаг 0** — прочитай инструментом Read `.claude/rules/infra-impact.md`.

Новая настройка сервиса из окружения — от поля конфига до handoff. Настройки китов (`HTTP_*`, `PG_*`,
`NATS_*`, …) уже есть у китов — README кита § Конфигурация; нужной там нет — путь
`docs/reference/architecture.md § Граница с golibinfra`.

## Когда применять

- «Вынеси TTL ссылки сброса пароля в env»
- «Сервису нужен ключ API провайдера»

## Шаги

### 1. Контракт

Имя (`<ОБЛАСТЬ>_<ЧТО>`), тип, `default` или `required`, секрет ли, нужен ли запрет вне `APP_ENV=dev`.

### 2. Конфиг

Поле структуры `App` (`internal/platform/config`): `envconfig:"<ИМЯ>"` и `default:"…"` или
`required:"true"`; настройки одного пакета платформы — его вложенная структура (как
`authgate.CookieConfig`). Проверка значения сверх разбора envconfig (диапазон) — в `Load`: ошибка
старта, а не странное поведение. Env сервиса читает конфиг: `os.Getenv` — только у входов `cmd/`
вне графа (`migrate`, `seed-admin`), корень домена `os` не видит (archtest).

Потребитель получает узкое значение, а не `App`: пакет платформы — провайдером `config.Module`
(`func(a *App) Env { return a.Env }`); домен — своим типом `Config` в корне, его собирает из
`*config.App` модуль домена (`module/module.go`): платформа доменов не знает.

### 3. Гард

Значение, небезопасное вне dev (выключает защиту, открывает данные), — в `config.Guard` и строкой в
`TestGuard` (`config_test.go`).

### 4. Примеры окружения

- `.env.example`: required — строкой `<ИМЯ>=<значение для локального запуска>` в разделе Required
  (иначе красный `TestEnvExampleHasRequired`), секрет — значение только для разработки; optional —
  закомментированной строкой с дефолтом и смыслом.
- Required — ещё в env `TestBoot` (`serviceEnv` в `cmd/api/boot_test.go`) и в `x-app-env` e2e-стенда
  (`test/stack/compose.yml`): без неё сервис там не стартует.
- Форк — строка в `docs/stack/config.md` (Required или Optional, секрет — ещё § Secrets).

### 5. Handoff

Новый или изменённый required env, секрет, дефолт, который меняет поведение в проде, — блок
`/infra-handoff` в `.claude/plans/handoff.md` тем же коммитом (rule `infra-impact`). Optional, чей
дефолт сохраняет поведение, — не инфра.

### 6. Тесты

- `config_test.go` — `Load` читает значение и дефолт, неверное значение — ошибка; required без
  значения — ошибка;
- потребитель — тест на значение из конфига (у домена — `Config` в фикстуре `newUC`).

### 7. Готово и коммит

Готово — по CLAUDE.md § Готово. Коммит — `feat(config): <ИМЯ> — <что настраивает>`.

## Anti-patterns

- ❌ `os.Getenv` мимо конфига: такой env не видят ни `.env.example`, ни его проверка, ни handoff.
- ❌ `*config.App` целиком в зависимостях потребителя — только его значение.
- ❌ Значение настоящего секрета в `.env.example` или в блоке handoff — только имя ключа.
