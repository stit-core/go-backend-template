---
status: in-progress
parent: —
blocked-by: —
---

# fix-reference-code — эталонный код, безопасность, контракт ошибок, тесты, покрытие 90–95%

## Цель
Вторая часть программы исправлений по аудиту (цель — каждый угол ≥ 9/10). Reference-домены
становятся настоящим каноном, который безопасно копировать: правильная конкурентность,
идемпотентность без потери эффектов, авторизация на уровне полей, управляемые сессии, безопасные
prod-дефолты, понятный контракт ошибок API, тесты, которые ловят поломки, покрытие ≥ 90% (цель
95%). Общая механика живёт в китах golibinfra, в шаблоне — доменная логика и тонкий клей.

## Контекст
- Гейт честный: integration-тесты в `make check`, archtest «границы 2.0», boot-тест `cmd/api`,
  `openapi.json` из `make gen`. Здесь опустошаются allowlist'ы (`internal/testkit/allowlist`) и
  снимается обход `beforeHTTP` (FX-01):
  - ADP-02 — 4 identity-closure: `auth/module/module.go` (`PasswordVerifier`, `PasswordSetter`),
    `verification/module/module.go` (`PasswordSetter`, `EmailVerificationSetter`);
  - ADP-03 — мёртвые ссылки `docs/reference/domain-mapping.md` на `internal/platform/shared/` и
    `internal/platform/shared/errors.go`;
  - SEC-10 — `JWT_SECRET` нет в `.env.example`.
- **golibinfra меняется своими планами** в `../golibinfra` (партии «исполнитель → валидатор», тег
  пушится с OK владельца), шаблон поднимает версию. Точный состав и шаги миграции — аннотации тегов:
  - **v1.2.0** — в шаблоне с S1; контракт ошибок httpkit (problem+json с `code` и `request_id`,
    `errors[]` у 422, `Retry-After`, `httpkit.ErrorMap`: `Map`, `Unmapped`, `Declare`) — основа S8.
  - **v1.3.0** — httpkit: операция без `Security` и без пометки `httpkit.Public`, дубль
    METHOD+path — ошибка старта; IP клиента в ctx стандартной цепочкой; безопасные дефолты (docs
    выключены, конечный `OUTBOX_MAX_ATTEMPTS`); auditkit: `RequestID` и `IP` из ctx; authnkit:
    непрозрачные одноразовые токены и сессии refresh `authnkit/session` (ротация, отзыв цепочки
    при повторе, отзыв всех сессий пользователя, абсолютный срок).
- **Отчёт аудита:** https://claude.ai/artifact/L3Y8vWRofsvzf8PGQos4dE (Artifact `action: read`);
  разделы COD, SEC, TST, ADP, DOG. **Отметки владельца:** `ArtifactData` `list`, `collection:
  review` — `disagree` не чинить, `discuss` спросить перед шагом.
- **Принципы:** без оверинжиниринга — понятный, красивый, консистентный код; один канонический
  способ; удалить лучше, чем добавить; общий примитив — в golibinfra, эквивалентов китов в шаблоне
  нет. Новые миграции — новыми файлами. Коммит на шаг, push — с OK владельца.
