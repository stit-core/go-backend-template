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
ACTION выкатка обычным порядком — образ migrate (`migrate up`) до api; JWT_REFRESH_TTL убрать из compose,
       сроки при нужде — SESSION_*; после выкатки все пользователи входят заново один раз
WHO    проект (compose env) + клиенты API
DECIDE сроки сессий: дефолты 168h и 720h ок?
RISK   api без миграций кита не стартует; прежние refresh-cookie недействительны — refresh отвечает 401,
       вход заново; клиент, который обновляет сессию параллельными запросами, её теряет (повтор токена
       закрывает сессию) — обновлять последовательно (описание refreshToken в openapi.json)
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
DECIDE число прокси перед сервисом, которые дописывают X-Forwarded-For (Traefik — 1, CDN → nginx — 2)
RISK   без HTTP_TRUST_PROXY_HOPS за прокси у всех клиентов один IP: общий bucket лимитера (429 всем) и
       блокировки входа по чужим ошибкам; без паузы балансировщик шлёт запросы в гаснущий инстанс;
       остановка из env сверх бюджета (30s) — сервис не стартует
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
