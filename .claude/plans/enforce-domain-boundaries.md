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
- [x] **AC2** — RADP-03, ошибка домена — только через таблицу, перевод — по методам типа: `n16p` (sentinel в
  `usecase.go`), `n18p` (`errors.New` в usecase), `n15` (своя функция перевода в хендлере), `n34` (метод
  адаптера без перевода), `n35` (метод `Trusted` вне перечня теста), `b15` (порт — структура со встроенным
  `*user.Trusted`) на базе — `✓ check clean`, на ветке — отказ; контроль `n17p` — отказ на обеих. Проверка:
  та же
  - Подтверждено: база — сводные прогоны B и C `check=clean`; ветка (`exp/branch-s2.tsv`): `n16p`, `n18p` —
    `TestDomainErrorsInErrorsGo`, `n15` — `TestDomainErrorTables` (хендлер отдаёт `statusErr(err)`), `n34` —
    `TestUserAdapter_MapsUserErrors` (метода `LockEmail` нет в `calls`), `n35` — `TestTrusted_ErrorsAreDeclared`
    (`Rename`), `b15` — `TestModulesEmbedNoForeignTypes`, `n17p` — `TestDomainErrorTables`; обходы `x03a`–`x03d`
    (`fmt.Errorf` без `%w`, тип ошибки в `usecase.go`, тип в `errors.go` без `Unwrap`, `huma.Error404NotFound`
    из хендлера) — отказ
- [x] **AC3** — RADP-04, подписка без события поставщика — отказ: `n22p` (подписка на `user.events.updated`
  со схемой `UserID` вместо `ID`, без контракт-теста) на базе — `✓ check clean`, на ветке — отказ
  контракт-теста; домен с подпиской без `delivery/nats/contract_test.go`, который сверяет подписки
  `Register`, — отказ archtest. Проверка: та же; второе — ручная инъекция в worktree ветки → отказ
  - Подтверждено: база — сводный прогон B `check=clean`; ветка (`exp/branch-s3.tsv`): `n22p` — `TestContracts`
    («subscription user.events.updated gets no event of a provider usecase»); `x04a` (подписка
    `user/delivery/nats`, подключена в `module/`, без `contract_test.go`) — `TestSubscriptionsHaveContractTest`;
    `x04b` (`n22p` + сценарий поставщика `user.Update` в `TestContracts`) — отказ: подписка разбирает `UserID`
    вместо `ID` поставщика и зовёт порт с пустым ID
- [x] **AC4** — RADP-09, операция под Security спрашивает policy: `n36-uc-no-policy` (`GET /api/users/emails`
  отдаёт адреса всех любому вошедшему) на базе — `✓ check clean`, на ветке — отказ archtest. Проверка: та же
  - Подтверждено: база — сводный прогон B `check=clean`; ветка (`exp/branch-s4.tsv`): `n36` —
    `TestSecuredOperationsAskPolicy` («calls user.Usecase.Emails, which neither asks policy…»); обходы `x09a`
    (только проверка, что вызывающий есть), `x09b` (вызывающий прочитан, но не ключ), `x09d` (хендлер под
    Security без usecase) — отказ; законная форма `x09c` (policy своим методом `uc.authorizeList`) — зелёная
- [x] **AC5** — Законные формы не краснеют: эталон и законные формы аудитора (`fp01`–`fp05`, `fp08`–`fp15`,
  `fp17`, `fp18`, `fp20`, `fp21p`, `fp22b`) на ветке — `✓ check clean`. Проверка: `batch.py <wt ветки>` →
  `check=clean`
  - Подтверждено: ветка `7ebaa69` (`exp/branch-legit.tsv`): сводный L1 — `fp01`, `fp02`, `fp04p`, `fp05`, `fp08`,
    `fp09`, `fp10`, `fp11`, `fp12`, `fp13`, `fp14`, `fp15`, `fp17`, `fp18`, `fp20`, `fp21p` — `check=clean`
    (99,90 %); L2 — `fp03`, L3 — `fp22b` — `check=clean`; законная форма трека `x09c` (policy своим методом) —
    быстрые шаги зелёные. Вне перечня: `fp16` и `fp07` — правило prune трека lifecycle (RADP-01), `fp10b`,
    `fp14b` — RADP-11 (бэклог), `fp06` — настройки archkit; правил трека они не касаются
