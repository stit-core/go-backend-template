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
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    --mount=type=ssh \
    --mount=type=secret,id=netrc,target=/root/.netrc \
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
