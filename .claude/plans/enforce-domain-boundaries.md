---
status: in-progress
parent: —
blocked-by: —
---

# enforce-domain-boundaries — вход только через usecase, контракты ошибок и событий держит машина

## Цель
Закрыть находки трека boundaries программы `reaudit-to-nine` (RADP-02, 03, 04, 09): вход домена ходит
только в свой usecase через порт, метод usecase за операцией под Security спрашивает policy, ошибка домена не
уходит клиенту 500 мимо таблицы, перевод ошибок на границе доменов идёт по методам типа, а подписка без
события поставщика в контракт-тесте не проходит гейт. Всё это держит `make check`; проза говорит ровно то,
что проверяется, и называет пределы.

## Контекст
- Программа — `.claude/plans/reaudit-to-nine.md` (трек boundaries, волна 2, S4): владение файлами, решения
  владельца, метод. Ветка `nine/boundaries`, worktree `N/wt-boundaries` от `main` `98e425e` (слиты identity и
  lifecycle; gate — ещё нет). `N` = `/home/mm/.cache/stit-audit-program/nine`, `R` =
  `/home/mm/.cache/stit-audit-program/reaudit`.
- Находки — `R/template-adapters.md` § RADP-02, 03, 04, 09; эксперименты — `R/tpl-adp/` (инжектор `lib.py`,
  кейсы `cases_*.py`, раннеры `run.py`, `batch.py`). Рабочая копия трека — `N/boundaries/exp/`: `make check`
  раннеров — под `flock N/gate.lock`, логи и TSV — там же; кейсы, которым база `98e425e` сдвинула якоря или
  имена, — в `cases_port.py` с суффиксом `-p` (смысл тот же). Worktree экспериментов — `N/wt-boundaries-<x>`.
- Что держит машина на базе: archkit (`TestLayout`) из правил входа — только импорт `infra` из `delivery`;
  `TestHTTPTypes` и `TestDomainErrorTables` (`internal/archtest/archtest_test.go`) читают пакет `delivery/http`
  ровно, ошибки домена — только `var Err*` из `errors.go`, у хендлера ловят только `return …, err`;
  `TestTrustedOnlyInModules` (`crossdomain_test.go`) узнаёт `Trusted` по имени типа; контракт-тест
  `internal/domain/verification/delivery/nats/contract_test.go` — сценарий на подписку, без сверки с
  `Register`; policy под Security — проза rule `domain-architecture`.
- Владение: `internal/archtest/{archtest,crossdomain,layout,layout_settings}_test.go` (в `archtest_test.go` —
  кроме `TestTestsAssert` и правила тегов сборки трека gate: он не в `main`, конфликты слияния сводит
  дирижёр), правки доменов под новые правила, rule `domain-architecture`,
  `scaffold/references/{port,endpoint,event}.md`, `docs/reference/architecture.md`.
- Новые правила читают и код identity: эталон, вывод `make new-domain` и законные формы аудитора не должны
  получить ни одного отказа.

## Не входит
- RADP-01 (трек lifecycle), minor угла — RADP-05…08, 10…13 (бэклог `minor-adapters`); правила в archkit
  (golibinfra) — правила трека живут в archtest шаблона, `go.mod` не трогаем.
- Смысл, который держат тест и ревью, а не синтаксис: чужая ошибка, узнанная по тексту (`n54`); ошибка
  сущности из нового метода `Trusted` на входе, которого нет в тесте; решение policy, которое метод спросил
  и не проверил. Пределы названы в `docs/reference/architecture.md`.
- `TestTestsAssert`, правило тегов сборки, rule `tests-pipeline` — трек gate.

