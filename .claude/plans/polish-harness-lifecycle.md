---
status: in-progress
parent: —
blocked-by: —
---

# polish-harness-lifecycle — обвязка и жизненный цикл форка без хвостов (S7)

## Цель
Закрыть находки переаудита углов «Обвязка» и «Жизненный цикл» (NHAR-01–04, NLIF-01–05), их minor из бэклога
(`minor-harness`, `minor-lifecycle`, RENF-12) и трения догфудинга T1, T2, T4, T5, T8, T10 — чтобы шаблон
`template/v1.2.0` выходил без хвостов, а форк доходил до зелёного гейта по тексту. Плюс черновик аннотации
тега для S8.

## Контекст
- Программа `reaudit-to-nine`, шаг S7, трек harness+lifecycle; ветка `nine/s7-harness`, worktree
  `N/wt-s7h` от `main` `a4020ee`. `N` = `/home/mm/.cache/stit-audit-program/nine`, `R` =
  `/home/mm/.cache/stit-audit-program/reaudit`. Песочница и логи — `N/s7h/`.
- Находки: `N/reaudit/template-lifecycle.md` (NLIF), `N/reaudit/template-harness.md` (NHAR),
  `R/template-harness.md` (RHAR-04–13), `R/template-lifecycle.md` (RLIF-06–13), `R/template-enforcement.md`
  (RENF-12), `N/dogfood.md` (T1, T2, T4, T5, T8, T10).
- Владеет: `CLAUDE.md` кроме § Команды; `docs/README.md`, `CONTRIBUTING.md`, `docs/contributing.md`,
  `docs/reference/{ecosystem,release}.md`, `docs/stack/**`, `.template/**`; скиллы `smartdev`,
  `triage-failure`, `plan`, `init-service`, `de-template`, `upgrade`, `release`, `stack-deep-dive`,
  `infra-handoff`, `scaffold/SKILL.md`, `scaffold/references/domain.md`; `scripts/release*.sh`,
  `scripts/tools/{template-update,doctor,status,validate-stack-doc,validate-plan}.sh`, цели `setup`,
  `template-update`, `release`; `internal/archtest/{harness,commitmsg,guard,template,onboarding}_test.go`,
  `scripts/git-hooks/commit-msg`; rules `infra-impact`, `forward-only`, `comments`, `env-local`.
- Параллельно — треки S7 gate+tests и code+security+adapters: чужие файлы — минимально, строкой в «Решения».
- Гейт базы `a4020ee` в этом worktree — `✓ check clean`, 795/795, 2 мин 50 с (`N/s7h/check-base-a4020ee.log`).

## Не входит
- `go.mod`/`go.sum`, `pkg/policy`, `internal/platform/authgate/`, существующие миграции, `backlog.md` и план
  программы (их правит дирижёр), чужие репо; флак `TestIdentity` (T4, сам тест) — другой трек.
- Тег `template/v1.2.0` и push — S8 с OK владельца; здесь — только черновик аннотации вне репо.

## Критерии приёмки
- [ ] **AC1** — Перенос обвязки не теряет код форка молча (NLIF-01, NLIF-04): при пунктах «Безопасность» или
  «Платформа» в аннотациях интервала перенос без `PATHS` — отказ с этими пунктами, `PATHS=-` — явное «ничего»;
  путь под `:(exclude)` шаблона в `PATHS` — отказ; `/upgrade` даёт первый перенос с PATHS форку без `.template/`
  и на template/v1.0.0–v1.1.0. Проверка: `go test -run 'TestTemplateUpdate' ./internal/archtest/` → ok; опыт в
  `N/s7h` (форк, тег с «Безопасность»): движок базы — перенос без кода и без слова, движок ветки — отказ;
  `PATHS=scripts/tools/mutants.txt`: база — тишина, ветка — отказ
- [ ] **AC2** — Prune по тексту `prune.md` на ветке доходит до `✓ check clean` и `make mutate A=--check` → OK,
  без догадок (NLIF-03, T5, RLIF-09, env лимита в `test/stack/compose.yml`). Проверка: клон-форк в `N/s7h`,
  `identity: external`, шаги буквально: на базе — красный `TestProseReferencesExist` (`cmd/api/http_test.go`),
  на ветке — зелёный
