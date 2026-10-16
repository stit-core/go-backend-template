# init-service › prune — удалить домены идентичности

Только у проекта с внешней идентичностью: `docs/stack/auth.md` — `identity: external` (токены выпускает
другой сервис или IdP). При `identity: own` домены идентичности `auth`, `user`, `verification` — домены
проекта, prune не нужен. Шаг — после своих доменов (этап 4 скилла): без них сервису нечего
регистрировать.

## Шаги

1. **Пути доменов идентичности** — по списку шаблона, целиком (с ними уходят и тесты `cmd/`, которые есть
   только ради них):
   ```bash
   git rm -r -q $(grep -v '^#' .template/identity)
   ```
   `.template/identity` остаётся: по нему и решению `identity: external` самопроверка прозы знает, что
   образцы в этих путях — вне дерева проекта («Что остаётся»).
2. **Подключение** — строки удалённых пакетов и китов, которые были нужны только им (сессии входа,
   антибрутфорс, строгий лимит `/api/auth/*` перед очередью хэша паролей — `internal/platform/authlimit`):
   - `cmd/api/main.go` — `usermod`, `authmod`, `verifmod` (импорты, `domainModules`), `session.NewConfig`,
     `bruteforcekit.NewConfig` (`kitConfigs`), `sessionpg.Module`, `bruteforcepg.Module`;
   - `cmd/api/http.go` — `userhttp`, `authhttp`, `verifhttp` (импорты, поля `httpHandlers`, вызовы в
     `registerHTTP`), `authlimit.Middleware` в `middlewares` (и её параметр `app`), тест лимита в
     `cmd/api/boot_test.go` — `TestBoot_AuthRateLimit`;
   - `internal/platform/config/config.go` — поля `App.AuthRateLimit` и `App.PasswordResetMailPause` (и их проверки
     в `Load`), их тесты в `internal/platform/config/config_test.go` — `TestLoad_AuthRateLimit`,
     `TestLoad_PasswordResetMailPause`;
   - `cmd/migrate/sources/sources.go` — `useroutbox`, `verifoutbox`, `sessionpg`, `bruteforcepg`;
   - `cmd/api/api_test.go` — их открытые операции из `publicOperations`, проверка 429 лимита (`authlimit`) в
     `TestHTTPOperations`.
3. **Подключение по стеку** — то, что de-template оставил ради доменов идентичности (wiring §2 скилла
   `de-template`):
   - кит, которого нет в `docs/stack/dependencies.md § golibinfra kits` (аудит — `auditpg.Module`,
     `auditpg.RetentionModule` и `auditpg.NewRetentionConfig` в `cmd/api/main.go`, его job `auditkit.retention` в
     `TestBoot`, `auditpg.Migrations` в `cmd/migrate/sources/sources.go`), — вон из графа и из источников миграций;
   - своих паролей у проекта нет — хэшер (`authnkit.NewArgon2idConfig` в `kitConfigs`, Argon2id в
     `internal/platform/authgate/authn.go`, его тест `TestModule_Hasher`) и правило пароля `PasswordStrong` в
     `pkg/policy/policy.go` (тест `TestPasswordStrong`) — вон;
   - описание схемы Cookie (`CookieScheme` в `internal/platform/authgate/module.go`) — вход внешней
     идентичности вместо `POST /api/auth/login`;
   - импорты — `go mod tidy`.
4. **Генерация и мутации** — блоки удалённых доменов в `sqlc.yaml` и `.mockery.yml`. В спецификации мутаций
   `scripts/tools/mutants.txt` блоки удалённых файлов снимает первый фильтр, блоки строк, которые сняли шаги 2–3
   (их якорь `make mutate A=--check` называет устаревшим), — второй:
   ```bash
   awk -v RS= -v ORS='\n\n' '{ n = split($0, l, "\n"); for (i = 2; i <= n; i++) if (l[i] !~ /^[-+=#]/ && system("test -e \"" l[i] "\"")) next; print }' \
       scripts/tools/mutants.txt >mutants.tmp && mv mutants.tmp scripts/tools/mutants.txt
   make mutate A=--check | sed -n 's/^  \([^ :]*\): .*/\1/p' >stale.ids
   awk 'FNR == NR { stale[$0]; next } { split($0, l, "\n"); split(l[1], h, " ") } !(h[2] in stale) { print; print "" }' \
       stale.ids RS= scripts/tools/mutants.txt >mutants.tmp && mv mutants.tmp scripts/tools/mutants.txt && rm stale.ids
   ```
5. **Миграции** — свои — номерами подряд с `0001` (`git mv` с сохранением порядка: migrations-check
   ждёт нумерацию с 0001, прод-БД у проекта ещё нет). Расширение `pgcrypto` и функция `set_updated_at()`
   из `0001_users` уходят с ней: своя миграция, которой они нужны, заводит их сама.
6. **Окружение** — `.env.example`: в `NATS_JS_STREAM_NAMES` — стримы своих доменов-публикаторов (без
   `user` и `verification`; публикаторов нет — `NATS_JS_STREAM_NAMES=`), комментарий над ним — про них;
   у `JWT_SECRET` — ключ проверки токенов внешней идентичности; строки `SESSION_*`, `BRUTEFORCE_SUBJECT_*`,
   `HTTP_AUTH_RATE_LIMIT_*` и `PASSWORD_RESET_MAIL_PAUSE` (и `AUDIT_RETENTION_DAYS`, если аудит ушёл шагом 3) с их
   комментариями удали, у e2e-стека `test/stack/compose.yml` — `HTTP_AUTH_RATE_LIMIT_RPS`.
7. **Раскладка** — `internal/archtest/layout_settings_test.go`: запись
   `archkit.WithUncheckedPackages("internal/platform/identity")` убери, если корень ни одного своего
   домена не берёт `internal/platform/identity` (сам или через `pkg/`): лишняя запись — `TestLayout`
   красный с подсказкой `remove it`.
8. **Свои домены поверх идентичности** — файлы раздела ниже: FK на `users` — вон (пользователь — строка из
   токена внешней идентичности), порт к `user` — к её источнику или вон, подписка на события `user` и
   `verification` — вон, сид `pgseed` в integration-тестах — свои строки без `users`.
9. `make fmt` (импорты после правок шагов 2–3) → `make gen` → `make check` → `✓ check clean`;
   `make mutate A=--check` → OK.

## Свои домены поверх идентичности

Файлы проекта, которые берут домены идентичности: Go-файл, который знает их пакеты (адаптер порта к `user`,
подписчик их событий, integration-тест с сидом `pgseed`), — его здесь требует archtest
`TestPruneNamesIdentityDependents`; миграция с FK на `users` или с `set_updated_at()` из `0001_users`. Строка —
путь в обратных кавычках и что с ним делает шаг 8; раздел — проекта, перенос обвязки правит остальной текст.

—

## Что остаётся

- Платформа: authgate проверяет токены внешней идентичности (wiring — `/de-template` по
  `docs/stack/auth.md`) и держит cookie (`COOKIE_SECURE` проверяет prod-гард конфигурации),
  `internal/testkit/identitytest` — личность в тестах своих доменов.
- e2e: smoke `test/smoke/test_boot.py`; фикстуры идентичности в `test/conftest.py` и её модели в
  `test/client/models.py` — до своих сценариев.
- Образцы в доменах идентичности, на которые ссылается проза обвязки (события, порты, integration-тесты), —
  в истории проекта до prune (CLAUDE.md § Проектирование первично).