## Критерии приёмки
- [x] **AC1** — RADP-02, вход только через свой usecase: кейсы `n12-h-repo-port` (порт корня
  `user.Repository` в хендлере), `n37-h-subpkg-admin` (хендлеры в `delivery/http/admin`),
  `n38-biz-in-platform` (операция в `internal/platform/reports`), `n70-sub-raw-sql` (`pgkit.DB` и SQL в
  подписчике), `b14-trusted-via-ctor` (`user.NewTrusted` к порту своего входа) на базе — `✓ check clean`, на
  ветке — отказ archtest, который называет файл и правило. Проверка: база —
  `python3 N/boundaries/exp/batch.py <wt базы> … ` → `check=clean`; ветка — `run.py <wt ветки> --no-full <кейс>`
  → `CAUGHT unit[Test…]`
  - Подтверждено: база — сводный прогон A `check=clean` (`N/boundaries/exp/base.tsv`); ветка
    (`exp/branch-s1.tsv`): `n12` — `TestDeliveryReachesUsecaseOnly` (`user.Repository` в хендлере), `n37` —
    `TestDeliveryPackages` и `TestInputsRegisterInDelivery`, `n38` — `TestInputsRegisterInDelivery`, `n70` —
    `TestDeliveryReachesUsecaseOnly` (импорт `pgkit`), `b14` — `TestDeliveryPortsBoundToUsecase`; обходы
    второго шага `x02a`–`x02f` (`exp/cases_bnd.py`: порт входа над `user.Repository`, `session.Store` в
    хендлере, хендлер из `internal/platform`, `huma.Get`, `huma.Register[I, O]` в platform, подписка natskit в
    `module/`) — отказ своим правилом
- [ ] **AC2** — RADP-03, ошибка домена — только через таблицу, перевод — по методам типа: `n16p` (sentinel в
  `usecase.go`), `n18p` (`errors.New` в usecase), `n15` (своя функция перевода в хендлере), `n34` (метод
  адаптера без перевода), `n35` (метод `Trusted` вне перечня теста), `b15` (порт — структура со встроенным
  `*user.Trusted`) на базе — `✓ check clean`, на ветке — отказ; контроль `n17p` — отказ на обеих. Проверка:
  та же
- [ ] **AC3** — RADP-04, подписка без события поставщика — отказ: `n22p` (подписка на `user.events.updated`
  со схемой `UserID` вместо `ID`, без контракт-теста) на базе — `✓ check clean`, на ветке — отказ
  контракт-теста; домен с подпиской без `delivery/nats/contract_test.go`, который сверяет подписки
  `Register`, — отказ archtest. Проверка: та же; второе — ручная инъекция в worktree ветки → отказ
- [ ] **AC4** — RADP-09, операция под Security спрашивает policy: `n36-uc-no-policy` (`GET /api/users/emails`
  отдаёт адреса всех любому вошедшему) на базе — `✓ check clean`, на ветке — отказ archtest. Проверка: та же
- [ ] **AC5** — Законные формы не краснеют: эталон и законные формы аудитора (`fp01`–`fp04`, `fp09`–`fp13`,
  `fp17`, `fp18`, `fp20`, `fp21p`, `fp22`) на ветке — `✓ check clean`. Проверка: `batch.py <wt ветки>` →
  `check=clean`
- [ ] **AC6** — Проза — ровно то, что проверяется: rule `domain-architecture`, `docs/reference/architecture.md`,
  `scaffold/references/{endpoint,port,event}.md` и комментарий пакета archtest называют новые правила
  (пометка archtest), пределы — словами; обещание «ни одна ошибка домена не уходит 500» — в форме, которую
  держит тест. Проверка: ручная сверка текста с тестами; `make check` → `✓ check clean`

## Шаги
- [x] **S1** — RADP-02: archtest — пакеты входа `delivery/<транспорт>` без подпакетов (кроме `mocks`);
  `huma.Register` — только в `delivery/http` с хендлером оттуда, подписка `natskit` — только в
  `delivery/nats`; вход берёт из корня своего домена данные, а не порты, usecase, `Trusted` и их зависимости,
  и не держит хранилищ, клиентов и драйверов; порт входа привязывает только identity-closure над
  `*<домен>.Usecase` в своём `module/`. Проза
  - Проверка: n12, n37, n38, n70, b14 на ветке → отказ; `make check` → ✓ · Закрывает: AC1, AC6 · Зависит от: —
  - Сделано: `internal/archtest/delivery_test.go` — `TestDeliveryPackages`, `TestInputsRegisterInDelivery`,
    `TestDeliveryReachesUsecaseOnly`, `TestDeliveryPortsBoundToUsecase`; эталон и код identity — без отказов;
    кейсы AC1 и обходы `x02a`–`x02f` — отказ; rule `domain-architecture`, `architecture.md` (§ Что запрещено,
    § fx, § HTTP DTO naming, предел — хранилище за `internal/platform`), `endpoint.md`, `event.md`, комментарий
    пакета archtest; `make check` → ✓