- [ ] **AC3** — Самопроверка обвязки видит то, что обещает, и называет верную причину (NHAR-02–04, T8, RHAR-04,
  сломанные шапки). Проверка: порчи в worktree на базе и ветке — своё правило с мёртвым glob у external без тегов,
  `disableAllHooks` в `.claude/settings.local.json`, `infra-handoff` без шага 0, заголовок `§`-якоря и цель
  markdown-ссылки переименованы, тип коммита в прозе, шаг гейта в прозе, `paths: [**/*.go]`: база — зелёный или
  не та причина, ветка — красный с верной; неотслеживаемый файл из прозы — подсказка `git add`
- [ ] **AC4** — `make status` зовёт INFRA-HANDOFF по существу (RENF-12, T10, RHAR-07): комментарий в
  `.env.example` и правка невыпущенной миграции — `none`, `Dockerfile` и переименование выпущенной миграции —
  infra-touch. Проверка: сценарии в worktree на базе и ветке → база шумит или молчит, ветка — как сказано
- [ ] **AC5** — Проза ведёт без догадок (T1, T2, T4, NHAR-01, RHAR-05, 06, 09, 11, 12, RLIF-06–08, 10–13,
  NLIF-02): задача юзера при чужом активном плане, след ревью агента без собеседника, флак вне своего диффа,
  job, релиз без TTY и push тега, порядок bump и переноса, блоки накопителя шаблона, прерывание переноса, номер
  миграции шаблона. Проверка: чтение по таблице находок в «Решениях»; `go test ./internal/archtest/` → ok
- [ ] **AC6** — Объём и акценты прозы не растут (RHAR-10, HAR-12): `stack-deep-dive` — минус треть строк, ячейки
  `contributing.md` — списком, жирного и ❌ меньше. Проверка: `wc -l`, счёт `**…**` и ❌ на базе и ветке
- [ ] **AC7** — Черновик аннотации `template/v1.2.0` доводит форк до зелёного гейта (NLIF-05). Проверка:
  `N/annotation-template-v1.2.0.md` с разделами; пробный тег из ветки, форк с template/v1.1.0 по командам
  аннотации → `✓ check clean`

## Шаги
- [x] **S1** — План трека
  - Проверка: `make plan-check` → OK · Закрывает: — · Зависит от: —
  - Сделано: план; `make plan-check` → OK, гейт — `✓ check clean`
- [x] **S2** — Перенос обвязки: решение по коду форка, исключённые пути, база переноса; `/upgrade` — первый
  перенос с PATHS, затравка и блоки мутаций, номер миграции, порядок с golibinfra, накопитель шаблона,
  прерывание, след ревью (NLIF-01, 02, 04; NHAR-04; RLIF-07, 08, 10, 12, 13)
  - Проверка: `go test -run 'TestTemplateUpdate' ./internal/archtest/` → ok; опыт AC1; `make check` → `✓ check clean`
    · Закрывает: AC1 · Зависит от: S1
  - Сделано: RED — `TestTemplateUpdatePaths` (перенос без PATHS шёл молча), `TestTemplateUpdateFirstBase` (нет строк
    о базе), `N/s7h/s2-red.log`; GREEN; опыт AC1 (`N/s7h/bin/exp-ac1.sh`, `N/s7h/exp-ac1-s2.log`): движок базы —
    перенос без кода и слова, `mutants.txt` из PATHS молча отброшен; ветки — отказ с пунктом и с путём
    `:(exclude)`, `PATHS=-` — обвязка и пункты для тела коммита; `make check` → `✓ check clean`
