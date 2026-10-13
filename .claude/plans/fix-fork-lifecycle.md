---
status: in-progress
parent: —
blocked-by: —
---

# fix-fork-lifecycle — жизненный цикл форка до девятки

## Цель
Закрыть пункты трека lifecycle программы `reaudit-to-nine` — RLIF-01…05, RENF-03, RADP-01: на ветке
эксперименты аудиторов не воспроизводят дефект. Форк узнаёт о новых тегах шаблона и тем же движком
переносит нужный ему код шаблона; образ релиза не делит путь с другим сервисом орги; онбординг с
внешней идентичностью проходим как написан; валидатор stack не пропускает заглушки; не подключённые
хуки видны; адаптер к `user` у проекта со своей идентичностью не красит гейт.

## Контекст
- Программа — `.claude/plans/reaudit-to-nine.md`: трек lifecycle, волна 1, владение файлами, решения
  владельца (не пересматриваются).
- Отчёты (`R` = `/home/mm/.cache/stit-audit-program/reaudit`): `R/template-lifecycle.md` — RLIF-01…05,
  `R/template-enforcement.md` § RENF-03, `R/template-adapters.md` § RADP-01. Эксперименты: `R/tpl-lif/`
  (форки `own`, `ext`, `half`, `template.git` с пробными тегами, `bin/vstack.sh`, `bin/tu-paths.sh`, логи),
  `R/tpl-enf/toy-fresh.sh`, `R/tpl-adp/run.py` (кейсы `fp22`, `fp22c`).
- Код: `scripts/release.sh` (путь образа), `scripts/tools/{template-update,doctor,status,validate-stack-doc}.sh`,
  цели `setup`, `template-update`, `release` в `Makefile`, `internal/archtest/{onboarding,template}_test.go`;
  `TestReleaseBuildForwardsSSHAgent` — в `internal/archtest/make_test.go` (файл трека gate). Проза: skills
  `init-service` (`references/prune.md`), `de-template` (`references/wiring.md`), `upgrade`, `release`,
  `smartdev`, `stack-deep-dive` (`references/draft-stable.md`), `scaffold/references/port.md`,
  `docs/reference/{ecosystem,release}.md`, `docs/contributing.md § Версия обвязки`.
- Материалы трека — `/home/mm/.cache/stit-audit-program/nine/lifecycle/` (логи, пробные форки); тяжёлые
  прогоны — под `flock /home/mm/.cache/stit-audit-program/nine/gate.lock`.

## Не входит
- Minor угла LIF вне трека (RLIF-06…13) — бэклог `minor-lifecycle`; RENF-04…16 — трек gate и бэклог.
- Хаб `<org>/infra` (периодический `/upgrade`, строка о канале обвязки) — раздел «Handoff».
- Настоящий релиз, push, теги; golibinfra, `go.mod`.
- Машинная проверка тела коммита `/upgrade` (перенесено и отклонено) — хук `commit-msg` вне трека.

## Критерии приёмки
- [x] **AC1** — RLIF-04: путь образа — из origin: форки `acme/own` и `acme/ext` одной орги выпускают
  `<host>/acme/own{,-migrate}` и `<host>/acme/ext{,-migrate}` (на базе оба — `<host>/acme/backend`);
  confirm-промпт `make release` называет полный путь образов, handoff — `<SVC>_TAG` и `make bump SVC=<репо>`.
  Проверка: `go test ./internal/archtest -run TestReleaseImage` → PASS; пробные форки → промпт с путём
  - Подтверждено: `TestReleaseImage` → PASS (scp, https, ssh с портом и регистром, `backend`, отказ на
    локальном пути — без тега); клоны форков `own` и `ext` (`nine/lifecycle/logs/s4-forks.log`): база —
    промпт без пути, `BACKEND_TAG`, `SVC=backend` у обоих; ветка — `→ git.stit.tech/acme/own{,-migrate}`,
    `OWN_TAG`, `SVC=own` и `→ …/acme/ext…`, `EXT_TAG`, `SVC=ext`
