---
status: in-progress
parent: —
blocked-by: —
---

# speed-up-mutate — make mutate в разы быстрее без потери проверки и выпуск template/v1.3.0

## Цель
Полный мутационный прогон шаблона идёт около двух часов: каждая мутация гоняет `internal/archtest`, линкует программы
`cmd/`, поднимает контейнеры integration на тест и ждёт соседей по одной. Раннер, который проект на шаблонах stit
довёл на своих сервисах (полная спецификация сервиса на 383 мутации — 1 ч 50 мин → 10,5 мин при тех же вердиктах),
перенести сюда без проектного, закрыть найденное ревью и выпустить `template/v1.3.0`: форки берут его через `/upgrade`.

## Контекст
- Раннер — `scripts/tools/mutate.py`, тесты раннера — archtest `mutate_test.go` (игрушечные модули, python3
  настоящий); правило — `.claude/rules/tests-pipeline.md` § Мутации. База раннера в проекте — тот же
  `template/v1.2.0`: `mutate.py` и `mutate_test.go` до его правок совпадают с этим репо побайтно.
- Что переносится: стадии (archtest последним, после integration; с тегом integration при тех же файлах — один раз),
  сборка без линковки (`go list -export`, линковка — только задетой программы: `api`, `migrate`, `seed-admin`),
  серверы integration один раз на прогон (общий режим testkit golibinfra `v1.7.0`, `TESTKIT_SHARED`), исполнители в
  своих worktree, ключ результата (блок и его файлы) с переиспользованием убитых, прогон по снимку рабочего дерева,
  `A=--build`, `A=--anchors`, `A=--check` (якоря и сборка), `A=--fresh`.
- Метка контейнеров прогона — нейтральная: `mutate.run=<прогон>`, `mutate.repo=<репо>`.
- Дефект archtest, найденный там же: `callerEnv` пропускает `PKG`, `FORMAT`, `VERBOSE`, `A` вызывающего — под
  `make test PKG=./internal/archtest/...` `TestLintUsesItsConfig` красный.
- Машина общая: прогон мутаций — в очереди `flock` с прогонами соседних сессий.

## Не входит
- Правки шаблона проекта вне раннера (трения сервисов) — следующий план.
- Улучшения раннера, которые ревью назвало и которые требуют замера, — бэклог (`mutate-*`).

## Критерии приёмки
- [x] **AC1** — Раннер: стадии и сборка без линковки, серверы integration на прогон с уборкой (в конце, при SIGTERM,
  оставленные, с анонимными томами), исполнители в своих worktree с тем же итогом и порядком строк, ключ результата
  (в счёт — последняя строка мутации), снимок рабочего дерева; `--strict` берёт только вердикты того же снимка.
  Проверка: `go test -run 'TestMutate' ./internal/archtest/` → PASS (`TestMutate`, `TestMutateStages`,
  `TestMutateShared`, `TestMutateParallel`)
  - Подтверждено: `TestMutate`, `TestMutateStages`, `TestMutateShared`, `TestMutateParallel` — PASS
- [ ] **AC2** — Прогон спецификации шаблона на новом раннере: базовый прогон зелёный, общие серверы — одним набором,
  исполнителей — по ядрам и памяти; итог и время — в «Сделано». Проверка: `make mutate A=--strict` → код 0;
  `docker ps -a --filter label=mutate.repo` после прогона → пусто
- [x] **AC3** — `make test PKG=./internal/archtest/...` → PASS: аргументы Makefile вызывающего до временных репо
  тестов не доходят. Проверка: `TestCallerEnvDropsMakeArgs` → PASS, `make test PKG=./internal/archtest/...` → PASS
  - Подтверждено: `TestCallerEnvDropsMakeArgs` — PASS; `make test PKG=./internal/archtest/...` — PASS
- [x] **AC4** — golibinfra `v1.7.0`. Проверка: `grep golibinfra go.mod` → `v1.7.0`
  - Подтверждено: `go.mod` — `git.stit.tech/stit-core/golibinfra v1.7.0`
