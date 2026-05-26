---
id: stack.config
kind: config
name: config
status: draft
---

# Config — конфигурация и секреты

## Required env
<Обязательные env-vars (приложение не стартует без них): имя, тип, смысл. Эти
переменные едут в INFRA-HANDOFF.>

## Optional env
<Опциональные env с дефолтами: имя, дефолт, когда менять.>

## Secrets
<Секреты Vault-класса (JWT-ключи, OIDC, OTEL_TOKEN, DB-пароль): имя ключа и факт
«нужен» — НЕ значение. Кто выдаёт (корп-Vault / проект).>

## Validation
<Как валидируется конфиг на старте (fail-fast): обязательность, формат, диапазоны.>

## Generates
<Что de-template родит:
- `internal/platform/config` (структуры + загрузка)
- `.env.example` (required+optional с дефолтами)
- seed для INFRA-HANDOFF (required env + секреты)>