- [ ] **S2** — RADP-03: archtest — в корне домена ошибки и их типы только в `errors.go` (тип — с `Unwrap` в
  sentinel), `fmt.Errorf` — с `%w`; хендлер отдаёт `nil` или `<домен>Errors.Map(…)`; структура `module/` не
  встраивает тип чужого домена; тесты адаптеров и `Trusted` сверяют перечень `calls` с методами типа. Проза
  контракта ошибок
  - Проверка: n16p, n18p, n15, n34, n35, b15 на ветке → отказ, n17p → отказ; `make check` → ✓ ·
    Закрывает: AC2, AC6 · Зависит от: —
- [ ] **S3** — RADP-04: контракт-тест verification — одна сверка: каждой подписке `Register` — событие
  настоящего usecase поставщика; archtest — домен с подпиской держит такой `contract_test.go`. Проза
  - Проверка: n22p на ветке → отказ; инъекция подписки без контракт-теста → отказ; `make check` → ✓ ·
    Закрывает: AC3, AC6 · Зависит от: S1
- [ ] **S4** — RADP-09: archtest — метод usecase за операцией под Security спрашивает policy
  (`uc.d.Policy.Can…`, в том числе через свои методы); `user.Me` спрашивает `CanGetByID` своего id. Проза
  - Проверка: n36 на ветке → отказ; `make check` → ✓ · Закрывает: AC4, AC6 · Зависит от: S1
- [ ] **S5** — Приёмка: эксперименты аудитора на базе и ветке, законные формы, гейт
  - Проверка: `N/boundaries/exp/{base,branch}.tsv`; `make check` → `✓ check clean`
  - Закрывает: AC1, AC2, AC3, AC4, AC5, AC6 · Зависит от: S1, S2, S3, S4

## Решения
- Правила входа — новым файлом `internal/archtest/delivery_test.go`: тема — вход домена, а не граница
  доменов; `archtest_test.go` правит и трек gate — чем меньше в нём новых строк, тем меньше конфликтов слияния.
- `NewTrusted` «только элементом `fx.Provide`» (пункт (г) аудитора) отдельным правилом не держим: `b14`
  ловит правило привязки порта входа (вход получает только `*<домен>.Usecase`), а оно шире — и для
  репозитория, и для хранилища кита за локальным портом входа.
- «Не данные» корня домена узнаются по форме, а не по именам: интерфейсы, структуры с `fx.In`, типы с полем
  такого типа и функции с ними в сигнатуре (`Usecase`, `Trusted`, `NewPolicy` и сервис форка под другим
  именем); переменные (`Err*`, `TrustedErrors`) — данные.
- Пакеты golibinfra во входе — перечнем разрешённого (транспорт `httpkit`, `natskit`, `grpckit`, лог и helpers
  без I/O `clockkit`, `idkit`, `ptrkit`, `mapkit`, `tzkit`), а не запретом хранилищ: новый кит с I/O во входе
  краснеет без правки правила; драйверы вне golibinfra — `database/sql` и pgx.
- Подпакет `delivery/<транспорт>/<x>` запрещён (вариант (б) аудитора «запрет глубже»), а не читается
  правилами: правила HTTP и контракт подписок читают один пакет, неэкспортируемая таблица домена в
  подпакете недоступна. Хендлер операции — из пакета `delivery/http` (не `<пакет>.<имя>`), подписка natskit —
  только в `delivery/nats`, dot-импорт huma и natskit — отказ: иначе регистрацию правила не видят.

## Где мы сейчас
S1 (RADP-02) сделан: правила входа — `internal/archtest/delivery_test.go`. База `98e425e`: сводные прогоны
кейсов трека — `✓ check clean`: A — `n12`, `n37`, `n38`, `n70`, `b14`; B — `n16p`, `n15`, `n34`, `n35`,
`b15`, `n22p`, `n36`; C — `n18p` (`N/boundaries/exp/base.tsv`, логи — `exp/logs/batch-base-*.log`).
Эксперименты ветки — в `N/wt-boundaries-exp` (снимок правил временным коммитом: раннер откатывает дерево к
HEAD). Следующий шаг — S2 (RADP-03).
