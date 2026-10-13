---
status: in-progress
parent: —
blocked-by: —
---

# fix-identity-edges — края доменов идентичности до девятки

## Цель
Закрыть трек identity программы `reaudit-to-nine`: medium-находки углов code и security (RCOD-01–04,
RSECT-01–04), RCOD-13, запись окна access-JWT и пробелы тестов RTSTT-02–06 — так, чтобы эксперименты
аудиторов на ветке дефекта не показывали, а исправление держал гейт.

## Контекст
- Программа — `.claude/plans/reaudit-to-nine.md`: метод, владение файлами, решения владельца (не
  пересматриваются). Ветка `nine/identity`, worktree `N/wt-identity`, база — `main` `e8ea150`;
  `N` = `/home/mm/.cache/stit-audit-program/nine`, `R` = `/home/mm/.cache/stit-audit-program/reaudit`.
- Находки — разделы `### <ID>` в `R/template-{code,security,tests}.md`; пробы для повтора —
  `R/tpl-code/*.go.txt`, `R/tplsec-probes/`, мутанты — `R/tpl-tests/new/mutants.py`; формулировки пунктов —
  `git show ab46f45:.claude/plans/backlog.md`. Логи экспериментов этого трека — `N/identity/`.
- Трек владеет `internal/domain/{user,auth,verification}/**`, `cmd/**`, новыми файлами `migrations/`,
  `openapi.json`, `SECURITY.md`, `docs/stack/_templates/auth.md`, `.env.example`, rule
  `events-and-messaging`, `scaffold/references/event.md`, `docs/reference/domain-mapping.md`. Чужой файл —
  минимальной правкой и строкой в «Решения».
- Решения владельца: 409 на занятый адрес остаётся (RSECT-03); строгий лимит на `/api/auth/*` и требование
  лимита в `docs/stack` (RSECT-01); миграция для `reset-timing` — новым файлом.
- Тяжёлые прогоны (`make check`, эксперименты с Docker) — только под `flock N/gate.lock`.

## Не входит
- Отказ при переполнении очереди Argon2id (golibinfra), `go.mod`, `pkg/policy`, `internal/platform/authgate/`,
  `internal/archtest`, `Makefile`, `scripts/`.
- Регистрация через подтверждение адреса до раскрытия занятости и server-side отзыв access-JWT —
  решения продукта форка; здесь они записаны как компромиссы.
- Прочие minor углов code и security (RCOD-05–12, 14–18; RSECT-05–08) — в бэклоге.

## Критерии приёмки
- [x] **AC1** — Один ящик — одна учётная запись: адрес с пробелом или табом по краям (и формы `Имя <…>`,
  `<…>`, с комментарием) регистрация отвергает 422 `USER_INVALID_EMAIL`, индекс не меняется; вход и сброс
  ищут адрес без пробелов по краям — как ключ guard'а `bruteforcekit.NormalizeKey`; `seed-admin` с таким
  адресом — код 1. На базе (E2) у ящика 4 учётные записи и 4 письма. Проверка: E2
  (`R/tpl-code/api_e1_e2_e3_e6_test.go.txt`) на ветке → 201, 422, 422, 422, 409, учётная запись одна;
  fuzz-свойства сущности и поиска → PASS
  - Подтверждено: E2 на ветке → 201, 422, 422, 422, 409, учётная запись ящика одна, письмо одно, вход адресом с
    пробелом — 200 в ту же учётную запись (`N/identity/s1-e2.log`); `FuzzNewUser`, `FuzzTrusted_LookupByEmail`
    по 15 с → PASS
- [x] **AC2** — Имя ограничено: регистрация и правка с именем длиннее 200 символов — 422 `VALIDATION_FAILED`
  (`maxLength` в `openapi.json`); сущность отвергает имя длиннее 200 символов, пустое после обрезки пробелов
  и с NUL, как скелет `make new-domain`. На базе (E6) имя в 100 000 символов — 201. Проверка: E6 на ветке →
  422; `make gen-check` → без дрейфа
  - Подтверждено: E6 на ветке → 422 (`N/identity/s2-e6.log`); `openapi.json` — `maxLength: 200` у name
    регистрации и правки; `make gen-check` → без дрейфа