- [x] **AC2** — RLIF-04: `TestReleaseBuildForwardsSSHAgent` красный на обоих мутантах `go mod download` —
  без `insteadOf` https → ssh и без `StrictHostKeyChecking=accept-new` (на базе оба зелёные). Проверка:
  мутант в копии `Dockerfile` → `go test -run TestReleaseBuildForwardsSSHAgent` → FAIL, без мутанта → PASS
  - Подтверждено: на базе оба мутанта (`Dockerfile` и `Dockerfile.migrate`) → `ok`; на ветке → FAIL с
    причиной («go fetches git.stit.tech over https…», «a fresh image knows no host key…»), дерево → PASS
- [x] **AC3** — RLIF-02: финал `/init-service` заводит remote `template`; `make doctor` спрашивает его теги
  (`git ls-remote`): тег новее версии форка — строка с ним и `/upgrade`, сбой сети — не ошибка (на базе
  doctor о шаблоне молчит). Проверка: тест archtest → PASS; копия форка `own` с remote на
  `R/tpl-lif/template.git` → `make doctor` называет `template/v1.3.0`; remote недоступен → без FAIL
  - Подтверждено: `TestTemplateUpdateRemote` → PASS; клон `own` на коммите «онбординг закрыт»
    (`nine/lifecycle/logs/s6-doctor.log`): база — `OK: doctor clean`, о шаблоне ни строки; ветка —
    `WARN : template: template/v1.0.0 · в шаблоне новее: template/v1.3.0 → skill /upgrade`, remote
    недоступен — `INFO … не ответил`, локальные теги и дерево не тронуты
- [x] **AC4** — RLIF-01: `make template-update TO=… PATHS='<пути>'` переносит с обвязкой и код вне
  `.template/paths` тем же трёхсторонним слиянием (module path форка, конфликты в индексе); формат
  аннотации — с разделом «Безопасность»; `/upgrade` пишет в тело коммита перенесённое и отклонённое из
  «Платформа» и «Безопасность»; `Dockerfile*` — решение в «Решениях». Проверка: `TestTemplateUpdate` →
  PASS (случай `PATHS`); копия форка `half` → перенос до `template/v1.3.0` с `PATHS` → код в индексе,
  конфликты названы (ср. `R/tpl-lif/logs/half-C-3way.log`)
  - Подтверждено: `TestTemplateUpdatePaths` → PASS; `half` — база 0 файлов кода, ветка 59 и те же два
    конфликта кода, что у аудитора (`nine/lifecycle/logs/s7-half.log`); формат аннотации с разделом
    `Безопасность` (`contributing.md § Версия обвязки`), `/upgrade` шаги 2 и 5 — строка «перенесено» или
    «отклонено — причина» на каждый пункт; `Dockerfile`, `Dockerfile.migrate`, `.dockerignore` — в
    `.template/paths` (`TestTemplatePaths` → PASS)
- [ ] **AC5** — RLIF-03: онбординг с `identity: external` проходим буквально: A.3 и wiring §2 оставляют
  стримы и киты доменов идентичности до prune, `prune.md` снимает киты, которых нет в `dependencies.md`
  (на базе A.3 буквально → красные `TestBoot`, `TestIdentity`; wiring §2 для auditkit → красный
  `TestFxGraphValidates`). Проверка: пробный форк со стеком `external` без auditkit по новому тексту →
  `make check` зелёный на этапе 2 и после prune, auditkit вне графа
- [x] **AC6** — RLIF-05: `validate-stack-doc.sh --stable` отвергает секцию-заглушку (`TBD`, `_TBD_`, `TODO…`,
  `—`, `...`, `- <заполнить>`, таблица без строк, одни подзаголовки), содержимое проходит (на базе — `OK`,
  `R/tpl-lif/logs/v-stack-validator.log`). Проверка: `bash R/tpl-lif/bin/vstack.sh <worktree> <каталог>` →
  V4–V7, V11, V15, V19, V20 — FAIL, V3 — OK; `TestStackValidator` → PASS
  - Подтверждено: `vstack.sh` на ветке — V4–V7, V11, V15, V19, V20 → FAIL, V3 → OK
    (`nine/lifecycle/logs/vstack-{base,s2}.log`); `TestStackValidator` → PASS (9 заглушек, 4 решения);
    вердикты «пуста» на всех `docs/stack` машины — те же, что у базового валидатора
