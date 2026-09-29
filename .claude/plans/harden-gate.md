---
status: in-progress
parent: —
blocked-by: —
---

# harden-gate — машинная страховка: честный гейт, закрытые обходы, archtest «границы 2.0»

## Цель
Первая из трёх частей программы исправлений по аудиту шаблона (цель — каждый из 8 углов аудита
≥ 9/10). Здесь гейт становится честным и строгим: не может молча пропустить линтер, не сужается
переменными, гоняет integration-тесты, а архитектурные границы держит машина для любого домена.
Идёт первой, чтобы всю дальнейшую работу уже проверяла машина.

## Контекст
- **Отчёт аудита** (источник всех находок, ID вида `ENF-01`):
  https://claude.ai/artifact/L3Y8vWRofsvzf8PGQos4dE — читать инструментом Artifact
  (`action: read`); локальная копия, пока жив worktree догфудера:
  `.claude/worktrees/agent-a261317a598ac6483/bin/audit-report.html`. Оценки на HEAD `03b57d2`:
  обвязка 6.5–7, код 6, тесты 5, адаптеры 5.5, принуждение 5.5, security 5, догфудинг 7–8,
  жизненный цикл 4.5.
- **Отметки владельца по находкам** — в БД страницы отчёта: `ArtifactData` `action: list`,
  `url` отчёта, `collection: review` (документ = ID находки, поля `verdict`
  agree/discuss/disagree и `note`). Прочитать в S1: `disagree` — находку не чинить (записать в
  «Решения»), `discuss` — спросить владельца перед шагом, `note` — учесть.
- **Следующие части:** `fix-reference-code` (эталонный код, security, тесты, покрытие) и
  `repair-harness` (проза обвязки, жизненный цикл, повторный аудит) — стартуют после этой.
- **Принципы программы** (задание владельца): всё найденное починить, но без оверинжиниринга —
  понятный, красивый, консистентный код, который приятно читать и развивать; никаких
  «заскриптованных-перескриптованных» решений: короткие читаемые скрипты или Go-тесты, один
  канонический способ на каждую вещь; удалить лучше, чем добавить; машина держит то, что важно.
  Владелец разрешил правки в зонах «Сначала спросить» (migrations, pkg/policy, pkg/tokens,
  authgate, конфиг гейта) в рамках программы. Новые миграции — только новыми файлами, уже
  существующие не править. Коммит на каждый шаг (Conventional Commits); push — только с OK
  владельца.
- **Находки этой части:** ENF-01 (`scripts/tools/lint.sh:45-53` игнорирует код выхода — «OK» при
  lock, битом конфиге, неизвестном линтере, таймауте, опечатке в PKG); ENF-02 (`make check PKG=…`
  сужает гейт, `COVER_FLOOR ?=` переопределяется из env); ENF-03 (обходы хуков: `commit -n`,
  `--no-veri`, `SKIP_PREPUSH`, `-c core.hooksPath`; pre-push проверяет дерево, а не пушимое);
  ENF-04 (изоляция доменов и моков — ручные списки в `.golangci.yml:61-111`, нет
  `verification/delivery/nats/mocks`); ENF-05 (down-миграции не проверяются, деструктивное
  проходит); ENF-06 (общий кеш golangci-lint загрязняется из worktree); ENF-07 (forbidigo без
  `log.Print*`/`println`, nolintlint и errcheck check-blank выключены); ENF-08 (fmt-check и
  pre-commit пропускают при отсутствии инструмента); ENF-09 (`make setup` копирует хуки вместо
  `core.hooksPath`); ENF-11 (release без гейта, `make release*` в allow); ENF-12 (`make test`
  прячет ошибки компиляции, шум mockery, `status.sh` не видит новые каталоги); TST-03
  (integration вне гейта); TST-05 (archtest: только имена файлов, точное совпадение IO-пакетов,
  алиасы `time`, нет `delivery → infra`); ADP-03 (мост через platform, sqlc-типы в домене, JOIN к
  чужой таблице через общий `schema: migrations/` в `sqlc.yaml`); SEC-03 (ручка без `Security:`
  публична: `golibinfra/httpkit/middleware.go:180`); ADP-02 (identity-closure через границу
  доменов); COD-05 (View-типы вне проверки уникальности, дубль имени huma — panic при
  регистрации, которую `fx.ValidateApp` не видит); HAR-11 (нет самопроверки ссылок в прозе).
- **Эксперименты аудита для перепроверки** — в разделах ENF и ADP отчёта: матрица из 24
  инъекций (поймано 10) и 14 нарушений в адаптерах (поймано 1.5 из 8 обязательных).