- [x] **AC3** — Вход не выдаёт сбой хэшера за неверный пароль: неверный — только `!ok` и пароль длиннее
  предела (401, аудит `login_failed`); дедлайн очереди Argon2id и нечитаемый хэш — ошибка сервера с причиной
  в логе, исход `error`, без записи аудита. На базе (E7) — `invalid credentials`, ложный аудит, лог пуст.
  Проверка: юнит-тест сбоев хэшера и integration «нечитаемый хэш → 500 с причиной в логе» → PASS
  - Подтверждено: `TestLogin_HasherFailure` (дедлайн очереди, нечитаемый хэш: ошибка с причиной, аудита нет,
    исход `error`), `TestLogin_PasswordOverHasherLimit` (401 и `login_failed`), `TestIdentity` (500, причина
    в логе под `request_id` ответа, `login_failed` нет) → PASS; на базе — 401 и ложный аудит
- [x] **AC4** — Мутант AI4 «роль в access-токене всегда admin» гейт ловит. На базе он проходит
  `make check`. Проверка: AI4 на ветке → красные `TestTokens_AccessCarriesRole` и `TestSessions` (клиент со
  своим cookie: `GET /api/users` и `PATCH` чужого — 403)
  - Подтверждено: AI4 на ветке → FAIL `TestTokens_AccessCarriesRole/client` и
    `TestSessions/a_client_session_is_refused_admin_operations` (`N/identity/s4-ai4-mutant.log`); без мутанта — PASS
- [x] **AC5** — Прежнего durable на `user.events.registered` нет, его удаление — блоком в накопителе.
  Проверка: `TestRegister` (`verification/delivery/nats`) → durable только у `.v2`; блок в
  `.claude/plans/handoff.md`
  - Подтверждено: `TestRegister` → один durable `verification-user-registered-v2`; блок «удалить durable
    verification-user-registered» — в накопителе
- [x] **AC6** — Повтор события регистрации после подтверждения адреса письма не шлёт: ключ подписчика —
  состояние агрегата (порт отдаёт «адрес подтверждён»), канон — в rule `events-and-messaging` и `event.md`.
  На базе (E1) после подтверждения — второе письмо и вторая запись. Проверка: юнит «повтор после погашения»
  → PASS; E1 на ветке → письмо одно, запись одна
  - Подтверждено: E1 на ветке → дубль после подтверждения: писем 1, записей 1, ожидающих 0
    (`N/identity/s6-e1.log`); `TestIssueEmailVerification_ReplayAfterConfirmation` → PASS
- [x] **AC7** — Поток входов с одного адреса не ставит вход других в очередь хэша на секунды: строгий лимит
  клиента на `/api/auth/*` поверх общего, требование лимита — в шаблоне `auth.md`. На базе (H2,
  GOMAXPROCS=4, 90 rps) вход жертвы с другого адреса — 13–22 с. Проверка: H2
  (`R/tplsec-probes/cmd-api/raprobe_dos_test.go`) на ветке → вход жертвы быстрее 1 с, атакующему —
  429 `RATE_LIMITED`; integration-тест лимита → PASS
  - Подтверждено: H2 на ветке, GOMAXPROCS=4, 90 rps → вход жертвы 40, 80, 60 мс (без атаки — 42 мс),
    атакующему 429 — 346 из 376; GOMAXPROCS=20, 95 rps → 50, 50, 40 мс (`N/identity/s7-h2-*.log`);
    `TestBoot_AuthRateLimit`, `TestAuthRateLimit`, `TestLoad_AuthRateLimit` → PASS
- [x] **AC8** — Время ответа на запрос сброса не зависит от того, есть ли учётная запись; письмо сброса
  шлёт подписчик своего события с `sent_at`, токен выпускает он, сбой провайдера письмо не теряет. На базе
  E8 (mailer 300 мс): 0–2 мс против 304–308 мс; B07b: z = 7,68. Проверка: E8 и B07b на ветке → медианы
  в пределах шума, |z| < 3; письмо сброса доходит (`TestIdentity`, `TestSessions`); INFRA-HANDOFF в накопителе
  - Подтверждено: E8 на ветке (mailer 300 мс) → неизвестный адрес 4, 1, 1 мс, известный 2, 1, 1 мс; B07b трижды →
    z = −0,05, −0,65, 0,46; B07c (mailer 150 мс) → медианы 1,18 и 1,19 мс, одним запросом не различить
    (`N/identity/s8-*.log`; пробам с регистрацией сотни учётных записей с одного адреса выключен лимит
    `/api/auth/*`, как общий); `TestIdentity`, `TestSessions` → PASS; блок в накопителе
