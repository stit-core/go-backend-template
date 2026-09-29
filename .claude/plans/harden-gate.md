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
- **Отчёт аудита** (ID вида `ENF-01`): https://claude.ai/artifact/L3Y8vWRofsvzf8PGQos4dE
  (Artifact `action: read`). Оценки на `03b57d2`: обвязка 6.5–7, код 6, тесты 5, адаптеры 5.5,
  принуждение 5.5, security 5, догфудинг 7–8, жизненный цикл 4.5.
- **Отметки владельца по находкам** — `ArtifactData` `list`, `url` отчёта, `collection: review`
  (`disagree` — не чинить, `discuss` — спросить, `note` — учесть); итог S1 — в «Решениях».
- **Следующие части:** `fix-reference-code` (эталонный код, security, тесты, покрытие) и
  `repair-harness` (проза обвязки, жизненный цикл, повторный аудит) — стартуют после этой;
  golibinfra меняется параллельно своими планами. Порядок, метод и решения владельца —
  `.claude/plans/audit-program.md`.
- **Принципы программы** (владелец): всё найденное починить без оверинжиниринга — понятный,
  консистентный код; короткие скрипты или Go-тесты, один канонический способ; удалить лучше, чем
  добавить; машина держит важное. Правки зон «Сначала спросить» (migrations, pkg/policy,
  pkg/tokens, authgate, конфиг гейта, bump golibinfra в `go.mod`) в рамках программы разрешены;
  миграции — только новыми файлами. Коммит на шаг (Conventional Commits); push — по политике
  программы (`audit-program`).
- **Находки этой части** (подробности и `file:line` — в отчёте): гейт и хуки — ENF-01–ENF-03,
  ENF-05–ENF-09, ENF-11, ENF-12, TST-03 (закрыты S2–S6); границы — ENF-04 (ручные списки
  depguard и `mocks-isolation`), TST-05 (дыры archtest: имена файлов, точное совпадение
  IO-пакетов, алиас `time`, нет `delivery → infra`), ADP-02 (identity-closure через границу
  доменов), ADP-03 (мост через platform, sqlc-типы в домене, JOIN к чужой таблице), SEC-03 (ручка
  без `Security` публична), COD-05 (дубль имени типа в huma — panic при регистрации, которую
  `fx.ValidateApp` не видит); самопроверка обвязки — HAR-11, DOG-02, SEC-10.
- **Эксперименты аудита для перепроверки** — разделы ENF и ADP отчёта: матрица из 24 инъекций
  (поймано 10) и 14 нарушений в адаптерах (поймано 1.5 из 8 обязательных).
- **fx-аудит** (29.09; DI — 6/10):
  `/home/mm/.cache/stit-audit-program/fx-audit.md`,
  прототипы и скрипты — `fxexp/` рядом. Из 20 поломок DI (X1–X17) гейт ловил 7: запуск
  (конструкторы, `Invoke`, хуки) не проверял никто. Находки: FX-02 (нет boot-теста), FX-03 (нет
  `AuthExtractor` — авторизации нет; client-IP без группы — нет brute-force), FX-04 (timerkit нужен
  outbox неявно), FX-01 (HTTP гасится после пула), FX-08 (коллизия METHOD+path, забытый модуль или
  `Register`), FX-09 (чужой outbox).

## Не входит
- Исправление кода доменов, security и тестов — `fix-reference-code`; сюда входят только проверки
  и явные allowlist'ы текущих нарушений.
- Правки прозы обвязки и жизненный цикл шаблона — `repair-harness` (кроме строк, без которых
  новые проверки непонятны).
- Правки golibinfra — планами в `../golibinfra` (партии, теги v1.2.0+); здесь — проверки и
  дешёвые обходы на стороне шаблона, их снимает bump в `fix-reference-code` (S1, S9).