- [x] **AC6** — Проза — ровно то, что проверяется: rule `domain-architecture`, `docs/reference/architecture.md`,
  `scaffold/references/{endpoint,port,event}.md` и комментарий пакета archtest называют новые правила
  (пометка archtest), пределы — словами; обещание «ни одна ошибка домена не уходит 500» — в форме, которую
  держит тест. Проверка: ручная сверка текста с тестами; `make check` → `✓ check clean`
  - Подтверждено: сверка диффа прозы с тестами — каждое новое правило названо там, где его ищет агент (rule,
    `architecture.md`, reference варианта scaffold), с именем теста; пределы — хранилище за `internal/platform`,
    подписка методом шины, ошибка сущности нового метода `Trusted`, чужая ошибка по тексту, ответ policy без
    проверки; «не 500» — для ошибок `errors.go` через таблицу, 500 — сбой порта или кита; `make check` → ✓

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
- [x] **S2** — RADP-03: archtest — в корне домена ошибки и их типы только в `errors.go` (тип — с `Unwrap` в
  sentinel), `fmt.Errorf` — с `%w`; хендлер отдаёт `nil` или `<домен>Errors.Map(…)`; структура `module/` не
  встраивает тип чужого домена; тесты адаптеров и `Trusted` сверяют перечень `calls` с методами типа. Проза
  контракта ошибок
  - Проверка: n16p, n18p, n15, n34, n35, b15 на ветке → отказ, n17p → отказ; `make check` → ✓ ·
    Закрывает: AC2, AC6 · Зависит от: —
  - Сделано: `TestDomainErrorsInErrorsGo` (`archtest_test.go`): в корне домена `errors.New`, `var Err…` и тип
    ошибки — только в `errors.go`, там — только экспортируемые `Err…`, тип — с `Unwrap` в sentinel,
    `fmt.Errorf` — с `%w`; `TestDomainErrorTables`: хендлер (метод, функция или литерал, и
    `huma.Register[I, O]`) отдаёт ошибкой только `nil` или `<домен>Errors.Map(…)`;
    `TestModulesEmbedNoForeignTypes` (`crossdomain_test.go`); тесты адаптеров auth и verification и
    `TestTrusted_ErrorsAreDeclared` — `calls` картой, ключи сверены с методами типа (`reflect`); rule
    `domain-architecture`, `architecture.md` (§ Контракт ошибок API — обещание в проверяемой форме и
    пределы, § Ports & adapters), `endpoint.md`, `port.md`, комментарий пакета archtest; `make check` → ✓
