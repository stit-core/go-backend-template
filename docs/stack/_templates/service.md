---
id: stack.service
kind: service
name: service
status: draft
---

# Service — идентичность сервиса

## Identity
<Имя сервиса, module path (`git.stit.tech/<org>/<service>`), одно-строчное описание роли.>

## Versioning
<Схема версий (semver `vX.Y.Z`). Как версия и commit попадают в бинарь (ldflags
`-X main.version` / `-X main.commit`). Что логируется на старте.>

## Binaries
<Какие бинари собираются и зачем: `cmd/api` (runtime), `cmd/migrate` (init-контейнер),
`cmd/seed-admin` (one-off bootstrap). Какие образы пушатся.>

## Generates
<Что de-template родит из этого артефакта:
- `go.mod` module path
- README-шапка
- ldflags version/commit в Makefile/build
- лог `"starting" service/version/commit`>
