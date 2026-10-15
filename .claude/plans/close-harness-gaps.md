---
status: in-progress
parent: —
blocked-by: —
---

# close-harness-gaps — след ревью, самопроверка доставки правил, правда прозы обвязки

## Цель
Закрыть пункты трека harness программы `reaudit-to-nine`: у ревью из «Готово» есть исполнитель и след
в истории (RHAR-01), самопроверка прозы видит имена тестов (RHAR-02) и механику доставки правил и
барьеров (RHAR-03), неточные утверждения скиллов поправлены (RHAR-08), поля нового домена идут одним
способом (трение T6), а `/release` гоняет мутационный прогон.

## Контекст
- Программа — `.claude/plans/reaudit-to-nine.md` (трек harness, волна 2): владение файлами, решения
  владельца (RHAR-01, RTSTT-01), метод. Ветка `nine/harness`, worktree `N/wt-harness` от `f825fac`,
  тяжёлые прогоны — под `flock N/gate.lock` (`N` = `/home/mm/.cache/stit-audit-program/nine`).
- Находки — `### RHAR-01`, `02`, `03`, `08` в `R/template-harness.md` (`R` =
  `/home/mm/.cache/stit-audit-program/reaudit`); мутации прозы аудитора M07, M17–M19, M30, M31 —
  его § 11. Трение T6 — `/home/mm/.cache/stit-audit-program/dog/progress.md:40`.
- Эксперименты — `N/harness/exp.py <worktree>`: порча файла по тексту → `go test ./internal/archtest/`
  (поймано — красный), сообщения хуку `commit-msg`, сверки текста, `make new-domain NAME=repo`.
  База `f825fac` — `N/harness/base-exp-f825fac.log`: ни одна порча не поймана, хук принимает все
  сообщения, сверки G01, G02, G08 — «нет», G03–G07 — «да», `repo` — отказ до правки.
- Хуки worktree — хуки основного дерева (в конфиге репо путь к ним абсолютный): `Review:` хук ветки
  потребует после слияния; коммиты `feat` и `fix` трека несут строку и до того.
- Валидатор трека — FAIL на `6ebf15f`: B1, B2, N1–N11 (`N/validation-harness.md`, раннеры `N/val-harness/`);
  круг фиксов — S8–S20, по коммиту на пункт. Кейсы круга — `N/harness/fix_cases.py <worktree>`.
- Параллельно идёт трек boundaries: `internal/archtest/archtest_test.go` (и комментарий пакета), rule
  `domain-architecture`, `docs/reference/architecture.md`, `scaffold/references/{port,endpoint,event}.md`
  здесь не правятся.

## Не входит
- RHAR-08, п. 5 — «файл > ~400 строк разбивается» стоит в rule `domain-architecture` и
  `docs/reference/architecture.md` (файлы трека boundaries): правка — дирижёру при слиянии (Handoff).
- Мелочь рядом с RHAR-08: `docs/reference/domain-mapping.md:80` (типизированный ID — совет, а не
  утверждение о коде); RHAR-04–07, 09–13 — бэклог `minor-harness`.
- Символы китов и имена env в прозе (мутации M13, M14 — вне заявленного объёма самопроверки).

## Критерии приёмки
- [x] **AC1** — RHAR-01: `CLAUDE.md § Готово` называет, кто проводит ревью (принимающая сторона), что
  пишет агент без собеседника, след — строку `Review:` — и права в списке `/security-review`; хук
  `commit-msg` отвергает `feat` и `fix` без строки `Review:` в теле. Проверка: `exp.py <wt> H01 … H10 G`
  → на базе H01–H10 «принято», G01, G02 «нет»; на ветке H01–H06 «отказ», H07–H10 «принято», G01, G02
  «да»; `go test -run TestCommitMsgHook ./internal/archtest/` → PASS
  - Подтверждено: база — H01–H10 «принято», G01, G02 «нет»; ветка — H01–H06 «отказ», H07–H10
    «принято», G01, G02 «да»; `TestCommitMsgHook` → PASS (66 случаев), на хуке базы — 7 случаев Review
    красные; настоящий `git commit` с хуком ветки: `feat` без строки — отказ, `-m … -m 'Review: …'` и
    `-F -` с трейлерами — коммит
