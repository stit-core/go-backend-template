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
  `openapi.json` из `make gen`; allowlist'ы (`internal/testkit/allowlist`) опустошает этот план.
- **golibinfra меняется своими планами** в `../golibinfra` (партии «исполнитель → валидатор», тег
  пушится с OK владельца), шаблон поднимает версию. Точный состав и шаги миграции — аннотации тегов:
  - **v1.2.0** — в шаблоне с S1; контракт ошибок httpkit (problem+json с `code` и `request_id`,
    `errors[]` у 422, `Retry-After`, `httpkit.ErrorMap`: `Map`, `Unmapped`, `Declare`) — основа S8.
  - **v1.3.0** — в шаблоне с S9: `httpkit.Public` и проверка операций на старте, IP клиента в ctx,
    docs выключены, `auditkit` берёт IP и request id из ctx, сессии refresh `authnkit/session`.
- **Отчёт аудита:** https://claude.ai/artifact/L3Y8vWRofsvzf8PGQos4dE (Artifact `action: read`);
  разделы COD, SEC, TST, ADP, DOG. **Отметки владельца:** `ArtifactData` `list`, `collection:
  review` — `disagree` не чинить, `discuss` спросить перед шагом.
- **Принципы:** без оверинжиниринга — понятный, красивый, консистентный код; один канонический
  способ; удалить лучше, чем добавить; общий примитив — в golibinfra, эквивалентов китов в шаблоне
  нет. Новые миграции — новыми файлами. Коммит на шаг, push — с OK владельца.
- **Находки этой части (подробности и `file:line` — в отчёте):**
  - COD-01 — токены гасятся check-then-act (refresh — S9, сброс и подтверждение — S5); COD-02 — S5,
    COD-03 — S4. SEC-01 — сессии не отзываются, Refresh не проверяет `CanLogin`, 720 ч на ротацию;
    SEC-07 — `seed-admin` повышает аккаунт без отзыва сессий.
  - SEC-02 — log-mailer пишет токены; SEC-04 — небезопасные prod-дефолты без охраны; SEC-05 — сбой
    guard → вход без защиты; SEC-06 — перебор аккаунтов через сброс; SEC-08 — образы от root, теги;
    SEC-10 — `JWT_SECRET` нет в `.env.example`; SEC-11 — мелочи.
  - ADP-01 — `module/` без тестов, адаптеры — обёртки китов; ADP-02 — identity-closure тащит чужие
    sentinel'ы (500 вместо 409); ADP-04 — `Active` в адаптере; ADP-05 — дубли адаптеров и доверенные
    методы без policy; ADP-06 — нет контракт-теста событий; ADP-08 — мелочи.
  - COD-04 — эталон нарушает канон (named returns, константы в usecase, метрики не на всех исходах,
    DTO, мёртвый код, эссе, `t.Fatalf`); COD-05 — `MapErr` → 500; COD-06 — нет общих примитивов;
    COD-07 — метрики в no-op; COD-08 — ничейные места (`pkg/` без хартии, mailer в `module/`); COD-09 —
    скелет `new-domain.sh` со стабами и тестами NotNil; COD-10 — мелочи.
  - TST-01 — мягкие моки; TST-03 — integration на 3 из 9 методов; TST-04 — порог покрытия впритык,
    скелет валит cover-check; TST-06 — 6 из 13 эндпоинтов без happy-path; TST-08 — мелочи.
- **Мутации для перепроверки:** 34 из раздела TST отчёта и D02 из DOG (Publish вне транзакции).
- **fx-аудит** — `/home/mm/.cache/stit-audit-program/fx-audit.md`: FX-01, FX-05 — S1, FX-06, FX-10, FX-11 — S2.

## Не входит
- Проза обвязки, рецепты, жизненный цикл шаблона — `repair-harness`.
- Правки golibinfra — планами в `../golibinfra`; здесь — bump и снятие того, что взял кит.
- e2e в гейте: `make e2e` (Docker, `.env.local` не нужен) — проверка шагов, меняющих API.

