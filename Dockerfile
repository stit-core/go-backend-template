ARG BUILDPLATFORM=linux/amd64
ARG TARGETPLATFORM=linux/amd64

FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
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
# --mount=type=cache: между билдами api/worker/migrate, между коммитами
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

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=build /app /app
EXPOSE 8080
ENTRYPOINT ["/app"]
