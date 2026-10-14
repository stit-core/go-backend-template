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
- [ ] **AC1** — RHAR-01: `CLAUDE.md § Готово` называет, кто проводит ревью (принимающая сторона), что
  пишет агент без собеседника, след — строку `Review:` — и права в списке `/security-review`; хук
  `commit-msg` отвергает `feat` и `fix` без строки `Review:` в теле. Проверка: `exp.py <wt> H01 … H10 G`
  → на базе H01–H10 «принято», G01, G02 «нет»; на ветке H01–H06 «отказ», H07–H10 «принято», G01, G02
  «да»; `go test -run TestCommitMsgHook ./internal/archtest/` → PASS
- [ ] **AC2** — RHAR-02: имя теста в коде прозы, которого нет функцией `*_test.go` репо, — отказ
  `TestProseReferencesExist`. Проверка: `exp.py` M07, M07b, M07c → на базе «не поймано», на ветке
  «поймано» этим тестом; CTRL — зелёный
- [ ] **AC3** — RHAR-03: glob `paths:`, который ни на что не ложится, правило без `paths:`, `name:`
  скилла не по каталогу, reference scaffold без «Шаг 0», снятый deny `.claude/settings.json`, отвязанный,
  подменённый или выключенный хук guard — отказ archtest. Проверка: `exp.py` M17, M17b, M17c, M18, M19,
  M30, M30b, M31, M31b, M31c → на базе «не поймано», на ветке «поймано»; CTRL — зелёный
- [ ] **AC4** — RHAR-08: утверждения п. 1, 2, 4 (smartdev, triage-failure, stack-deep-dive) и заголовки
  smartdev с «(фикс …)» поправлены; п. 3 правда (`make new-domain NAME=repo` — отказ до правки);
  п. 5 — в Handoff. Проверка: `exp.py` G03–G06 → на базе «да», на ветке «нет»; N01 → «отказ до правки»
- [ ] **AC5** — T6: поля нового домена — одним способом в `scaffold/references/domain.md § 4` и в выводе
  `make new-domain`. Проверка: `exp.py` G07 → на базе «да», на ветке «нет»; текст обоих мест
- [ ] **AC6** — `/release`: шаг `make mutate A=--strict` перед релизом; на коде ветки он даёт код 0.
  Проверка: `exp.py` G08 → на ветке «да»; `flock … make mutate A=--strict` → `EXIT 0`
  (`N/harness/mutate-strict-*.log`)

## Шаги
- [ ] **S1** — RHAR-01: «Готово» — ревью у принимающей стороны и след `Review:`; хук `commit-msg` и
  `TestCommitMsgHook`; проза о коммитах (CLAUDE.md § Git workflow, `CONTRIBUTING.md`, scaffold, triage)
  - Проверка: `exp.py` H, G01, G02; `make check` → `✓ check clean` · Закрывает: AC1 · Зависит от: —
- [ ] **S2** — RHAR-02: самопроверка прозы сверяет имена тестов с функциями `*_test.go`
  - Проверка: `exp.py` CTRL, M07, M07b, M07c; `make check` → `✓ check clean` · Закрывает: AC2 · Зависит от: —
- [ ] **S3** — RHAR-03: тесты доставки правил и скиллов и подключения guard и deny
  - Проверка: `exp.py` CTRL, M17–M31c; `make check` → `✓ check clean` · Закрывает: AC3 · Зависит от: —
- [ ] **S4** — RHAR-08: неточные утверждения smartdev, triage-failure, stack-deep-dive
  - Проверка: `exp.py` G03–G06, N01; `make check` → `✓ check clean` · Закрывает: AC4 · Зависит от: —
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

## Handoff
- Дирижёру при слиянии: RHAR-08, п. 5 — в rule `domain-architecture` («Файл > ~400 строк») и
  `docs/reference/architecture.md` § Разбиение переростков — «файл кода (не `_test.go`)»: тесты доменов —
  545–1012 строк, код — до 303.

## Где мы сейчас
План записан, база снята (`N/harness/base-exp-f825fac.log`), полный `make mutate A=--strict` на
`f825fac` идёт под замком (`N/harness/mutate-strict-f825fac.log`). Следующий шаг — S1 (RHAR-01).
