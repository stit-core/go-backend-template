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
- [ ] **AC5** — T6: поля нового домена — одним способом в `scaffold/references/domain.md § 4` и в выводе
  `make new-domain`. Проверка: `exp.py` G07 → на базе «да», на ветке «нет»; текст обоих мест
- [ ] **AC6** — `/release`: шаг `make mutate A=--strict` перед релизом; на коде ветки он даёт код 0.
  Проверка: `exp.py` G08 → на ветке «да»; `flock … make mutate A=--strict` → `EXIT 0`
  (`N/harness/mutate-strict-*.log`)

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
- [ ] **S5** — T6: поля нового домена — одним способом
  - Проверка: `exp.py` G07; `make check` → `✓ check clean` · Закрывает: AC5 · Зависит от: —
- [ ] **S6** — шаг `make mutate A=--strict` в `/release`
  - Проверка: `exp.py` G08; лог прогона → `EXIT 0`; `make check` → `✓ check clean` · Закрывает: AC6 ·
    Зависит от: —
- [ ] **S7** — приёмка трека: `exp.py` на базе и ветке, гейт
  - Проверка: `exp.py` — как в AC1–AC6; `make check` → `✓ check clean` · Закрывает: AC1–AC6 ·
    Зависит от: S1–S6

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
- Glob `paths:` сверяет `git ls-files ':(glob)…'` — те же `**` и `*`, что у Claude Code; без файлов —
  только литерал, который игнорирует git (`.env.local`). У проекта с внешней идентичностью проверка glob'ов
  молчит: слои, которые были только у её доменов (`events.go`, `delivery/nats`, outbox), пустуют до своих.
- Чужой файл rule `infra-impact` (в волне 2 ничей): glob `.env.example.*` снят — файлов под ним нет, и
  проза таких вариантов не заводит; канон — один `.env.example` (`TestEnvExampleHasRequired`).
- deny проверяется списком всех запретов `.claude/settings.json`: снять любой — ослабить гейт (CLAUDE.md
  § Сначала спросить) правкой archtest, новый запрет теста не требует. Matcher хука — имя или регулярное
  выражение, пусто и `*` — любой инструмент, как у Claude Code.
- «Шаг 0» у triage-failure (`tests-pipeline`): багфикс начинается с теста, новый файл теста пишет Write — и
  правило по `paths:` не придёт; список процедур с шагом 0 — `readRulesFirst`.
- Чужой файл `docs/contributing.md`: строки deny, guard и «Самопроверка обвязки» называют новые проверки.

## Handoff
- Дирижёру при слиянии: в комментарий пакета `internal/archtest` (файл трека boundaries) — «доставка
  правил и скиллов (paths:, name:, шаг 0), подключение guard-bash и deny .claude/settings.json»
  (`TestRulesAndSkillsLoad`, `TestGuardWired`).
- Дирижёру при слиянии: RHAR-08, п. 5 — в rule `domain-architecture` («Файл > ~400 строк») и
  `docs/reference/architecture.md` § Разбиение переростков — «файл кода (не `_test.go`)»: тесты доменов —
  545–1012 строк, код — до 303.

## Где мы сейчас
S1–S4 закрыты: «Готово» называет принимающую сторону ревью, хук требует `Review:` у `feat` и `fix`,
самопроверка прозы видит имена тестов, доставку правил и подключение guard; S4 — правда прозы
smartdev, triage-failure, stack-deep-dive. Полный `make mutate A=--strict` на `f825fac` прерван на 101/356
ради гейта шагов, результаты — в `N/wt-harness-mut/.cache/mutate/`, прогон продолжится с места.
Следующий шаг — S5 (T6).