- [x] **AC7** — RENF-03: клон без `make setup` — `make status` пишет строку `HOOKS:` о неподключённых хуках и
  `ATTENTION: hooks`, `make doctor` — FAIL о хуках, `/smartdev` ведёт в `make setup`; `make setup`
  подключает хуки первой строкой — сбой установки инструментов их не отменяет (на базе — `OK`, о хуках ни
  строки: `R/tpl-enf/toy-fresh.tsv`). Проверка: тест archtest → PASS; свежий клон ветки → status и doctor
  - Подтверждено: `TestHooksVisible` (4 случая) и `TestSetupWiresHooksFirst` → PASS; свежие клоны
    (`nine/lifecycle/logs/s3-{fresh,setup}.log`): база — `OK`, ни строки о хуках, сбой `make setup` оставляет
    `.git/hooks`; ветка — `HOOKS: не подключены — make setup (.git/hooks: нет исполняемого commit-msg)`,
    `ATTENTION: hooks`, doctor `FAIL : git hooks`, после сбоя установки — `HOOKS: OK`
- [x] **AC8** — RADP-01: при `identity: own` адаптер нового домена к `user` по `port.md` гейт не красит, без
  решения и при `external` проверка прежняя; `port.md` об этом говорит (на базе `fp22c` красный).
  Проверка: тест archtest → PASS; `run.py --no-full <worktree ветки> fp22c-new-consumer-identity-own` →
  зелёный, `fp22` — красный, как задумано
  - Подтверждено: `TestIdentityDependents` → PASS (6 случаев); `run.py --no-full` на копии ветки: `fp22c` —
    GREEN-quick, `fp22` — CAUGHT `TestPruneNamesIdentityDependents` (`nine/lifecycle/logs/s1-fp22.tsv`)

## Шаги
- [x] **S1** — RADP-01: проверка prune молчит при `identity: own`, тест законной формы, строка в `port.md`
  - Проверка: тест → RED на базе, GREEN после; кейсы `fp22c` и `fp22` · Закрывает: AC8 · Зависит от: —
  - Сделано: проверка — функция над корнем репо (`unnamedIdentityDependents`), решение `identity` читает
    `identityChoice`; `TestIdentityDependents` без выхода при `own` — RED ровно в случае `own identity`, с
    выходом — PASS; `fp22c` зелёный, `fp22` пойман; `make check` → `✓ check clean`
- [x] **S2** — RLIF-05: валидатор stack не считает заглушку содержимым; тест и проза `draft-stable.md`
  - Проверка: `TestStackValidator` с заглушками → RED, затем PASS; `vstack.sh` · Закрывает: AC6 · Зависит от: —
  - Сделано: `TestStackValidator` — RED на всех девяти заглушках, после правки `section_body_nonempty` —
    PASS; `vstack.sh` — заглушки FAIL, заполненные артефакты OK; проза `draft-stable.md`, `wiring.md`;
    `make check` → `✓ check clean`
- [x] **S3** — RENF-03: хуки в `make status` и `make doctor`, ATTENTION в `/smartdev`, `make setup` — хуки первыми
  - Проверка: тест archtest → RED, затем PASS; свежий клон ветки · Закрывает: AC7 · Зависит от: —
  - Сделано: `hooks_test.go` — RED на базе (status `OK` без строки о хуках, `.git/hooks` после сбоя setup),
    после правки PASS; проверка — `doctor.sh` (`--hooks` для status), строка `HOOKS` и `ATTENTION: hooks`,
    FAIL doctor, хуки — первая строка `make setup`; строка в `/smartdev`; `make check` → `✓ check clean`
- [x] **S4** — RLIF-04: путь образа и имя сервиса в handoff — из origin, путь в confirm-промпте; доки и
  INFRA-HANDOFF о пути образа
  - Проверка: `TestReleaseImage` → RED, затем PASS; пробные форки · Закрывает: AC1 · Зависит от: —
  - Сделано: `release_test.go` — RED на базе (промпт без пути), после правки `release.sh` PASS; доки
    `release.md`, `ecosystem.md`, `/release`, плейсхолдер `service.md § Binaries`; блок в накопителе
    `handoff.md`; `make check` → `✓ check clean`
