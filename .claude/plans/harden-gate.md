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
- **fx-аудит** (отдельный агент, 29.09; DI — 6/10): полный отчёт —
  `/home/mm/.cache/tmp/claude-1000/-home-mm-git-stit-core-go-backend-template/62f196b7-5be8-49bb-ba1f-05d2c3d76bb0/scratchpad/fx-audit.md`,
  скрипты и прототип boot-теста — `fxexp/` рядом. Статический граф держит `TestFxGraphValidates`;
  запуск (конструкторы, `Invoke`, lifecycle-хуки) не проверяет ничего: из 20 поломок DI (таблица
  «Эксперименты», X1–X17) гейт ловит 7. Находки этой части: FX-02 (нет boot-теста), FX-03 (без
  `AuthExtractor` httpkit пропускает авторизацию; client-IP без тега группы молча выключает
  brute-force), FX-04 (`timerkit.Module` нужен outbox неявно), FX-01 (HTTP останавливается после
  пула БД — 500 запросам в полёте), FX-08 (коллизия METHOD+path не паникует; забытый модуль
  домена или `fx.Invoke(Register)` проходит), FX-09 (outbox домена можно смонтировать чужим).

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
  - Подтверждено: `make lint` и `make check` → exit 2 и `FAIL: …`: битый YAML с `fmt.Println` (13c)
    и узел depguard не на месте (13d) — `config verify`; нет `.golangci.yml` — exit 6; неизвестный
    линтер — exit 3; `--timeout=100ms` — exit 4; `PKG=./intenral/...` — exit 7;
    `issues-exit-code: 0` — «без строки «0 issues.»». Lock 15 с: ждёт и даёт настоящий вердикт;
    `flock … -c 'make lint'` — взаимная блокировка без «OK». Чистое дерево → `OK`/`✓ check clean`
- [x] **AC2** — Гейт не сужается и не ослабляется из окружения. Проверка: `make check
  PKG=./internal/domain/auth/...` и `COVER_FLOOR=0 make check` при нарушении в другом домене →
  FAIL
  - Подтверждено: с `fmt.Println` в user `make check PKG=…auth…` (и с `VERBOSE=1 FORMAT=detailed`)
    → `FAIL: lint`; с падающим тестом → `FAIL: go test exit 1`; с 60 непокрытыми statement'ами
    `COVER_FLOOR=0 make check` и `make cover-check COVER_FLOOR=0` → `FAIL: coverage 68.2% < 75%`
- [x] **AC3** — `make check` гоняет integration-тесты и roundtrip миграций up → down → up; без
  docker падает с понятным сообщением. Проверка: `make check` → в выводе шаг integration;
  мутация `users.sql` (`version=$3 OR TRUE`) → FAIL
  - Подтверждено: `make check` → шаг `make test-int`, `✓ check clean`; M23 + `make gen` →
    `TestUpdate_StaleVersion/profile` FAIL; без Docker (`unshare -rm`) → `TestDockerRequired`
    FAIL; лесенка ловит пустой, мусорный, `DROP TABLE users CASCADE`, `-- noop` down 0003 и down
    0001 без `DROP FUNCTION`
