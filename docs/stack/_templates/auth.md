---
id: stack.auth
kind: auth
name: auth
status: draft
---

# Auth — аутентификация и авторизация

## Authn
<Механизм: JWT (access/refresh) и/или session; где верифицируется (authgate), какие
claims/principal. authnkit-примитивы (хеш паролей, TOTP).>

## Authgate
<`internal/platform/authgate`: как middleware достаёт principal, что видят хендлеры,
публичные vs защищённые роуты.>

## Roles
<Роли и их смысл (client/admin/…), как назначаются, где проверяются (policy-слой доменов).>

## Seed-admin
<Bootstrap первого admin (`cmd/seed-admin`, идемпотентный upsert, `SEED_ADMIN_*`).
Публичного пути сделать admin нет.>

## Brute-force
<bruteforcekit: что защищается (login), счётчики, lock-политика, cleanup-job.>

## Generates
<Что de-template родит:
- authgate-wiring + middleware
- authnkit-провайдеры (JWT verifier, hasher)
- `cmd/seed-admin`
- bruteforce guard + cleanup-job>