- **Находки этой части (подробности и `file:line` — в отчёте):**
  - COD-01 — одноразовые токены (refresh, сброс пароля, подтверждение email) гасятся
    check-then-act: `auth/usecase.go:206-251`, `refresh_sessions.sql:10-14`,
    `verification/usecase.go:194-253`.
  - COD-02 — канон идемпотентности теряет письмо при сбое mailer
    (`verification/usecase.go:100-140`, правило `events-and-messaging.md:43-48`, `event.md:192`);
    `TokenID` не хранится.
  - COD-03 — `user.Update`: `CanUpdate` = сам или админ, статус без проверки роли (self-unblock,
    self-delete в обход события), PATCH теряет status при name+status, нет enum, 409 вместо 404,
    `version` нет во view (`user/policy.go:35-37`, `user/usecase.go:181-212`).
  - SEC-01 — нет отзыва сессий, Refresh не проверяет `CanLogin`, reuse только аудируется, смена и
    сброс пароля сессии не гасят, 720 ч на каждую ротацию. SEC-07 — `seed-admin` повышает
    существующий аккаунт без отзыва сессий.
  - SEC-02 — log-mailer пишет сырые токены в Info (`verification/module/adapters.go:62, 72`).
    SEC-04 — `COOKIE_SECURE=false`, `TRUST_PROXY=false`, маскирование логов выключено, docs
    включены, нигде не охраняется. SEC-05 — ошибка guard → логин без защиты
    (`auth/usecase.go:117`). SEC-06 — перебор аккаунтов через сброс пароля. SEC-08 — образы от
    root, плавающие теги. SEC-10 — `JWT_SECRET` нет в `.env.example`. SEC-11 — мелочи.
  - ADP-01 — `module/` без тестов и вне покрытия, 4 из 6 адаптеров — обёртки китов; ADP-02 —
    identity-closure протаскивает чужие sentinel'ы (`auth/module/module.go:16-17`,
    `auth/usecase.go:329-331` → 500 вместо 409); ADP-04 — `Active` вычисляется в адаптере;
    ADP-05 — дубли адаптеров и доверенные методы без policy на общем типе; ADP-06 — нет
    контракт-теста событий; ADP-08 — мелочи.
  - COD-04 — эталон нарушает канон (named returns, константы в usecase, метрики не на всех
    исходах, конструктор адаптера в module.go, DTO без префикса и envelope в auth и
    verification, мёртвый код, эссе-комментарии, `t.Fatalf`); COD-05 — `MapErr` без маппинга →
    500 (refresh с битым токеном); COD-06 — нет общих примитивов (ForbiddenError ×2, правило
    пароля ×6, аудит в трёх формах, пагинация, метки метрик, `pkg/tokens` дублирует кит, idkit не
    используется); COD-07 — `metrics.go` пишет в no-op; COD-08 — ничейные места (`pkg/` без
    хартии, mailer в `module/`, неиспользуемые киты, `/ready` зависит от Redis); COD-09 — скелет
    `new-domain.sh` со стабами → nil и тестами NotNil; COD-10 — мелочи.
  - TST-01 — мягкие моки (27 `.Maybe()`, policy-мок игнорирует аргументы, outbox-фейк не проверяет
    транзакцию); TST-03 — эталонный integration-тест покрывает 3 из 9 методов; TST-04 — порог
    покрытия впритык, скелет валит cover-check; TST-06 — 6 из 13 эндпоинтов без happy-path;
    TST-08 — мелочи.
- **Мутации для перепроверки:** 34 мутации из раздела TST отчёта (сейчас гейт убивает 9) и D02
  из раздела DOG (Publish вне транзакции).
- **fx-аудит** (29.09; полный отчёт — `/home/mm/.cache/stit-audit-program/fx-audit.md`): FX-01
  и FX-05 закрыл кит (S1), FX-06 (X16), FX-10 и FX-11 — S2.

## Не входит
- Проза обвязки, рецепты, жизненный цикл шаблона — `repair-harness`.
- Правки golibinfra — планами в `../golibinfra`; здесь — bump и снятие того, что взял кит.
- Запуск e2e без `.env.local` — модели и сценарии обновить, прогон — когда появится окружение.

## Критерии приёмки
- [ ] **AC1** — Одноразовый токен гасится ровно один раз при параллельных запросах (refresh,
  сброс пароля, подтверждение email). Проверка: integration-тест с параллельным погашением → один
  успех, остальные — sentinel «уже использован»
- [ ] **AC2** — Сессии управляемы: заблокированный не обновляет токены; смена и сброс пароля,
  блокировка, удаление, смена роли и повторное использование refresh гасят сессии; у сессии есть
  абсолютный срок жизни; `seed-admin` не отдаёт права чужим сессиям. Проверка: тесты usecase и
  integration на каждый случай → PASS
