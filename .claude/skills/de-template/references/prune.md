# de-template › prune reference-доменов

`auth` / `user` / `verification` — учебные reference-домены. Удаляются на де-темплейтизации
**после** того как свои домены спроектированы (в соседнем strategy-репо) и скаффолжены (`scaffold`
domain) — иначе теряешь живой образец. Удаляй все точки; недоудаление оставит висящие импорты
или моки на несуществующие пакеты.

## Чеклист (7 точек)

1. **Доменные пакеты** — `internal/domain/{auth,user,verification}/` целиком (`git rm -r`) и
   их записи в allowlist'ах `internal/archtest`.
2. **`.mockery.yml`** — блоки `packages` удаляемых доменов (`internal/domain/{user,auth,verification}`
   и их `delivery/http`, `delivery/nats`); иначе `make gen` падает на несуществующих пакетах.
3. **sqlc.yaml** — три блока `engine` с `queries/out` на `internal/domain/{user,verification,auth}/...`.
4. **Миграции** — `migrations/*` этих доменов: `0001_users.*` (таблицы `users`,
   `password_resets`, `email_verifications` — user+verification), `0002_auth.*`
   (`refresh_sessions`) и `0003_email_verifications_idempotency.*`. Удаляй и `.up.sql`, и
   `.down.sql`. Свои миграции перенумеруй подряд с `0001` — `migrations-check` (шаг `make check`)
   требует непрерывной нумерации; прод-БД у форка на этом шаге ещё нет. Если свои миграции
   опираются на общие объекты из `0001_users` (`CREATE EXTENSION pgcrypto`, функция
   `set_updated_at()`), перенеси их в первую свою миграцию.
5. **fx-wiring** — `cmd/api/main.go`: убери `usermod`/`authmod`/`verifmod` из `domainModules`
   (импорты + строки в `fx.Options`); `cmd/api/http.go` — их хендлеры из `httpHandlers` и
   `registerHTTP`; `cmd/api/api_test.go` — их операции из `publicOperations`.
6. **e2e** — их сценарии/фикстуры в `test/` (auth-flow, user CRUD, verification, admin-fixtures).
7. **`cmd/seed-admin/`** — bootstrap админа завязан на `user`/`auth`; удали каталог + его
   сборку в `test/stack/` (`up.sh`, `Dockerfile.local`, сервис в `compose.yml`) + e2e-использование.
   Если форку нужен свой seed — заведёт заново под свой домен.

## Что НЕ удалять (платформа, не домены)

- `internal/platform/{authgate,identity,config}` — платформенная инфра (golibinfra-backed).
  Если форк имеет авторизацию (обычно да) — `authgate`/`identity` остаются и переиспользуются
  своими доменами. Удаляй их **только** если форк осознанно без authn — это решение стека
  (`docs/stack/auth.md`), не дефолт prune.
- `docs/stack/_templates/` — скаффолд-шаблоны стека для будущих артефактов (шаблоны доменов — в соседнем strategy-репо).

## Verify

```bash
make gen          # sqlc/mocks без удалённых доменов
go build ./...    # нет висящих импортов на удалённые пакеты
make lint
bash scripts/tools/migrations-check.sh   # нумерация миграций непрерывна
```

Зелёные — prune полный. Красный `build` с `cannot find package …/domain/<X>` → недочистил
точку 5 (fx-wiring) или 7 (seed-admin); красный `make gen` → точку 2 или 3.