- [x] **AC4** — Обходы хуков закрыты. Проверка: в сессии Claude Code `git commit -n`,
  `git commit --no-veri`, `SKIP_PREPUSH=1 git push`, `git -c core.hooksPath=/dev/null push`
  блокируются хуком; pre-push с грязным деревом → отказ; `make setup` выставляет `core.hooksPath`
  - Подтверждено: `guard-bash.sh` с JSON PreToolUse на stdin — наборы валидатора (79) и свой (49):
    все обходы → exit 2, в том числе многострочные (`git add -A⏎git commit -n`, `git status⏎git
    push --no-veri`), `git --git-dir .git commit -n`, `git merge --no-veri`, `GIT_CONFIG_KEY_0`;
    обычные команды (heredoc-коммиты с этими словами, `git push -n`, `git log -n`) → exit 0,
    ложные — только `git config --get core.hooksPath` и `git grep "SKIP_PREPUSH="`; ≈8 мс. Живьём
    двухстрочный `git commit -n` заблокирован. `make check` с `-i`, `MAKEFLAGS=i`, `-n`,
    `GATE=true`, `DETAIL=…`, `MAKE=true` (аргумент и env), `MAKEFLAGS=SHELL=/bin/true`,
    `.SHELLFLAGS`, `GOFLAGS=-skip=…` (env и файл go env) → «Останов», exit 2. В клоне pre-push на
    коммите с падающим тестом при `MAKEFLAGS=i`, `…SHELL=/bin/true`, `MAKEFILES` → `FAIL: go test
    exit 1`; чистый HEAD при `MAKEFLAGS=i` → `✓ check clean`; `make setup` →
    `core.hookspath=scripts/git-hooks`; грязное дерево и пуш не-HEAD → отказ
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
- [ ] **AC9** — Поломки DI, которые видны только при запуске, ловит гейт: boot-тест поднимает
  `cmd/api` целиком — без cookie 401, перебор паролей 429, событие доходит до подписчика, HTTP
  останавливается раньше пула БД (или allowlist FX-01). Проверка: повторить эксперименты
  fx-аудита X4–X16 → пойманы все, кроме X16 (закрывает `fix-reference-code`) и явно исключённых
  в «Решениях»

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
  - Сделано: `lint.sh` — `config verify`, затем `run` с разбором кода выхода, OK только при exit 0
    со строкой «0 issues.»; кеш `.cache/golangci-lint` своего дерева; `allow-serial-runners`;
    `make lint`, pre-commit и `status --full` — через `lint.sh`; fmt-check и pre-commit падают без
    инструментов; `test.sh` показывает ошибки сборки; mockery `log-level: warn`; `status.sh` видит
    новые каталоги и `cmd/migrate/`; installer golangci-lint — с тега. Проверка — AC1
- [x] **S3** — `check` игнорирует `PKG`/`VERBOSE`, `COVER_FLOOR :=`; integration-тесты и
  roundtrip миграций в `make check` (docker обязателен, `make doctor` его проверяет); `make test`
  и `-short` приведены к правде (ENF-02, TST-03, ENF-05 частично)
  - Проверка: AC2, AC3 → как описано · Закрывает: AC2, AC3 · Зависит от: S2
  - Сделано: шаги `check` — через `GATE` (под-make с пустыми `PKG`/`VERBOSE`/`FORMAT`), `override
    COVER_FLOOR := 75`; шаг `make test-int` вместо `make test`; лесенка миграций и
    `TestDockerRequired` в `cmd/migrate/migrate_test.go`; `-short` удалён; vet и lint с тегом
    `integration`; `make doctor` проверяет Docker. Проверка — AC2, AC3
- [x] **S4** — Хуки: `make setup` ставит `core.hooksPath`; pre-push отказывает на грязном дереве;
  commit-msg принимает сообщения, которые генерирует сам git (Revert, fixup); хук Claude Code
  PreToolUse (короткий скрипт) блокирует обходы; `make release*` и `docker buildx*` убраны из
  allow; release гоняет `make check` до тега (ENF-03, ENF-09, ENF-11)
  - Проверка: AC4 → как описано · Закрывает: AC4 · Зависит от: S1
  - Сделано: `guard-bash.sh` (PreToolUse Bash); pre-push — только чистый HEAD; commit-msg
    пропускает сообщения git; `make setup` — `core.hooksPath`; `make release*`, `docker buildx*`
    вне allow; `release.sh` — `make check` и `make e2e-smoke` до тега; по замечаниям валидатора —
    хук построчно, guard `make check`, pre-push без make-флагов вызывающего. Проверка — AC4