- [x] **AC9** — Компромиссы записаны, и код им не противоречит: 409 на занятый адрес (перечисление) и окно
  access-JWT до `exp` после выхода, блокировки, удаления и смены пароля (Z01) с тем, чем оно ограничено, — в
  `SECURITY.md` и шаблоне `auth.md`; комментарии кода «по ответу не узнать» правдивы. Проверка: чтение
  `SECURITY.md` и `docs/stack/_templates/auth.md`; `grep -rn "не узнать\|неотличим" internal/` — только о
  входе и сбросе
  - Подтверждено: `SECURITY.md` § Принятые компромиссы — 409 регистрации, окно access-JWT (Z01: заблокированный
    admin с живым токеном), лимит входа по клиенту; шаблон `auth.md` § Authn — оба решения продукта; grep →
    одна строка, о запросе сброса (`verification/delivery/http/handler.go`)
- [ ] **AC10** — Выжившие мутанты угла тестов SD1, SD5, SD6, SD7, K01, K04, MI1, A36, UH7 и AD1 (юнитом
  адаптера) гейт убивает; на базе каждый проходит `make check`. Проверка: каждый мутант на ветке → красный
  тест своего пакета
- [ ] **AC11** — Гейт зелёный. Проверка: `flock N/gate.lock make check` → `✓ check clean`

## Шаги
### Находки code и security
- [x] **S1** — email-canonical: сущность берёт только канонический адрес, поиск по адресу — без пробелов
  по краям, `seed-admin` — правилом сущности; fuzz-свойства; INFRA-HANDOFF
  - Проверка: RED → GREEN тестов `user`; integration (вход с пробелом — та же учётная запись) → PASS; E2 →
    одна учётная запись · Закрывает: AC1 · Зависит от: —
  - Сделано: `validEmail` (адрес = свой разбор `mail.ParseAddress`) в `NewUser`, `emailKey` (без пробелов по
    краям) в `Trusted.LookupByEmail`, `seed-admin` проверяет адрес и имя `user.NewUser`; RED на базе — кейсы
    `TestNewUser`, fuzz-сиды, `TestSessions/a_padded_address_is_the_same_mailbox`,
    `TestRun_RejectsAddressOutsideUserRules` (`N/identity/s1-red-*.log`), GREEN; блок в накопителе;
    `make check` → `✓ check clean`
- [x] **S2** — name-max-length: предел 200 символов в сущности и `maxLength` в DTO, `make gen`; INFRA-HANDOFF
  - Проверка: RED → GREEN тестов `user`; `make gen-check`; E6 → 422 · Закрывает: AC2 · Зависит от: —
  - Сделано: `validName` — не пустое после обрезки, до 200 символов (`maxNameLen`), без NUL, как скелет;
    `maxLength:"200"` у name в DTO, `make gen`; RED на базе — кейсы `TestNewUser`, `TestUser_Rename`, сиды
    fuzz, `TestRegister_NameTooLong`, `TestUpdate_InvalidBody`, имя `seed-admin` (`N/identity/s2-red-*.log`),
    GREEN; блок в накопителе; `make check` → `✓ check clean`
- [x] **S3** — login-hasher-failure: сбой `Verify` — ошибка сервера, неверный пароль — `!ok` и предел
  - Проверка: RED → GREEN `TestLogin_HasherFailure`; integration «нечитаемый хэш» → PASS · Закрывает: AC3
  - Сделано: `login` — `switch` по итогу `Verify`: `ErrPasswordTooLong` и `!ok` — неверный пароль, прочая ошибка —
    `verify password: %w` с исходом `error`; RED на базе — `TestLogin_HasherFailure` и `TestIdentity` (401
    вместо 500, `N/identity/s3-red-*.log`), GREEN; `make check` → `✓ check clean`