- [x] **AC2** — RHAR-02: имя теста в коде прозы, которого нет функцией `*_test.go` репо, — отказ
  `TestProseReferencesExist`. Проверка: `exp.py` M07, M07b, M07c → на базе «не поймано», на ветке
  «поймано» этим тестом; CTRL — зелёный
  - Подтверждено: база — M07, M07b, M07c «не поймано»; ветка — «поймано» `TestProseReferencesExist`
    («….md: TestHTTPTypez: the prose points to a missing … test»), CTRL — зелёный
- [x] **AC3** — RHAR-03: glob `paths:`, который ни на что не ложится, правило без `paths:`, `name:`
  скилла не по каталогу, reference scaffold без «Шаг 0», снятый deny `.claude/settings.json`, отвязанный,
  подменённый или выключенный хук guard — отказ archtest. Проверка: `exp.py` M17, M17b, M17c, M18, M19,
  M30, M30b, M31, M31b, M31c → на базе «не поймано», на ветке «поймано»; CTRL — зелёный
  - Подтверждено: база — все десять «не поймано»; ветка — M17–M19 ловит `TestRulesAndSkillsLoad`, M30–M31c —
    `TestGuardWired`, CTRL — зелёный; matcher `Bash|Write` — зелёный (регулярное выражение с Bash); проба
    проекта с внешней идентичностью (пути `.template/identity` удалены, `identity: external`) — новые тесты
    зелёные, с `identity: own` — 9 пустых glob'ов
- [x] **AC4** — RHAR-08: утверждения п. 1, 2, 4 (smartdev, triage-failure, stack-deep-dive) и заголовки
  smartdev с «(фикс …)» поправлены; п. 3 правда (`make new-domain NAME=repo` — отказ до правки);
  п. 5 — в Handoff. Проверка: `exp.py` G03–G06 → на базе «да», на ветке «нет»; N01 → «отказ до правки»
  - Подтверждено: база — G03–G06 «да», ветка — «нет»; N01 — «отказ до правки» на базе и ветке (п. 3
    закрыл трек gate: имена из кода среза, `TestNewDomainRefusesOrRollsBack`); `ATTENTION:` печатает
    только `status.sh` — hooks, build, sqlc-drift, migrations, infra-touch, lint (`A=--full`), тестов он
    не гоняет; enum env — «где живёт» (`field-checklists.md`), секции `Architecture notes` у шаблонов
    `docs/stack/_templates/` нет
- [x] **AC5** — T6: поля нового домена — одним способом в `scaffold/references/domain.md § 4` и в выводе
  `make new-domain`. Проверка: `exp.py` G07 → на базе «да», на ветке «нет»; текст обоих мест
  - Подтверждено: база — G07 «да», ветка — «нет»; `make new-domain NAME=order` (временный worktree)
    печатает «Домен — по артефакту strategy: …/domain.md § 4 (поля — в миграцию среза до первого
    коммита; …)», § 4 и «Что не делать» domain.md говорят то же
- [x] **AC6** — `/release`: шаг `make mutate A=--strict` перед релизом; на коде ветки он даёт код 0.
  Проверка: `exp.py` G08 → на ветке «да»; `flock … make mutate A=--strict` → `EXIT 0`
  (`N/harness/mutate-strict-*.log`)
  - Подтверждено: G08 — на базе «нет», на ветке «да»; `make mutate A=--strict` на `f825fac` (код под
    мутациями тот же, что на ветке): 356 мутаций, убиты 346 (юнит 275, integration 71), выжили 10 известных
    эквивалентных, «спецификация совпадает с кодом», `EXIT 0` — прогон прерван на 101 и продолжен с места
    (`mutate-strict-f825fac.log`, `-2.log`), около 70 минут работы

