.PHONY: help test lint build fmt check gen gen-check migrate-up migrate-down e2e e2e-smoke new-domain status map search logs release setup doctor plan-check test-int test-fuzz test-bench test-cover tidy

# Release-вычисления (image-path host/org, lockstep tag) self-contained в
# scripts/release.sh — выводятся там из `git remote get-url origin`, чтобы
# шаблон работал без правки при форке в `<client-org>/<service>`.

# Один словарь для агента: `make <verb>`. Concise-by-default; FORMAT=detailed
# или VERBOSE=1 → полный вывод (test/lint/build — см. A3). Passthrough-аргументы
# идут через A= (status/logs/plan-check) / Q=,P= (search) / NAME= (new-domain) /
# VERSION= (release).

help: ## сгруппированный список команд
	@awk 'BEGIN {FS = ":.*## "} \
		/^##@ / {printf "\n\033[1m%s\033[0m\n", substr($$0, 5); next} \
		/^[a-zA-Z0-9_-]+:.*## / {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

##@ LOOP
# test/lint/build: concise-by-default (scripts/tools/* за make). FORMAT=detailed
# — расширенный разбор фейлов; VERBOSE=1 — сырой инструмент. PKG= — подмножество
# (default ./...). test.sh сам гейтит -race по CGO_ENABLED.
test: ## Юнит-тесты concise (PKG=./x/... FORMAT=detailed VERBOSE=1)
	@if [ -n "$(VERBOSE)" ]; then \
		go test -short $$([ "$$(go env CGO_ENABLED)" = "1" ] && echo -race) $(if $(PKG),$(PKG),./...); \
	else \
		bash scripts/tools/test.sh $(if $(filter detailed,$(FORMAT)),--format=detailed) $(if $(PKG),$(PKG),./...); \
	fi

lint: ## golangci-lint concise (PKG=./x/... FORMAT=detailed VERBOSE=1)
	@if [ -n "$(VERBOSE)" ]; then \
		golangci-lint run $(PKG); \
	else \
		bash scripts/tools/lint.sh $(if $(filter detailed,$(FORMAT)),--format=detailed) $(PKG); \
	fi

build: ## Сборка бинарника bin/api concise (VERBOSE=1 — сырой go build)
	@if [ -n "$(VERBOSE)" ]; then \
		go build -o ./bin/api ./cmd/api; \
	else \
		out=$$(go build -o ./bin/api ./cmd/api 2>&1) && echo "OK: build bin/api" \
		  || { echo "$$out" | head -20; echo "FAIL: build"; exit 1; }; \
	fi

fmt: ## gofumpt -w .
	gofumpt -w .

check: ## Канонический gate: fmt+vet+lint+test+build+gen-check (pre-push)
	@echo "→ make fmt"
	@$(MAKE) fmt
	@echo "→ go vet ./..."
	@go vet ./...
	@echo "→ make lint"
	@$(MAKE) lint
	@echo "→ make test"
	@$(MAKE) test
	@echo "→ make build"
	@$(MAKE) build
	@echo "→ make gen-check"
	@$(MAKE) gen-check
	@echo "✓ check clean"

##@ GEN
gen: ## Регенерация сгенерированного кода: sqlc + mockery
	sqlc generate
	@command -v mockery >/dev/null || go install github.com/vektra/mockery/v3@latest
	mockery

gen-check: ## Проверка, что sqlc/mocks актуальны (для check/CI)
	sqlc diff
	@command -v mockery >/dev/null || go install github.com/vektra/mockery/v3@latest
	mockery
	@git diff --exit-code -- '**/mocks/*.go' || (echo "mocks out of date — run make gen" && exit 1)

##@ DB
migrate-up: ## Накатить миграции (требует PG_DSN; PG_MIGRATIONS_PATH default ./migrations)
	PG_MIGRATIONS_PATH=$${PG_MIGRATIONS_PATH:-./migrations} go run ./cmd/migrate up

migrate-down: ## Откатить последнюю миграцию (требует PG_DSN)
	PG_MIGRATIONS_PATH=$${PG_MIGRATIONS_PATH:-./migrations} go run ./cmd/migrate down

##@ E2E
e2e: ## Типизированный e2e-харнес (test/): up → pytest → down. Docker + ./.env.local
	@bash scripts/tools/e2e.sh --format=detailed

e2e-smoke: ## Только smoke-набор (-m smoke) — быстрый gate
	@bash scripts/tools/e2e.sh --smoke --format=detailed

##@ SCAFFOLD
new-domain: ## new-domain NAME=order — скелет нового домена
	@bash scripts/new-domain.sh $(NAME)

##@ INTEL
status: ## Сводка состояния репо для smartdev (A= "--full")
	@bash scripts/tools/status.sh $(A)

map: ## Список пакетов (A=./internal/...; символы пакета — go doc <pkg>)
	@go list $(if $(A),$(A),./...) 2>/dev/null | sed 's|^|  |'

search: ## Поиск по коду (Q=pattern [P=path]) — rg, fallback grep -rn
	@if command -v rg >/dev/null 2>&1; then \
		rg -n --max-count=80 -- "$(Q)" $(if $(P),$(P),.); \
	else \
		grep -rn --exclude-dir=.git -- "$(Q)" $(if $(P),$(P),.); \
	fi

logs: ## Прод-логи через корп-Loki (A= "query --service api --since 1h")
	@bash scripts/tools/logs.sh $(A)

##@ RELEASE
release: ## release VERSION=vX.Y.Z — confirm→tag→preflight(+doctor)→push lockstep→handoff→STOP
	@bash scripts/release.sh "$(VERSION)"

##@ SETUP
setup: ## Поставить pinned dev-tools + git-хуки (commit-msg, pre-commit, pre-push)
	@bash scripts/install-tools.sh
	@cp scripts/git-hooks/commit-msg .git/hooks/commit-msg
	@cp scripts/git-hooks/pre-push .git/hooks/pre-push
	@cp scripts/git-hooks/pre-commit .git/hooks/pre-commit 2>/dev/null || true
	@chmod +x .git/hooks/commit-msg .git/hooks/pre-push .git/hooks/pre-commit 2>/dev/null || true
	@echo "✓ setup: dev-tools + git hooks"

doctor: ## Fail-fast preflight: auth/env/drift
	@bash scripts/tools/doctor.sh $(A)

##@ META
plan-check: ## Валидатор структуры планов
	@bash scripts/tools/validate-plan.sh $(A)

##@ EXTRA
test-int: ## Только integration (testcontainers); требует docker
	go test -race -tags=integration ./...

test-fuzz: ## Короткие fuzz-прогоны (10s на цель) — для CI smoke
	go test -run=^$$ -fuzz=. -fuzztime=10s ./internal/domain/...

test-bench: ## Бенчмарки
	go test -run=^$$ -bench=. -benchmem ./internal/domain/...

test-cover: ## Coverage по доменным пакетам
	go test -short -coverprofile=coverage.out ./internal/domain/... ./internal/platform/...
	go tool cover -func=coverage.out | tail -1

tidy: ## go mod tidy
	go mod tidy