## Не входит
- Исправление кода доменов, security и тестов — `fix-reference-code`; сюда входят только проверки
  и явные allowlist'ы текущих нарушений.
- Правки прозы обвязки и жизненный цикл шаблона — `repair-harness` (кроме строк, без которых
  новые проверки непонятны).
- Правки `../golibinfra` — только строкой в `backlog.md`.

## Критерии приёмки
- [x] **AC1** — Линтер не может пройти молча: при битом `.golangci.yml`, неизвестном линтере,
  таймауте и несуществующем `PKG` и `make lint`, и `make check` падают; при занятом lock lint
  ждёт его и затем проверяет по-настоящему — «OK» без прогона невозможен.
  Проверка: воспроизведения из ENF-01 (копия конфига с ошибкой, `--timeout=100ms`, опечатка в
  `PKG`; lock держит фоновый `flock "$TMPDIR/golangci-lint.lock" sleep 15`) → exit ≠ 0 и
  понятное сообщение, при lock — ожидание и настоящий вердикт
  - Подтверждено: в scratch-worktree `make lint` и `make check` → exit 2 и `FAIL: …` на битом
    YAML с `fmt.Println` в usecase (13c) и на узле depguard не на своём месте (13d) — оба
    `config verify`; на удалённом `.golangci.yml` — `config verify` exit 6; неизвестный линтер
    — `golangci-lint exit 3`; `PKG='--timeout=100ms ./...'` — `exit 4`; `PKG=./intenral/...` —
    `exit 7`; `run.issues-exit-code: 0` с нарушением — «вышел с 0 без строки «0 issues.»».
    Lock 15 с: `make lint` ждёт ≈15 с и даёт forbidigo-FAIL (с нарушением) или `OK` (без),
    `make check` так же; дословное `flock … -c 'make lint'` — взаимная блокировка, `timeout 30`
    → exit 124 без «OK». Контроль: чистое дерево → `OK` / `✓ check clean`