## Критерии приёмки
- [x] **AC1** — Линтер не может пройти молча: при битом `.golangci.yml`, неизвестном линтере,
  таймауте и несуществующем `PKG` и `make lint`, и `make check` падают; при занятом lock lint
  ждёт его и затем проверяет по-настоящему — «OK» без прогона невозможен.
  Проверка: воспроизведения из ENF-01 (копия конфига с ошибкой, `--timeout=100ms`, опечатка в
  `PKG`; lock держит фоновый `flock "$TMPDIR/golangci-lint.lock" sleep 15`) → exit ≠ 0 и
  понятное сообщение, при lock — ожидание и настоящий вердикт
  - Подтверждено: `make lint`/`make check` → FAIL на битом YAML (13c), узле не на месте (13d), без
    `.golangci.yml`, неизвестном линтере, `--timeout=100ms`, `PKG=./intenral/...`,
    `issues-exit-code: 0`; при lock — ждёт и даёт настоящий вердикт; чистое дерево → `OK`
- [x] **AC2** — Гейт не сужается и не ослабляется из окружения. Проверка: `make check
  PKG=./internal/domain/auth/...` и `COVER_FLOOR=0 make check` при нарушении в другом домене →
  FAIL
  - Подтверждено: с `fmt.Println` в user `make check PKG=…auth…` (и с `VERBOSE=1 FORMAT=detailed`)
    → `FAIL: lint`; с падающим тестом → `FAIL: go test exit 1`; с 60 непокрытыми statement'ами
    `COVER_FLOOR=0 make check` и `make cover-check COVER_FLOOR=0` → `FAIL: coverage 68.2% < 75%`
- [x] **AC3** — `make check` гоняет integration-тесты и roundtrip миграций up → down → up; без
  docker падает с понятным сообщением. Проверка: `make check` → в выводе шаг integration;
  мутация `users.sql` (`version=$3 OR TRUE`) → FAIL
  - Подтверждено: шаг `make test-int` в `make check`; M23 + `make gen` → FAIL; без Docker →
    `TestDockerRequired` FAIL; лесенка ловит пустой, мусорный, разрушительный и `-- noop` down
- [x] **AC4** — Обходы хуков закрыты. Проверка: в сессии Claude Code `git commit -n`,
  `git commit --no-veri`, `SKIP_PREPUSH=1 git push`, `git -c core.hooksPath=/dev/null push`
  блокируются хуком; pre-push с грязным деревом → отказ; `make setup` выставляет `core.hooksPath`
  - Подтверждено: `guard-bash.sh` на наборах валидатора (79) и своём (49) — все обходы git-хуков
    → exit 2, в том числе многострочные, `--git-dir`, `git merge --no-veri`, `GIT_CONFIG_KEY_0`;
    обычные команды → exit 0 (ложные — `git config --get core.hooksPath`, `git grep
    "SKIP_PREPUSH="`); make- и `GOFLAGS`-кейсы останавливает `make check` («Останов»: `-i`, `-n`,
    `GATE=…`, `MAKE=true`, `SHELL`, `.SHELLFLAGS`, `GOFLAGS` из env и go env). В клоне pre-push на
    падающем коммите при `MAKEFLAGS=i`, `…SHELL=/bin/true`, `MAKEFILES` → FAIL; чистый HEAD → `✓`;
    `make setup` → `core.hookspath=scripts/git-hooks`; грязное дерево и не-HEAD → отказ
- [ ] **AC5** — Матрица инъекций аудита: пойманы все 24 строки, кроме явно исключённых в
  «Решениях» с причиной. Проверка: повторить инъекции из раздела ENF отчёта → таблица в «Где мы
  сейчас»
- [ ] **AC6** — Нарушения границ из эксперимента ADP (b, c, d, e, f2, g, h, i, j, k) ловит гейт;
  a и l закрывает `fix-reference-code` (правила и тесты адаптеров, контракт событий). Проверка:
  повторить эксперимент → таблица
- [ ] **AC7** — Каждая новая проверка, которая срабатывает на текущем коде, имеет явный
  allowlist с ID находки на каждую запись; итоговый список в «Где мы сейчас» (его опустошает
  `fix-reference-code`). Проверка: `grep -rnE '"[A-Z]{2,3}-[0-9]{2}"' internal/archtest cmd` →
  записи с ID; запись без ID находки или без нарушения → FAIL теста
- [ ] **AC8** — Гейт зелёный. Проверка: `make check` → `✓ check clean`, сырой
  `golangci-lint run ./...` → exit 0
