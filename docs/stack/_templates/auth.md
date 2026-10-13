---
id: stack.auth
kind: auth
name: auth
status: draft
identity: <own|external>
---

# Auth — аутентификация и авторизация

`identity` — где живут учётные записи. `own` — в этом сервисе: домены идентичности шаблона (`auth`,
`user`, `verification` — вход, сессии, регистрация, подтверждение адреса, сброс пароля; механика — в
китах authnkit, bruteforcekit, httpkit) остаются и растут по артефактам strategy. `external` — токены
выпускает другой сервис или IdP, этот только проверяет их (authgate): домены идентичности удаляет prune
(skill init-service).

## Authn
<Механизм: JWT (access/refresh) и/или session; где верифицируется (authgate), какие
claims/principal. authnkit-примитивы (хеш паролей, TOTP). При `external` — кто выпускает токены и как
сервис их проверяет.>

## Authgate
<`internal/platform/authgate`: как middleware достаёт principal, что видят хендлеры,
публичные vs защищённые роуты.>

## Roles
<Роли и их смысл (client/admin/…), как назначаются, где проверяются (policy-слой доменов).>

## Seed-admin
<Bootstrap первого admin (`cmd/seed-admin`, идемпотентный upsert, `SEED_ADMIN_*`).
Публичного пути сделать admin нет. При `external` — admin назначает внешняя идентичность.>

## Brute-force
<bruteforcekit: что защищается (вход, запросы сброса пароля), счётчики, lock-политика, поведение при сбое guard'а (вход — fail-closed), cleanup-job. Лимит клиента на `/api/auth/*` (`HTTP_AUTH_RATE_LIMIT_*`, по умолчанию 5 rps и запас 10) обязателен при публичном входе: его значение — по ёмкости очереди Argon2id (слотов `GOMAXPROCS/ARGON2_THREADS`, хэш — десятки мс), клиент — IP по `HTTP_TRUST_PROXY_HOPS`; поток с многих адресов держит край (прокси, WAF) — кто и как. При `external` вход — вне сервиса.>

## Generates
<Что de-template родит:
- authgate-wiring + middleware
- authnkit-провайдеры (JWT verifier, hasher)
- `own`: `cmd/seed-admin`, bruteforce guard + cleanup-job>
