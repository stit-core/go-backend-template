# Handoff

=== INFRA HANDOFF · передай инфра-админу ===
WHAT   env APP_ENV — required, без default (dev | staging | prod …); вне dev обязательны
       HTTP_DOCS_ENABLED=false (golibinfra v1.2.0 включает docs по умолчанию), COOKIE_SECURE
       не false (по умолчанию true), LOG_REDACT_DISABLED не true
WHY    fix-reference-code S10 — безопасные дефолты: prod-гард конфигурации (config.Guard)
ACTION compose `environment:` каждого окружения: APP_ENV, HTTP_DOCS_ENABLED=false; проверить, что
       COOKIE_SECURE=false и LOG_REDACT_DISABLED=true нигде вне dev не выставлены
WHO    проект (compose env)
DECIDE —
RISK   без APP_ENV или с небезопасной настройкой вне dev сервис не стартует; вне dev он не стартует
       и с dev-заглушкой писем (verification/infra/logmail) — нужен настоящий Mailer форка
===

=== INFRA HANDOFF · передай инфра-админу ===
WHAT   образы api и migrate — distroless static (без shell и apk), процесс UID:GID 65532:65532,
       базы закреплены digest'ом
WHY    fix-reference-code S10 — образы не от root (SEC-08)
ACTION тома, секреты-файлы и сертификаты, которые монтируются в контейнер, — читаемы UID 65532;
       отладка — без `docker exec … sh` (в образе нет shell)
WHO    проект (compose)
DECIDE —
RISK   смонтированный файл, читаемый только root, — сервис его не прочтёт и не стартует
===

=== INFRA HANDOFF · передай инфра-админу ===
WHAT   subject'ы без email: user.events.registered → user.events.registered.v2,
       verification.events.email_verification_requested и …password_reset_requested → ….v2 (payload —
       ID и время); durable verification-user-registered → verification-user-registered-v2 (стрим user)
WHY    fix-reference-code S11 — в логах и событиях только ID
ACTION до выкатки — дренировать outbox_user и консьюмер verification-user-registered (v1 новая версия
       не читает); после — удалить durable verification-user-registered; внешних потребителей старых
       subject'ов перевести на .v2 (адрес — по ID у user)
WHO    проект + потребители событий
DECIDE —
RISK   событие v1, не вычитанное до выкатки, не даст письма подтверждения email
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
