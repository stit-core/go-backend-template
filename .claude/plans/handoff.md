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