## Критерии приёмки
- [x] **AC1** — Одноразовый токен гасится ровно один раз при параллельных запросах (refresh,
  сброс пароля, подтверждение email). Проверка: integration-тест с параллельным погашением → один
  успех, остальные — sentinel «уже использован»
  - Подтверждено: сброс и подтверждение — `TestMarkUsed_Concurrent` (8 транзакций → 1 успех, 7
    `ErrTokenUsed`); refresh — `TestSessions` (8 обновлений одним cookie → 1 × 204, 7 × `AUTH_REFRESH_REUSED`)
- [x] **AC2** — Сессии управляемы: заблокированный не обновляет токены; смена и сброс пароля,
  блокировка, удаление, смена роли и повторное использование refresh гасят сессии; у сессии есть
  абсолютный срок жизни; `seed-admin` не отдаёт права чужим сессиям. Проверка: тесты usecase и
  integration на каждый случай → PASS
  - Подтверждено: usecase — `TestRefresh_UserCannotLogIn`, `TestRefresh_Reused`, `TestRefresh_Expired`
    (срок токена и абсолютный), `TestUpdate`, `TestDelete`, `TestTrusted_UpdatePassword` (смена и
    сброс; `RevokeAll` в транзакции записи), `TestLogin_CredentialsChangedDuringLogin`; integration —
    `TestSessions` (повтор, выход, смена и сброс пароля, блокировка, удаление),
    `TestSessions_LoginDuringPasswordChange` (вход в гонке со сменой), seed-admin (и откат) — PASS
- [x] **AC3** — Контракт user: клиент не меняет свой статус (403); name и status применяются вместе
  или 400; неизвестный статус → 422; несуществующий id → 404; устаревшая версия → 409; `version`
  есть в ответе. Проверка: handler- и integration-тесты → PASS
  - Подтверждено: `TestUpdate`, `TestUpdate_*` (usecase с настоящей policy, handler), integration
    `TestUpdate`, `TestUpdate_StaleVersion`, `TestDeletedUserIsUntouched` — PASS
- [x] **AC4** — Идемпотентный консьюмер не теряет эффект: сбой mailer на первой доставке → письмо
  уходит на повторной; повтор после успеха письмо не дублирует. Проверка: тест redelivery → PASS
  - Подтверждено: `TestIssueEmailVerification_Redelivery`, integration `TestUpsertEmailVerification_Redelivery`
- [x] **AC5** — Ошибки через границу доменов маппятся: change-password при конкурентном
  обновлении → 409; адаптер переводит каждый sentinel поставщика, который отдаёт порт, в sentinel
  потребителя. Проверка: тесты адаптеров и usecase → PASS
  - Подтверждено: `TestUserAdapter_MapsUserErrors` (auth, verification), `TestChangePassword`,
    `TestChangePassword_Errors` (409 `AUTH_CONCURRENT_UPDATE`) — PASS
- [x] **AC6** — Секретов и токенов нет в логах; дефолты безопасны; вне `APP_ENV=dev` сервис не
  стартует с небезопасной конфигурацией (insecure cookie, лог-mailer, выключенное маскирование,
  docs). Проверка: тест конфигурации и тест лог-полей → PASS
  - Подтверждено: `TestGuard`, `TestModule` (config), `TestNew_DevOnly` (logmail), `TestBoot` — пароли,
    cookie сессии и токены писем не в логе даже до маскирования; docs по умолчанию выключены (v1.3.0)
- [x] **AC7** — Каждая операция либо под `Security`, либо помечена `httpkit.Public` — иначе сервис
  не стартует; без cookie → 401, чужой ресурс → 403. Проверка: тест сборки API (операция без
  `Security` и пометки → ошибка старта), handler-тесты и boot-тест → PASS
  - Подтверждено: `TestHTTPOperations` (`Server.CheckRoutes`, коды 401 сессии),
    `TestHTTPOperations_UnmarkedFailsStart` (`httpkit.ErrRoutes` на старте), `TestBoot` (401 без
    cookie у каждой операции под `Security`), 403 — тесты policy и handler
