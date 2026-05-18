# cmd/seed-admin — bootstrap первого admin (План A)

**Slug:** `cmd-seed-admin`
**Создан:** 2026-05-18
**Статус:** planning
**Последний чекпоинт:** —

## Контекст

В шаблоне **нет пути создать первого admin**: `register` всегда даёт
`role=client` (`user/usecase.go:97`), bootstrap/seed отсутствует (recon).
Это реальный продуктовый пробел (как поднять админа на свежем деплое), а не
только e2e-блокер. Решение — отдельный one-off `cmd/seed-admin` по образцу
`cmd/migrate`: принимает email+password, идемпотентный upsert юзера с
`role=admin`, хеш через `golibinfra/authnkit` (Argon2id, теми же дефолтными
`Argon2idParams`, какими приложение верифицит на login — encoded PHC
self-describing, совпадение гарантировано). Разблокирует authz-тесты Плана B
(`e2e-exhaustive-coverage`) и даёт ops-инструмент.

Recon-факты: `authnkit.Hasher{Hash,Verify}`; `cmd/migrate` = envconfig +
pgkit + logkit (forbidigo: только logkit, без fmt/slog); уникальность users —
частичный индекс `LOWER(email) WHERE status!='deleted'` ⇒ upsert через tx
select→insert/update; login перечитывает роль из БД (`auth/usecase.go:141,174`)
⇒ seeded-admin логинится штатно.

## Не входит в скоуп

- Сама coverage-матрица (authz/write/negatives/list) — это План B.
- Прод-деплой/CI-проводка seed-admin — отдельная ответственность infra-репо.
- Любые изменения auth/hashing-логики приложения (используем authnkit как есть).
- Verification confirm-roundtrip (вне скоупа всей инициативы, см. План B).

## Открытые вопросы

- [x] Q1 Форма → полноценный `cmd/seed-admin` (email+pw upsert admin), one-off
  как `cmd/migrate`. Не «только hash».
- [x] Q2 Хешер → строго `golibinfra/authnkit` дефолтными параметрами
  (encoded PHC self-describing ⇒ app.Verify пройдёт).
- [x] Q3 Потребление e2e → seed-admin гоняется при старте стека, креды
  admin в `.env.local`, фикстура логинится ими.
- [x] Q4 Идемпотентность → upsert по `LOWER(email)` в tx (повторный запуск
  не падает, обновляет role/hash).

## Атомы

- [ ] **PA1:** `cmd/seed-admin/main.go`
  - Что делаем: envconfig (pgkit.Config PG_DSN, authnkit.Argon2idParams),
    logkit-логгер (НЕ fmt/slog), pgkit-коннект. Флаги/env `SEED_ADMIN_EMAIL`,
    `SEED_ADMIN_PASSWORD`. Tx: SELECT по `LOWER(email) WHERE status!='deleted'`
    → INSERT (role=admin, status=active, email_verified=true, version=1) либо
    UPDATE (role=admin, password_hash). Идемпотентно. Exit 0/non-0 + лог.
  - Файлы: `cmd/seed-admin/main.go`
  - Verify: `go build ./cmd/seed-admin`; `make pre-merge` (vet/lint/forbidigo) green
  - Зависит от: —

- [ ] **PA2:** Сборка/образ — добавить seed-admin
  - Что делаем: `test/stack/up.sh` build-loop `for target in api migrate
    seed-admin`; `test/stack/Dockerfile.local` COPY seed-admin.
  - Файлы: `test/stack/up.sh`, `test/stack/Dockerfile.local`
  - Verify: `bash test/stack/up.sh` собирает 3 бинаря; `docker compose -f
    test/stack/compose.yml config -q` OK
  - Зависит от: PA1

- [ ] **PA3:** compose one-off `seed-admin` + креды в `.env.local`
  - Что делаем: сервис `seed-admin` (image local, entrypoint seed-admin,
    env `SEED_ADMIN_EMAIL/PASSWORD` ← `${E2E_ADMIN_*}`, app-env PG_DSN,
    `depends_on: migrate completed + postgres healthy`); `api.depends_on +=
    seed-admin: service_completed_successfully`. `up.sh` резолвит+экспортит
    `E2E_ADMIN_EMAIL/PASSWORD` из `.env.local` (как STIT_OBS_TOKEN).
    `.env.local.example` += ключи (дефолты); `.claude/rules/env-local.md`
    known-keys += `E2E_ADMIN_*`.
  - Файлы: `test/stack/compose.yml`, `test/stack/up.sh`, `.env.local.example`, `.claude/rules/env-local.md`
  - Verify: стек up → seed-admin отрабатывает один раз →
    `curl POST /api/auth/login` под admin-кредами → 200 + cookies
  - Зависит от: PA2

- [ ] **PA4:** conftest `admin` / `other_user` фикстуры + чистка
  - Что делаем: `admin` — читает `E2E_ADMIN_EMAIL/PASSWORD` из `.env.local`
    (in-process, без эха), логинит, `Api` c admin-cookie. `other_user` —
    свежий client. Удалить obsolete `test/infra/seed.py`.
  - Файлы: `test/conftest.py`, удаление `test/infra/seed.py`
  - Verify: temp-probe `admin` GET `/api/users` → 200, удалить probe;
    `cd test && .venv/bin/mypy conftest.py infra` strict clean
  - Зависит от: PA3

- [ ] **PA5:** Документация (forward-only)
  - Что делаем: `README.md`/`AGENTS.md`/`test/README.md` — `cmd/seed-admin`
    как ops-bootstrap первого admin + e2e-admin; security-ремарка (минтит
    admin, нужен PG_DSN, one-off не runtime). roadmap.
  - Файлы: `README.md`, `AGENTS.md`, `test/README.md`, `.claude/plans/roadmap.md`
  - Verify: `make pre-merge` green; forward-only diff-review
  - Зависит от: PA4

## Чекпоинты

—