- [x] **S4** — role-claim-test: таблица ролей выпуска токена и 403 клиенту с настоящим cookie
  - Проверка: мутант AI4 → оба теста красные, без мутанта — зелёные · Закрывает: AC4
  - Сделано: таблица ролей client и admin в `TestTokens_AccessCarriesRole`; подтест `TestSessions` — сессия
    клиента: `GET /api/users` и `PATCH` чужого статуса — 403 `FORBIDDEN`; AI4 → оба красные; `make check` →
    `✓ check clean`
- [x] **S5** — events-v1-reader: снять подписку прежним durable; блок «удалить durable» в накопитель
  - Проверка: `make test PKG=./internal/domain/verification/...` → PASS · Закрывает: AC5
  - Сделано: сняты `subjectUserRegisteredV1`, `durableNameV1` и вторая подписка, комментарии-история; RED на
    базе — `TestRegister` (два durable), GREEN; блок в накопителе; `make check` → `✓ check clean`
- [x] **S6** — subscriber-replay: подписчик пропускает подтверждённый адрес (порт отдаёт состояние), канон
  в rule и `event.md`
  - Проверка: RED → GREEN юнита «повтор после погашения»; E1 → письмо одно · Закрывает: AC6
  - Сделано: `verification.User.EmailVerified` из адаптера, `IssueEmailVerification` пропускает подтверждённый
    адрес до записи; канон «ключ — событие или состояние агрегата» — rule `events-and-messaging`, `event.md`
    § 6, `architecture.md` § Idempotency; RED на базе — юнит и маппинг адаптера (`N/identity/s6-red.log`),
    GREEN; `make check` → `✓ check clean`
- [x] **S7** — login-hash-queue: лимит клиента на `/api/auth/*` (env `HTTP_AUTH_RATE_LIMIT_*`), требование в
  шаблоне `auth.md`; INFRA-HANDOFF
  - Проверка: RED → GREEN тестов лимита; H2 → вход жертвы быстрее 1 с · Закрывает: AC7
  - Сделано: `App.AuthRateLimit` (`HTTP_AUTH_RATE_LIMIT_RPS` 5, `_BURST` 10, отрицательное — отказ старта),
    `authRateLimit` в `cmd/api/http.go` — `httpkit.RateLimit` на префиксе `/api/auth/` через
    `httpkit.Middlewares`; потоки идентичности в тестах лимит выключают, 429 brute-force сверяется по коду
    `AUTH_ACCOUNT_LOCKED`; `.env.example`, требование в шаблоне `auth.md` § Brute-force, блок в накопителе; RED на
    базе — `TestBoot_AuthRateLimit` (401 вместо 429), GREEN; `make check` → `✓ check clean`
- [x] **S8** — reset-timing: запрос сброса делает одну и ту же работу для любого адреса, письмо — подписчик
  события (`sent_at`, миграция `0006`), строка о гарантии доставки в `domain-mapping.md`; INFRA-HANDOFF
  - Проверка: RED → GREEN тестов `verification`; E8 и B07b → |z| < 3 · Закрывает: AC8 · Зависит от: S7
  - Сделано: запрос — поиск, лимит (ключ — учётная запись, без неё — адрес), событие `.v3` в транзакции для
    любого адреса, `format:"email"` в схеме; `IssuePasswordReset` — подписчик своего события: ссылка под id
    запроса (`UpsertPasswordReset … ON CONFLICT (id)`), письмо, `sent_at` (миграция `0006`), запрос старше
    срока ссылки и учётная запись, которую не пускают, письма не получают; durable
    `verification-password-reset-requested`; контракт-тест своего события; строки в `domain-mapping.md`,
    `event.md`, rule, `architecture.md`; блок в накопителе; RED на базе — `TestRequestPasswordReset_*` (письмо
    в запросе, у неизвестного адреса — ни лимита, ни события; `N/identity/s8-red-request.log`), GREEN;
    `make check` → `✓ check clean`
- [x] **S9** — компромиссы входа: перечисление через регистрацию и окно access-JWT — в `SECURITY.md` и
  шаблоне `auth.md`, комментарии правдивы
  - Проверка: чтение и grep из AC9 · Закрывает: AC9 · Зависит от: S8
  - Сделано: раздел «Принятые компромиссы» в `SECURITY.md`, вопросы продукту в шаблоне `auth.md` § Authn,
    409 — в комментарии таблицы ошибок user; комментарии входа и сброса правдивы после S8; `make check` →
    `✓ check clean`