- [ ] **AC8** — Покрытие по широкому scope (domain, delivery, platform, module, infra; без
  сгенерированного, моков и `cmd/`), unit + integration в одном профиле: гейт ≥ 90%, фактически
  ≥ 95% или поштучное объяснение, почему нет. Проверка: `make cover-check` → OK
- [ ] **AC9** — Мутации аудита: из 34 гейт убивает не меньше 32, каждый выживший объяснён в
  «Решениях» (эквивалентный мутант); D02 убит. Проверка: повторить мутации → таблица
- [x] **AC10** — `make new-domain NAME=probe` без правок проходит `make check`, включая покрытие.
  Проверка: в worktree → `make check` зелёный
  - Подтверждено: в worktree `make new-domain NAME=probe` (и `order_item`) → `make check` → `✓ check clean`
- [x] **AC11** — Allowlist'ы archtest и `cmd/api` пусты, обходов в ожидании китов
  нет (`beforeHTTP`, client-IP middleware, `pkg/tokens`, `publicOperations`); гейт зелёный.
  Проверка: `grep -rn "beforeHTTP\|WithIP\|pkg/tokens\|publicOperations" --include=*.go .` → пусто;
  `make check` → `✓ check clean`
  - Подтверждено: grep пуст (S9), allowlist'ы `cmd/api` (S10) и archtest (S11) пусты, `make check` →
    `✓ check clean`
- [x] **AC12** — Сборка DI единообразна: одна идиома привязки к интерфейсу, имя модуля — путь
  пакета, конфиги китов — через их `NewConfig`, `main`, тест графа и boot-тест собирают один
  `service`, зависимости usecase — одной структурой, события fx — в logkit, порядок старта и
  остановки держит httpkit, таймауты старта и остановки явные; в графе нет `context.Context`,
  flagkit и cachekit. Проверка: boot-тест зелёный; fx X16 невозможен по построению; X6 и X15
  ловят кит или `fx.ValidateApp`
  - Подтверждено: `TestBoot` зелёный без Redis; X16 невозможен (`Usecase{d UsecaseDeps}`); X6 —
    порядок держит httpkit, X15 — `fx.ValidateApp`; `fx.As`, `context.Context`, flagkit, cachekit нет
- [x] **AC13** — Контракт ошибок API: каждая ошибка — problem+json (RFC 9457) со стабильным
  машинным `code`; 422 перечисляет поля (путь, код, сообщение); 401 и 403 различают «войти
  заново», «обновить сессию» и «нет прав»; 429 несёт `Retry-After`; 409 — код конфликта; 5xx без
  внутренностей, с ID запроса, тот же ID — в логах; каждая операция объявляет свои ошибки в
  OpenAPI. Проверка: тест полноты — каждый sentinel каждого домена → (статус, `code`), ожидаемая
  ошибка не даёт 500; `openapi.json` содержит ошибки операций; handler-тесты по кодам → PASS
  - Подтверждено: archtest `TestDomainErrorTables` (правило на каждую ошибку, `Map` таблицей домена, 500
    исключён, таблица операции — в `Declare`); `TestErrorMaps`; handler-тесты кодов, 422, 401, 429 с
    `Retry-After`; `TestBoot` — 5xx без причины, она в логе под тем же `request_id`

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
  - Сделано: `make check` → `✓ check clean`, grep пусто; X6 держит кит (HTTP последним и первым), X11 —
    `ErrAuthNotConfigured`, X15 — `TestFxGraphValidates`; `TestBoot` ловит X12, X13 и `registerHTTP` в
    корне при зависимости с хуком только у хендлеров (T-C); логи — `fdn-s1-*.log` в материалах
- [x] **S2** — DI-канон: зависимости usecase — одной структурой; одна идиома привязки к интерфейсу;
  имя модуля — путь пакета; конфиги китов — через их `NewConfig`, `App` — только конфиги приложения;
  без `context.Context` в графе; flagkit и cachekit вне графа до первого потребителя — `/ready`,
  boot-тест, e2e-стек и local-deps без Redis; явные `fx.StartTimeout`/`fx.StopTimeout`; раздел «fx»
  в `architecture.md` тем же коммитом (FX-06, FX-10, FX-11)
  - Проверка: AC12 → выполнен · Закрывает: AC12 · Зависит от: S1
  - Сделано: AC12 подтверждён, `make check` → `✓ check clean`; X6, X15 повторены (`fdn-s2-*.log`);
    скелет `make new-domain` по новому канону собирается, lint и его тесты зелёные