- [x] **S5** — RLIF-04: `TestReleaseBuildForwardsSSHAgent` держит `insteadOf` и `accept-new`
  - Проверка: два мутанта → FAIL, дерево → PASS · Закрывает: AC2 · Зависит от: —
  - Сделано: тест сверяет `go mod download` с хостами `ENV GOPRIVATE` Dockerfile'а: `url.git@<host>:.insteadOf`
    для `https://<host>/` и `StrictHostKeyChecking=accept-new`; мутанты убиты; `make check` → `✓ check clean`
- [x] **S6** — RLIF-02: remote `template` в финале онбординга, теги шаблона в `make doctor`, проза `/upgrade`
  - Проверка: тест → RED, затем PASS; копия `own` с `template.git` · Закрывает: AC3 · Зависит от: S3
  - Сделано: `template-update.sh --remote` (`git ls-remote`, exit 0/1/2), строка `template:` в doctor
    (OK/WARN/INFO), общая проверка канона с `--status`; финал `/init-service` заводит remote, `/upgrade`,
    `/smartdev`, `ecosystem.md`; `TestTemplateUpdateRemote` RED → PASS; `make check` → `✓ check clean`
- [x] **S7** — RLIF-01: перенос по запросу `PATHS=` в движке `make template-update`
  - Проверка: `TestTemplateUpdate` с `PATHS` → RED, затем PASS; копия `half` · Закрывает: AC4 · Зависит от: —
  - Сделано: `PATHS` добавляет пути к путям шаблона того же переноса, опечатка — отказ до переноса;
    `TestTemplateUpdatePaths` RED (третий аргумент игнорировался) → PASS; клоны `half`
    (`nine/lifecycle/logs/s7-half.log`): база — кода в индексе 0, ветка — 59 файлов кода, 6 конфликтов
    (4 обвязки и `cmd/api/boot_test.go`, `internal/domain/auth/usecase.go`, как в `half-C-3way.log`),
    module path форка; `/upgrade` шаг 2, `contributing.md`; `make check` → `✓ check clean`
- [x] **S8** — RLIF-01: раздел «Безопасность» в аннотации, `/upgrade` — перенесено и отклонено в теле
  коммита, `Dockerfile*` в `.template/paths`
  - Проверка: `make check` → `✓ check clean`; проза сверена с движком · Закрывает: AC4 · Зависит от: S7
  - Сделано: раздел `Безопасность` в формате аннотации, `/upgrade` (вступление, шаги 2 и 5, anti-pattern),
    `ecosystem.md`; `Dockerfile`, `Dockerfile.migrate`, `.dockerignore` в `.template/paths`;
    `make check` → `✓ check clean`
- [ ] **S9** — RLIF-03: A.3 и wiring §2 — строки и киты доменов идентичности до prune, шаг wiring в `prune.md`
  - Проверка: пробный форк `external` без auditkit по новому тексту · Закрывает: AC5 · Зависит от: —
- [ ] **S10** — Гейт и приёмка: эксперименты всех критериев на итоговом дереве
  - Проверка: `make check` → `✓ check clean`; AC1–AC8 подтверждены · Закрывает: AC1, AC2, AC3, AC4, AC5,
    AC6, AC7, AC8 · Зависит от: S1, S2, S3, S4, S5, S6, S7, S8, S9

## Решения
- RADP-01: проверка prune молчит только при `identity: own`; без решения (шаблон, `<own|external>`) и при
  `external` она прежняя — там prune возможен. `identityExternal` оставлен обёрткой над `identityChoice`:
  его берёт самопроверка прозы (`harness_test.go`, файл трека harness) — без правки чужого файла.
- RLIF-05: заглушка — строка или пункт из одного `TBD`, `TODO…`, прочерка или многоточия (регистр и
  `_*` не в счёт), плюс подзаголовок и таблица без строк под разделителем. Размытое решение (V10
  «соответствующий код в platform/») и слово «потом» рядом с плейсхолдером (V21) машина не судит — это
  смысл, его держит чек-лист `draft-stable.md` § 2 и архитектор.
- RENF-03: «подключены» — каждый хук `scripts/git-hooks` есть в каталоге хуков git
  (`git rev-parse --git-path hooks`), исполняемый и тот же по содержимому: так OK и у worktree, чей
  `core.hooksPath` абсолютный на хуки основного дерева с тем же содержимым, а чужой или устаревший каталог
  хуков — отказ. Проверка одна — `doctor.sh` (`--hooks` для `status.sh`, как `--status` у
  `template-update.sh`); doctor — FAIL, поэтому `release-preflight env` без хуков останавливает релиз.