- [x] **S3** — Prune и онбординг: файлы prune — в `.template/identity`, раздел своих доменов, клей идентичности,
  env e2e-стека, фильтр мутаций; образцы после prune — история форка, одно место; de-template A.1–A.2; канон
  без артефактов стека (NLIF-03, T5, RLIF-06, 09, 13, RHAR-12)
  - Проверка: `go test ./internal/archtest/` → ok; `make check` → `✓ check clean` · Закрывает: AC2 · Зависит от: S2
  - Сделано: опыт AC2 (`N/s7h/bin/{mkfork,onboard}.sh`, `prune.py` — шаги prune.md как есть, команды — из её
    блоков): база — красный `TestProseReferencesExist` (`prune.md: cmd/api/http_test.go`); ветка, форк
    `git.stit.tech/zeta/shop3` с доменом `product` → фильтр мутаций 356 → 61, `A=--check` называет 3 блока шагов
    2–3 → 58 OK, `make check` → `✓ check clean` 136/136 (`N/s7h/check-prune-shop3-s3.log`), `make e2e-smoke` — 1
    passed; A.1 базы — 47
    неотформатированных файлов, с `make fmt` — 0; `make new-domain` с этим module path: база — 7 файлов fmt-check,
    ветка — чисто; `make check` → `✓ check clean`
- [x] **S4** — Самопроверка обвязки: пропуск external без тегов, `settings.local.json`, шаг 0 `infra-handoff`,
  подсказка `git add`, разбор сломанных шапок, якоря `§`, markdown-ссылки, типы коммитов и состав гейта в прозе
  (NHAR-02, 03, 04, T8, RHAR-04)
  - Проверка: порчи AC3 на базе и ветке; `make check` → `✓ check clean` · Закрывает: AC3 · Зависит от: S3
  - Сделано: порчи `N/s7h/bin/exp-ac3.py` (worktree `1428558` и он же с патчем S4): C1–C13 на базе — зелёные
    или с не той причиной (C3 без подсказки, C4 «правило всегда»), на ветке — красные с верной; законные P1, P2 —
    зелёные (`N/s7h/exp-ac3-{base,branch}.log`); NHAR-02 — форк external без тегов (`exp-nhar02.sh`): своё правило
    с мёртвым glob и тестом — база зелёная, ветка красная; весь archtest в пруненом форке с S4 — зелёный;
    `make check` → `✓ check clean`
- [ ] **S5** — Маршрут, ревью, разбор отказов, релиз: T1, T2, T4, NHAR-01, хук — `Review:` у `chore(template)` и
  перенос чужого коммита (RENF-11, часть хука), RHAR-05 (операции, права, вариант job), 06, 09, 11, 12, RLIF-11
  - Проверка: `go test -run 'TestCommitMsgHook|TestProseReferencesExist|TestRulesAndSkillsLoad' ./internal/archtest/`
    → ok; `make check` → `✓ check clean` · Закрывает: AC5 · Зависит от: S4
- [ ] **S6** — Инфра-поверхность: `Dockerfile*` в правиле и сигнале, список поверхности — в одном месте, сигнал
  `make status` по существу (RHAR-07, RENF-12, T10)
  - Проверка: сценарии AC4; `make check` → `✓ check clean` · Закрывает: AC4 · Зависит от: S5
- [ ] **S7** — Объём и акценты: `stack-deep-dive` без дублей, ячейки `contributing.md` списком, акценты прозы;
  `domain-mapping.md:80` (RHAR-10, HAR-12)
  - Проверка: счёт AC6; `make check` → `✓ check clean` · Закрывает: AC6 · Зависит от: S6
- [ ] **S8** — Приёмка: prune целиком на ветке, черновик аннотации и пробный перенос форка, повтор опытов AC1–AC4
  - Проверка: AC2, AC7; `make check` → `✓ check clean` · Закрывает: AC2, AC7 · Зависит от: S7

## Решения
- NLIF-01: движок не только печатает пункты «Безопасность» и «Платформа» интервала, а требует решения до переноса
  (`PATHS=…` или `PATHS=-`) — после переноса код интервала движку не взять, тишина и есть потеря; пункты он
  печатает и после переноса — для строк «перенесено/отклонено» тела коммита.
- RLIF-07: при golibinfra форка ниже тега — сначала перенос (движку нужно чистое дерево), затем шаги golibinfra в
  том же дереве, гейт и коммит — один: код и обвязка тега рассчитаны на его golibinfra.
- NLIF-03: `cmd/api/http_test.go` (тест только строгого лимита `/api/auth/*`) — в `.template/identity`: шаг 1 prune
  снимает его, проза называет лишь то, что prune оставляет.
- T5: пути своих доменов поверх идентичности — в своём разделе `prune.md`: строки форка там не спорят с правками
  шаблона в остальном тексте при переносе.