- [x] **S5** — Линтеры: nolintlint с обязательным объяснением, errcheck `check-blank`, forbidigo
  на `log.Print*`, `println`, `print`, запись в `os.Stdout`/`os.Stderr`; testifylint (ENF-07)
  - Проверка: инъекции 2b, 10a, 16 → пойманы · Закрывает: AC5 · Зависит от: S2
  - Сделано: nolintlint, testifylint, errcheck `check-blank`, forbidigo на `log.*`, `print(ln)`,
    `os.Stdout`/`os.Stderr`; 4 места в `cmd/` — `//nolint:<линтер> // <причина>`. Проверка: 2b →
    forbidigo, 10a → errcheck, 16 (`//nolint:depguard` без причины) → nolintlint
- [x] **S6** — Миграции: запрет пустого down; деструктивные операторы (`DROP`, `TRUNCATE`,
  `ALTER … TYPE`, `RENAME`) только с маркером `-- infra-handoff:` (ENF-05)
  - Проверка: инъекции 7c, 7d, 8 → пойманы · Закрывает: AC5 · Зависит от: S3
  - Сделано: `migrations-check.sh` — пустой down и разрушительный up без `-- infra-handoff:` →
    FAIL, в гейте до тестов. Проверка: 7c → migrations-check, 7d → лесенка S3, 8 (`DROP INDEX` без
    маркера) → migrations-check, с маркером → clean; 17 синтетических случаев — как ожидалось

### archtest «границы 2.0»
- [x] **S7** — Импорты для любого каталога `internal/domain/*` без ручных списков: вне `module/`
  и тестов — никаких чужих доменов; `module/` импортирует чужой домен только корневым пакетом;
  `delivery` не импортирует `infra`; `infra/postgres/db` — только из `infra/postgres`; `platform`
  и `pkg` не импортируют домены; `*/mocks` — только из `_test.go`; IO-пакеты по префиксу, все
  файлы корня домена; `infra/outbox` домена импортируют только его `module/` и `cmd/migrate`.
  Удалить per-domain depguard и `mocks-isolation` из `.golangci.yml` и `new-domain.sh` (ENF-04,
  ADP-03, TST-05, FX-09)
  - Проверка: инъекции 1b, 13, M, ADP b, c, g, h, i и fx X9 → пойманы · Закрывает: AC5, AC6 · Зависит от: S3
  - Сделано: `internal/archtest/imports_test.go` — `TestImportBoundaries` (все правила шага для
    не-тестового кода), `TestDomainRootIsTransportAgnostic` (все файлы корня: транспорт, драйверы,
    `os`/`net`/`syscall`/`database/sql` по префиксу), `TestPureLayersHaveNoIO` по префиксу;
    depguard удалён из `.golangci.yml` и `new-domain.sh`, проза и комментарии — про archtest.
    Проверка (`make check` в worktree): 1b, 13 (= ADP i), M, ADP b, c, g, h, fx X9, TST-05
    (`policy.go` с `net/http/httputil` и `os/signal`, `rules.go`, `service.go`), 12
    (`os.Getenv` в usecase) и сосед под `//nolint:depguard // причина` → FAIL archtest; чистое
    дерево и скелет `make new-domain` → зелёные. Allowlist не понадобился
- [ ] **S8** — Поведенческие инварианты: `time.Now` через типы (алиасы не проходят); запрет
  identity-closure для портов между доменами в `module/`; у каждой huma-операции `Security` или
  запись в явном списке публичных; уникальность всех экспортируемых типов `delivery/http`; тест,
  регистрирующий все хендлеры на одном huma API, с уникальностью пары (method, path); запросы в
  `queries/*.sql` — только к таблицам своего домена (SEC-03, ADP-02, COD-05, ADP-03, FX-08)
  - Проверка: инъекция 3b, ADP k, мутант M22 (убран Security), fx X5b и X5c → пойманы · Закрывает: AC5, AC6, AC7 · Зависит от: S7