## Шаги
- [x] **S1** — RHAR-01: «Готово» — ревью у принимающей стороны и след `Review:`; хук `commit-msg` и
  `TestCommitMsgHook`; проза о коммитах (CLAUDE.md § Git workflow, `CONTRIBUTING.md`, scaffold, triage)
  - Проверка: `exp.py` H, G01, G02; `make check` → `✓ check clean` · Закрывает: AC1 · Зависит от: —
  - Сделано: «Готово» — ревью проводит принимающая сторона (юзер, хаб, дирижёр), агент без собеседника
    коммитит и отдаёт ей дифф, след — `Review:` в теле или в «Сделано», права — в списке
    `/security-review`; хук отвергает `feat`/`fix` без строки `Review: <текст>` в начале строки тела;
    `exp.py` и `TestCommitMsgHook` — как в AC1; `make check` → `✓ check clean`
- [x] **S2** — RHAR-02: самопроверка прозы сверяет имена тестов с функциями `*_test.go`
  - Проверка: `exp.py` CTRL, M07, M07b, M07c; `make check` → `✓ check clean` · Закрывает: AC2 · Зависит от: —
  - Сделано: `TestProseReferencesExist` сверяет слово `Test…` кода прозы с функциями `Test…` файлов
    `*_test.go` индекса git (разбор go/parser); плейсхолдеры `TestX`, `TestNew` скилла triage-failure —
    `Test<Name>`, списка исключений нет; `docs/contributing.md` § Кто что enforce'ит называет проверку;
    `exp.py` — как в AC2; `make check` → `✓ check clean`
- [x] **S3** — RHAR-03: тесты доставки правил и скиллов и подключения guard и deny
  - Проверка: `exp.py` CTRL, M17–M31c; `make check` → `✓ check clean` · Закрывает: AC3 · Зависит от: —
  - Сделано: `TestRulesAndSkillsLoad` — glob `paths:` ложится на файл индекса git, правило без `paths:` —
    отказ, `name:` скилла — его каталог, reference scaffold и triage-failure начинаются с «Шаг 0» с
    существующими правилами; `TestGuardWired` — guard-bash — хук PreToolUse инструмента Bash, хуки не
    выключены, deny держит каждый запрет списка; triage-failure получил «Шаг 0» (`tests-pipeline`);
    `exp.py` — как в AC3; `make check` → `✓ check clean`
- [x] **S4** — RHAR-08: неточные утверждения smartdev, triage-failure, stack-deep-dive
  - Проверка: `exp.py` G03–G06, N01; `make check` → `✓ check clean` · Закрывает: AC4 · Зависит от: —
  - Сделано: smartdev — строка `ATTENTION:` называет сигналы `make status` (build, sqlc-drift, migrations,
    lint), красный `make check` или тест — отдельно, заголовки без «(фикс …)»; triage-failure — `ATTENTION:`
    даёт `make status`; stack-deep-dive — «где живёт» у env вместо `Source`, строка `Architecture notes`
    снята (такой секции нет); `exp.py` — как в AC4; `make check` → `✓ check clean`
- [x] **S5** — T6: поля нового домена — одним способом
  - Проверка: `exp.py` G07; `make check` → `✓ check clean` · Закрывает: AC5 · Зависит от: —
  - Сделано: domain.md § 4 — колонки контракта пишутся в `CREATE TABLE` миграции среза, пока срез не
    закоммичен, остальное — шаги 3–7 варианта `field`; после первого коммита — `field` целиком, новой
    миграцией; вывод `make new-domain` ссылается на § 4; `make check` → `✓ check clean`
