# de-template › prune reference-доменов

`auth` / `user` / `verification` — учебные reference-домены. Удаляются на де-темплейтизации
**после** того как свои домены спроектированы (в соседнем docs-репо) и скаффолжены (`scaffold`
domain) — иначе теряешь живой образец. Удаляй все точки; недоудаление оставит висящие импорты
или depguard-правила на несуществующие пакеты.

## Чеклист (8 точек)

1. **Доменные пакеты** — `internal/domain/{auth,user,verification}/` целиком (`git rm -r`).
2. **DDD-артефакты** — удалять нечего: reference-домены поставляются **code-only** (артефактов
   доменов в этом репо нет; домены живут в соседнем docs-репо). Точка остаётся для симметрии
   чеклиста — просто отметь «N/A».
3. **sqlc.yaml** — три блока `engine` с `queries/out` на `internal/domain/{user,verification,auth}/...`.
4. **Миграции** — `migrations/*` этих доменов (их таблицы: users, sessions, verification_*…).
   Удаляй и `.up.sql`, и `.down.sql`. Нумерация остальных не трогается.
5. **fx-wiring** — `cmd/api/main.go`: убери `usermod`/`authmod`/`verifmod` из `domainModules`
   (импорты + строки в `fx.Options`).
6. **depguard** — `.golangci.yml`: правила `user-domain-isolation`, `auth-domain-isolation` и
   любые `deny`-пути на удаляемые домены (в т.ч. `verification`).
7. **e2e** — их сценарии/фикстуры в `test/` (auth-flow, user CRUD, verification, admin-fixtures).
8. **`cmd/seed-admin/`** — bootstrap админа завязан на `user`/`auth`; удали каталог + его
   Makefile-таргет (если есть) + e2e-использование. Если форку нужен свой seed — заведёт заново
   под свой домен.

## Что НЕ удалять (платформа, не домены)

- `internal/platform/{authgate,identity,config}` — платформенная инфра (golibinfra-backed).
  Если форк имеет авторизацию (обычно да) — `authgate`/`identity` остаются и переиспользуются
  своими доменами. Удаляй их **только** если форк осознанно без authn — это решение стека
  (`docs/stack/auth.md`), не дефолт prune.
- `docs/stack/_templates/` — скаффолд-шаблоны стека для будущих артефактов (шаблоны доменов — в соседнем docs-репо).

## Verify

```bash
make gen          # sqlc/mocks без удалённых доменов
go build ./...    # нет висящих импортов на удалённые пакеты
make lint         # depguard не ругается на несуществующие правила
```

Зелёные — prune полный. Красный `build` с `cannot find package …/domain/<X>` → недочистил
точку 5 (fx-wiring) или 8 (seed-admin).