- [ ] **AC5** — Выпуск `template/v1.3.0`: аннотация по `docs/contributing.md` § Версия обвязки, тег в origin и
  github. Проверка: `git ls-remote origin refs/tags/template/v1.3.0` → тег

## Шаги
- [x] **S1** — archtest: `callerEnv`, `runEnv` и песочница релиза — без аргументов Makefile вызывающего
  - Проверка: `TestCallerEnvDropsMakeArgs` → PASS (до фикса — FAIL) · Закрывает: AC3
  - Сделано: `makeArgs` — аргументы Makefile, которые он берёт у вызывающего; их снимают `callerEnv`, `runEnv` и
    песочница релиза; `TestCallerEnvDropsMakeArgs` (перечень — из самого Makefile) — до фикса FAIL, после — PASS
- [x] **S2** — golibinfra `v1.7.0`
  - Проверка: `make check` → `✓ check clean` · Закрывает: AC4
  - Сделано: golibinfra `v1.6.0` → `v1.7.0` (общий режим testkit, образ MinIO — Silo, ожидание потока natskit,
    compress v1.20.2); в граф тестов — minio-go и nats.go через testkit; `make check` — `✓ check clean`
- [x] **S3** — Раннер мутаций: стадии, сборка, серверы на прогон, исполнители, ключ результата, снимок; находки ревью;
  правило, скиллы, Makefile, шапка спецификации, `.dockerignore`
  - Проверка: `go test -run TestMutate ./internal/archtest/` → PASS · Закрывает: AC1 · Зависит от: S2
  - Сделано: `mutate.py` и `mutate_test.go` раннера проекта (метки `mutate.run`, `mutate.repo`) и находки ревью
    (`/code-review` max, добор): вердикт — по последней строке, `--strict` — вердикты того же снимка без несобравшихся и
    таймаутов, `rm -f -v`, уборка при втором сигнале, сброс worktree, `-f -f`, `GOWORK=off`, проба мимо прокси,
    `-skip ^TestMutate`; тест на каждый фикс — без фикса FAIL; правило, скиллы, Makefile, шапка спецификации,
    `.dockerignore`; `make check` — `✓ check clean`
- [ ] **S4** — Строгий прогон спецификации шаблона, время и ресурсы
  - Проверка: `make mutate A=--strict` → код 0 · Закрывает: AC2 · Зависит от: S3
- [ ] **S5** — Гейт и выпуск `template/v1.3.0`: `.template/version`, аннотация, тег, push
  - Проверка: `make check` → `✓ check clean`; тег в origin и github · Закрывает: AC5 · Зависит от: S1–S4

## Решения
- `--strict` (шаг релиза) берёт только вердикты прогонов того же снимка (дерево — шестая колонка `results.tsv`), кроме
  несобравшихся и таймаутов (их даёт и окружение): ключ результата не видит теста, ослабленного в другом файле, и релиз гоняет мутации заново, как раннер
  по HEAD; оборванный строгий прогон продолжается с места. Dev-прогон — по ключу, в счёт — последняя строка мутации.
- Стадии мутации пропускают тесты раннера (`-skip ^TestMutate` у `internal/archtest`): они гоняют `mutate.py` на
  игрушечных модулях, правка кода сервиса их вердикт не меняет, а каждый — десятки прогонов раннера; их держит гейт.
- Уборка серверов прогона — `docker rm -f -v` (образы Postgres, Redis и Silo объявляют тома), по id и по метке
  прогона, её не обрывает второй сигнал; `git worktree add/remove -f -f` — запись оборванного создания под замком;
  `GOWORK=off` — go.work разработчика в корне; проба MinIO — мимо `http_proxy`. Проверка «демон docker не на этой
  машине» по `DOCKER_HOST` снята: локальный демон по tcp (WSL2) и `TESTCONTAINERS_HOST_OVERRIDE` (colima) она
  выключала зря, а удалённый контекст docker не видит — бэклог `mutate-remote-docker`.

## Handoff
—

## Где мы сейчас
S1–S3 закрыты; прогон S4 на раннере до двух правок добора остановлен на 340 из 429 мутаций (20 мин, 5 исполнителей)
— дальше S4 на HEAD, затем S5 (гейт и выпуск).