- [x] **S3** — Общие примитивы без лишнего: `ForbiddenError` и его маппинг в одном месте, единственный
  источник правила пароля, тип пагинации, константы исходов метрик, ID — через `idkit` (COD-06)
  - Проверка: `make check` → зелёный; `grep -rn "type ForbiddenError" internal pkg` → одно место ·
    Закрывает: — · Зависит от: S2
  - Сделано: `make check` → `✓ check clean`; grep → `pkg/policy/policy.go`; скелет `make new-domain`
    на `outcome.Result` собирается, lint и его тесты зелёные

### Эталон доменов
- [x] **S4** — user: переходы статуса методами entity, смена статуса только админом, PATCH целиком
  или 400, enum, `version` в ответе, 404 и 409 раздельно, update не трогает удалённых (COD-03)
  - Проверка: AC3 → PASS · Закрывает: AC3 · Зависит от: S3
  - Сделано: AC3 подтверждён, `make check` → `✓ check clean`; e2e обновлены (ruff, mypy, сбор)
- [x] **S5** — verification: канон идемпотентности без потери эффекта, `TokenID` из БД, правило
  событий и `event.md` под новый канон (COD-02); сброс пароля и подтверждение email гасятся
  условным `UPDATE … WHERE used_at IS NULL` (`:execrows`, 0 строк → sentinel) (COD-01)
  - Проверка: AC4 → PASS; AC1 для сброса и подтверждения → PASS · Закрывает: AC1, AC4 · Зависит от: S3
  - Сделано: AC4 и AC1 (сброс, подтверждение) — PASS, `make check` → `✓ check clean`; поломки S4 и S5
    (условия SQL, порядок погашения, `sent_at`, TokenID, правила 409) — 20 из 20 пойманы
- [x] **S6** — Адаптеры: типовые с исчерпывающим маппингом вместо identity-closure; вычисленные
  свойства — методы сущности поставщика; обёртки китов вынесены из `module/`; доверенные методы
  поставщика отделены от методов с policy (ADP-01, ADP-02, ADP-04, ADP-05)
  - Проверка: AC5 → PASS, allowlist ADP-02 пуст · Закрывает: AC5 · Зависит от: S4
  - Сделано: AC5 подтверждён, allowlist closure удалён, `make check` → `✓ check clean`; 9 поломок
    адаптеров, `Trusted`, closure и 409 — пойманы
- [x] **S7** — HTTP-канон в auth и verification: префиксы DTO, envelope `data`, `Security` на всех
  непубличных операциях; e2e-модели обновлены и строгие (SEC-03)
  - Проверка: handler-тесты и `TestHTTPOperations` → PASS · Закрывает: AC7 · Зависит от: S6
  - Сделано: handler-тесты, `TestHTTPOperations`, `TestHTTPTypes`, `TestBoot` — PASS, `make check` →
    `✓ check clean`; e2e-модели сверены с `openapi.json` (поля и обязательность), ruff, mypy
- [x] **S8** — Контракт ошибок API поверх контракта httpkit: машинный `code` на каждую доменную
  ошибку, поля в 422, разные коды 401 и 403 (войти заново, обновить сессию, нет прав),
  `Retry-After` у 429 блокировки логина, код конфликта у 409, 5xx с ID запроса; ошибки операций
  объявлены в OpenAPI (`ErrorMap.Declare`); тест полноты маппинга — каждый sentinel → (статус,
  `code`), `ErrorMap.Unmapped` пуст (COD-05)
  - Проверка: AC13 → PASS; `make gen` → `openapi.json` с ошибками операций, без дрейфа ·
    Закрывает: AC13 · Зависит от: S7
  - Сделано: AC13 подтверждён, `make gen` без дрейфа, `make check` → `✓ check clean`; поломки
    таблиц, `Declare`, `Secured`, полноты и 5xx — пойманы