- [ ] **AC3** — Контракт user: клиент не меняет свой статус (403); name и status применяются вместе
  или 400; неизвестный статус → 422; несуществующий id → 404; устаревшая версия → 409; `version`
  есть в ответе. Проверка: handler- и integration-тесты → PASS
- [ ] **AC4** — Идемпотентный консьюмер не теряет эффект: сбой mailer на первой доставке → письмо
  уходит на повторной; повтор после успеха письмо не дублирует. Проверка: тест redelivery → PASS
- [ ] **AC5** — Ошибки через границу доменов маппятся: change-password при конкурентном
  обновлении → 409; адаптер переводит каждый sentinel поставщика, который отдаёт порт, в sentinel
  потребителя. Проверка: тесты адаптеров и usecase → PASS
- [ ] **AC6** — Секретов и токенов нет в логах; дефолты безопасны; вне `APP_ENV=dev` сервис не
  стартует с небезопасной конфигурацией (insecure cookie, лог-mailer, выключенное маскирование,
  docs). Проверка: тест конфигурации и тест лог-полей → PASS
- [ ] **AC7** — Каждая операция либо под `Security`, либо помечена `httpkit.Public` — иначе сервис
  не стартует; без cookie → 401, чужой ресурс → 403. Проверка: тест сборки API (операция без
  `Security` и пометки → ошибка старта), handler-тесты и boot-тест → PASS
- [ ] **AC8** — Покрытие по широкому scope (domain, delivery, platform, module, infra; без
  сгенерированного, моков и `cmd/`), unit + integration в одном профиле: гейт ≥ 90%, фактически
  ≥ 95% или поштучное объяснение, почему нет. Проверка: `make cover-check` → OK
- [ ] **AC9** — Мутации аудита: из 34 гейт убивает не меньше 32, каждый выживший объяснён в
  «Решениях» (эквивалентный мутант); D02 убит. Проверка: повторить мутации → таблица
- [ ] **AC10** — `make new-domain NAME=probe` без правок проходит `make check`, включая покрытие.
  Проверка: в worktree → `make check` зелёный
- [ ] **AC11** — Allowlist'ы archtest и `cmd/api` пусты, обходов в ожидании китов
  нет (`beforeHTTP`, client-IP middleware, `pkg/tokens`, `publicOperations`); гейт зелёный.
  Проверка: `grep -rn "beforeHTTP\|WithIP\|pkg/tokens\|publicOperations" --include=*.go .` → пусто;
  `make check` → `✓ check clean`
- [x] **AC12** — Сборка DI единообразна: одна идиома привязки к интерфейсу, имя модуля — путь
  пакета, конфиги китов — через их `NewConfig`, `main`, тест графа и boot-тест собирают один
  `service`, зависимости usecase — одной структурой, события fx — в logkit, порядок старта и
  остановки держит httpkit, таймауты старта и остановки явные; в графе нет `context.Context`,
  flagkit и cachekit. Проверка: boot-тест зелёный; fx X16 невозможен по построению; X6 и X15
  ловят кит или `fx.ValidateApp`
  - Подтверждено: `TestBoot` зелёный без Redis; X16 невозможен (`Usecase{d UsecaseDeps}`); X6 —
    порядок держит httpkit, X15 — `fx.ValidateApp`; `fx.As`, `context.Context`, flagkit, cachekit нет
- [ ] **AC13** — Контракт ошибок API: каждая ошибка — problem+json (RFC 9457) со стабильным
  машинным `code`; 422 перечисляет поля (путь, код, сообщение); 401 и 403 различают «войти
  заново», «обновить сессию» и «нет прав»; 429 несёт `Retry-After`; 409 — код конфликта; 5xx без
  внутренностей, с ID запроса, тот же ID — в логах; каждая операция объявляет свои ошибки в
  OpenAPI. Проверка: тест полноты — каждый sentinel каждого домена → (статус, `code`), ожидаемая
  ошибка не даёт 500; `openapi.json` содержит ошибки операций; handler-тесты по кодам → PASS