- [ ] **S9** — Самопроверка обвязки (небольшой Go-тест или скрипт в `make check`): упомянутые в
  прозе make-цели и пути существуют; required envconfig ⊆ `.env.example`; каждый домен с
  `infra/outbox` зарегистрирован в `cmd/migrate`; модуль каждого домена подключён в `cmd/api`, а
  его delivery зарегистрирован (`fx.Invoke(Register)`) (HAR-11, DOG-02, SEC-10, FX-08)
  - Проверка: удалить цель из Makefile, источник из `cmd/migrate`, модуль домена из `cmd/api` → FAIL · Закрывает: AC5 · Зависит от: S3

### Запуск приложения
- [ ] **S10** — Boot-тест `cmd/api` (integration, контейнеры testkit): тот же набор fx-опций, что
  в `main`, реально стартует и останавливается; без cookie 401; перебор паролей → 429; событие
  доходит до подписчика; HTTP останавливается раньше пула БД — дешёвый обход на стороне шаблона
  или allowlist FX-01 до правки httpkit; миграции — тем же списком источников, что `cmd/migrate`
  (FX-02, FX-03, FX-04, FX-01)
  - Проверка: fx X4, X5b, X5c, X6, X9–X12, X14, X15 → пойманы · Закрывает: AC9 · Зависит от: S3

### Итог
- [ ] **S11** — Повторить матрицу инъекций, эксперимент ADP и эксперименты fx-аудита, записать
  таблицы и allowlist в «Где мы сейчас»; гейт зелёный
  - Проверка: `make check` → `✓ check clean` · Закрывает: AC5, AC6, AC7, AC8, AC9 · Зависит от: S2–S10

## Решения
- Integration-тесты — часть `make check`, docker обязателен: CI нет, единственный гейт должен
  проверять БД (владелец).
- Изоляция доменов и моков — в archtest вместо ручных списков depguard: любой новый домен покрыт
  без правки конфигов.
- Новые проверки приходят с явным allowlist текущих нарушений (ID находки на запись): гейт зелёный
  на каждом шаге, долг виден и конечен; опустошает `fix-reference-code`.
- Отметок владельца по отчёту нет (коллекция `review` и комментарии пусты, 29.09): в работе все
  находки программы.
- Lock golangci-lint — `run.allow-serial-runners: true` в `.golangci.yml` (действует на любой
  запуск); второй экземпляр ждёт lock без лимита, поэтому AC1 для lock — «ждёт и проверяет
  по-настоящему».
- `lint.sh` читает текст и код выхода golangci-lint вместо JSON (коды 1/3/4/5/6/7 различают сбои,
  jq не нужен); «непустой отчёт» — строка «0 issues.» при exit 0, заодно ловит
  `run.issues-exit-code: 0`. Кеш выставляет `lint.sh` — единственная точка входа lint; он же
  сбрасывает `GOLANGCI_DIFF_PROCESSOR_PATCH` — единственную env golangci-lint, которая меняет набор
  замечаний (`GL_DEBUG`, `GL_TEST_RUN` — только вывод).
- `test.sh` печатает обычный вывод `go test` без строк прошедших пакетов: ошибки сборки видны как
  есть.
- `cmd/migrate/**` — в эвристике инфра-поверхности (`status.sh`, rule `infra-impact`, skill
  `infra-handoff`): там регистрируются источники миграций (ENF-12).
- Integration в гейте — шагом `make test-int` (юнит + integration одним прогоном) с `-count=1`:
  исход зависит от Docker, кеш вернул бы «ok» прогона с пропущенными тестами. `make test` — быстрый
  цикл без Docker. Тесты репозиториев — `t.Parallel()` (у каждого свой контейнер).
- Docker обязателен через `TestDockerRequired`, а не `docker info`: тест видит Docker так же, как
  testcontainers; `make doctor` проверяет демон для диагностики.
- Лесенка миграций сравнивает отпечаток схемы (колонки, индексы, ограничения, триггеры, функции
  без объектов расширений) до `up` и после `down`: иначе пустой или разрушительный down проходит.
- `TestUpdate_StaleVersion` добавлен здесь, хотя тесты — зона `fix-reference-code`: без него
  проверка AC3 (M23) невыполнима.