### Сессии и безопасность
- [x] **S9** — Bump golibinfra до v1.3.0 и сессии на ките: refresh — `authnkit/session` (ротация,
  повтор → отзыв цепочки, `CanLogin` в Refresh, абсолютный срок), отзыв всех сессий пользователя
  при блокировке, удалении, смене роли, смене и сбросе пароля; `refresh_sessions` — новой
  миграцией с `-- infra-handoff:`; `seed-admin` не повышает чужие сессии; снять `pkg/tokens`
  (токены authnkit), client-IP middleware и `identity.WithIP` (IP из ctx httpkit; флаг доверия
  прокси — в `.env.example` и Handoff: за Traefik без него один bucket лимитера и антибрутфорса),
  `publicOperations` (пометки `httpkit.Public` в `Register` доменов), ручные `IP` и `RequestID` в
  записях аудита; вход и сброс с пустым IP (сейчас 401) — по IP из ctx httpkit; `api healthcheck` — на функцию httpkit из v1.3.0 (SEC-01, SEC-07, COD-01, COD-06, FX-03)
  - Проверка: AC1 для refresh, AC2 → PASS; операция без `Security` и пометки → ошибка старта в
    тесте сборки API; тесты usecase кладут IP и request id в ctx и сверяют их в аудите и лимитере
    (валидатор D, 3) · Закрывает: AC1, AC2, AC7, AC11 · Зависит от: S12, тег golibinfra v1.3.0
  - Сделано: коммиты от `72fab84`; AC1, AC2, AC7, AC11 подтверждены, `make check` → `✓ check clean`,
    `make e2e` → 16 passed, AC10 (`probe`, `order_item`) зелёный; 18 поломок (отзыв сессий, CanLogin,
    IP, аудит, стримы, `Public`, бюджет, гард, лесенка) — пойманы тестами (`s9/` в материалах)
- [x] **S10** — Безопасные дефолты шаблона: mailer не логирует токен и email и работает только в dev;
  prod-гард конфигурации (`APP_ENV` обязателен; вне dev — secure cookie, без лог-mailer,
  маскирование включено, docs выключены); явное поведение при ошибке guard (fail-closed для
  логина); единый ответ и троттлинг сброса пароля; `JWT_SECRET` в `.env.example`; образы nonroot
  с закреплённой базой (SEC-02, SEC-04 – SEC-10)
  - Проверка: AC6 → PASS, allowlist SEC-10 пуст · Закрывает: AC6 · Зависит от: S8
  - Сделано: AC6 подтверждён, allowlist `cmd/api` пуст, `make check` → `✓ check clean`, `make e2e` →
    16 passed (стек — distroless от UID 65532); 12 поломок гарда, входа, лимита и отзыва ссылок
    сброса, лог-mailer, cookie, правил email и пароля — пойманы, токен в логе — `TestBoot`

### Чистка и скелет
- [x] **S11** — Чистка эталона: канон соблюдён без исключений, мёртвый код и эссе удалены; метрики
  — экспорт через telemetry (выключен, пока в infra нет коллектора), бизнес-метрики только
  там, где несут смысл, с проверкой записи в тестах; хартия каталогов, место mailer, `platform/shared` убран из доков (allowlist
  ADP-03); в логах и событиях — только ID (email — портом `UserReader`; поле из payload — subject `.v2`; email в метаданных аудита — решить; Handoff) (COD-04, COD-07, COD-08, COD-10, ADP-08, SEC-11)
  - Проверка: `make check` → зелёный, allowlist ADP-03 пуст · Закрывает: AC11 · Зависит от: S10
  - Сделано: `make check` → `✓ check clean` (покрытие 88,1%), allowlist ADP-03 пуст, `make e2e` → 16
    passed (payload `.v2` — только ID); 12 поломок метрик, аудита, событий, guard'а смены пароля,
    telemetry и пагинации — пойманы, email в логе — `TestBoot`