- [x] **S6** — шаг `make mutate A=--strict` в `/release`
  - Проверка: `exp.py` G08; лог прогона → `EXIT 0`; `make check` → `✓ check clean` · Закрывает: AC6 ·
    Зависит от: —
  - Сделано: Pre-flight `/release` — `make mutate A=--strict` → код 0 (больше часа, в фоне, продолжение с
    места), код 1 — STOP с разбором по rule `tests-pipeline`; прогон — как в AC6; «Шаг 0» triage-failure —
    «багфикс начинается с теста» (build и lint тест не пишут); `make check` → `✓ check clean`
- [x] **S7** — приёмка трека: `exp.py` на базе и ветке, гейт
  - Проверка: `exp.py` — как в AC1–AC6; `make check` → `✓ check clean` · Закрывает: AC1–AC6 ·
    Зависит от: S1–S6
  - Сделано: `exp.py` на `f825fac` (`base-exp-f825fac.log`) — 13 порч «не поймано», H01–H10 «принято»,
    G01, G02, G08 «нет», G03–G07 «да»; на `d332b9e` (`branch-exp-d332b9e.log`) — 13 порч «поймано»
    своими тестами, CTRL зелёный, H01–H06 «отказ», H07–H10 «принято», G01, G02, G08 «да», G03–G07 «нет»,
    N01 — отказ до правки; сообщения коммитов ветки — через её хук `commit-msg` (`check-msg.py`) — OK;
    пробное слияние с `nine/boundaries` (`38cc26c`) — без конфликтов, `make check` → `✓ check clean`,
    `exp.py` — как на ветке (`trial-merge-exp.log`); `make check` → `✓ check clean`

