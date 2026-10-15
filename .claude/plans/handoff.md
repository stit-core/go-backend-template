# Handoff

=== INFRA HANDOFF · передай инфра-админу ===
WHAT   env APP_ENV — required, без default (dev | staging | prod …); вне dev обязательны JWT_AUDIENCE
       (aud access-токенов — имя сервиса, через запятую — несколько), COOKIE_SECURE не false (по
       умолчанию true), LOG_REDACT_DISABLED и HTTP_DOCS_ENABLED не true (по умолчанию false);
       optional JWT_ISSUER (iss) и JWT_PREVIOUS_SECRET (прежний ключ на время ротации JWT_SECRET)
WHY    fix-reference-code S10, S9 — безопасные дефолты: prod-гард конфигурации (config.Guard)
ACTION compose `environment:` каждого окружения: APP_ENV, вне dev — JWT_AUDIENCE (у каждого сервиса на
       общем JWT_SECRET свой); проверить, что COOKIE_SECURE=false, LOG_REDACT_DISABLED=true и
       HTTP_DOCS_ENABLED=true нигде вне dev не выставлены
WHO    проект (compose env)
DECIDE значение JWT_AUDIENCE
RISK   без APP_ENV, вне dev без JWT_AUDIENCE или с небезопасной настройкой сервис не стартует; вне dev он
       не стартует и с dev-заглушкой писем (verification/infra/logmail) — нужен настоящий Mailer форка
===

=== INFRA HANDOFF · передай инфра-админу ===
WHAT   образы api и migrate — distroless static (без shell, wget, curl и apk), процесс UID:GID
       65532:65532, базы закреплены digest'ом; проверку здоровья api делает сам бинарь:
       `/app healthcheck` — GET /health по HTTP_ADDR контейнера, код выхода 0 — здоров
WHY    fix-reference-code S10 — образы не от root (SEC-08)
ACTION healthcheck сервиса api в compose — `test: ["CMD", "/app", "healthcheck"]` вместо wget, curl
       или CMD-SHELL (интервалы прежние); тома, секреты-файлы и сертификаты, которые монтируются в
       контейнер, — читаемы UID 65532; отладка — без `docker exec … sh` (в образе нет shell)
WHO    проект (compose)
DECIDE —
RISK   прежний healthcheck через wget, curl или shell не найдёт программу: контейнер навсегда
       unhealthy, прокси, который маршрутизирует только healthy-контейнеры, снимает api с
       маршрутизации (404), `compose up --wait` падает; смонтированный файл, читаемый только root, —
       сервис его не прочтёт и не стартует
===

=== INFRA HANDOFF · передай инфра-админу ===
WHAT   subject'ы без email: user.events.registered → user.events.registered.v2,
       verification.events.email_verification_requested и …password_reset_requested → ….v2 (payload —
       ID и время); новый durable verification-user-registered-v2 (стрим user), а прежний
       verification-user-registered этот релиз дочитывает user.events.registered
WHY    fix-reference-code S11 — в логах и событиях только ID
ACTION до выкатки — ничего: события user.events.registered, записанные до неё (строки outbox_user,
       недоставленное прежнему консьюмеру), дочитывает прежний durable; внешних потребителей старых
       subject'ов user и verification перевести на .v2 (адрес — по ID у user); durable
       verification-user-registered не удалять — его удаление придёт блоком релиза, который снимет его
       подписку
WHO    проект + потребители событий
DECIDE —
RISK   внешний потребитель старых subject'ов user и verification после выкатки получит только события,
       записанные до неё; durable verification-user-registered, удалённый раньше, сервис создаст заново
       с начала стрима user — повторные письма подтверждения
===

=== INFRA HANDOFF · передай инфра-админу ===
WHAT   env OTEL_SDK_DISABLED=true — нужен, пока нет коллектора: telemetry включена по умолчанию и без
       OTEL_SERVICE_NAME не стартует; с коллектором — OTEL_SDK_DISABLED=false, OTEL_SERVICE_NAME,
       OTEL_EXPORTER_OTLP_ENDPOINT, при авторизации — секрет OTEL_TOKEN
WHY    fix-reference-code S11 — метрики, трейсы и логи через golibinfra/telemetry
ACTION compose `environment:` каждого окружения: OTEL_SDK_DISABLED=true
WHO    проект (compose env)
DECIDE когда в infra появится коллектор — включить экспорт
RISK   без переменной (и без OTEL_SERVICE_NAME) сервис не стартует
===

