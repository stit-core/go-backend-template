---
name: upgrade
description: Обновить сервис по двум каналам — golibinfra по аннотациям тегов (`go get`, правки по Breaking, `make check`, handoff) и обвязку шаблона (CLAUDE.md, `.claude`, скрипты, Makefile, Dockerfile'ы, lint-конфиг, archtest, доки шаблона; код форка — по запросу) от тега `template/vX.Y.Z` через `make template-update`. Используй, когда просят поднять golibinfra или подтянуть улучшения шаблона, когда `make doctor` (строка `template:`) или `make status` (`== Template ==`) назвал тег шаблона новее версии форка или у форка нет `.template/version`.
---

# Skill: upgrade

Улучшения приходят в сервис двумя каналами:
- **Киты** — версия golibinfra в `go.mod`: поведение китов и их общие правила приходят bump'ом.
- **Обвязка шаблона** — пути из `.template/paths`: их изменения между тегами шаблона
  `template/vX.Y.Z` переносит `make template-update`, версия обвязки форка — `.template/version`.

Остальное — код форка (домены, `internal/platform`, `cmd`, `pkg`, миграции): правки шаблона там
называют разделы `Безопасность` и `Платформа` аннотации тега, нужные форк берёт тем же переносом по
запросу (`PATHS='<пути>'`), каждый пункт — перенесён или отклонён с причиной в теле коммита. Обвязка и код
тега рассчитаны на golibinfra из `go.mod` этого тега: у форка ниже — перенос и bump одним заходом.

## Когда применять

- Задача или план хаба: «подними golibinfra до vX.Y.Z», «подтяни обновления шаблона».
- `make doctor` → `template: … · в шаблоне новее: template/vX.Y.Z` (теги remote `template`,
  `git ls-remote`) или `make status` → `== Template ==` с той же строкой (теги последнего
  `make template-update`) — предложи перенос юзеру.
- У форка нет `.template/version` или он на template/v1.1.0 и раньше — раздел «Первый перенос движком тега».

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
2. Аннотация каждого тега после текущего до целевого, не только последнего:
   `git -C ../golibinfra tag -l --format='%(contents)' vX.Y.Z`. `Breaking` — что изменилось → как
   мигрировать; `Поведение` — дефолты и исходы без правок кода; `Изменения` — новое.
3. `go get git.stit.tech/stit-core/golibinfra@vX.Y.Z && go mod tidy`.
4. Правки по каждому пункту `Breaking`; пункты `Поведение` сверь с env и конфигом сервиса
   (`.env.example`, `internal/platform/config`).
5. `make gen` (схемы операций и ошибок — в `openapi.json`), затем готово по CLAUDE.md § Готово.
6. Handoff: новый или изменённый env, дефолт, таблица или требование к окружению из аннотаций —
   блок `/infra-handoff` тем же коммитом; ничего такого нет — блок не нужен.
7. Коммит `feat(deps): golibinfra vX.Y.Z — <главное>` (`!` — если меняется API или события
   сервиса), в теле — пункты `Breaking`, задевшие код, и строка `Review:` (CLAUDE.md § Готово).

## Обвязка шаблона

Remote шаблона без его тегов заводит финал `/init-service`; нет его (`make doctor`: «remote template не
заведён») — заведи: теги релизов шаблона `vX.Y.Z` смешались бы с тегами форка, теги обвязки
`template/*` приносит `make template-update`.

```bash
git remote add --no-tags template git@git.stit.tech:stit-core/go-backend-template.git
```

1. `make template-update` — версия форка, теги шаблона новее неё с аннотациями и golibinfra
   последнего тега; дерево не меняется. Формат аннотации — `docs/contributing.md § Версия обвязки`.
2. Пункты `Безопасность` и `Платформа` всех тегов до `TO` реши до переноса: после него версия форка — `TO`,
   и код этого интервала движок уже не возьмёт. Нужный код — тем же переносом, `PATHS='<пути из аннотаций>'`
   (каталог, файл или `:(exclude)<путь>`), ненужный — `PATHS=-`; без решения движок отказывает и называет
   пункты, путь, которого нет ни в теге, ни в базе, или который исключает `.template/paths`, — тоже отказ.
   Фикс `Безопасность` у форка со своей идентичностью отклоняют только с причиной.
3. `make template-update TO=template/vX.Y.Z PATHS=…` на чистом дереве — трёхстороннее слияние путей
   шаблона до тега: первый перенос — от коммита шаблона, из которого создан форк (по дереву первого
   коммита форка), следующие — от `.template/version`. golibinfra форка ниже, чем у тега, — после переноса
   в том же дереве шаги 1–4 и 6 раздела golibinfra: код и обвязка тега рассчитаны на его golibinfra, гейт и
   коммит — один (шаги 5–6 ниже). Итог — в индексе (`git diff --cached`), `.template/version` уже сдвинут:
   - **конфликты** (`git status` — `UU`, `AA`): шаблон и форк правили одно место — сведи обе правки
     по смыслу и `git add`; забытый маркер конфликта роняет `make check` (archtest
     `TestNoConflictMarkers`);
   - **нет в форке — не тронуты**: файл, который форк удалил, а шаблон правил; правка нужна — верни
     файл (`git show template/vX.Y.Z:<путь>`, в Go-файле — module path форка);
   - **шаблон удалил, форк менял**: `git rm` или оставь как файл форка;
   - **`internal/archtest/layout_settings_test.go`** — настройки archtest форка (`layout` — `archkit`, `rootLibs` и
     `uncheckedPackages` — что видят корень домена и адаптер `module/`, `releasedSubjects` — выпущенные `.v1`,
     `deliveryKits` — киты входа), перенос его не трогает: без него или без новой настройки пакет `internal/archtest`
     не собирается — возьми образец
     (`git show template/vX.Y.Z:internal/archtest/layout_settings_test.go`) и сведи со своей раскладкой.
     Нарушения, которые `TestLayout` нашёл в коде форка, — правкой кода или записью
     `archkit.WithKnownViolations` с ID находки (перечень проверок — README `archkit`);
   - **миграция шаблона на номере, занятом миграцией форка** (migrations-check: «ровно одна пара») —
     следующим свободным номером форка (`git mv`), строка `.template/identity` — под новое имя; свою
     выпущенную миграцию не переименовывай: прод накатил бы её второй раз;
   - **мутации**: спецификация `scripts/tools/mutants.txt` — код форка, перенос её не трогает: блоки кода
     `PATHS` — из `git show template/vX.Y.Z:scripts/tools/mutants.txt`; спецификации нет — затравка оттуда
     же (с внешней идентичностью — без блоков её путей, фильтр — шаг «Генерация и мутации»
     `.claude/skills/init-service/references/prune.md`); `make mutate A=--check` → OK.
4. Шаги `Breaking` и `Гейт` из аннотаций всех тегов до `TO`. Блоки INFRA-HANDOFF шаблона за перенесённое —
   из `git show template/vX.Y.Z:.claude/plans/handoff.md` в свой накопитель (skill `/infra-handoff`) с
   номерами миграций и именами форка.
5. `make gen`, затем готово по CLAUDE.md § Готово: перенесённый код входа и сессий — ещё `/security-review`.
6. Коммит `chore(template): обвязка template/vX.Y.Z`, в теле — что форк правил руками, строка на каждый
   пункт `Безопасность` и `Платформа` всех тегов до `TO` — `перенесено: <пункт> (PATHS)` или
   `отклонено: <пункт> — <причина>` (движок печатает пункты после переноса), строка `Review:` (её требует
   хук `commit-msg`); с bump golibinfra — ещё пункты его `Breaking`, задевшие код.

Прервать перенос до коммита — `git restore --source=HEAD --staged --worktree -- .`: дерево и версия
прежние, добавленные файлы уходят (`git reset --hard` и `git clean` — в deny); маркер конфликта в `Makefile`
ломает любую цель `make` — сначала сведи его. Закоммиченный перенос — `git revert`.

## Первый перенос движком тега

`PATHS` знают `make template-update` и движок с `template/v1.2.0`. Форк без `.template/` (создан до неё) и
форк на template/v1.0.0–v1.1.0 переносят первый раз движком нужного тега, решение по коду (шаг 2 обвязки) —
третьим аргументом; база — `.template/version`, у форка без неё — коммит шаблона с деревом первого коммита
форка (движок находит сам), `.template/` приходит тем же переносом. Итог — как в шаге 3 обвязки, дальше —
шаги 4–6:

```bash
git remote add --no-tags template git@git.stit.tech:stit-core/go-backend-template.git   # если его нет
git fetch --no-tags template 'refs/tags/template/*:refs/tags/template/*'
bash <(git show template/vX.Y.Z:scripts/tools/template-update.sh) template/vX.Y.Z "" '<пути или ->'
# «базы нет» — форк правили до первого коммита: база — шаблон на дату первого коммита форка
git fetch template
bash <(git show template/vX.Y.Z:scripts/tools/template-update.sh) template/vX.Y.Z \
    "$(git rev-list -1 --before="$(git log --reverse --format=%cI | head -1)" template/main)" '<пути или ->'
```

## Anti-patterns

- `git fetch template --tags` или remote без `--no-tags`: теги релизов шаблона смешаются с тегами форка.
- Копировать файлы шаблона поверх своих — правки форка пропадут молча; перенос — `make template-update`.
- Коммит обвязки тега без golibinfra, на которую она рассчитана: перенос и bump — одним заходом.
- Bump golibinfra через несколько тегов по одной последней аннотации.
- `git add` файла с маркерами конфликта.
- Пункт `Безопасность` или `Платформа` без строки в теле коммита: отказ от фикса не оставляет следа.