### Круг фиксов валидатора
- [x] **S8** — B1: `paths:` и `name:` — как их читает Claude Code
  - Проверка: `val_archtest.py` P09–P15 → зелёные, M17–M19, V04–V06 → красные; `val_fork.py own` F2, F5 →
    зелёные; `make check` → `✓ check clean` · Закрывает: AC3 · Зависит от: —
  - Сделано: шапка — список YAML (блочный и «[a, b]»), строка через запятую, кавычки и комментарий сняты,
    значение без кавычек со знаком YAML («- **/*.go») — шапки нет, как у Claude Code; «{a,b}» раскрыт, «/**» на
    конце снят, paths: из одних «**» — правило грузится всегда; glob ложится на файл по правилам .gitignore
    (`git ls-files -c -i -x`, как пакет ignore у Claude Code); скилл без `name:` — по каталогу. P09–P15, F2,
    F5 — зелёные, M17–M19, V04–V06 — красные; `fix_cases.py` E01–E12 — как ждали; `make check` → `✓ check clean`
- [x] **S9** — B2: guard, обезвреженный полем обработчика, — отказ `TestGuardWired`
  - Проверка: W01–W04 и поля `once`, `timeout` → красные; P01–P08 законные → зелёные; `make check` ✓ ·
    Закрывает: AC3 · Зависит от: —
  - Сделано: обработчик guard засчитывается, только если в нём нет полей, кроме `type`, `command`, `shell`
    (`bash`), `statusMessage`, `onFailure`; W01–W04 и E20–E27 (`if`, `async`, `asyncRewake`, `once`, `args`,
    `shell: powershell`, `timeout`, неизвестное поле) — красные, P01–P08 и E28–E30 — зелёные, M30–M31c,
    V07–V11 — красные; `make check` → `✓ check clean`
- [x] **S10** — N1: matcher хука — как у Claude Code
  - Проверка: P03, P03b, P04, P07 → зелёные; V09 (`Bas`) → красный; `make check` ✓ · Закрывает: AC3 · Зависит от: S9
  - Сделано: matcher читается, как у клиента 2.1.300: пусто и `*` — любой инструмент, только буквы, цифры, `_`,
    `-`, пробелы, `|` и `,` — точные имена, иначе — регулярное выражение без якорей; выражение, которое RE2 не
    разбирает, — чужая запись, пропуск. P01–P04, P06, P07, E40–E43, E47 — зелёные, V09, M31c, E44–E46, E48 —
    красные; `make check` → `✓ check clean`
- [ ] **S11** — N2: при `identity: external` пропуск только того, что так написано в шаблоне
  - Проверка: `val_fork.py external` X1 → зелёный, X3, X4 → красные; `make check` ✓ · Закрывает: AC2, AC3 ·
    Зависит от: S8
- [ ] **S12** — N3: правила в подкаталогах `.claude/rules/**`
  - Проверка: F3 и E16, E17 → красные, E18 → зелёный; `make check` ✓ · Закрывает: AC3 · Зависит от: S8
- [ ] **S13** — N4: что грузит правило — правда в `CLAUDE.md`, `docs/contributing.md`, тесте и плане
  - Проверка: проба Bash-чтения файла под `paths:`; `make check` ✓ · Закрывает: AC3 · Зависит от: —
- [ ] **S14** — N5: пределы хука `commit-msg` названы в прозе
  - Проверка: строка «git-хуки» `docs/contributing.md`; `make check` ✓ · Закрывает: AC1 · Зависит от: —
- [ ] **S15** — N7: «Готово» — ревью юзера или хаба, без причины «вызов повиснет»
  - Проверка: `CLAUDE.md § Готово`; `make check` ✓ · Закрывает: AC1 · Зависит от: —
- [ ] **S16** — N8: имена тестов в `-run '^TestX$'`, `TestA|TestB`, `TestX/sub`, `Test1…`
  - Проверка: мёртвое имя в каждой форме → красный; `make check` ✓ · Закрывает: AC2 · Зависит от: —
- [ ] **S17** — N9: шапка с CRLF
  - Проверка: E13, E15 → зелёные, E14 → красный; `make check` ✓ · Закрывает: AC3 · Зависит от: S8
- [ ] **S18** — N11: строка `Review:` в формах коммита скиллов и шаблона плана
  - Проверка: `grep` форм `feat(`/`fix(`; `make check` ✓ · Закрывает: AC1 · Зависит от: —
- [ ] **S19** — N6: Handoff трека — строки для аннотации `template/v1.2.0`
  - Проверка: раздел Handoff; `make check` ✓ · Закрывает: AC1–AC3 · Зависит от: S8–S18
- [ ] **S20** — перенос на `main`, если он ушёл с `f825fac`: тесты трека — в перечень комментария пакета
  - Проверка: `git rebase main`; `make check` → `✓ check clean`; `exp.py` и кейсы круга — на итоге ·
    Закрывает: AC1–AC6 · Зависит от: S8–S19

## Решения
- Полный `make mutate A=--strict` — на `f825fac` в своём worktree: код под мутациями (домены, platform,
  cmd, `pkg`) трек не меняет, вердикт тот же, а прогон не ждёт конца трека.
- `Review:` — трейлер git: в начале строки тела, точный регистр, после пробела — текст; хук ищет её по всему
  сообщению — первая строка начинается с типа. Требование — только у `feat` и `fix` (решение владельца),
  сообщения самого git (`Revert`, `fixup!` …) — без строки.
- Чужой файл `docs/contributing.md` (в волне 2 ничей): строки хука `commit-msg` в «Кто что enforce'ит» и
  в troubleshooting называют `Review:` — шаг меняет поведение хука, которое они описывают.
- Имя теста в прозе проверяется без списка плейсхолдеров: плейсхолдер — `Test<Name>` (угловые скобки
  уже не ссылка); у проекта с внешней идентичностью проверка имён тестов, как и путей API, молчит — тесты
  её доменов (`TestIdentity`, `TestErrorMaps`) ушли вместе с ними.
- `paths:` читается, как у Claude Code 2.1.300 (разбор шапки, раскрытие скобок и сверка — по коду клиента и
  документации memory § Path-specific rules): список YAML или строка через запятую вне «{…}», «/**» на конце
  снят, из одних «**» — правило без условия; glob сверяет `git ls-files -c -i -x` — правила .gitignore, как пакет
  ignore клиента (glob без «/» — на любой глубине). Без файлов — только литерал, который игнорирует git
  (`.env.local`). Шапку разбирает свой разбор подмножества YAML: библиотеки YAML среди прямых зависимостей нет,
  а `go.mod` трек не правит. У проекта с внешней идентичностью проверка glob'ов молчит: слои, которые были
  только у её доменов (`events.go`, `delivery/nats`, outbox), пустуют до своих.
- Чужой файл rule `infra-impact` (в волне 2 ничей): glob `.env.example.*` снят — файлов под ним нет, и
  проза таких вариантов не заводит; канон — один `.env.example` (`TestEnvExampleHasRequired`).
- Поля обработчика guard — белый список (`type`, `command`, `shell: bash`, `statusMessage`, `onFailure`), а
  не перечень опасных: в схеме Claude Code 2.1.300 гасят guard не только `if`, `async`, `args`, `shell`, но и
  `asyncRewake`, `once` и короткий `timeout`, а новое поле клиента без правки archtest не засчитается.
- deny проверяется списком всех запретов `.claude/settings.json`: снять любой — ослабить гейт (CLAUDE.md
  § Сначала спросить) правкой archtest, новый запрет теста не требует. Matcher хука — по коду Claude Code 2.1.300
  (сверка matcher'а с именем инструмента): пусто и `*` — любой инструмент, `^[a-zA-Z0-9_|, -]+$` — имена через
  `|` или `,`, иначе — регулярное выражение JavaScript без якорей; RE2 не разбирает lookahead — такая запись
  не guard и пропускается, а guard с ней не засчитывается.
- «Шаг 0» у triage-failure (`tests-pipeline`): багфикс начинается с теста, новый файл теста пишет Write — и
  правило по `paths:` не придёт; список процедур с шагом 0 — `readRulesFirst`.
- Чужой файл `docs/contributing.md`: строки deny, guard и «Самопроверка обвязки» называют новые проверки.
- T6: колонки нового домена — в миграцию среза, пока срез не закоммичен; порог — первый коммит, а не
  релиз: его видно по `git status`, а правку закоммиченной миграции `make status` показывает в
  Infra-touch. `ALTER TABLE` над пустой таблицей — лишняя операция выкатки, `DROP COLUMN` `name` — ещё и
  миграция с эффектом (маркер и handoff). Вывод `make new-domain` не повторяет § 4, а ссылается на него.
- Чужой файл `scripts/new-domain.sh` (в волне 2 ничей): две строки `echo` «Дальше» — ссылка на § 4.
- Шаг `make mutate` — в Pre-flight `/release`, а не в `release.sh` (файл трека lifecycle): решение владельца —
  шаг скилла; прогон больше часа, и в Bash-вызове агента он идёт в фоне.

## Handoff
- Дирижёру при слиянии: в комментарий пакета `internal/archtest` (файл трека boundaries) — «доставка
  правил и скиллов (paths:, name:, шаг 0), подключение guard-bash и deny .claude/settings.json»
  (`TestRulesAndSkillsLoad`, `TestGuardWired`).
- Дирижёру при слиянии: RHAR-08, п. 5 — в rule `domain-architecture` («Файл > ~400 строк») и
  `docs/reference/architecture.md` § Разбиение переростков — «файл кода (не `_test.go`)»: тесты доменов —
  545–1012 строк, код — до 303.

## Где мы сейчас
Круг фиксов валидатора: S8 (B1), S9 (B2) закрыты — `paths:` и `name:` читаются, как у Claude Code, guard с
полем, которое его гасит, не засчитывается; S10 (N1) — matcher хука читается, как у клиента. Следующий шаг —
S11 (N2). Кейсы круга — `N/harness/fix_cases.py`, раннеры валидатора — `N/val-harness/`.