- [x] **AC2** — Гейт не сужается и не ослабляется из окружения. Проверка: `make check
  PKG=./internal/domain/auth/...` и `COVER_FLOOR=0 make check` при нарушении в другом домене →
  FAIL
  - Подтверждено: в scratch-worktree с `fmt.Println` в user `make check
    PKG=./internal/domain/auth/...` и то же с `VERBOSE=1 FORMAT=detailed` → `FAIL: lint`; с
    падающим тестом в user и `PKG=auth` → `FAIL: go test exit 1`; с 60 непокрытыми
    statement'ами в user `COVER_FLOOR=0 make check` → `FAIL: coverage 68.2% < floor 75%`, `make
    cover-check COVER_FLOOR=0` из командной строки → тоже FAIL
- [x] **AC3** — `make check` гоняет integration-тесты и roundtrip миграций up → down → up; без
  docker падает с понятным сообщением. Проверка: `make check` → в выводе шаг integration;
  мутация `users.sql` (`version=$3 OR TRUE`) → FAIL
  - Подтверждено: `make check` → шаг `→ make test-int (юнит + integration)`, `✓ check clean`;
    мутация M23 (`users.sql:23`) + `make gen` → `TestUpdate_StaleVersion/profile` FAIL; Docker
    скрыт (`unshare -rm` + bind `/dev/null` на сокет) → `TestDockerRequired`: «Docker is
    unavailable: make check requires integration tests». Лесенка ловит пустой, мусорный,
    деструктивный (`DROP TABLE users CASCADE`) и `-- noop` down и down 0001 без `DROP FUNCTION`
- [x] **AC4** — Обходы хуков закрыты. Проверка: в сессии Claude Code `git commit -n`,
  `git commit --no-veri`, `SKIP_PREPUSH=1 git push`, `git -c core.hooksPath=/dev/null push`
  блокируются хуком; pre-push с грязным деревом → отказ; `make setup` выставляет `core.hooksPath`
  - Подтверждено: `guard-bash.sh` с JSON PreToolUse на stdin — 25 обходов → exit 2 (все четыре
    из проверки, а ещё `-anm`, `-qn`, `git -C … commit -n`, `--no-verif`, `-c "core.hooksPath=…"`,
    `GIT_CONFIG_KEY_0=core.hooksPath`, `GOFLAGS=… make check`, `go env -w GOFLAGS`), 24 обычные
    команды → exit 0 (heredoc-коммиты с этими словами в теле, `-m "… -n …"`, `git push -n`,
    `git log -n`, grep по `SKIP_PREPUSH`/`core.hooksPath`); ≈10 мс на вызов. В клоне: `make
    setup` → `core.hookspath=scripts/git-hooks` в `.git/config`; pre-push → отказ на грязном
    дереве (`?? stray.txt`) и на пуше не-HEAD, чистый HEAD → `make check` → `✓ check clean`
- [ ] **AC5** — Матрица инъекций аудита: пойманы все 24 строки, кроме явно исключённых в
  «Решениях» с причиной. Проверка: повторить инъекции из раздела ENF отчёта → таблица в «Где мы
  сейчас»
- [ ] **AC6** — Нарушения границ из эксперимента ADP (b, c, d, e, f2, g, h, i, j, k) ловит гейт;
  a и l закрывает `fix-reference-code` (правила и тесты адаптеров, контракт событий). Проверка:
  повторить эксперимент → таблица
- [ ] **AC7** — Каждая новая проверка, которая срабатывает на текущем коде, имеет явный
  allowlist с ID находки на каждую запись; итоговый список в «Где мы сейчас» (его опустошает
  `fix-reference-code`). Проверка: `grep -rn "allowlist" internal/archtest` → записи с ID
- [ ] **AC8** — Гейт зелёный. Проверка: `make check` → `✓ check clean`, сырой
  `golangci-lint run ./...` → exit 0

## Шаги
### Подготовка
- [x] **S1** — Прочитать отчёт и отметки владельца (`review`), записать исключения и вопросы в
  «Решения»; закоммитить висящее удаление агентов reviewer/explorer (`refactor(harness): …`);
  зафиксировать базовую линию: `make check`, `make test-int`, время
  - Проверка: `git status` чистый; «Решения» содержат итог по отметкам · Закрывает: — · Зависит от: —
  - Сделано: удаление агентов — `b15a893`; отметок нет (см. «Решения»); базовая линия — в «Где мы
    сейчас»; `git status` чистый

### Честный гейт
- [x] **S2** — `lint.sh` fail-closed (код выхода, непустой отчёт, `--allow-serial-runners`,
  `golangci-lint config verify`); отдельный `GOLANGCI_LINT_CACHE` внутри репо (gitignored);
  fmt-check и pre-commit падают без инструмента; `make test` показывает ошибки компиляции;
  mockery без INF-шума (ENF-01, ENF-06, ENF-08, ENF-12)
  - Проверка: воспроизведения AC1 → FAIL · Закрывает: AC1 · Зависит от: S1
  - Сделано: `lint.sh` — `config verify`, затем `run` с `case` по коду выхода, OK только при
    exit 0 со строкой «0 issues.», JSON-разбор (jq и sed-фолбэк) удалён; кеш
    `.cache/golangci-lint` своего дерева; `allow-serial-runners` в `.golangci.yml`; `make lint`
    (и `VERBOSE=1`), pre-commit и `status --full` — через `lint.sh`; fmt-check и pre-commit
    падают без gofumpt/golangci-lint и на ошибке gofumpt; `test.sh` — `./...` по умолчанию,
    ошибки сборки и загрузки пакетов в выводе; mockery `log-level: warn`; `status.sh` видит
    файлы новых каталогов и `cmd/migrate/`; installer golangci-lint — с тега версии. Проверка
    — AC1; `make check` → `✓ check clean`
- [x] **S3** — `check` игнорирует `PKG`/`VERBOSE`, `COVER_FLOOR :=`; integration-тесты и
  roundtrip миграций в `make check` (docker обязателен, `make doctor` его проверяет); `make test`
  и `-short` приведены к правде (ENF-02, TST-03, ENF-05 частично)
  - Проверка: AC2, AC3 → как описано · Закрывает: AC2, AC3 · Зависит от: S2
  - Сделано: шаги `check` — через `GATE` (под-make с пустыми `PKG`/`VERBOSE`/`FORMAT`);
    `override COVER_FLOOR := 75`; шаг `make test-int` (юнит + integration, `-count=1`) вместо
    `make test`; `cmd/migrate/migrate_test.go` — лесенка up → down → up с отпечатком схемы и
    `TestDockerRequired`; integration-тест optimistic lock user; `-short` удалён (ничего не
    делал); vet и lint — с тегом `integration`; `make doctor` проверяет Docker-демон. Проверка —
    AC2, AC3; `make check` → `✓ check clean`
- [x] **S4** — Хуки: `make setup` ставит `core.hooksPath`; pre-push отказывает на грязном дереве;
  commit-msg принимает сообщения, которые генерирует сам git (Revert, fixup); хук Claude Code
  PreToolUse (короткий скрипт) блокирует обходы; `make release*` и `docker buildx*` убраны из
  allow; release гоняет `make check` до тега (ENF-03, ENF-09, ENF-11)
  - Проверка: AC4 → как описано · Закрывает: AC4 · Зависит от: S1
  - Сделано: `.claude/hooks/guard-bash.sh` (PreToolUse Bash) в `settings.json`; pre-push
    отказывает на грязном дереве и на пуше не-HEAD; commit-msg пропускает `Revert "…"`,
    `Reapply "…"`, `fixup!`/`squash!`/`amend!`; `make setup` — `git config core.hooksPath
    scripts/git-hooks`; `make release*` и `docker buildx*` убраны из allow; `release.sh` — чистое
    дерево и `make check` до тега. Проверка — AC4; `make check` → `✓ check clean`
- [ ] **S5** — Линтеры: nolintlint с обязательным объяснением, errcheck `check-blank`, forbidigo
  на `log.Print*`, `println`, `print`, запись в `os.Stdout`/`os.Stderr`; testifylint (ENF-07)
  - Проверка: инъекции 2b, 10a, 16 → пойманы · Закрывает: AC5 · Зависит от: S2
- [ ] **S6** — Миграции: запрет пустого down; деструктивные операторы (`DROP`, `TRUNCATE`,
  `ALTER … TYPE`, `RENAME`) только с маркером `-- infra-handoff:` (ENF-05)
  - Проверка: инъекции 7c, 7d, 8 → пойманы · Закрывает: AC5 · Зависит от: S3

### archtest «границы 2.0»
- [ ] **S7** — Импорты для любого каталога `internal/domain/*` без ручных списков: вне `module/`
  и тестов — никаких чужих доменов; `module/` импортирует чужой домен только корневым пакетом;
  `delivery` не импортирует `infra`; `infra/postgres/db` — только из `infra/postgres`; `platform`
  и `pkg` не импортируют домены; `*/mocks` — только из `_test.go`; IO-пакеты по префиксу, все
  файлы корня домена. Удалить per-domain depguard и `mocks-isolation` из `.golangci.yml` и
  `new-domain.sh` (ENF-04, ADP-03, TST-05)
  - Проверка: инъекции 1b, 13, M и ADP b, c, g, h, i → пойманы · Закрывает: AC5, AC6 · Зависит от: S3
- [ ] **S8** — Поведенческие инварианты: `time.Now` через типы (алиасы не проходят); запрет
  identity-closure для портов между доменами в `module/`; у каждой huma-операции `Security` или
  запись в явном списке публичных; уникальность всех экспортируемых типов `delivery/http`; тест,
  регистрирующий все хендлеры на одном huma API; запросы в `queries/*.sql` — только к таблицам
  своего домена (SEC-03, ADP-02, COD-05, ADP-03)
  - Проверка: инъекция 3b, ADP k, мутант M22 (убран Security) → пойманы · Закрывает: AC5, AC6, AC7 · Зависит от: S7
- [ ] **S9** — Самопроверка обвязки (небольшой Go-тест или скрипт в `make check`): упомянутые в
  прозе make-цели и пути существуют; required envconfig ⊆ `.env.example`; каждый домен с
  `infra/outbox` зарегистрирован в `cmd/migrate` (HAR-11, DOG-02, SEC-10)
  - Проверка: удалить цель из Makefile или источник из `cmd/migrate` → FAIL · Закрывает: AC5 · Зависит от: S3

### Итог
- [ ] **S10** — Повторить матрицу инъекций и эксперимент ADP, записать таблицы и allowlist в «Где
  мы сейчас»; гейт зелёный
  - Проверка: `make check` → `✓ check clean` · Закрывает: AC5, AC6, AC7, AC8 · Зависит от: S2–S9

## Решения
- Integration-тесты — часть `make check`, docker обязателен — владелец хочет, чтобы в SQL не было
  сюрпризов, а CI нет; единственный гейт должен проверять БД.
- Изоляция доменов и моков — в archtest вместо ручных списков depguard — покрывает любой новый
  домен без правки конфигов и упрощает `new-domain.sh`.
- Новые проверки приходят с явным allowlist текущих нарушений (ID находки на запись) — гейт
  зелёный на каждом шаге, а долг виден и конечен; опустошает `fix-reference-code`.
- Отметок владельца по отчёту нет: коллекция `review` и комментарии страницы пусты (29.09) —
  исключений (`disagree`) и вопросов (`discuss`) нет, в работе все находки программы.
- Lock golangci-lint — `run.allow-serial-runners: true` в `.golangci.yml`, а не флаг в `lint.sh`
  — действует на любой запуск (сырой, хуки, `lint.sh`). Второй экземпляр ждёт lock без лимита,
  поэтому AC1 для lock — «ждёт и проверяет по-настоящему»: дословное `flock … -c 'make lint'`
  блокирует само себя.
- `lint.sh` читает текстовый вывод и код выхода golangci-lint вместо JSON — коды 1/3/4/5/6/7
  различают все пути сбоя, а jq не нужен. «Непустой отчёт» — строка «0 issues.» при exit 0:
  заодно ловит `run.issues-exit-code: 0` в конфиге.
- Кеш линтера выставляет `lint.sh` — единственная точка входа lint (`make lint` с любым
  `FORMAT`/`VERBOSE`, pre-commit, `status --full`); сырой `golangci-lint` остаётся с общим кешем.
- `test.sh` печатает обычный вывод `go test` без строк прошедших пакетов вместо разбора `-json`
  — ошибки сборки видны как есть; сводка считает пакеты с тестами, число тестов доступно
  только из `-json`/`-v`.
- `cmd/migrate/**` — в эвристике инфра-поверхности (`status.sh`, rule `infra-impact`, skill
  `infra-handoff`): там регистрируются источники миграций (ENF-12).
- Integration в гейте — шагом `make test-int` (юнит + integration одним прогоном) вместо
  `make test`: каждый тест бежит один раз; `make test` остаётся быстрым циклом без Docker.
- `-count=1` у integration: исход зависит от Docker, а не только от кода, и кеш `go test`
  вернул бы «ok» прогона, где testkit пропустил тесты. Цена — ≈11 с на каждый `make check`.
- Docker обязателен через тест `TestDockerRequired`, а не через `docker info` в скрипте: он
  видит Docker так же, как testcontainers (тот игнорирует недоступный `DOCKER_HOST` и берёт
  сокет по умолчанию), и не требует docker CLI; `make doctor` проверяет демон для диагностики.
- Лесенка миграций сравнивает отпечаток схемы (колонки, индексы, ограничения, триггеры,
  функции без объектов расширений) до `up` и после `down`, а не только «up повторно проходит»:
  иначе пустой или разрушительный down 0003 проходит. Расширения вне отпечатка — `down` их не
  удаляет по правилу `sqlc-and-migrations`.
- Integration-тест optimistic lock user (`TestUpdate_StaleVersion`) добавлен здесь, хотя тесты —
  зона `fix-reference-code`: без него проверка AC3 (мутант M23) невыполнима; остальные методы
  репозиториев — S11 `fix-reference-code`.
- `PKG`/`VERBOSE`/`FORMAT` гасятся на под-make, а не `override` в Makefile: в цикле разработки
  они нужны. `GOFLAGS` из окружения тоже меняет `go test` (вне находок аудита) — префикс
  `GOFLAGS=` у команд гейта блокирует хук S4.
- Хук Claude Code проверен прямым запуском с JSON PreToolUse на stdin: живые `git push` и
  `SKIP_PREPUSH` в сессии запрещены. Хук подхватился в идущей сессии сразу после правки
  `settings.json` — заблокировал мой grep по `SKIP_PREPUSH`, поэтому `SKIP_PREPUSH=` и
  `core.hooksPath` блокируются только в команде с `git`, а флаги ищутся вне кавычек и тел heredoc.
- Хук — страховка от «удобного» обхода, не от обфускации (`git commit '-n'`, `sh -c`, base64
  его проходят): полный разбор shell в хуке не окупается.
- pre-push, кроме грязного дерева, отказывает на пуше ref не на HEAD (ENF-03 «не читает пушимые
  refs»): `make check` проверяет рабочее дерево, и только так оно равно пушимому коммиту. Дешевле
  временного worktree на пушимом SHA из предложения аудита.
- В основном репо `core.hooksPath` не менял: он выставлен вручную абсолютным путём и общий с
  чужим worktree `.claude/worktrees/agent-…`; `make setup` проверен в клоне.

## Handoff
—

## Где мы сейчас
S2–S4 закрыты. S4: обходы хуков из сессии Claude блокирует `.claude/hooks/guard-bash.sh`
(действует уже в этой сессии), pre-push пушит только чистый HEAD, `make setup` подключает хуки
через `core.hooksPath`, release гоняет `make check` до тега. `make check` → `✓ check clean` за
≈14 с (integration ≈11 с без кеша). Обнаружено: в S2 `config verify` закрыл строку матрицы 13d —
учесть в S10. Следующий шаг — S5.