=== INFRA HANDOFF · передай инфра-админу ===
WHAT   сессии refresh — authnkit/session (golibinfra v1.3.0): таблицы authn_sessions и
       authn_session_tokens (источник authnkit_session в cmd/migrate, таблица версий
       schema_migrations_authnkit_session); миграция 0005 удаляет refresh_sessions; JWT_REFRESH_TTL не
       читается — optional SESSION_IDLE_TTL (default 168h), SESSION_ABSOLUTE_TTL (720h),
       SESSION_CLEANUP_INTERVAL (1h)
WHY    fix-reference-code S9 — управляемые сессии: ротация, повтор токена закрывает сессию, абсолютный срок
ACTION образ migrate (`migrate up`) и сразу замена api — между ними прежний api отвечает 500 на вход и
       refresh (refresh_sessions уже нет), окно держать коротким; откат на прежний тег — сначала
       `migrate down` образом migrate этого релиза (вернёт пустую refresh_sessions; прежний образ версию
       0005 не знает), затем `make bump`; JWT_REFRESH_TTL убрать из compose, сроки при нужде — SESSION_*;
       после выкатки (и после отката) все пользователи входят заново один раз
WHO    проект (compose env) + клиенты API
DECIDE сроки сессий: дефолты 168h и 720h ок?
RISK   api без миграций кита не стартует; между `migrate up` и заменой api вход и refresh прежнего api —
       500; откат без `migrate down` — прежний api не входит и не обновляет сессии (500); прежние
       refresh-cookie недействительны — refresh отвечает 401, вход заново; клиент, который обновляет
       сессию параллельными запросами, её теряет — обновлять последовательно (описание refreshToken)
===

=== INFRA HANDOFF · передай инфра-админу ===
WHAT   env golibinfra v1.3.0 для деплоя за прокси: HTTP_RATE_LIMIT_TRUST_PROXY не читается —
       HTTP_TRUST_PROXY_HOPS (default 0; за Traefik — 1), X-Real-IP не читается; HTTP_SHUTDOWN_TIMEOUT
       default 10s, новый HTTP_SHUTDOWN_DRAIN_DELAY (default 0s, за балансировщиком — 5s): всю паузу
       /ready отвечает 503; их сумма — не больше 20s, NATS_DRAIN_TIMEOUT — не больше 10s
WHY    fix-reference-code S9 — IP клиента из цепочки httpkit (лимитер, антибрутфорс, аудит), остановка
       без потерянных запросов
ACTION compose api за Traefik: HTTP_TRUST_PROXY_HOPS=1, HTTP_SHUTDOWN_DRAIN_DELAY=5s, убрать
       HTTP_RATE_LIMIT_TRUST_PROXY; stop_grace_period не меньше 35s
WHO    проект (compose env)
DECIDE число прокси перед сервисом, которые дописывают X-Forwarded-For (Traefik — 1, CDN → nginx — 2),
       ровно: порт api — только за ними
RISK   без HTTP_TRUST_PROXY_HOPS за прокси у всех клиентов один IP: общий bucket лимитера (429 всем) и
       блокировки входа по чужим ошибкам; hops больше числа прокси или порт api доступен мимо прокси —
       IP задаёт сам клиент (X-Forwarded-For): обход антибрутфорса и лимитера, чужой IP в аудите; без
       паузы балансировщик шлёт запросы в гаснущий инстанс; остановка сверх бюджета (30s) — нет старта
===

=== INFRA HANDOFF · передай инфра-админу ===
WHAT   поведение golibinfra v1.3.0 без новых обязательных env: OUTBOX_MAX_ATTEMPTS default 25 (0 —
       ошибка старта, -1 — без предела), событие после 25 неудачных публикаций — dead (ERROR
       `outboxkit: event dead`); ретраи консьюмеров JetStream — NAK от NATS_JS_NAK_BACKOFF вдвое до
       NATS_JS_NAK_MAX_BACKOFF (default 1m), NATS_JS_MAX_DELIVER default 21: ≈15 минут, затем DLQ
WHY    fix-reference-code S9 — bump golibinfra v1.3.0
ACTION убрать OUTBOX_MAX_ATTEMPTS=0 из compose, если задан; алерты на ERROR `outboxkit: event dead` и
       `jetstream message dead`
WHO    проект (compose env, алерты)
DECIDE предел попыток outbox: 25 ок?
RISK   с OUTBOX_MAX_ATTEMPTS=0 сервис не стартует; без алерта выпавшее из доставки событие не заметят
===