## Шаги
### Фундамент
- [x] **S1** — Bump golibinfra до v1.2.0 по аннотации тега (`git -C ../golibinfra cat-file -p
  v1.2.0`: Breaking, Поведение) и снять то, что взял кит: `beforeHTTP` (порядок хуков держит
  httpkit, FX-01); client-IP — в `httpkit.Middlewares`; `HumaConfigGroup` → `httpkit.HumaConfigs`;
  маппинг ошибок хендлеров — `httpkit.ErrorMap`; логин — `Guard.Attempt` до проверки пароля и
  `RecordSuccess`; подписчик verification — `natskit.SubscribeOnStart`, шаблон подписчика в
  `event.md` — тем же коммитом; `logkit.FxLogger` и `fx.RecoverFromPanics` в `service` (FX-05);
  источники миграций сверяет boot-тест с группой `pgkit.migrations` графа (строка outbox в
  `TestDomainsAreWired` удаляется); тесты — под строгий `outboxtest`; `openapi.json`
  перегенерирован; `NATS_JETSTREAM_ENABLED=true`, `LOG_REDACT_DISABLED` и прочие env из аннотации
  тега — в `.env.example` и Handoff; `fx.Invoke(registerHTTP)` — по правилу порядка из аннотации тега
  (зависимость с хуком, которую берут только хендлеры, не должна стартовать позже HTTP), а
  boot-тест проверяет, что HTTP-сервер стартует последним и гасится первым
  - Проверка: `make check` → зелёный; `grep -rn "beforeHTTP\|MapErr" --include=*.go .` → пусто;
    fx X6, X11, X15 → пойманы китом, `fx.ValidateApp` или boot-тестом · Закрывает: AC11, AC12 · Зависит от: —
  - Сделано: `make check` → `✓ check clean`, grep пусто; X6 — порядок держит кит (HTTP последним
    и первым при любом месте `pgkit.Module`), X11 — старт падает `ErrAuthNotConfigured`, X15 —
    `TestFxGraphValidates`; `TestBoot` ловит и X12, X13, и `registerHTTP` в корне при зависимости с
    хуком только у хендлеров (T-C); логи — `fdn-s1-*.log` в материалах программы
- [x] **S2** — DI-канон: зависимости usecase — одной структурой; одна идиома привязки к
  интерфейсу; имя модуля — путь пакета; конфиги китов — через их `NewConfig`, `App` — только
  конфиги приложения; без `context.Context` в графе; flagkit и cachekit вне графа до первого
  потребителя — `/ready`, boot-тест, e2e-стек и local-deps без Redis; явные
  `fx.StartTimeout`/`fx.StopTimeout`; раздел «fx» в `architecture.md` тем же коммитом (FX-06,
  FX-10, FX-11)
  - Проверка: AC12 → выполнен · Закрывает: AC12 · Зависит от: S1
  - Сделано: AC12 подтверждён, `make check` → `✓ check clean`; X6, X15 повторены (`fdn-s2-*.log`);
    скелет `make new-domain` по новому канону собирается, lint и его тесты зелёные
- [x] **S3** — Общие примитивы без лишнего: `ForbiddenError` и его маппинг в одном месте,
  единственный источник правила пароля, тип пагинации, константы исходов метрик, ID — через
  `idkit` (COD-06)
  - Проверка: `make check` → зелёный; `grep -rn "type ForbiddenError" internal pkg` → одно место ·
    Закрывает: — · Зависит от: S2
  - Сделано: `make check` → `✓ check clean`; grep → `pkg/policy/policy.go`; скелет `make new-domain`
    на `outcome.Result` собирается, lint и его тесты зелёные

### Эталон доменов
- [ ] **S4** — user: переходы статуса методами entity, смена статуса только админом, PATCH целиком
  или 400, enum, `version` в ответе, 404 и 409 раздельно, update не трогает удалённых (COD-03)
  - Проверка: AC3 → PASS · Закрывает: AC3 · Зависит от: S3
