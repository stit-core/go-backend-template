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
   образцы в этих путях — в версии шаблона.
2. **Подключение** — строки удалённых пакетов и китов, которые были нужны только им (сессии входа и
   антибрутфорс):
   - `cmd/api/main.go` — `usermod`, `authmod`, `verifmod` (импорты, `domainModules`), `session.NewConfig`,
     `bruteforcekit.NewConfig` (`kitConfigs`), `sessionpg.Module`, `bruteforcepg.Module`;
   - `cmd/api/http.go` — `userhttp`, `authhttp`, `verifhttp` (импорты, поля `httpHandlers`, вызовы в
     `registerHTTP`);
   - `cmd/migrate/sources/sources.go` — `useroutbox`, `verifoutbox`, `sessionpg`, `bruteforcepg`;
   - `cmd/api/api_test.go` — их открытые операции из `publicOperations`.
3. **Генерация** — блоки удалённых доменов в `sqlc.yaml` и `.mockery.yml`.
4. **Миграции** — свои — номерами подряд с `0001` (`git mv` с сохранением порядка: migrations-check
   ждёт нумерацию с 0001, прод-БД у проекта ещё нет). Расширение `pgcrypto` и функция `set_updated_at()`
   из `0001_users` уходят с ней: своя миграция, которой они нужны, заводит их сама.
5. **Окружение** — `.env.example`: в `NATS_JS_STREAM_NAMES` — стримы своих доменов-публикаторов (без
   `user` и `verification`), комментарий над ним — про них; строки `SESSION_*` удали.
6. `make gen` → `make check` → `✓ check clean`.

## Что остаётся

- Платформа: authgate проверяет токены внешней идентичности (wiring — `/de-template` по
  `docs/stack/auth.md`), `internal/testkit/identitytest` — личность в тестах своих доменов.
- e2e: smoke `test/smoke/test_boot.py`; фикстуры идентичности в `test/conftest.py` и её модели в
  `test/client/models.py` — до своих сценариев.
- Проза обвязки ссылается на образцы в доменах идентичности (события, порты, integration-тесты): у
  проекта они — в версии шаблона, `git show <версия>:<путь>` (версия — `.template/version`, теги приносит
  `make template-update`, skill `/upgrade`).
