#!/usr/bin/env bash
# tools/kits.sh — киты golibinfra с внешней зависимостью, которые собирает cmd/api: по строке на кит.
#
# Зависимость кита поднимают сами smoke-стек test/stack (сервис-профиль с именем кита, test/stack/lib.sh), make
# local-deps-up (scripts/local-deps.sh) и make mutate (пул серверов вида — только у кита графа, scripts/tools/mutate.py):
# сервису с китом в графе их пути шаблона править не нужно. Boot-тест (serviceEnv, cmd/api/boot_test.go) берёт те же
# киты из своего графа — по конструкторам kitConfigs. Кит в сборке cmd/api и транзитивно, через другой кит (oidckit и
# hubkit/issuer берут cachekit), — тоже кит сервиса: лишний контейнер стека безвреден, недостающий — красный смок.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

# KITS — киты с внешней зависимостью: cachekit — Redis, natskit — NATS с JetStream (сервис с событиями), storagekit —
# MinIO (S3). Новый — строкой здесь, сервисом-профилем в test/stack/compose.yml, веткой в scripts/local-deps.sh,
# записью kitDeps в cmd/api/boot_test.go и KIT_SERVERS в scripts/tools/mutate.py.
KITS="cachekit natskit storagekit"
golibinfra=git.stit.tech/stit-core/golibinfra

deps=$(go list -e -deps ./cmd/api)
for kit in $KITS; do
    if grep -qxF "$golibinfra/$kit" <<<"$deps"; then
        echo "$kit"
    fi
done