- [x] **AC9** — Поломки DI, которые видны только при запуске, ловит гейт: boot-тест поднимает
  `cmd/api` целиком — без cookie 401, перебор паролей 429, событие доходит до подписчика, HTTP
  останавливается раньше пула БД (до httpkit v1.2.0 — обходом `beforeHTTP`). Проверка: повторить эксперименты
  fx-аудита X4–X16 → пойманы все, кроме X16 (закрывает `fix-reference-code`) и явно исключённых
  в «Решениях»
  - Подтверждено: `make check` в worktree — X4 («didn't start cleanly»), X5b (паника huma), X5c,
    X9, X10, X10b, X11 (403 вместо 200/401), X12 (нет 429), X13, X14 (ошибка конструктора), X15
    (событие не доходит) → FAIL `TestBoot` (и статических проверок S7–S9, где они есть); X6
    невозможен по построению (`beforeHTTP`), а без `beforeHTTP` порядок остановки валит
    `TestBoot`; X16 — `fix-reference-code`
- [x] **AC10** — Контракт API — файл `openapi.json` в репо: его пишет `make gen` без запуска
  сервиса из тех же регистраций, что у сервиса, и он совпадает с тем, что отдаёт живой сервис.
  Проверка: правка ручки (например, Summary) без `make gen` → `make check` FAIL; boot-тест
  сверяет `/openapi.json` с файлом
  - Подтверждено: Summary `getUser` без `make gen` → FAIL `TestOpenAPISpec` (и `make gen-check`),
    с `make gen` → `✓ check clean`; ручная правка файла → FAIL; операция, которую модуль домена
    регистрирует мимо `registerHTTP`, → FAIL `TestBoot` (живой `/openapi.json` ≠ файлу); `make
    gen` дважды — файл тот же; `HTTP_*` из окружения на файл не влияют

## Шаги
### Подготовка
- [x] **S1** — Прочитать отчёт и отметки владельца (`review`), записать исключения и вопросы в
  «Решения»; закоммитить висящее удаление агентов reviewer/explorer (`refactor(harness): …`);
  зафиксировать базовую линию: `make check`, `make test-int`, время
  - Проверка: `git status` чистый; «Решения» содержат итог по отметкам · Закрывает: — · Зависит от: —
  - Сделано: удаление агентов — `b15a893`; отметок нет; базовая линия — `make check` ≈14 с

### Честный гейт
- [x] **S2** — `lint.sh` fail-closed (код выхода, непустой отчёт, `--allow-serial-runners`,
  `golangci-lint config verify`); отдельный `GOLANGCI_LINT_CACHE` внутри репо (gitignored);
  fmt-check и pre-commit падают без инструмента; `make test` показывает ошибки компиляции;
  mockery без INF-шума (ENF-01, ENF-06, ENF-08, ENF-12)
  - Проверка: воспроизведения AC1 → FAIL · Закрывает: AC1 · Зависит от: S1
  - Сделано: `lint.sh` fail-closed (`config verify`, код выхода, «0 issues.»), свой кеш; fmt-check
    и pre-commit без инструмента — FAIL; ошибки сборки видны; mockery тихий. Проверка — AC1
- [x] **S3** — `check` игнорирует `PKG`/`VERBOSE`, `COVER_FLOOR :=`; integration-тесты и
  roundtrip миграций в `make check` (docker обязателен, `make doctor` его проверяет); `make test`
  и `-short` приведены к правде (ENF-02, TST-03, ENF-05 частично)
  - Проверка: AC2, AC3 → как описано · Закрывает: AC2, AC3 · Зависит от: S2
  - Сделано: `GATE` (под-make без `PKG`/`VERBOSE`/`FORMAT`), `override COVER_FLOOR`, шаг
    `make test-int`, лесенка миграций и `TestDockerRequired`, `-short` удалён. Проверка — AC2, AC3
- [x] **S4** — Хуки: `make setup` ставит `core.hooksPath`; pre-push отказывает на грязном дереве;
  commit-msg принимает сообщения, которые генерирует сам git (Revert, fixup); хук Claude Code
  PreToolUse (короткий скрипт) блокирует обходы; `make release*` и `docker buildx*` убраны из
  allow; release гоняет `make check` до тега (ENF-03, ENF-09, ENF-11)
  - Проверка: AC4 → как описано · Закрывает: AC4 · Зависит от: S1
  - Сделано: `guard-bash.sh`, pre-push только чистого HEAD, commit-msg для сообщений git, `make
    setup` — `core.hooksPath`, release без allow и с `make check` + `make e2e-smoke` до тега; по
    замечаниям валидатора — хук построчно и guard `make check`. Проверка — AC4
- [x] **S5** — Линтеры: nolintlint с обязательным объяснением, errcheck `check-blank`, forbidigo
  на `log.Print*`, `println`, `print`, запись в `os.Stdout`/`os.Stderr`; testifylint (ENF-07)
  - Проверка: инъекции 2b, 10a, 16 → пойманы · Закрывает: AC5 · Зависит от: S2
  - Сделано: nolintlint, testifylint, errcheck `check-blank`, forbidigo на `log.*`, `print(ln)`,
    `os.Stdout`/`os.Stderr`. Проверка: 2b → forbidigo, 10a → errcheck, 16 → nolintlint
- [x] **S6** — Миграции: запрет пустого down; деструктивные операторы (`DROP`, `TRUNCATE`,
  `ALTER … TYPE`, `RENAME`) только с маркером `-- infra-handoff:` (ENF-05)
  - Проверка: инъекции 7c, 7d, 8 → пойманы · Закрывает: AC5 · Зависит от: S3
  - Сделано: `migrations-check.sh` — пустой down и разрушительный up без `-- infra-handoff:` → FAIL,
    до тестов. Проверка: 7c, 8 → migrations-check; 7d → лесенка S3; с маркером → clean

### archtest «границы 2.0»
- [x] **S7** — Импорты для любого каталога `internal/domain/*` без ручных списков: вне `module/`
  и тестов — никаких чужих доменов; `module/` импортирует чужой домен только корневым пакетом;
  `delivery` не импортирует `infra`; `infra/postgres/db` — только из `infra/postgres`; `platform`
  и `pkg` не импортируют домены; `*/mocks` — только из `_test.go`; IO-пакеты по префиксу, все
  файлы корня домена; `infra/outbox` домена импортируют только его `module/` и `cmd/migrate`.
  Удалить per-domain depguard и `mocks-isolation` из `.golangci.yml` и `new-domain.sh` (ENF-04,
  ADP-03, TST-05, FX-09)
  - Проверка: инъекции 1b, 13, M, ADP b, c, g, h, i и fx X9 → пойманы · Закрывает: AC5, AC6 · Зависит от: S3
  - Сделано: `imports_test.go` — `TestImportBoundaries`, `TestDomainRootIsTransportAgnostic` (весь
    корень), `TestPureLayersHaveNoIO` (по префиксу); depguard удалён. Проверка (`make check`): 1b,
    13 (= ADP i), M, ADP b, c, g (и мост вне `platform`), h, fx X9, TST-05 (`net/http/httputil`
    и `os/signal` в `policy.go`, `rules.go`, `service.go`), 12 и сосед под `//nolint:depguard` →
    FAIL archtest
- [x] **S8** — Поведенческие инварианты: `time.Now` через типы (алиасы не проходят); запрет
  identity-closure для портов между доменами в `module/`; у каждой huma-операции `Security` или
  запись в явном списке публичных; уникальность всех экспортируемых типов `delivery/http`; тест,
  регистрирующий все хендлеры на одном huma API, с уникальностью пары (method, path); запросы в
  `queries/*.sql` — только к таблицам своего домена (SEC-03, ADP-02, COD-05, ADP-03, FX-08)
  - Проверка: инъекция 3b, ADP k, мутант M22 (убран Security), fx X5b и X5c → пойманы · Закрывает: AC5, AC6, AC7 · Зависит от: S7
  - Сделано: archtest — `time.Now` по импортам файла, `TestNoCrossDomainIdentityClosure`,
    уникальность типов `delivery/http`, `TestQueriesUseOwnTables`; `registerHTTP` в `cmd/api` и
    `cmd/api/api_test.go` (API как у сервиса без БД). Проверка (`make check`): 3b и dot-import,
    ADP k (и JOIN через запятую, подзапрос, `"users"`), M22, X5b, X5c и тот же маршрут с другим
    параметром, `UserView` в двух доменах (COD-05, и `userView`), новая closure, лишняя запись
    allowlist или запись без ID → FAIL с точной причиной
- [x] **S9** — Самопроверка обвязки (небольшой Go-тест или скрипт в `make check`): упомянутые в
  прозе make-цели и пути существуют; required envconfig ⊆ `.env.example`; каждый домен с
  `infra/outbox` зарегистрирован в `cmd/migrate`; модуль каждого домена подключён в `cmd/api`, а
  его delivery зарегистрирован (`fx.Invoke(Register)`) (HAR-11, DOG-02, SEC-10, FX-08)
  - Проверка: удалить цель из Makefile, источник из `cmd/migrate`, модуль домена из `cmd/api` → FAIL · Закрывает: AC5 · Зависит от: S3
  - Сделано: `harness_test.go` — `TestProseReferencesExist`, `TestDomainsAreWired`;
    `cmd/api/main_test.go` — `TestEnvExampleHasRequired`. Проверка (`make check`): удалены цель
    `plan-check` и `docs/reference/release.md`, X13, X10, X10b, `fx.Invoke(verifnats.Register)`,
    `PG_DSN` из `.env.example`, ссылка на неотслеживаемый файл → FAIL с точной причиной; свежий
    worktree → clean

### Запуск приложения
- [x] **S10** — Boot-тест `cmd/api` (integration, контейнеры testkit): тот же набор fx-опций, что
  в `main`, реально стартует и останавливается; без cookie 401; перебор паролей → 429; событие
  доходит до подписчика; HTTP останавливается раньше пула БД — дешёвый обход на стороне шаблона
  до httpkit v1.2.0; миграции — тем же списком источников, что `cmd/migrate`
  (FX-02, FX-03, FX-04, FX-01)
  - Проверка: fx X4, X5b, X5c, X6, X9–X12, X14, X15 → пойманы · Закрывает: AC9 · Зависит от: S3
  - Сделано: `cmd/api/boot_test.go` — граф `service` (общий с `main`) на контейнерах testkit,
    миграции — `sources.Kits` (общий с `cmd/migrate`); `beforeHTTP` — пул, NATS и Redis раньше
    HTTP. Проверка — AC9

### Контракт API
- [x] **S11** — `make gen` без запуска сервиса пишет `openapi.json` (OpenAPI 3.1, детерминированно,
  с отступами) из того же набора регистраций huma, что у `main`; `gen-check` ловит дрейф;
  boot-тест сверяет `/openapi.json` живого сервиса с файлом
  - Проверка: правка Summary без `make gen` → `make check` FAIL; операция мимо `registerHTTP` →
    boot-тест FAIL · Закрывает: AC10 · Зависит от: S8, S10
  - Сделано: `TestOpenAPISpec` (`cmd/api/api_test.go`) собирает API как сервис (`buildAPI`) при
    конфиге по умолчанию: `-update` пишет файл (`make gen`), без флага — проверка дрейфа (шаг
    тестов и `gen-check`); `TestBoot` сверяет живой `/openapi.json` с файлом. Проверка — AC10

### Итог
- [ ] **S12** — Повторить матрицу инъекций, эксперимент ADP и эксперименты fx-аудита, записать
  таблицы и allowlist в «Где мы сейчас»; гейт зелёный
  - Проверка: `make check` → `✓ check clean` · Закрывает: AC5, AC6, AC7, AC8, AC9, AC10 · Зависит от: S2–S11

## Решения
- Integration-тесты — часть `make check`, docker обязателен: CI нет, гейт должен проверять БД.
  Новые проверки — с allowlist текущих нарушений (ID находки на запись): гейт зелёный на каждом
  шаге, долг виден и конечен. Отметок владельца по отчёту нет (29.09): в работе все находки.
- Lock golangci-lint — `allow-serial-runners: true`: второй прогон ждёт lock без лимита (AC1).
- `lint.sh` читает код выхода и текст, не JSON (коды 1/3/4/5/6/7 различают сбои); «непустой отчёт»
  — «0 issues.» при exit 0 (ловит и `issues-exit-code: 0`). `lint.sh` — единственная точка входа:
  ставит кеш и сбрасывает `GOLANGCI_DIFF_PROCESSOR_PATCH` (единственная env, меняющая набор
  замечаний). `test.sh` — вывод без прошедших пакетов; `cmd/migrate/**` — инфра-поверхность.
- Integration — шагом `make test-int` (юнит + integration одним прогоном) с `-count=1`: исход
  зависит от Docker, кеш вернул бы «ok» прогона с пропущенными тестами; тесты с контейнером —
  `t.Parallel()`. Docker обязателен через `TestDockerRequired` (видит Docker как testcontainers).
- Лесенка миграций сравнивает отпечаток схемы (колонки, индексы, ограничения, триггеры, функции
  без объектов расширений) до `up` и после `down`: иначе пустой или разрушительный down проходит.
- `TestUpdate_StaleVersion` добавлен здесь (зона `fix-reference-code`): без него AC3 (M23) не
  проверить. `PKG`/`VERBOSE`/`FORMAT` гасятся на под-make, а не `override`: в цикле они нужны.
- `make check` не запускается (`$(error)` до шагов) с `-i`/`-n`/`-q`/`-t`, с подменой `MAKE`,
  `SHELL`, `.SHELLFLAGS`, `GATE`, `DETAIL` (аргумент, env, `MAKEFLAGS`) и с `GOFLAGS` (env или
  `go env -w`): один guard закрывает класс «clean без проверки», включая `MAKE=true` из окружения,
  — вместо `override` по переменной и правила `GOFLAGS=` в хуке (валидатор, B2); сам guard —
  `override`. pre-push зовёт `make check` без `MAKEFLAGS`/`MFLAGS`/`GNUMAKEFLAGS`/`MAKEFILES`
  вызывающего: `--eval=.IGNORE:`, `MAKEFILES`, лишний `-f` в ручном прогоне — обфускация, до
  remote её не пускает pre-push.
- Без cgo `test.sh` идёт без `-race` и печатает WARN, а не падает: гейт не требует C-компилятор.
- Хук Claude Code — страховка от «удобного» обхода, не от обфускации (`git commit '-n'`, `sh -c`,
  скрипт-файл, alias): флаги ищутся построчно, вне кавычек и тел heredoc, `SKIP_PREPUSH=` и
  `core.hooksPath` — только в команде с `git`. Проверен JSON PreToolUse на stdin и живьём.
- pre-push отказывает на грязном дереве и на пуше ref не на HEAD: `make check` проверяет рабочее
  дерево — только так оно равно пушимому коммиту (дешевле временного worktree). pre-commit
  проверяет дерево, а не индекс (ENF-03) — осознанно: авторитетный гейт — pre-push.
- `core.hooksPath` основного репо не менял (общий с чужим worktree); `make setup` — в клоне. Конфиг
  гейта — в «Сначала спросить» CLAUDE.md (ENF-02); `//nolint` в triage-failure — `repair-harness`.
- `make e2e-smoke` — шаг `release.sh` после `make check` (TST-03): проходит без `.env.local`.
- Срабатывания S5 на текущем коде — законные исключения (stderr без логгера, отложенные
  `Close`/`Rollback` в CLI): `//nolint:<линтер> // <причина>`, а не allowlist.
- HTTP-операции регистрирует одна функция `registerHTTP` в `cmd/api`, а не `fx.Invoke` в модуле
  домена: это единственный набор операций, который регистрирует сервис и по которому тесты (и
  OpenAPI S11) собирают API без БД и брокера; хендлеры там пустые — huma нужны только типы.
  NATS-подписки остаются в модулях: к контракту API они не относятся.
- Проверки операций — в `cmd/api` рядом с `registerHTTP`, не в archtest: пакет `main` не
  импортируется. `publicOperations` — перечень публичных по замыслу, не долг; запись без
  операции — провал; после httpkit v1.3.0 список заменяют пометки `httpkit.Public` в `Register`
  доменов (`fix-reference-code` S9). `authgate.SecurityScheme` вынесен из `Module`, чтобы собрать
  API без JWT.
- `time.Now` — по таблице импортов файла (алиас, dot-import), без go/types: тот же охват без
  загрузки пакетов. Владелец таблицы — единственный домен, чьи запросы её трогают (таблицы — из
  `CREATE TABLE` в `migrations/`); запрос трогает таблицу, если её имя стоит в позиции таблицы:
  после FROM, JOIN, INTO, UPDATE, USING и в списке FROM через запятую (схема, кавычки, алиас) —
  так видны неявный JOIN, подзапрос и `"users"`, а колонка `orders.notes` при таблице `notes`
  соседа — не обращение (N2, B2-B1). Ключевое слово и список разбираются отдельно: однострочная
  регулярка принимает `JOIN` без алиаса за алиас и теряет таблицу. kit-таблиц в запросах нет —
  схема sqlc только `migrations/`. Allowlist — один тип
  `allowlist.List` (`internal/testkit/allowlist`) в archtest и `cmd/api`: нарушение → ID
  находки; запись без нарушения или без ID валит тест, проверка AC7 — grep по ID (N4).
- Мост к чужому домену закрыт слоями, а не списком импортёров: домен вне доменов импортирует только
  `internal/platform` и `pkg`, а они — только друг друга; пакет-мост (`internal/<каталог>`,
  `cmd/…`, `internal/testkit`) домену недоступен, а `cmd/` и `internal/testkit` (`pgseed`)
  импортируют домены (N1). Столкновение `userView` с `UserView` ловит `buildAPI` (паника huma
  `duplicate name` в `TestHTTPOperations` и `TestOpenAPISpec`) — archtest его не дублирует (N3).
- `gen-check` — как остальные шаги: вывод только на провале, последняя строка `OK`/`FAIL` (N5).
  Порог покрытия не трогаю и тестов доменов не дописываю (N7): запас ≈1 statement, но тесты —
  `fix-reference-code` (S13 — канон и happy-path, S14 — широкий scope, порог 90), а тесты к
  `Lookup*` и `changePassword` сейчас переписали бы её S6–S8.
- Матрица ENF: строка 15 (тест без ассертов) — `TestTestsAssert`: тест домена или платформы без
  утверждения — провал (пустышка поднимала покрытие 75,1 → 75,6%). Утверждение — `require`,
  `assert`, `Assert*` тестового кита или хелпер пакета: функция без результатов или общая таблица
  (var) с утверждением; фикстура с `require` (`newUC`) возвращает значение и не в счёт — иначе
  пустышка на фикстуре проходила бы (NB1). Исключены: строка 9 (статус в хендлере) — где живёт
  бизнес-правило, решает смысл, а не синтаксис: `if` в хендлере не отличить от маппинга
  транспорта, эвристика по константам домена пропустит литерал и поле; держат правило и ревью
  (`repair-harness` S1–S2, HAR-01). Строка 11 (≈30 непокрытых statement'ов) — сейчас `cover-check` → 71,6%, но только
  из-за запаса ≈1 statement; надёжно ловит лишь покрытие по функциям или диффу (TST-04), а это
  перестройка покрытия `fix-reference-code` S14 — здесь вышел бы allowlist из 16 функций с 0%.
- Самопроверка обвязки — Go-тесты, не скрипт: кто на что ссылается в Go-коде (AST) и какие env
  required (reflect конфигов) надёжнее grep. Проза — только в коде (inline и fenced): цели `make`
  и пути репо (и `.claude/…`); репо — что отслеживает git (`git ls-files`, файл есть на диске),
  игнорируемое (`git check-ignore`: `test/.venv`) — локальное, не ссылка: исход не зависит от
  клона, а неотслеживаемый файл мёртвую ссылку не оживляет; плейсхолдеры (`<d>`, `NNNN`, `*`) —
  не ссылки; цели хаба (`bump`, `ps`, `ship`) — своим списком; ссылка на `docs/stack/<kind>.md` —
  по шаблону, а артефакты стека, планы и память stack-deep-dive (там и будущие файлы) не
  проверяются. Env — в `cmd/api` рядом с
  `kitConfigs` (стал списком конструкторов): allowlist'ы — в `internal/archtest` и `cmd/api`.
- Скелет `make new-domain` без подключения в `cmd/api` валит `TestDomainsAreWired` (как и покрытие,
  TST-04): подключение скелета — `fix-reference-code` S12 (AC10).
- FX-01 — дешёвым обходом, без allowlist: `beforeHTTP` поднимает пул, NATS и Redis раньше сервера,
  и fx гасит HTTP до них (запросы в полёте не ловят закрытых клиентов); порядок больше не зависит
  от места `pgkit.Module`. HTTP по-прежнему гасится после подписчика и джобов — это запросам не
  мешает; «HTTP первым» и снятие `beforeHTTP` — bump httpkit v1.2.0 в `fix-reference-code` S1.
- Boot-тест — не параллельный (`t.Setenv`: конфиг из env, как у `main`), ≈4 с; `make check` ≈10,5 с.
  `TestFxGraphValidates` — с `fx.NopLogger`: лог fx на провале прятал соседние падения.
- `openapi.json` — в корне репо: одна спецификация сервиса рядом с `go.mod`, каталог `api/` ради
  одного файла — лишняя структура. Генератор — тест `TestOpenAPISpec` с `-update` рядом с
  `buildAPI`, а не `cmd/openapi`: `main` не импортируется, отдельной команде пришлось бы выносить
  регистрацию в свой пакет; без флага тот же тест ловит дрейф. Спецификация — при конфиге по
  умолчанию (`HTTP_*` окружения сняты), поэтому файл не зависит от шелла.
- depguard удалён целиком: изоляцию доменов и моков держит archtest без списков доменов в
  конфигах, `//nolint` на него не действует. Правила импортов — для не-тестового кода: тесты
  берут фикстуры откуда угодно, моки — только тесты. `io`/`bufio` в usecase разрешены (поток —
  данные, не I/O), в чистых слоях — нет.
- Пустой down — без непробельных символов (`-- noop` с причиной законен для миграции данных, DDL
  с таким down ловит лесенка). Разрушительный оператор — по up без комментариев и строк;
  `ALTER … TYPE` — смена типа колонки; down не проверяется. `migrations-check` — до тестов.
- Находки fx-аудита: проверки (FX-01–FX-04, FX-08, FX-09) — сюда (S7–S10, AC9); код: FX-01 и
  FX-05 — кит (golibinfra v1.2.0) и `fix-reference-code` S1; FX-06, FX-10, FX-11 —
  `fix-reference-code` S2; проза (FX-07) — `repair-harness`; дефекты китов — планы `../golibinfra`.

## Handoff
—

## Где мы сейчас
S2–S11 закрыты, AC1–AC4, AC9 и AC10 подтверждены. Этап S2–S6 провалидирован: PASS после фиксов
`fix(gate)`. Этап S7–S11: валидатор B2 — FAIL (отчёт —
`/home/mm/.cache/stit-audit-program/validation-harden-gate-B2.md`, кейсы — `valb2/` рядом); шаги
S7–S11, AC9, AC10 и фиксы прошлого круга (проза по путям git, N1–N5, строка 15) он воспроизвёл.
Его блокирующий B1 исправлен: таблица запроса — только в позиции таблицы, колонка с именем
чужой таблицы гейт не роняет (кейсы `k*` → FAIL, `k-column` → clean). NB1 исправлен: утверждение
в хелпере засчитывается (`t15-helper` → clean, `t15` → FAIL). NB4, NB5, NB6 (AC8), NB8 — следующими
фиксами; NB2 и NB3 переносит в планы дирижёр. Строки матрицы ENF:
15 закрыта (`TestTestsAssert`), 9 и 11 исключены («Решения»). Allowlist (опустошает
`fix-reference-code`): ADP-02 — 4 identity-closure в `auth/module/module.go` и
`verification/module/module.go`; ADP-03 — `internal/platform/shared` в
`docs/reference/domain-mapping.md` (2 ссылки); SEC-10 — `JWT_SECRET` нет в `.env.example`. Обходы
до bump golibinfra (снимает `fix-reference-code` S1): `beforeHTTP` (FX-01). Хвост: запас покрытия
≈1 statement (75,1% при пороге 75%) — первый шаг `fix-reference-code` с непокрытым кодом упрётся в
`cover-check`; снимают S13–S14 там, и там же — покрытие по функциям или диффу (строка 11).
Следующее: фиксы NB4–NB6, NB8 → перепроверка валидатором B2 → S12 отдельным агентом →
закрыть план, push шаблона.