- [x] **S12** — Скелет `new-domain.sh`: минимальный вертикальный срез (Create и Get) по канону, с
  настоящими тестами, без стабов → nil, подключён в `cmd/api` (COD-09)
  - Проверка: AC10 → PASS · Закрывает: AC10 · Зависит от: S11
  - Сделано: AC10 подтверждён, `make check` → `✓ check clean`; 10 поломок среза — пойманы его тестами

### Тесты и итог
- [ ] **S13** — Канон тестов: без `.Maybe()` на побочных эффектах, policy с точными аргументами или
  настоящая, `Recorder.Last(subject)`, тесты адаптеров, happy-path на все эндпоинты, integration на
  все методы репозиториев, контракт-тесты событий; разбор подкоманды `healthcheck` в `main()`, исход метрики при сбое лимитера и записи ссылки сброса (TST-01, TST-03, TST-06, ADP-06, TST-08)
  - Проверка: `make check` → зелёный · Закрывает: AC9 · Зависит от: S12 (user, verification), S9 (auth)
- [ ] **S14** — Покрытие: широкий scope, общий профиль unit + integration, `COVER_FLOOR` 90; довести до
  ≥ 95% осмысленными тестами и поднять порог до 95, если держится стабильно (TST-04); строка 11 матрицы
  ENF (непокрытый новый код) ловится с запасом, а не только потому, что до порога ≈1 statement (сейчас
  75,1% при 75%): иначе — проверка покрытия новых функций или исключение с причиной в «Решениях»
  - Проверка: AC8 → OK · Закрывает: AC8 · Зависит от: S13
- [ ] **S15** — Итог: повторить 34 мутации аудита и D02, таблица — в «Где мы сейчас»; allowlist'ы
  пусты, обходов нет, гейт зелёный
  - Проверка: AC9 → ≥ 32 из 34; `make check` → `✓ check clean` · Закрывает: AC9, AC11 · Зависит от: S14

## Решения
- Порог покрытия: гейт 90%, цель 95% (владелец: 90–95) — осмысленными тестами (силу держат мутации, AC9).
- Граница с golibinfra: общая механика — в китах, здесь — bump и снятие взятого китом; эквивалентов китов
  в шаблоне нет, даже временно (auth-стек скопирован в `ai-gateway-template`, форки пишут его сами).
- Два bump'а: v1.2.0 (S1) и v1.3.0 (S9); S10–S12 и S13 для user и verification от v1.3.0 не зависят и
  идут раньше S9. Сессии — `authnkit/session`, токены сброса и подтверждения — SQL домена (S5); bump
  `go.mod` разрешён, env — в Handoff; мутации, D02, allowlist'ы — S15.
- S1–S8, S10–S12: решения — в rules, `docs/reference/` и телах коммитов шагов; здесь — опора S9, S13–S15:
  - вход — fail-closed (неучитываемый ключ — 401); сброс — один ответ 204 при любом сбое после поиска
    учётной записи (лог и метрика), лимит — bruteforcekit на (пользователь, IP) портом `ResetLimiter`,
    confirm гасит прочие ссылки, тайминг — бэклог `reset-timing`;
  - метрики — `telemetry.Module`, `Meter` доменов — из `*telemetry.Providers`, в тестах —
    `metrictest.NewProvider`; события `.v2` — только ID, старый
    `user.events.registered` дочитывает прежний durable (бэклог `events-v1-reader`);
  - здоровье — `api healthcheck` = `httpkit.Healthcheck` (в distroless нет shell); `HEALTHCHECK` в
    Dockerfile нет — интервалы задаёт деплой.