- [ ] **S5** — verification: канон идемпотентности без потери эффекта, `TokenID` из БД, правило
  событий и `event.md` под новый канон (COD-02); сброс пароля и подтверждение email гасятся
  условным `UPDATE … WHERE used_at IS NULL` (`:execrows`, 0 строк → sentinel) (COD-01)
  - Проверка: AC4 → PASS; AC1 для сброса и подтверждения → PASS · Закрывает: AC1, AC4 · Зависит от: S3
- [ ] **S6** — Адаптеры: типовые с исчерпывающим маппингом вместо identity-closure; вычисленные
  свойства — методы сущности поставщика; обёртки китов вынесены из `module/`; доверенные методы
  поставщика отделены от методов с policy (ADP-01, ADP-02, ADP-04, ADP-05)
  - Проверка: AC5 → PASS, allowlist ADP-02 пуст · Закрывает: AC5 · Зависит от: S4
- [ ] **S7** — HTTP-канон в auth и verification: префиксы DTO, envelope `data`, `Security` на всех
  непубличных операциях; e2e-модели обновлены и строгие (SEC-03)
  - Проверка: handler-тесты и `TestHTTPOperations` → PASS · Закрывает: AC7 · Зависит от: S6
- [ ] **S8** — Контракт ошибок API поверх контракта httpkit: машинный `code` на каждую доменную
  ошибку, поля в 422, разные коды 401 и 403 (войти заново, обновить сессию, нет прав),
  `Retry-After` у 429 блокировки логина, код конфликта у 409, 5xx с ID запроса; ошибки операций
  объявлены в OpenAPI (`ErrorMap.Declare`); тест полноты маппинга — каждый sentinel → (статус,
  `code`), `ErrorMap.Unmapped` пуст (COD-05)
  - Проверка: AC13 → PASS; `make gen` → `openapi.json` с ошибками операций, без дрейфа ·
    Закрывает: AC13 · Зависит от: S7

### Сессии и безопасность
- [ ] **S9** — Bump golibinfra до v1.3.0 и сессии на ките: refresh — `authnkit/session` (ротация,
  повтор → отзыв цепочки, `CanLogin` в Refresh, абсолютный срок), отзыв всех сессий пользователя
  при блокировке, удалении, смене роли, смене и сбросе пароля; `refresh_sessions` — новой
  миграцией с `-- infra-handoff:`; `seed-admin` не повышает чужие сессии; снять `pkg/tokens`
  (токены authnkit), client-IP middleware и `identity.WithIP` (IP из ctx httpkit),
  `publicOperations` (пометки `httpkit.Public` в `Register` доменов), ручные `IP` и `RequestID` в
  записях аудита (SEC-01, SEC-07, COD-01, COD-06, FX-03)
  - Проверка: AC1 для refresh, AC2 → PASS; операция без `Security` и пометки → ошибка старта в
    тесте сборки API · Закрывает: AC1, AC2, AC7, AC11 · Зависит от: S8, тег golibinfra v1.3.0
- [ ] **S10** — Безопасные дефолты шаблона: mailer не логирует токен и работает только в dev;
  prod-гард конфигурации (`APP_ENV` обязателен; вне dev — secure cookie, без лог-mailer,
  маскирование включено, docs выключены); явное поведение при ошибке guard (fail-closed для
  логина); единый ответ и троттлинг сброса пароля; `JWT_SECRET` и флаг доверия прокси в
  `.env.example` и Handoff (за Traefik без флага все клиенты делят один bucket лимитера и ключ
  антибрутфорса); образы nonroot с закреплённой базой (SEC-02, SEC-04 – SEC-10)
  - Проверка: AC6 → PASS, allowlist SEC-10 пуст · Закрывает: AC6 · Зависит от: S9

### Чистка и скелет
- [ ] **S11** — Чистка эталона: канон соблюдён без исключений, мёртвый код и эссе удалены; метрики
  — экспорт через telemetry (выключен, пока в infra нет коллектора), бизнес-метрики только
  там, где несут смысл, с проверкой записи в тестах; хартия каталогов, место mailer, `platform/shared` убран из доков (allowlist
  ADP-03) (COD-04, COD-07, COD-08, COD-10, ADP-08, SEC-11)
  - Проверка: `make check` → зелёный, allowlist ADP-03 пуст · Закрывает: AC11 · Зависит от: S10
