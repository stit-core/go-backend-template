---
name: upgrade
description: Обновить сервис по двум каналам — golibinfra по аннотациям тегов (`go get`, правки по Breaking, `make check`, handoff) и обвязку шаблона (CLAUDE.md, `.claude`, скрипты, Makefile, lint-конфиг, archtest, доки шаблона) от тега `template/vX.Y.Z` через `make template-update`. Используй, когда просят поднять golibinfra или подтянуть улучшения шаблона, когда `make doctor` (строка `template:`) или `make status` (`== Template ==`) назвал тег шаблона новее версии форка или у форка нет `.template/version`.
---

# Skill: upgrade

Улучшения приходят в сервис двумя каналами:
- **Киты** — версия golibinfra в `go.mod`: поведение китов и их общие правила приходят bump'ом.
- **Обвязка шаблона** — пути из `.template/paths`: их изменения между тегами шаблона
  `template/vX.Y.Z` переносит `make template-update`, версия обвязки форка — `.template/version`.

Остальное — код форка (домены, `internal/platform`, `cmd`, `pkg`, миграции): правки шаблона там
называет раздел `Платформа` аннотации тега, форк переносит их сам, если нужны. Сначала golibinfra,
потом обвязка: обвязка тега рассчитана на golibinfra из `go.mod` этого тега.

## Когда применять

- Задача или план хаба: «подними golibinfra до vX.Y.Z», «подтяни обновления шаблона».
- `make doctor` → `template: … · в шаблоне новее: template/vX.Y.Z` (теги remote `template`,
  `git ls-remote`) или `make status` → `== Template ==` с той же строкой (теги последнего
  `make template-update`) — предложи перенос юзеру.
- У форка нет `.template/version` — раздел «Форк без версии обвязки».

Оба канала трогают «Сначала спросить» и гейт (`go.mod`; `Makefile`, `scripts/`, `internal/archtest`,
`.claude/settings.json`) — по задаче, плану хаба или с OK юзера (CLAUDE.md § Границы).

## Когда НЕ применять

- Канонический репо шаблона (`== Template ==` — «канон шаблона»: тег версии в истории и module path
  шаблона): его обвязка — источник, из этого скилла ему нужен только раздел golibinfra. Так же
  выглядит форк через `git clone` до смены module path.

## golibinfra

1. Версии: текущая — `grep golibinfra go.mod`; теги — в соседнем checkout `../golibinfra` (нет —
   склонируй рядом): `git -C ../golibinfra fetch --tags`, затем
   `git -C ../golibinfra tag -l 'v*' --sort=v:refname`.
2. Аннотация **каждого** тега после текущего до целевого, не только последнего:
   `git -C ../golibinfra tag -l --format='%(contents)' vX.Y.Z`. `Breaking` — что изменилось → как
   мигрировать; `Поведение` — дефолты и исходы без правок кода; `Изменения` — новое.
3. `go get git.stit.tech/stit-core/golibinfra@vX.Y.Z && go mod tidy`.
4. Правки по каждому пункту `Breaking`; пункты `Поведение` сверь с env и конфигом сервиса
   (`.env.example`, `internal/platform/config`).
5. `make gen` (схемы операций и ошибок — в `openapi.json`), затем готово по CLAUDE.md § Готово.
6. Handoff: новый или изменённый env, дефолт, таблица или требование к окружению из аннотаций —
   блок `/infra-handoff` тем же коммитом; ничего такого нет — блок не нужен.
7. Коммит `feat(deps): golibinfra vX.Y.Z — <главное>` (`!` — если меняется API или события
   сервиса), в теле — пункты `Breaking`, задевшие код.

## Обвязка шаблона

Remote шаблона без его тегов заводит финал `/init-service`; нет его (`make doctor`: «remote template не
заведён») — заведи: теги релизов шаблона `vX.Y.Z` смешались бы с тегами форка, теги обвязки
`template/*` приносит `make template-update`.

```bash
git remote add --no-tags template git@git.stit.tech:stit-core/go-backend-template.git
```

1. `make template-update` — версия форка, теги шаблона новее неё с аннотациями и golibinfra
   последнего тега; дерево не меняется. Формат аннотации — `docs/contributing.md § Версия обвязки`.
   golibinfra форка ниже, чем у тега, — сначала раздел golibinfra.
2. `make template-update TO=template/vX.Y.Z` на чистом дереве — трёхстороннее слияние путей шаблона
   до тега: первый перенос — от коммита шаблона, из которого создан форк (по дереву первого коммита
   форка), следующие — от `.template/version`; итог — в индексе (`git diff --cached`),
   `.template/version` уже сдвинут:
   - **конфликты** (`git status` — `UU`, `AA`): шаблон и форк правили одно место — сведи обе правки
     по смыслу и `git add`; забытый маркер конфликта роняет `make check` (archtest
     `TestNoConflictMarkers`);
   - **нет в форке — не тронуты**: файл, который форк удалил, а шаблон правил; правка нужна — верни
     файл (`git show template/vX.Y.Z:<путь>`, в Go-файле — module path форка);
   - **шаблон удалил, форк менял**: `git rm` или оставь как файл форка;
   - **`internal/archtest/layout_settings_test.go`** — настройки `archkit` форка, перенос его не
     трогает: без него пакет `internal/archtest` не собирается — возьми образец
     (`git show template/vX.Y.Z:internal/archtest/layout_settings_test.go`) и сведи со своей раскладкой.
     Нарушения, которые `TestLayout` нашёл в коде форка, — правкой кода или записью
     `archkit.WithKnownViolations` с ID находки (перечень проверок — README `archkit`).
3. Шаги `Breaking` и `Гейт` из аннотаций всех тегов до `TO`; правки раздела `Платформа` — по нужде.
4. `make gen`, затем готово по CLAUDE.md § Готово.
5. Коммит `chore(template): обвязка template/vX.Y.Z`, в теле — что форк правил руками.

## Форк без версии обвязки

Форк, созданный до `.template/`, переносит обвязку скриптом нужного тега: базу — коммит шаблона с
деревом первого коммита форка — скрипт находит сам, `.template/` приходит тем же переносом; итог
разбирай как в шаге 2 обвязки (конфликты, пропущенные файлы), дальше — шаги 3–5:

```bash
git remote add --no-tags template git@git.stit.tech:stit-core/go-backend-template.git
git fetch --no-tags template 'refs/tags/template/*:refs/tags/template/*'
bash <(git show template/vX.Y.Z:scripts/tools/template-update.sh) template/vX.Y.Z
# «базы нет» — форк правили до первого коммита: база — шаблон на дату первого коммита форка
git fetch template
bash <(git show template/vX.Y.Z:scripts/tools/template-update.sh) template/vX.Y.Z \
    "$(git rev-list -1 --before="$(git log --reverse --format=%cI | head -1)" template/main)"
```

## Anti-patterns

- ❌ `git fetch template --tags` или remote без `--no-tags`: теги релизов шаблона смешаются с тегами форка.
- ❌ Копировать файлы шаблона поверх своих — правки форка пропадут молча; перенос — `make template-update`.
- ❌ Обвязка тега раньше golibinfra, на которую она рассчитана.
- ❌ Bump golibinfra через несколько тегов по одной последней аннотации.
- ❌ `git add` файла с маркерами конфликта.