- S9 (подробно — тела коммитов): auth берёт `session.Store` кита напрямую (порт и значения без I/O —
  `rootLibs`), тесты — фейк `sessiontest`; user — свой порт `Sessions` (`RevokeAll`), строгий мок с
  `pgkit.InTx`. Отзыв: смена и сброс пароля — `user.Trusted.UpdatePassword` (одно место обоим потокам),
  блокировка — `Update` по `!CanLogin()`, удаление — `Delete`, повышение — `seed-admin`; смены роли в
  API нет. Вход открывает сессию под `FOR SHARE` строки пользователя и сверяет пароль, роль и статус
  (B1 валидатора E: иначе вход в гонке со сменой пароля переживал её). Refresh — `Rotate`, затем
  пользователь: удалённый или заблокированный — 401, выданный токен закрыт. Выход без аудита (кит не
  отдаёт subject) — бэклог `logout-audit`. Гард вне dev — `JWT_AUDIENCE`, бюджет остановки —
  `checkStopBudget`; `OUTBOX_MAX_ATTEMPTS`, `SESSION_*` проверяет кит. IP в тестах — `identitytest` через
  `httpkit.ClientIP` (бэклог `clientip-test-setter`). Лесенка — `testkit.MigrationLadder` (Handoff golibinfra).
- S13: policy в тестах usecase — настоящая, мока `Authorizer` нет; порты — строгие моки (rule
  `tests-pipeline`). Контракт события — у потребителя: событие usecase поставщика в JSON → `Register`.
  `main()` — перезапуск тестового бинаря с пустой командной строкой (`go test` передаёт `-test.*`), гейт
  не запускается с `API_TEST_MAIN_ARGS`. Путь писем держит `TestBoot`, e2e сброса не нужен (TST-08).
  S15: M04 и M05 аудита — правило длины пароля (`pkg/policy.PasswordStrong`, `TestPasswordStrong`).

## Handoff
- API reference-доменов меняется (префиксы DTO, `data`, problem+json с `code`, коды и ошибки в OpenAPI) —
  breaking для форков. user: PATCH целиком, `status` — только админ, неизвестный id и повторный DELETE —
  404, `version` в ответе, невалидный email регистрации — 422 поля `body.email` (был 400); auth: login —
  `data`, change-password — 404, 409, старый пароль — 422, битый refresh — 401, истёкший access —
  `AUTH_ACCESS_EXPIRED`, 429 — `Retry-After` (и у change-password); refresh — непрозрачный одноразовый
  токен, повтор и параллельное обновление одним cookie закрывают сессию; тела ответов без `$schema`;
  verification: погашенный токен — 409, запрос сброса — 204; `page` ≤ 100000.
- События verification: `TokenID` — id строки; `email_verification_requested` повторяется при перевыпуске
  неотправленного письма — дедуп по `TokenID`. Миграция 0004 — nullable-колонка без блокировок; после
  выкатки ожидающие — неотправленные: повтор старой доставки пришлёт второе письмо, первая ссылка умрёт.
- Лёг S1–S3 (golibinfra v1.2.0): маскирование логов всегда (`LOG_REDACT_ENABLED` не читается);
  `BRUTEFORCE_IPV6_PREFIX` по умолчанию 64 (старые блокировки теряются ≤ `BRUTEFORCE_LOCK_DURATION`);
  Redis, `REDIS_URL`, `FEATURE_FLAGS` не нужны; `PG_*`/`HTTP_*` без приставок `POSTGRES_`/`HTTP_HTTP_`.
  Argon2id — разом не больше `max(1, GOMAXPROCS/ARGON2_THREADS)` хэшей: при `ARGON2_THREADS=4` и < 8 CPU
  пароли хэшируются по одному — CPU пода и реплики под пик.
- Блоки S9–S11 — в накопителе `.claude/plans/handoff.md`: гард (`APP_ENV`, вне dev `JWT_AUDIENCE`), сессии
  (миграция 0005, `SESSION_*`, вход заново), прокси и остановка (`HTTP_TRUST_PROXY_HOPS`,
  `HTTP_SHUTDOWN_*`, `stop_grace_period` ≥ 35s), стримы публикаторов, дефолты китов, образы, `.v2`, OTel.

## Где мы сейчас
S1–S8, S10–S12 и часть 1 S13 (user, verification) приняты (отчёты A–D); S9 сделан (golibinfra v1.3.0,
сессии на ките, обходы сняты) — ждёт валидатора. Дальше: валидатор S9, затем часть 2 S13 — auth:
`.Maybe()`, мок `Authorizer`, happy-path change-password, адаптеры (M15, M30, M31); потом S14, S15.
