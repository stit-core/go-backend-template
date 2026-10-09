ARG BUILDPLATFORM=linux/amd64
ARG TARGETPLATFORM=linux/amd64

# Базы закреплены digest'ом: тег переезжает молча, образ с тем же тегом — уже другой.
FROM --platform=$BUILDPLATFORM golang:1.26.9-alpine@sha256:cdfd4fe2da6b225d8b40c6b7a105736e548e83ff56d5d8f9394446eeb5eb84e0 AS build
WORKDIR /src

ARG TARGET=api
ARG TARGETPLATFORM
ARG VERSION=dev
ARG GIT_SHA=unknown

# apk кеш переживает между билдами (≈60s → ≈1s).
RUN --mount=type=cache,target=/var/cache/apk,sharing=locked \
    apk add --update-cache git openssh-client ca-certificates

ENV GOPRIVATE=git.stit.tech/*

# go-mod cache (/go/pkg/mod) и go-build cache (/root/.cache/go-build) через
# --mount=type=cache: между билдами api/migrate, между коммитами
# без изменения go.mod — переиспользуются. Эффект: первый билд тот же,
# повторные ~5-15s вместо ~60-90s.
# Приватные модули GOPRIVATE git берёт по SSH ключом ssh-agent хоста (docker buildx build --ssh default):
# сокет агента виден только этому RUN, в слои образа ключ не попадает. Ключ хоста принимается при первом
# соединении (accept-new): чужой сервер под этим именем модуль не подменит — содержимое сверяет go.sum.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    --mount=type=ssh,required=true \
    GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=url.git@git.stit.tech:.insteadOf GIT_CONFIG_VALUE_0=https://git.stit.tech/ \
    GIT_SSH_COMMAND='ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new' \
    go mod download

COPY . ./

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    set -eux; \
    case "$TARGETPLATFORM" in \
      linux/amd64) GOARCH=amd64 ;; \
      linux/arm64) GOARCH=arm64 ;; \
      *) GOARCH=amd64 ;; \
    esac; \
    CGO_ENABLED=0 GOOS=linux GOARCH=$GOARCH \
    go build -trimpath \
      -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${GIT_SHA}" \
      -o /app ./cmd/${TARGET}

# Без shell и пакетного менеджера, CA-сертификаты в базе, процесс — не root.
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
COPY --from=build /app /app
EXPOSE 8080
USER 65532:65532
ENTRYPOINT ["/app"]