- Правка чужого файла: перечень в комментарии пакета `internal/archtest/archtest_test.go` (трек gate) —
  «подключение git-хуков (make setup, status, doctor)», «путь образа релиза»: CLAUDE.md держит перечень
  проверок там.
- RLIF-04: путь образа — только из origin (`<host>/<org>/<репо>`, как у `ai-gateway-template` и форков
  `stit-hub/*`), без второго источника в `docs/stack/service.md`: имена репо в орге не совпадают, значит и
  пути образов; хаб и так берёт образ как `${PROJECT_NAME}/<svc>`. `backend` — не литерал скрипта, а имя
  репо форка по конвенции: у `<org>/backend` путь и `BACKEND_TAG` прежние. Регистр — нижний (ссылка docker),
  порт ssh в путь registry не идёт; origin, из которого путь не вывести, — отказ до вопроса и тега.
  `release_test.go` — новый файл трека (функции песочницы релиза — из `make_test.go`, без правки).
- Правка чужого файла: `TestReleaseBuildForwardsSSHAgent` в `internal/archtest/make_test.go` (трек gate) —
  без неё мутантов не убить, а копия проверки в своём файле дала бы два источника одного факта.
  Хосты — из `ENV GOPRIVATE` того же Dockerfile'а: форк на своём хосте правит его, а не тест.
- RLIF-02: теги remote спрашивает `make doctor`, а не `make status`: cockpit гоняют каждую сессию, он без
  сети; doctor идёт в `release-preflight env` — у каждого релиза форк видит новый тег. Новый тег и
  отсутствие remote — WARN (не блокирует), сеть — INFO; `git ls-remote` без fetch — doctor read-only,
  `timeout 30` и `BatchMode` — без зависания и вопросов. Канон шаблона remote не спрашивает.
- RLIF-01: `PATHS` добавляет пути к путям шаблона, а не заменяет их: тег — единица выпуска, версия
  обвязки сдвигается одним переносом, код и обвязка — от одной базы, одним гейтом и коммитом (так же
  советует RLIF-07). Код без обвязки тега не переносится: следующий перенос начинался бы с базы, которую
  код уже обогнал. Пути — каталог, файл или `:(exclude)`; шаблонов имён нет (`read -ra`, без glob).
- RLIF-01: `Dockerfile`, `Dockerfile.migrate` и `.dockerignore` — в `.template/paths`: сборка образа —
  контракт с `scripts/release.sh` и `TestReleaseBuildForwardsSSHAgent`, которые уже в путях шаблона;
  вне путей они расходились бы (ssh-mount, `insteadOf`, база по digest), а правку форка перенос
  покажет конфликтом. Шаг «возьми Dockerfile из тега» аннотации больше не нужен.
- RLIF-01: «Безопасность» — отдельный раздел аннотации для фиксов вне путей шаблона; перенос или
  отказ — строкой в теле коммита `/upgrade`. Машиной (хук `commit-msg`) это не держится — вне трека.

## Handoff
- Хаб `<org>/infra` (RLIF-02): периодический `/upgrade` по сервисам — сервис говорит о новом теге обвязки
  строкой `make doctor`; строка о канале обвязки (`template/vX.Y.Z`, remote `template`) — в его
  `docs/reference/ecosystem.md`. В хаб не пишем — отчёт дирижёру.
- Аннотация следующего тега обвязки (дирижёр, S8 программы), Breaking: путь образа — `<host>/<org>/<репо>`
  из origin, тег в хабе — `<РЕПО>_TAG`; форк с репо не `backend` — image и тег в compose хаба по handoff
  релиза (блок RLIF-04 — в накопителе `handoff.md`).

## Где мы сейчас
S1 (RADP-01), S2 (RLIF-05), S3 (RENF-03), S4–S5 (RLIF-04), S6 (RLIF-02), S7–S8 (RLIF-01) закрыты.
Следующее: S9 — RLIF-03, онбординг с `identity: external` (A.3, wiring §2, шаг в `prune.md`).