- [ ] **S12** — Скелет `new-domain.sh`: минимальный вертикальный срез (Create и Get) по канону, с
  настоящими тестами, без стабов → nil, подключён в `cmd/api` (COD-09)
  - Проверка: AC10 → PASS · Закрывает: AC10 · Зависит от: S11

### Тесты и итог
- [ ] **S13** — Канон тестов: без `.Maybe()` на побочных эффектах, policy с точными аргументами или
  настоящая, `Recorder.Last(subject)`, тесты адаптеров, happy-path на все эндпоинты, integration на
  все методы репозиториев, контракт-тесты событий (TST-01, TST-03, TST-06, ADP-06, TST-08)
  - Проверка: `make check` → зелёный · Закрывает: AC9 · Зависит от: S12
- [ ] **S14** — Покрытие: широкий scope, общий профиль unit + integration, `COVER_FLOOR` 90;
  довести до ≥ 95% осмысленными тестами и поднять порог до 95, если держится стабильно (TST-04);
  строка 11 матрицы ENF (непокрытый новый код) ловится с запасом, а не только потому, что до
  порога ≈1 statement (сейчас 75,1% при 75%): иначе — проверка покрытия новых функций или
  исключение с причиной в «Решениях»
  - Проверка: AC8 → OK · Закрывает: AC8 · Зависит от: S13
- [ ] **S15** — Итог: повторить 34 мутации аудита и D02, таблица — в «Где мы сейчас»; allowlist'ы
  пусты, обходов нет, гейт зелёный
  - Проверка: AC9 → ≥ 32 из 34; `make check` → `✓ check clean` · Закрывает: AC9, AC11 · Зависит от: S14

## Решения
- Порог покрытия: гейт 90%, цель 95% — владелец назвал 90–95; 95 закрепляется, только если
  достигается осмысленными тестами, а не пустышками (сила тестов проверяется мутациями, AC9).
- Граница с golibinfra: механика, нужная больше чем одному сервису, — в китах, своими планами в
  `../golibinfra`; здесь — bump и снятие того, что взял кит. Тот же auth-стек с теми же дефектами
  скопирован в `ai-gateway-template`, живые форки сами пишут контракт ошибок, список анонимных
  ручек и условное погашение refresh — поэтому сессии, одноразовые токены, IP клиента и явные
  публичные операции живут в китах. Эквиваленты китов на стороне шаблона не пишутся, даже временно.
- Два bump'а: v1.2.0 (S1) и v1.3.0 (S9). Шагам S2–S8 вторая партия не нужна; если тег v1.3.0
  задерживается, S11 и S12 можно выполнить раньше S9 и S10 — от второй партии они не зависят.
- DI-канон — вторым шагом, раньше доменов: переписываемые usecase сразу по канону; FX-01 и FX-05
  закрывает кит на S1.
- Контракт ошибок — отдельный шаг S8 и AC13 поверх контракта httpkit; полнота маппинга — там же, а
  не в HTTP-каноне.
- Сессии — `authnkit/session`: отзыв и ротация — механика, не домен. Погашение токенов сброса и
  подтверждения — условным `UPDATE` в SQL домена: таблицы доменные, кит даёт генерацию и хеш.
- flagkit и cachekit — вне графа до первого потребителя (FX-10): сервис и `/ready` без Redis.
- bump golibinfra в `go.mod` разрешён владельцем; изменения env из аннотаций тегов — в INFRA-HANDOFF.
- Мутации, D02 и пустые allowlist'ы — одним итоговым шагом S15.
- S1: `registerHTTP` и `httpkit.Middlewares` — в `fx.Module("cmd/api")`: Invoke модулей fx идёт
  раньше корневого хука сервера, зависимость хендлеров стартует до HTTP; `Middlewares` — от
  приложения, не `authgate`. `ErrorMap` — таблица на прежнюю функцию маппинга; `Declare` — в S8 с
  точными таблицами операций. `LoginGuard` повторяет кит; ошибка guard — в лог до S10 (SEC-05).