- `PKG`/`VERBOSE`/`FORMAT` гасятся на под-make, а не `override`: в цикле разработки они нужны.
- `make check` не запускается (`$(error)` до шагов) с `-i`/`-n`/`-q`/`-t`, с подменой `MAKE`,
  `SHELL`, `.SHELLFLAGS`, `GATE`, `DETAIL` (аргумент, env, `MAKEFLAGS`) и с `GOFLAGS` (env или
  `go env -w`): один guard закрывает класс «clean без проверки», включая `MAKE=true` из окружения,
  — вместо `override` по переменной и правила `GOFLAGS=` в хуке (валидатор, B2). pre-push зовёт
  `make check` без `MAKEFLAGS`/`MFLAGS`/`GNUMAKEFLAGS`/`MAKEFILES` вызывающего.
- Хук Claude Code — страховка от «удобного» обхода, не от обфускации (`git commit '-n'`, `sh -c`,
  скрипт-файл, alias): флаги ищутся построчно, вне кавычек и тел heredoc, `SKIP_PREPUSH=` и
  `core.hooksPath` — только в команде с `git`. Проверен JSON PreToolUse на stdin и живьём.
- pre-push отказывает на грязном дереве и на пуше ref не на HEAD: `make check` проверяет рабочее
  дерево — только так оно равно пушимому коммиту (дешевле временного worktree). pre-commit
  проверяет дерево, а не индекс (ENF-03) — осознанно: авторитетный гейт — pre-push.
- В основном репо `core.hooksPath` не менял (выставлен вручную, общий с чужим worktree); `make
  setup` проверен в клоне.
- Конфиг гейта — в «Сначала спросить» CLAUDE.md (ENF-02); совет про `//nolint` в triage-failure —
  `repair-harness` S1.
- `make e2e-smoke` — шаг `release.sh` после `make check` (TST-03): на этой машине проходит без
  `.env.local` (5 passed, ≈18 с и venv).
- Срабатывания S5 на текущем коде — законные исключения (stderr без логгера, отложенные
  `Close`/`Rollback` в CLI): `//nolint:<линтер> // <причина>`, а не allowlist.
- depguard удалён целиком: изоляцию держит archtest, на который `//nolint` не действует. Правила
  импортов — для не-тестового кода: тесты берут фикстуры откуда угодно, моки — только тесты.
  `io`/`bufio` в usecase разрешены (поток — данные, не I/O), в чистых слоях — нет.
- Пустой down — файл без непробельных символов; `-- noop` с причиной законен для миграции только
  данных, DDL-миграцию с таким down ловит лесенка.
- Разрушительный оператор ищется по up-файлу без комментариев и строковых литералов, по
  операторам между `;`; `ALTER … TYPE` — только смена типа колонки; down-файлы не проверяются.
  `migrations-check` в гейте — до тестов: дешёвая статика раньше долгого integration.
- Находки fx-аудита: проверки (FX-01–FX-04, FX-08, FX-09) — сюда (S7–S10, AC9); код (FX-05,
  FX-06, FX-10, FX-11, порядок модулей) — `fix-reference-code` S9–S10; проза (FX-07) —
  `repair-harness`; дефекты китов — `backlog.md`.

## Handoff
—

## Где мы сейчас
Этап «Честный гейт» (S2–S6) закрыт, замечания валидатора к нему — коммитом `fix(gate)`, AC4
перепроверен. S7 закрыт: границы импортов любого домена держит archtest, depguard удалён;
инъекции шага пойманы, allowlist пуст. `make check` → `✓ check clean` за ≈8 с (тёплый кеш). Уже
пойманы строки матрицы 1b, 2b, 7c, 7d, 8, 10a, 12, 13, 13c, 13d, 16, M и ADP b, c, g, h, i, fx X9.
Исключения без долга — 4 `//nolint` с причиной в `cmd/`. Следующее — мелочи валидатора к
`72bb8ca` (`fix(gate)`), затем S8.
