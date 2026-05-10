.PHONY: help tools test test-int test-fuzz test-bench test-cover lint fmt build build-worker tidy migrate-up migrate-down new-domain hooks-install sqlc sqlc-diff mocks mocks-check pre-merge

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

tools: ## install pinned dev tools (gofumpt, golangci-lint, sqlc)
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

mocks: ## generate mocks via mockery v3
	@command -v mockery >/dev/null || go install github.com/vektra/mockery/v3@latest
	mockery

mocks-check: ## verify generated mocks are up to date
	@command -v mockery >/dev/null || go install github.com/vektra/mockery/v3@latest
	mockery
	@git diff --exit-code -- '**/mocks/*.go' || (echo "mocks out of date — run make mocks" && exit 1)

hooks-install: ## install repo git hooks (commit-msg, pre-commit, pre-push)
	@cp scripts/git-hooks/commit-msg .git/hooks/commit-msg
	@cp scripts/git-hooks/pre-push .git/hooks/pre-push
	@cp scripts/git-hooks/pre-commit .git/hooks/pre-commit 2>/dev/null || true
	@chmod +x .git/hooks/commit-msg .git/hooks/pre-push .git/hooks/pre-commit 2>/dev/null || true
	@echo "git hooks installed"

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