- S2: порт — тип результата провайдера (`NewPolicy` → `Authorizer`), identity-closure —
  delivery-порты и типы китов, без `fx.As` (closure видит archtest). `App` (Cookie) читает
  envconfig без `golibinfra/config` (`config.Load` требует ctx); строка старта —
  `context.Background()` корня процесса. `fx.StopTimeout` 30s = HTTP 15s + NATS 10s + запас.
- S3: `ForbiddenError` и `policy.MinPasswordLen` — в `pkg/policy`, 403 — `authgate.ForbiddenRule`;
  тег `minLength` схем паролей повторяет правило (держат handler-тесты), `ErrWeakPassword` — 422 с
  `Field` (AC13). `paging.Page` — вход huma, usecase и SQL; исходы — `outcome.Result`, `weak` →
  `weak_password`; ID — `idkit.NewV7()`, `google/uuid` вне `rootLibs` archtest.
- Фиксы «Фундамента»: N2 — стрим публикатора старт не проверяет: покрытие subject'ов видит JetStream
  API кита, эквивалента в шаблоне нет (бэклог `publisher-stream-check`), `TestBoot` ждёт PubAck строк
  `outbox_verification`. N4, N7 — порядок хуков `TestBoot` читает из лога сервиса (тройник поверх
  logkit): без `FxLogger` событий нет, HTTP — ровно `httpkit.serve`; `context.Context` —
  `TestFxGraphHasNoContext`, `fx.As` — archtest (через него порт обошёл бы проверку closure).

## Handoff
- API reference-доменов меняется (префиксы DTO, envelope `data`, problem+json с машинным `code`,
  новые коды ответов, ошибки в OpenAPI) — breaking для форков, которые повторяли пример API.
- Сессии на `authnkit/session`: при выкатке все пользователи входят заново один раз;
  `refresh_sessions` удаляется новой миграцией (`-- infra-handoff:`), таблица сессий кита — новый
  источник `cmd/migrate`.
- Env: `APP_ENV` обязателен; secure cookie по умолчанию; NATS с JetStream обязателен на старте
  (`SubscribeOnStart`); дефолты китов (маскирование, docs, `OUTBOX_MAX_ATTEMPTS`, флаг доверия
  прокси) — по аннотациям v1.2.0 и v1.3.0; `REDIS_URL` сервису не нужен; `OTEL_*` — если метрики
  экспортируются через telemetry.
- Образы от nonroot с закреплённой базой.
- Лёг S1–S3 (golibinfra v1.2.0): `NATS_JETSTREAM_ENABLED=true` и
  `NATS_JS_STREAM_NAMES=user,verification` — без JetStream или стрима `user` старт падает, без
  `verification` её события не публикуются; маскирование логов всегда (`LOG_REDACT_ENABLED` не
  читается, `LOG_REDACT_DISABLED=true` — не в проде); `BRUTEFORCE_IPV6_PREFIX` по умолчанию 64
  (старые блокировки теряются ≤ `BRUTEFORCE_LOCK_DURATION`); Redis, `REDIS_URL`, `FEATURE_FLAGS`
  не нужны; `PG_*`/`HTTP_*` без приставок `POSTGRES_`/`HTTP_HTTP_`; остановка до 30s —
  `stop_grace_period` ≥ 35s. API: problem+json с `code` и `request_id`.
- Блок — через `/infra-handoff`, когда изменения лягут.

## Где мы сейчас
«Фундамент» (S1–S3) сделан, валидатор — FAIL (`validation-fix-reference-code-A.md`); круг фиксов:
закрыты B1 (`0d78024`), N1 и N3 (`2177f66`), N2 (`f03d5c9`), N4 и N7. Остаток: N5 (Argon2id в
Handoff), N6 (`StopTimeout` ↔ env), N8. Дальше — перепроверка валидатором (всё после `77528f3`),
затем S4; v1.3.0 — bump в S9.