### Пробелы тестов
- [ ] **S10** — RTSTT-03: `seed-admin` — роль и подтверждённый адрес после первого запуска, отказ без адреса
  или пароля, удалённая учётная запись с тем же адресом
  - Проверка: мутанты SD1, SD5, SD6, SD7 → красные · Закрывает: AC10
- [ ] **S11** — RTSTT-04: `fx.StopTimeout` графа и граница ⅔ бюджета остановки
  - Проверка: мутанты K01, K04 → красные · Закрывает: AC10
- [ ] **S12** — RTSTT-05: триггер `updated_at` держат тесты репозитория user
  - Проверка: мутант MI1 → красный · Закрывает: AC10
- [ ] **S13** — RTSTT-06: вход сверяет `out.User`, схема регистрации — формат адреса, адаптер auth —
  `VerifyPassword`
  - Проверка: мутанты A36, UH7, AD1 → красные юнит-тесты · Закрывает: AC10

### Гейт
- [ ] **S14** — гейт и повтор экспериментов на итоге ветки
  - Проверка: `flock N/gate.lock make check` → `✓ check clean`; E1, E2, E6, E8, B07b, H2 — как в критериях ·
    Закрывает: AC11 · Зависит от: S13

## Решения
- Поиск по адресу обрезает пробелы по краям, а регистрация их отвергает — у guard'а входа и домена один
  субъект: без обрезки попытку с пробелом guard считал бы попыткой на учётную запись, а домен её не видел.
- `seed-admin` проверяет адрес и имя конструктором `user.NewUser` — одно правило сущности без нового экспорта.
- Пустое после обрезки имя сущность отвергает, как скелет `make new-domain`: «как в скелете» — один способ
  проверки строкового поля; `minLength:"1"` схемы пустое имя из пробелов не ловит.
- Блок S5 в накопителе прямо отменяет дочитывание прежним durable из блока fix-reference-code S11: оба едут
  одним релизом, а правка чужого блока — не дело трека; порядок для окружения до `.v2` — в ACTION.
- RCOD-01: состояние пользователя читается до upsert — окно в миллисекунды между чтением и записью, когда
  параллельное подтверждение может закоммититься, остаётся: закрыть его — блокировка строки пользователя в
  подписчике и в подтверждении, то есть смена канона «погашение — первым»; цена окна — лишнее письмо уже
  подтверждённому адресу.
- Лимит `/api/auth/*` — путь-префикс в `cmd/api` поверх `httpkit.RateLimit`, конфиг — в `App` платформы:
  одно значение `httpkit.Middlewares` на граф задаёт приложение; refresh под тем же лимитом — клиент обновляет
  сессию раз в `JWT_ACCESS_TTL`. Дефолт 5 rps и запас 10: при одном слоте Argon2id (~25 хэшей/с) клиент
  занимает не больше пятой части очереди.
- RCOD-04: запрос сброса пишет событие и для адреса без учётной записи (UserID пуст) — одинаковая работа и
  время для любого адреса; адрес в событие не идёт (PII), ссылку подписчик выпускает под id запроса из
  события — ключ-событие, `password_resets` получает только `sent_at`. Payload и смысл события сменились —
  subject `.v3`: новый durable не дочитывает события `.v2`, отправленные синхронно, и миграции не нужен
  backfill. Срок ссылки — час от запроса: повтор старого запроса письма не шлёт.
- Сбой лимитера или записи события в запросе сброса — 500, а не тихий 204: работа запроса одна для любого
  адреса, и сбой учётную запись не выдаёт; `format:"email"` отсекает ключи, которых guard не учтёт.
- Чужие файлы: `internal/platform/config` (ничей в таблице программы) — `App.AuthRateLimit` и проверка в `Load`.
- Чужие файлы: `docs/reference/architecture.md` (трек boundaries, волна 2) — фраза о ключе в § Idempotency и
  пример сигнала `metrics.go` (письма сброса больше не уходят из запроса).
- Чужой файл: rule `domain-architecture` (трек boundaries, волна 2) — фраза о канонической форме ключа
  уникальности в «Вход — проверка поля»: конвенция, которую копирует следующая сущность.

## Handoff
—

## Где мы сейчас
S1–S9 закрыты. Следующее: S10 — тесты `seed-admin` (RTSTT-03).
