# E2E coverage-матрица (План B)

**Slug:** `e2e-exhaustive-coverage`
**Создан:** 2026-05-18
**Статус:** planning (blocked-by `cmd-seed-admin`)
**Последний чекпоинт:** 2026-05-18 (см. ниже)

## Контекст

Поверх примитивов `e2e-infra-introspection` (Plan 1, закрыт) и admin-фикстур
из `cmd-seed-admin` (План A) — закрыть приоритетные дыры покрытия: authz,
bruteforce-LOCK, write-ручки, тонкие негативы, list. Go-сторона **не
трогается** этим планом (Go-часть — целиком в Плане A).

Зафиксировано разведкой:
- Authz (`internal/domain/user/policy.go`): `CanList` = **admin-only**;
  `CanGetByID/CanUpdate/CanDelete` = **selfOrAdmin**.
- Коды (user delivery `handler.go` errMapping): 409 version-conflict,
  404 `ErrNotFound`, 400 validation («at least one of name or status»),
  403 `ForbiddenError`.
- bruteforce: `BRUTEFORCE_MAX_FAILURES=5`, lock → 429
  (`auth/delivery/http/handler.go:141`), ключ `bruteforce_attempts(subject,ip)`,
  subject = email (свежий per-test ⇒ изоляция лока).
- Фикстуры `admin` / `other_user` — поставляет План A.

## Не входит в скоуп

- Go-изменения, `cmd/seed-admin`, stack-wiring, `.env.local` — это План A.
- **Verification confirm-roundtrip**: у шаблона нет канала доставки
  (`logMailerAdapter` заглушка by design, raw-токен в outbox/NATS осознанно
  отсутствует) — не баг и не дыра харнеса. Confirm — дом Go usecase/integration.
  request-side уже в `test_event_flow.py` (Plan 1). Mailpit — extension point
  форка.
- Пер-параметрический фаззинг, конкурентные/race-тесты.

## Открытые вопросы

- [x] Q1 Охват → максимум по приоритетам + тонкие негативы + list;
  verification confirm пропущен (обоснование выше).
- [x] Q2 Admin-принципал → из Плана A (`cmd/seed-admin` + фикстура `admin`).
- [x] Q3 Потолок → ≤10 атомов в этом плане; фаззинг/race вне скоупа.

## Атомы

- [ ] **B1:** authz — list (admin-only)
  - Что делаем: `e2e/test_authz.py` — client GET `/api/users` → **403**;
    `admin` → 200 + форма (data/total/page/size).
  - Файлы: `test/e2e/test_authz.py`, при нужде `scenarios/`
  - Verify: полный `e2e.sh` — новый тест зелёный
  - Зависит от: План A

- [ ] **B2:** authz — get-by-id (selfOrAdmin)
  - Что делаем: client GET свой `/users/{id}` → 200; чужой → 403;
    `admin` любой → 200. Сверка с `pg.users_row`.
  - Файлы: `test/e2e/test_authz.py`
  - Verify: полный e2e зелёный
  - Зависит от: B1

- [ ] **B3:** authz — update/delete forbidden (cross-user)
  - Что делаем: client PATCH/DELETE чужой `/users/{id}` → 403;
    целевая строка в БД не изменилась.
  - Файлы: `test/e2e/test_authz.py`
  - Verify: полный e2e зелёный
  - Зависит от: B1

- [ ] **B4:** write — PATCH `/users/{id}`
  - Что делаем: happy (`admin` меняет name/status) → 200, `users.version`
    инкремент, outbox `user.events.updated`; stale version → **409**;
    пустой body → **400**.
  - Файлы: `test/e2e/test_user_write.py`, `client/`, `scenarios/user.py`
  - Verify: полный e2e зелёный (вкл. outbox-ассерт)
  - Зависит от: План A

- [ ] **B5:** write — DELETE `/users/{id}`
  - Что делаем: `admin` DELETE → **204**, `users.status='deleted'`, outbox
    `user.events.deleted`; повторный GET → **404**; DELETE несуществующего → 404.
  - Файлы: `test/e2e/test_user_write.py`, `client/`
  - Verify: полный e2e зелёный
  - Зависит от: План A

- [ ] **B6:** write — change-password
  - Что делаем: `POST /api/auth/change-password` верный old → **204**, логин
    старым паролём фейлит, новым ок; неверный old → 401.
  - Файлы: `test/smoke/test_auth.py`, `client/`
  - Verify: `e2e.sh --smoke` зелёный
  - Зависит от: План A

- [ ] **B7:** bruteforce-LOCK 429
  - Что делаем: 5 неудачных login свежего email → **429**;
    `bruteforce_attempts.locked_until IS NOT NULL`; верный пароль во время
    лока тоже 429; другой свежий email не залочен.
  - Файлы: `test/smoke/test_auth.py`
  - Verify: `e2e.sh --smoke` зелёный
  - Зависит от: —

- [ ] **B8:** тонкие негативы 400/404/401
  - Что делаем: register 400 (битый email / короткий пароль);
    GET `/users/{random-uuid}` под `admin` → 404; без cookie `/users/me` и
    `/api/users` → 401.
  - Файлы: `test/e2e/test_contract_negatives.py`
  - Verify: полный e2e зелёный
  - Зависит от: План A

- [ ] **B9:** list пагинация/фильтр
  - Что делаем: `admin` GET `/api/users?page&size&status` — форма ответа,
    `status`-фильтр режет, граница `size` (max 100), арифметика page/total.
  - Файлы: `test/e2e/test_user_list.py`, `client/`
  - Verify: полный e2e зелёный
  - Зависит от: B1

- [ ] **B10:** docs + закрытие
  - Что делаем: `test/README.md`/`README.md`/`AGENTS.md` — покрытие
    authz/write/negatives + ремарка про verification confirm (Go-side, не e2e).
    roadmap: убрать строки обоих планов по завершении.
  - Файлы: `test/README.md`, `README.md`, `AGENTS.md`, `.claude/plans/roadmap.md`
  - Verify: `make pre-merge` green; forward-only diff-review; `e2e.sh` → OK
  - Зависит от: B1–B9

## Чекпоинты

### 2026-05-18 — переоценка: admin → Plan A
- Переоценил: admin-принципал вынесен в отдельный `cmd-seed-admin` (План A)
  как Go ops-bootstrap (в шаблоне нет пути сделать первого admin). Прежние
  атомы seed/фикстуры удалены отсюда; этот план — чистая coverage-матрица,
  blocked-by Плана A.
- Obsolete `test/infra/seed.py` (написан до пивота) удаляется в Плане A (PA4).
- Следующий шаг: после approve — выполнить План A, затем разблокировать B1.
