# init-service › prune — удалить домены идентичности

Только у проекта с внешней идентичностью: `docs/stack/auth.md` — `identity: external` (токены выпускает
другой сервис или IdP). При `identity: own` домены идентичности `auth`, `user`, `verification` — домены
проекта, prune не нужен. Шаг — после своих доменов (этап 4 скилла): без них сервису нечего
регистрировать.

## Шаги

1. **Пути доменов идентичности** — по списку шаблона, целиком:
   ```bash
   git rm -r -q $(grep -v '^#' .template/identity)
   ```
   `.template/identity` остаётся: по нему и решению `identity: external` самопроверка прозы знает, что
   образцы в этих путях — в теге шаблона («Что остаётся»).
2. **Подключение** — строки удалённых пакетов и китов, которые были нужны только им (сессии входа и
   антибрутфорс):
   - `cmd/api/main.go` — `usermod`, `authmod`, `verifmod` (импорты, `domainModules`), `session.NewConfig`,
     `bruteforcekit.NewConfig` (`kitConfigs`), `sessionpg.Module`, `bruteforcepg.Module`;
   - `cmd/api/http.go` — `userhttp`, `authhttp`, `verifhttp` (импорты, поля `httpHandlers`, вызовы в
     `registerHTTP`);
   - `cmd/migrate/sources/sources.go` — `useroutbox`, `verifoutbox`, `sessionpg`, `bruteforcepg`;
   - `cmd/api/api_test.go` — их открытые операции из `publicOperations`.
3. **Подключение по стеку** — киты, которые de-template оставил ради доменов идентичности (wiring §2
   скилла `de-template`): кит, которого нет в `docs/stack/dependencies.md § golibinfra kits` (аудит —
   `auditpg.Module` в `cmd/api/main.go` и `auditpg.Migrations` в `cmd/migrate/sources/sources.go`), —
   вон из графа и из источников миграций, импорты — `go mod tidy`.
4. **Генерация** — блоки удалённых доменов в `sqlc.yaml` и `.mockery.yml`.
5. **Миграции** — свои — номерами подряд с `0001` (`git mv` с сохранением порядка: migrations-check
   ждёт нумерацию с 0001, прод-БД у проекта ещё нет). Расширение `pgcrypto` и функция `set_updated_at()`
   из `0001_users` уходят с ней: своя миграция, которой они нужны, заводит их сама.
6. **Окружение** — `.env.example`: в `NATS_JS_STREAM_NAMES` — стримы своих доменов-публикаторов (без
   `user` и `verification`; публикаторов нет — `NATS_JS_STREAM_NAMES=`), комментарий над ним — про них;
   у `JWT_SECRET` — ключ проверки токенов внешней идентичности; строки `SESSION_*` и
   `BRUTEFORCE_SUBJECT_*` с их комментариями удали.
7. **Раскладка** — `internal/archtest/layout_settings_test.go`: запись
   `archkit.WithUncheckedPackages("internal/platform/identity")` убери, если корень ни одного своего
   домена не берёт `internal/platform/identity` (сам или через `pkg/`): лишняя запись — `TestLayout`
   красный с подсказкой `remove it`.
8. `make gen` → `make check` → `✓ check clean`.

## Что остаётся

- Платформа: authgate проверяет токены внешней идентичности (wiring — `/de-template` по
  `docs/stack/auth.md`), `internal/testkit/identitytest` — личность в тестах своих доменов.
- e2e: smoke `test/smoke/test_boot.py`; фикстуры идентичности в `test/conftest.py` и её модели в
  `test/client/models.py` — до своих сценариев.
- Проза обвязки ссылается на образцы в доменах идентичности (события, порты, integration-тесты): у
  проекта они — в шаблоне, `git show <тег>:<путь>`: тег — `.template/version`, а пути нет в нём (форк
  создан с main новее тега) — последний тег шаблона; теги приносит `make template-update` (skill
  `/upgrade`).
