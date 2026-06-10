---
id: stack.service
kind: service
name: service
status: draft
---

# Service — идентичность сервиса

## Identity
<Имя сервиса, module path (`git.stit.tech/<org>/<service>`), одно-строчное описание роли.>
<`project-slug`: slug продукта в соседнем strategy-репо — `scaffold` читает домены из
`<strategy-path>/docs/projects/<project-slug>/domains/`. `strategy-path`: путь к форку `<org>/strategy`
(дефолт `../strategy` — sibling-чекаут рядом с этим репо). На каноническом шаблоне не задан.>

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
<NB: `project-slug`/`strategy-path` из § Identity — не код-вывод de-template, а указатель,
по которому `scaffold` (вариант domain) находит домены в соседнем strategy-репо.>