- [x] **S3** — RADP-04: контракт-тест verification — одна сверка: каждой подписке `Register` — событие
  настоящего usecase поставщика; archtest — домен с подпиской держит такой `contract_test.go`. Проза
  - Проверка: n22p на ветке → отказ; инъекция подписки без контракт-теста → отказ; `make check` → ✓ ·
    Закрывает: AC3, AC6 · Зависит от: S1
  - Сделано: `verification/delivery/nats/contract_test.go` — один `TestContracts`: сценарии поставщиков
    (`userRegistered`, `passwordResetRequested`) публикуют события настоящими usecase и ставят ожидания
    порта, события доставляются подпискам `Register`, каждая подписка `bus.Subscriptions()` получила событие
    поставщика; `TestSubscriptionsHaveContractTest` (`delivery_test.go`): `delivery/nats` с подпиской держит
    `contract_test.go` со сверкой `Subscriptions()`; `event.md` (тест и прежние версии subject'а),
    `architecture.md` § События, rule `events-and-messaging`, комментарий пакета archtest; `make check` → ✓
- [x] **S4** — RADP-09: archtest — метод usecase за операцией под Security спрашивает policy
  (`uc.d.Policy.Can…`, в том числе через свои методы); без policy — только чтение вызывающего: ключ порта —
  его `UserID` из `identity.From` (`Me`). Проза
  - Проверка: n36 на ветке → отказ; `make check` → ✓ · Закрывает: AC4, AC6 · Зависит от: S1
  - Сделано: `TestSecuredOperationsAskPolicy` (`delivery_test.go`): операция без `httpkit.Public` → методы
    usecase, которые хендлер зовёт полем-портом (и функциями пакета) → метод `*<домен>.Usecase` (и его методы)
    зовёт `…Policy.Can…` или передаёт вызову `UserID` из `identity.From`; хендлер под Security без вызова
    usecase — отказ; эталон и законные формы — без отказов; rule `domain-architecture`, `architecture.md`
    § Usecase-канон, `endpoint.md`, комментарий пакета archtest; `make check` → ✓
- [x] **S5** — Приёмка: эксперименты аудитора на базе и ветке, законные формы, гейт
  - Проверка: `N/boundaries/exp/{base,branch}.tsv`; `make check` → `✓ check clean`
  - Закрывает: AC1, AC2, AC3, AC4, AC5, AC6 · Зависит от: S1, S2, S3, S4
  - Сделано: на содержимом `7ebaa69` кейсы трека (`exp/branch-final.tsv`) — 14 аудитора (`n12`, `n37`, `n38`,
    `n70`, `b14`, `n16p`, `n18p`, `n15`, `n34`, `n35`, `b15`, `n17p`, `n22p`, `n36`) и 15 обходов (`x02a`–`x09d`) —
    отказ, `x09c` — зелёный; законные формы — AC5; пробное слияние с `nine/gate` `02f7845` — без конфликтов
    (`archtest_test.go` — автослияние), archtest и тесты доменов на слитом дереве — PASS; `make check` → ✓

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
- Правило «ошибки — только в `errors.go`» — для корня домена (как у аудитора): ошибка `infra/<kit>` вроде
  `logmail.ErrOutsideDev` — отказ старта, а не ответ клиенту. Сверх пункта (а) аудитора: тип ошибки — только в
  `errors.go` и с `Unwrap` в sentinel (`auth.LockedError`), неэкспортируемая переменная в `errors.go` — отказ:
  у обоих иначе нет правила в таблице, и обещание «не 500» не держалось бы. Проверка интерфейса
  `var _ error = (*LockedError)(nil)` — не переменная-ошибка: правило её пропускает (проверено инъекцией).
- Встроенный тип чужого домена в `module/` (пункт (г) аудитора — правило archkit) держит archtest шаблона:
  golibinfra в треке не трогаем.
- Сверка `calls` с методами — картой «метод → вызов» и `reflect.TypeFor[…]().Methods()` в самом тесте, без
  общего хелпера: тест адаптера копируют с канона, сверка едет вместе с ним (`port.md`).
- Контракт подписок — один тест со сценариями поставщиков, а не тест на подписку: сверке подписок нужны все
  события сразу. Перечня прежних версий subject'а в коде нет — прежних подписок у verification нет, пустой
  перечень был бы мёртвым кодом; его форму и место описывает `event.md` § Versioning.
- Чужой файл: rule `events-and-messaging` (владелец волны 1 — трек identity, в волне 2 не назначен) — одна
  строка: отказ теста и archtest у подписки без события поставщика, иначе правило молчало бы о машине.
- RADP-09 держит ровно правило прозы: policy или чтение самого вызывающего (ключ порта — `UserID` из
  `identity.From`). `Me` не переводим на `CanGetByID` своего id: настоящая policy пускает своё чтение всегда,
  ветка отказа была бы недостижимой — при пороге покрытия 100 трека gate красный cover-check; «policy или
  `identity.From`» аудитора слабее: проверка «вызывающий есть» без решения (`x09a`) прошла бы. Policy — поле
  `Policy` с методом `Can…` (канон `UsecaseDeps`).

## Где мы сейчас
Трек закончен: S1–S5, критерии AC1–AC6 подтверждены. Правила — `internal/archtest/delivery_test.go` (вход,
подписки, policy), `archtest_test.go` (ошибки корня, ответ хендлера), `crossdomain_test.go` (встроенный тип
чужого домена); сверка методов — в тестах адаптеров и `Trusted`; контракт подписок — `TestContracts`
verification. Эксперименты: база `98e425e` — `N/boundaries/exp/base.tsv`, ветка — `branch-*.tsv`, логи —
`exp/logs/`, раннер — `exp/run.py` и `exp/batch.py` (`make check` под замком), кейсы трека —
`exp/cases_port.py` (перенос на базу) и `exp/cases_bnd.py` (обходы). Временные worktree сняты. Следующее —
валидатор трека, затем перенос на `main` дирижёром (с треком gate — без конфликтов, см. S5).