=== INFRA HANDOFF · передай инфра-админу ===
WHAT   стрим каждого домена-публикатора — условие старта: NATS_JETSTREAM_ENABLED=true и
       NATS_JS_STREAM_NAMES со всеми публикаторами — user и verification (natskit создаёт стрим
       `<имя>` с subject'ами `<имя>.>`)
WHY    fix-reference-code S9 — outbox домена объявляет свои subject'ы (outboxkit.WithSubjects): без стрима
       его строки копились бы без PubAck
ACTION compose api: NATS_JETSTREAM_ENABLED=true, NATS_JS_STREAM_NAMES=user,verification (или стримы на
       стороне NATS с теми же subject'ами)
WHO    проект (compose env, NATS)
DECIDE —
RISK   без стрима verification или user сервис не стартует
===

=== INFRA HANDOFF · передай инфра-админу ===
WHAT   env golibinfra v1.4.0. Новые optional: HTTP_REQUEST_TIMEOUT (default 25s), HTTP_TRUST_TRACE_CONTEXT
       (false), BRUTEFORCE_SUBJECT_MAX_FAILURES (20) и BRUTEFORCE_SUBJECT_LOCK_DURATION (1m) — блокировка
       учётной записи со всех адресов, PG_MIGRATE_LOCK_TIMEOUT (5s, образ migrate),
       OTEL_ALLOW_INSECURE_TOKEN (false). `KEY=` (задана пустой) — как незаданная: default, у required —
       отказ старта (APP_ENV=); пустое значение и пустой список — `-`. Старт падает, если:
       HTTP_REQUEST_TIMEOUT не меньше HTTP_WRITE_TIMEOUT (30s) или HTTP_IDEMPOTENCY_RESERVE_TTL (60s);
       BRUTEFORCE_CLEANUP_RETENTION (24h) меньше BRUTEFORCE_LOCK_DURATION (15m) или
       BRUTEFORCE_SUBJECT_LOCK_DURATION; OUTBOX_TICK_TIMEOUT (30s) не меньше PG_IDLE_IN_TX_SESSION_TIMEOUT
       (60s; 0 — без лимита, сверки нет); в NATS_URL или OTEL_EXPORTER_OTLP_ENDPOINT неэкранированный `@`
       после хоста; OTEL_TOKEN уходит на http:// не на loopback (и при OTEL_SDK_DISABLED=true).
       HTTP_CORS_HEADERS — default с Idempotency-Key. Киты сняли STORAGE_S3_USE_SSL — сервис её не читал.
       Сборка: toolchain go1.26.9 в go.mod, образ сборки golang:1.26.9-alpine по digest — действий нет
WHY    repair-harness S7 — bump golibinfra v1.4.0: дедлайн запроса, бюджет попыток учётной записи, ридер
       ENV китов
ACTION compose `environment:` каждого окружения сверить с условиями: required — со значением (не `KEY=`),
       пустое значение — `-`; свои HTTP_WRITE_TIMEOUT, HTTP_IDEMPOTENCY_RESERVE_TTL, BRUTEFORCE_* и
       PG_IDLE_IN_TX_SESSION_TIMEOUT — по условиям выше; в URL `/` пароля — `%2F`, `@` пути — `%40`; свой
       HTTP_CORS_HEADERS — дописать Idempotency-Key; коллектор с токеном — https или
       OTEL_ALLOW_INSECURE_TOKEN=true в доверенной сети
WHO    проект (compose env)
DECIDE дефолты ок: 25s на запрос, 20 попыток и 1m блокировки учётной записи?
RISK   с нарушенным условием сервис не стартует; запрос дольше HTTP_REQUEST_TIMEOUT получает 503
       REQUEST_TIMEOUT — долгие операции клиента оборвутся
===

=== INFRA HANDOFF · передай инфра-админу ===
WHAT   миграция outbox кита 0003 (источники outbox_user и outbox_verification в cmd/migrate): ALTER TABLE
       … ADD COLUMN IF NOT EXISTS trace_context JSONB, down — no-op; api сверяет схему kit-таблиц на
       старте (pgkit.ErrSchema)
WHY    repair-harness S7 — golibinfra v1.4.0: трасса запроса продолжается через outbox
ACTION образ migrate (`migrate up`) до замены api. Блокировку таблицы миграция ждёт не дольше
       PG_MIGRATE_LOCK_TIMEOUT (5s): отказ оставляет источник на версии 3 с флагом dirty и без колонки
       (`migrate version` называет источник). Ремонт — в таблице версий этого источника вернуть версию 2
       и снять флаг: `UPDATE schema_migrations_outbox_<домен> SET version = 2, dirty = false`, затем
       `migrate up` в окно без долгих транзакций по outbox. Откат на прежний тег: колонка остаётся,
       прежний api с ней работает, а migrate прежнего тега на версии 3 падает (no migration found for
       version 3) — до `make bump` вернуть версию 2 в обеих таблицах версий (`UPDATE
       schema_migrations_outbox_user SET version = 2`, то же — schema_migrations_outbox_verification;
       выкатка вперёд применит 0003 заново) или поднять прежний api без прогона его migrate
WHO    проект (порядок выкатки)
DECIDE —
RISK   api этого релиза на схеме без колонки не стартует; снять только флаг dirty — `migrate up` пройдёт
       без колонки, api стартует и отвечает 500 на операции с событием (регистрация)
===

=== INFRA HANDOFF · передай инфра-админу ===
WHAT   сигналы golibinfra v1.4.0: ключ ошибки в логах сервиса и китов — `error` вместо `err`; метрики
       outbox — без label subject; bruteforce_locks_total — с label scope; новый ответ 503
       REQUEST_TIMEOUT; спан otelpgx — `query <имя запроса>` (текст SQL — в db.query.text); спан
       публикации natskit — `natskit.publish` без subject'а в имени (он — в messaging.destination.name);
       429, preflight и 405 — под маршрутом пути, unmatched — только путь без маршрута; новое действие
       аудита `logout` (audit_log: actor_id и resource_id — владелец закрытой сессии)
WHY    repair-harness S7 — bump golibinfra v1.4.0
ACTION запросы логов, панели и алерты по полю err, label subject метрик outbox и именам спанов otelpgx
       и natskit — переписать; завести алерт на долю 503 REQUEST_TIMEOUT
WHO    проект (наблюдаемость)
DECIDE —
RISK   панели и алерты по прежним полям молча пустеют
===

=== INFRA HANDOFF · передай инфра-админу ===
WHAT   API: предел пароля — 1024 байта (хэшер golibinfra v1.4.0). Регистрация, смена и сброс пароля
       отвечают 422 USER_PASSWORD_TOO_LONG, AUTH_PASSWORD_TOO_LONG, VERIFICATION_PASSWORD_TOO_LONG; поля
       пароля в openapi.json — maxLength 1024 (длиннее — 422 VALIDATION_FAILED, и на входе); вход с
       паролем длиннее предела — 401
WHY    repair-harness S7 — пароль длиннее предела хэшера — ошибка ввода, а не 500
ACTION клиентам API — новые коды 422 и maxLength полей пароля; учётной записи с паролем длиннее 1024
       байт (заведён до релиза) — сброс пароля по почте
WHO    клиенты API + проект (поддержка)
DECIDE —
RISK   пользователь с паролем длиннее 1024 байт после выкатки не входит, пока не сбросит пароль
===

=== INFRA HANDOFF · передай инфра-админу ===
WHAT   сигналы golibinfra v1.5.0, без новых env, секретов и миграций: сервис с OTEL_SDK_DISABLED=true не
       шлёт соседям traceparent с sampled=0 (трассу они сэмплируют сами) и пишет trace_id в лог только у
       работы с доверенным входящим trace-context (сообщение шины; HTTP — при
       HTTP_TRUST_TRACE_CONTEXT=true); outbox_lag_seconds и outbox_pending_count — снимок прошлого сбора
       метрик (отстают на OTEL_METRICS_INTERVAL), при занятом пуле БД этих серий нет
       (outbox_backlog_errors_total), остальные метрики идут; новые WARN остановки — `timerkit: jobs are
       still running after the stop budget` и `natskit: subscription drain timeout, handlers cancelled`
WHY    repair-harness S10 — bump golibinfra v1.5.0
ACTION запросы логов по trace_id у сервиса с выключенной телеметрией — на request_id; алерт «серий
       outbox нет» — с порогом не короче двух интервалов экспорта; алерты на WARN остановки — по желанию
WHO    проект (панели и алерты)
DECIDE —
RISK   остановка: ждущий хук кита берёт ⅔ остатка бюджета — после HTTP на все 20s подписке NATS
       достаётся до 6,7s, незавершённым обработчикам отменяется ctx, код выхода 0; stop_grace_period
       прежний (35s)
===

=== INFRA HANDOFF · передай инфра-админу ===
WHAT   путь образов — <host>/<org>/<репо>{,-migrate} из origin репо сервиса, имя репо — имя сервиса для
       хаба: тег — <РЕПО>_TAG так, как его строит make bump (верхний регистр, `-` → `_`), выкатка —
       make bump SVC=<репо>; у репо backend — те же backend, backend-migrate и BACKEND_TAG; шаблон
       stit-core/go-backend-template выпускает stit-core/go-backend-template{,-migrate} и
       GO_BACKEND_TEMPLATE_TAG; имя репо, из которого не выходит имя переменной (1c-gateway,
       my.service), релиз отвергает до тега
WHY    fix-fork-lifecycle S4 — RLIF-04: путь <org>/backend был зашит, и второй сервис орги перезаписывал
       образ первого
ACTION сервис из шаблона, чей репо назван не backend: image в compose — ${REGISTRY_HOST}/${PROJECT_NAME}/<репо>
       (migrate — <репо>-migrate), тег — <РЕПО>_TAG в .env; путь и имя тега — в handoff его релиза
WHO    проект (compose, .env)
DECIDE —
RISK   compose с путём <org>/backend для такого сервиса тянет образ другого сервиса или прежний: релизы
       этого сервиса лежат под <org>/<репо>
===

=== INFRA HANDOFF · передай инфра-админу ===
WHAT   API: регистрация отвечает 422 USER_INVALID_EMAIL на адрес с пробелом или табом по краям и в форме с
       именем, угловыми скобками или комментарием; вход и запрос сброса ищут адрес без пробелов по краям;
       cmd/seed-admin с таким SEED_ADMIN_EMAIL — код 1
WHY    fix-identity-edges S1 — один ящик — одна учётная запись (RCOD-02, RSECT-04)
ACTION клиентам API — слать адрес без пробелов по краям; в БД каждого окружения найти учётные записи, заведённые
       с ними: `SELECT id FROM users WHERE email ~ '^[[:space:]]|[[:space:]]$' AND status <> 'deleted'`
WHO    клиенты API + проект (данные)
DECIDE судьба найденных учётных записей: удалить или исправить адрес (если ящик ещё свободен)
RISK   найденная учётная запись после выкатки не входит: поиск по адресу без пробелов её не находит
===

=== INFRA HANDOFF · передай инфра-админу ===
WHAT   API: имя пользователя — до 200 символов (maxLength 200 у name регистрации и правки в openapi.json:
       длиннее — 422 VALIDATION_FAILED), пустое после обрезки пробелов — 422 USER_INVALID_NAME;
       cmd/seed-admin с таким SEED_ADMIN_NAME — код 1
WHY    fix-identity-edges S2 — предел свободного текста: имя в мегабайт раздувало таблицу и ответ списка
       (RSECT-02, RCOD-11)
ACTION клиентам API — maxLength поля name; учётным записям с именем длиннее 200 символов (заведены до
       выкатки) правка имени — только новым именем по правилам
WHO    клиенты API
DECIDE предел 200 символов ок?
RISK   клиент, который шлёт длинное или пустое имя, получает 422
===

=== INFRA HANDOFF · передай инфра-админу ===
WHAT   подписки на user.events.registered больше нет: регистрацию читает только durable
       verification-user-registered-v2 (user.events.registered.v2); прежний durable verification-user-registered
       (стрим user) сервис не создаёт и не читает — дочитывания из блока fix-reference-code S11 нет
WHY    fix-identity-edges S5 — переходная подписка снята (RCOD-13)
ACTION окружение, где работает релиз до .v2: до выкатки дождаться, пока прежний api опубликует строки outbox_user с
       subject user.events.registered и durable verification-user-registered их дочитает (`nats consumer info
       user verification-user-registered` → Unprocessed 0); после выкатки — `nats consumer rm user
       verification-user-registered`
WHO    проект (NATS)
DECIDE —
RISK   событие user.events.registered, не дочитанное до выкатки, письма подтверждения адреса не получит;
       неудалённый durable остаётся в стриме user без читателя
===

=== INFRA HANDOFF · передай инфра-админу ===
WHAT   лимит клиента на /api/auth/* поверх общего: optional HTTP_AUTH_RATE_LIMIT_RPS (default 5) и
       HTTP_AUTH_RATE_LIMIT_BURST (default 10), 0 — выключен, отрицательное — отказ старта; сверх лимита —
       429 RATE_LIMITED; клиент — IP по HTTP_TRUST_PROXY_HOPS, как у общего лимитера
WHY    fix-identity-edges S7 — поток входов с одного адреса в пределах общего лимита занимал общую очередь
       Argon2id процесса: вход остальных ждал секунды (RSECT-01)
ACTION compose api за прокси — проверить HTTP_TRUST_PROXY_HOPS (число прокси, которые дописывают
       X-Forwarded-For); значения лимита — по ёмкости хэша (слотов GOMAXPROCS/ARGON2_THREADS) и числу клиентов
       за одним NAT; поток с многих адресов — лимит на краю (прокси, WAF)
WHO    проект (compose env, край)
DECIDE значения лимита: 5 rps и запас 10 на клиента ок? клиенты за общим NAT?
RISK   без HTTP_TRUST_PROXY_HOPS за прокси у всех клиентов один bucket: вход и обновление сессии сверх 5 rps на
       весь сервис — 429; клиенты за одним NAT делят его так же
===

=== INFRA HANDOFF · передай инфра-админу ===
WHAT   письмо сброса пароля шлёт подписчик события verification.events.password_reset_requested.v3 (durable
       verification-password-reset-requested, стрим verification): запрос отвечает 204 до письма; событие —
       на каждый запрос (RequestID, UserID, At; UserID пуст у адреса без учётной записи), .v2 не публикуется;
       миграция 0006 — password_resets.sent_at (ADD COLUMN, без backfill); POST /api/auth/password-reset/request —
       адрес по схеме (format email: не адрес — 422 VALIDATION_FAILED), сбой поиска, лимитера или записи
       события — 500
WHY    fix-identity-edges S8 — время ответа на запрос сброса выдавало учётную запись, сбой провайдера терял
       письмо (RCOD-04, SEC-06)
ACTION образ migrate (`migrate up`) до замены api; внешних потребителей verification.events.password_reset_requested.v2
       перевести на .v3 (UserID может быть пуст — запрос на адрес без учётной записи)
WHO    проект + потребители событий
DECIDE —
RISK   api этого релиза без колонки sent_at — подписчик сброса падает на записи, письмо ждёт повтора доставки;
       сбой Mailer'а — повтор до NATS_JS_MAX_DELIVER, затем DLQ: письмо уходит позже, а не теряется молча;
       откат на релиз до .v3 теряет запросы сброса, которые подписчик не дочитал (прежний api .v3 не читает) —
       их авторы запрашивают сброс снова; событие пишется на каждый запрос, и на адрес без учётной записи, —
       стрим verification растёт с анонимным трафиком: его держат лимиты /api/auth/* и NATS_JS_MAX_AGE
===

=== INFRA HANDOFF · передай инфра-админу ===
WHAT   вне APP_ENV=dev сервис не стартует, если JWT_SECRET или JWT_PREVIOUS_SECRET равен ключу из примеров
       окружения репо (`.env.example`, `test/stack/compose.yml`)
WHY    polish-code-security-adapters S1 — NSEC-02: ключом из примеров подписывается токен любой роли
ACTION проверить секреты окружений вне dev: JWT_SECRET и JWT_PREVIOUS_SECRET — свои, не из примеров репо
WHO    проект (секреты compose)
DECIDE —
RISK   окружение с ключом из примеров не стартует: ErrUnsafe «JWT_SECRET from an example env» в логе старта
===

=== INFRA HANDOFF · передай инфра-админу ===
WHAT   миграция 0007 — password_resets.user_version (ADD COLUMN, без backfill): ссылка сброса меняет пароль при
       версии учётной записи на момент письма; ссылка, выданная до выкатки (версии нет), пароль не меняет —
       400 VERIFICATION_INVALID_RESET_TOKEN, её автор запрашивает сброс снова (ссылка живёт час)
WHY    polish-code-security-adapters S3 — ссылка сброса переживала смену пароля (RCOD-18), два подтверждения
       ссылок одного пользователя ждали друг друга по кругу (RCOD-10)
ACTION образ migrate (`migrate up`) до замены api
WHO    проект
DECIDE —
RISK   api этого релиза без колонки user_version — выпуск и подтверждение ссылок сброса падают (500), письма
       ждут повтора доставки; откат на прежний релиз — `migrate down` не нужен (прежний api колонку не читает)
===
