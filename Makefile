.PHONY: help tools test test-int test-fuzz test-bench test-cover lint fmt build build-worker tidy migrate-up migrate-down new-domain hooks-install sqlc sqlc-diff mocks mocks-check pre-merge release-check release image-api image-migrate image-worker

# Release-вычисления. Image-path выводится из `git remote get-url origin`
# (host+org), repo-name всегда `backend` — совпадает с infra-template
# (compose/backend.yml ожидает `<host>/<org>/backend:<tag>`). Берём из
# remote, а не из go.mod module path, чтобы шаблон работал без правки
# Makefile при копировании в `<client-org>/<service>`.
RELEASE_BASE := $(shell git remote get-url origin 2>/dev/null | sed -E 's#^(git@|https?://)([^:/]+)[:/]([^/]+)/.*$$#\2/\3#')
IMAGE_BASE := $(RELEASE_BASE)/backend
TAG ?= $(shell git describe --tags --exact-match HEAD 2>/dev/null)
GIT_SHA := $(shell git rev-parse --short HEAD 2>/dev/null)

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

tools: ## Поставить pinned версии dev-инструментов (gofumpt, golangci-lint, sqlc)
	@bash scripts/install-tools.sh

test: ## Юнит-тесты (без integration build-tag). -race гонится только при CGO_ENABLED=1.
	@if [ "$$(go env CGO_ENABLED)" = "1" ]; then \
		go test -short -race ./...; \
	else \
		echo "(CGO_ENABLED=0, race detector unavailable — see ci.yml/pre-push fallback)"; \
		go test -short ./...; \
	fi

test-int: ## Только integration (testcontainers); требует docker
	go test -race -tags=integration ./...

test-fuzz: ## Короткие fuzz-прогоны (10s на цель) — для CI smoke
	go test -run=^$$ -fuzz=. -fuzztime=10s ./internal/domain/...

test-bench: ## Бенчмарки
	go test -run=^$$ -bench=. -benchmem ./internal/domain/...

test-cover: ## Coverage по доменным пакетам
	go test -short -coverprofile=coverage.out ./internal/domain/... ./internal/platform/...
	go tool cover -func=coverage.out | tail -1

lint: ## golangci-lint
	golangci-lint run

fmt: ## gofumpt
	gofumpt -w .

build: ## Локальная сборка бинарника (для CI image — см. Dockerfile)
	go build -o ./bin/api ./cmd/api

build-worker: ## Сборка бинарника worker (фоновые подписчики, jobs, outbox drain)
	go build -o ./bin/worker ./cmd/worker

new-domain: ## new-domain NAME=order — создать скелет нового домена в internal/domain/<NAME>/
	@bash scripts/new-domain.sh $(NAME)

tidy: ## go mod tidy
	go mod tidy

sqlc: ## Сгенерировать pgx-код из internal/domain/<domain>/infra/postgres/queries/
	sqlc generate

sqlc-diff: ## Проверить, что sqlc generate чист (для CI)
	sqlc diff

migrate-up: ## Накатить миграции (требует PG_DSN; PG_MIGRATIONS_PATH default ./migrations)
	PG_MIGRATIONS_PATH=$${PG_MIGRATIONS_PATH:-./migrations} go run ./cmd/migrate up

migrate-down: ## Откатить последнюю миграцию (требует PG_DSN)
	PG_MIGRATIONS_PATH=$${PG_MIGRATIONS_PATH:-./migrations} go run ./cmd/migrate down

mocks: ## Сгенерировать моки через mockery v3
	@command -v mockery >/dev/null || go install github.com/vektra/mockery/v3@latest
	mockery

mocks-check: ## Проверить, что сгенерированные моки актуальны
	@command -v mockery >/dev/null || go install github.com/vektra/mockery/v3@latest
	mockery
	@git diff --exit-code -- '**/mocks/*.go' || (echo "mocks out of date — run make mocks" && exit 1)

hooks-install: ## Установить git-хуки репо (commit-msg, pre-commit, pre-push)
	@cp scripts/git-hooks/commit-msg .git/hooks/commit-msg
	@cp scripts/git-hooks/pre-push .git/hooks/pre-push
	@cp scripts/git-hooks/pre-commit .git/hooks/pre-commit 2>/dev/null || true
	@chmod +x .git/hooks/commit-msg .git/hooks/pre-push .git/hooks/pre-commit 2>/dev/null || true
	@echo "git hooks installed"

release-check: ## Pre-flight для release: ветка=main, чистое дерево, annotated tag vX.Y.Z на HEAD, buildx
	@bash scripts/release-preflight.sh "$(TAG)"

image-api: release-check ## Собрать и запушить api-image (require: annotated tag на HEAD, docker login git.stit.tech)
	docker buildx build --platform linux/amd64 --push \
	  --build-arg TARGET=api \
	  --build-arg VERSION=$(TAG) \
	  --build-arg GIT_SHA=$(GIT_SHA) \
	  -t $(IMAGE_BASE):$(TAG) .

image-migrate: release-check ## Собрать и запушить migrate-image (Dockerfile.migrate, в image вложены SQL-файлы)
	docker buildx build --platform linux/amd64 --push -f Dockerfile.migrate \
	  --build-arg VERSION=$(TAG) \
	  --build-arg GIT_SHA=$(GIT_SHA) \
	  -t $(IMAGE_BASE)-migrate:$(TAG) .

image-worker: release-check ## Собрать и запушить worker-image (silent skip если cmd/worker отсутствует)
	@if [ ! -d cmd/worker ]; then echo "image-worker: cmd/worker не найден — skip"; exit 0; fi; \
	docker buildx build --platform linux/amd64 --push \
	  --build-arg TARGET=worker \
	  --build-arg VERSION=$(TAG) \
	  --build-arg GIT_SHA=$(GIT_SHA) \
	  -t $(IMAGE_BASE)-worker:$(TAG) .

release: release-check image-api image-migrate image-worker ## Полный релиз: pushed lockstep tag для api+migrate+worker
	@echo "✓ released $(IMAGE_BASE){,-migrate,-worker}:$(TAG)"
	@echo "→ next: на корп-стороне vault kv patch kv/projects/<p>/<env>/meta backend_tag=$(TAG)"
	@echo "→ then: на VPS bash scripts/deploy.sh (или make ship в infra-репо)"

pre-merge: ## канонический pre-push/pre-merge gate: fmt + vet + lint + test + build + sqlc-diff + mocks-check
	@echo "→ make fmt"
	@$(MAKE) fmt
	@echo "→ go vet ./..."
	@go vet ./...
	@echo "→ make lint"
	@$(MAKE) lint
	@echo "→ make test"
	@$(MAKE) test
	@echo "→ make build && make build-worker"
	@$(MAKE) build
	@$(MAKE) build-worker
	@echo "→ make sqlc-diff"
	@$(MAKE) sqlc-diff
	@echo "→ make mocks-check"
	@$(MAKE) mocks-check
	@echo "✓ pre-merge clean"