- RLIF-06, RHAR-12: правило образцов после prune — одно место, `CLAUDE.md § Проектирование первично`; первый
  источник — история форка до prune (его module path, его дерево), тег — запасной.
- RLIF-09: клей идентичности после prune — хэшер, `PasswordStrong`, описание схемы Cookie — снимает шаг 3;
  построители cookie и `COOKIE_SECURE` остаются: это контракт платформы, его держит prod-гард конфигурации.
  `scripts/new-domain.sh` (файл трека gate) — одна строка: `gofumpt` и по каталогу среза.
- Шаг 9 prune — `make fmt` первым: ручные правки шагов 2–3 оставляют импорты и пустые строки (опыт: fmt-check
  красный без него).
- RLIF-13: в каноне archtest не пускает артефакты стека целиком, а не только `identity` в `auth.md`.
- NHAR-02: файл шаблона у форка external — путь из тега `template/v*` или из первого коммита репо (форк создан
  из шаблона, теги приносит первый перенос): своя проза форка проверяется и до первого переноса.
- NHAR-03: `TestGuardWired` читает и `.claude/settings.local.json`, если он есть: `make check` в дереве разработчика
  краснеет, pre-push в свежем worktree — нет (файла там нет); выключить барьер — правка гейта, как и в
  `settings.json`.
- RHAR-04: якоря, ссылки, состав и порог гейта — расширение `TestProseReferencesExist`, типы коммитов —
  `TestCommitMsgHook` (`TYPES` хука — одно место в нём): новых тестов в перечне комментария пакета нет.
  Якорь годен, если заголовок или жирная метка пункта начинается с имени раздела: проза пишет и «§ Стиль» на
  «Стиль кода (инварианты)». Файл, имя которого в репо не одно (`README.md`, `SKILL.md` без пути), — вне проверки.
- Сломанные шапки: второй разбор Claude Code берёт в кавычки и закрытый flow-список, который YAML не разбирает
  (`[**/*.go]` — glob-класс одного символа, правило не грузится никогда); шапка, которую клиент не разобрал ни
  разу, — свой отказ (`name:`, `description:` и `paths:` потеряны), а не «правило без paths:».
- NLIF-02: шаблонная миграция на занятом номере — следующим свободным номером форка, `.template/identity` форк
  правит сам; конфликт следующего переноса по этой строке — настоящий. Отдельный источник миграций — Q1.
- RHAR-13 (`pkg/util` проходит `TestLayout`) — правило раскладки держит archkit golibinfra: перечень пакетов
  `pkg/` — опция кита, а не второй механизм раскладки в шаблоне; пункт — в «Другие репозитории» бэклога (Handoff).

## Открытые вопросы
- [ ] **Q1** — NLIF-02 глубже: миграции доменов идентичности — отдельным источником `cmd/migrate` со своей
  таблицей версий (как kit-таблицы), чтобы их номера не пересекались с миграциями форка? Меняет `cmd/migrate`,
  `migrations/` и выкатку — блокирует: — · к кому: владелец

## Handoff
- Дирижёру в бэклог, раздел «Другие репозитории — сшивка владельцем» (NHAR-01): «хаб: приёмка фазы сервиса —
  ревью диффа (`/code-review`, вход и права — `/security-review`); итог — строкой в «Сделано» фазы. Триггер:
  сшивка экосистемы».
- Дирижёру в бэклог, тот же раздел (RHAR-13): «golibinfra-archkit-pkg — archkit: перечень пакетов `pkg/`
  сервиса опцией (новый пакет — правка настроек, видимая в диффе). Триггер: сшивка экосистемы».
- Черновик аннотации `template/v1.2.0` — `N/annotation-template-v1.2.0.md` (S8 программы).

## Где мы сейчас
S4 закрыт: самопроверка ловит якоря, ссылки, факты гейта, локальное выключение хуков и сломанные шапки. Дальше —
S5: маршрут и ревью (T1, T2, T4, NHAR-01, RENF-11 в хуке, RHAR-05/06/09/11/12, RLIF-11). Experiment-worktree
`N/wt-s7h-base` (`1428558`) и `N/wt-s7h-br` — снять в конце трека.
