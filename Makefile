.PHONY: help test lint build fmt fmt-check check gen gen-check migrate-up migrate-down e2e e2e-smoke new-domain status map search release setup doctor plan-check validate-stack-docs local-deps-up local-deps-down test-int test-fuzz test-bench test-cover cover-check tidy

# Release-вычисления (image-path host/org, lockstep tag) self-contained в
# scripts/release.sh — выводятся там из `git remote get-url origin`, чтобы
# шаблон работал без правки при форке в `<client-org>/<service>`.

# Один словарь для агента: `make <verb>`. Concise-by-default; FORMAT=detailed
# или VERBOSE=1 → полный вывод (test/lint/build — см. A3). Passthrough-аргументы
# идут через A= (status/plan-check) / Q=,P= (search) / NAME= (new-domain) /
# VERSION= (release).

help: ## сгруппированный список команд
	@awk 'BEGIN {FS = ":.*## "} \
		/^##@ / {printf "\n\033[1m%s\033[0m\n", substr($$0, 5); next} \
		/^[a-zA-Z0-9_-]+:.*## / {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

##@ LOOP
# test/lint/build: concise-by-default (scripts/tools/* за make). FORMAT=detailed или
# VERBOSE=1 — полный вывод (у build VERBOSE=1 — сырой go build). PKG= — подмножество
# (default ./...). test.sh сам гейтит -race по CGO_ENABLED. make check эти переменные
# не читает — гейт всегда проверяет всё дерево.
DETAIL = $(if $(or $(VERBOSE),$(filter detailed,$(FORMAT))),--format=detailed)

test: ## Юнит-тесты без Docker (PKG=./x/... FORMAT=detailed)
	@bash scripts/tools/test.sh $(DETAIL) $(PKG)

test-int: ## Юнит + integration (testcontainers, нужен Docker) — шаг make check (PKG= FORMAT=)
	@bash scripts/tools/test.sh --integration $(DETAIL) $(PKG)

lint: ## golangci-lint concise (PKG=./x/... FORMAT=detailed)
	@bash scripts/tools/lint.sh $(DETAIL) $(PKG)

build: ## Сборка бинарника bin/api concise (VERBOSE=1 — сырой go build)
	@if [ -n "$(VERBOSE)" ]; then \
		go build -o ./bin/api ./cmd/api; \
	else \
		out=$$(go build -o ./bin/api ./cmd/api 2>&1) && echo "OK: build bin/api" \
		  || { echo "$$out" | head -20; echo "FAIL: build"; exit 1; }; \
	fi

fmt: ## gofumpt -w .
	gofumpt -w .

fmt-check: ## Проверка форматирования без мутации дерева (gate)
	@command -v gofumpt >/dev/null || { echo "FAIL: gofumpt не найден — make setup"; exit 1; }
	@files=$$(gofumpt -l .) || { echo "FAIL: gofumpt не разобрал дерево (ошибка выше)"; exit 1; }; \
	if [ -n "$$files" ]; then \
		echo "$$files" | sed 's/^/  /'; \
		echo "FAIL: fmt — файлы не отформатированы, запусти make fmt"; exit 1; \
	fi

# Шаги гейта не видят PKG, VERBOSE и FORMAT ни из окружения, ни из командной строки.
GATE = $(MAKE) --no-print-directory PKG= VERBOSE= FORMAT=

check: ## Канонический gate: fmt-check+vet+lint+test-int+build+gen-check+migrations+cover (pre-push)
	@echo "→ make fmt-check"
	@$(GATE) fmt-check
	@echo "→ go vet -tags=integration ./..."
	@go vet -tags=integration ./...
	@echo "→ make lint"
	@$(GATE) lint
	@echo "→ make test-int (юнит + integration)"
	@$(GATE) test-int
	@echo "→ make build"
	@$(GATE) build
	@echo "→ make gen-check"
	@$(GATE) gen-check
	@echo "→ migrations-check"
	@bash scripts/tools/migrations-check.sh
	@echo "→ make cover-check"
	@$(GATE) cover-check
	@echo "✓ check clean"

##@ GEN
gen: ## Регенерация сгенерированного кода: sqlc + mockery
	sqlc generate
	@command -v mockery >/dev/null || { echo "FAIL: mockery не найден — запусти make setup (pinned версия)"; exit 1; }
	mockery

gen-check: ## Проверка, что sqlc/mocks актуальны (для check/CI)
	sqlc diff
	@command -v mockery >/dev/null || { echo "FAIL: mockery не найден — запусти make setup (pinned версия)"; exit 1; }
	mockery
	@git diff --exit-code -- '**/mocks/*.go' || { echo "mocks out of date — run make gen"; exit 1; }
	@untracked=$$(git ls-files --others --exclude-standard -- '**/mocks/*.go'); \
	if [ -n "$$untracked" ]; then \
		echo "$$untracked" | sed 's/^/  /'; \
		echo "FAIL: untracked mocks — интерфейс не в .mockery.yml был, либо мок не закоммичен"; exit 1; \
	fi

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
plan-check: ## Формат планов .claude/plans/ (A=--summary — сводка активных планов)
	@bash scripts/tools/validate-plan.sh $(A)

validate-stack-docs: ## Валидатор stack-артефактов docs/stack (8 kinds, status/frontmatter)
	@bash scripts/tools/validate-stack-doc.sh docs/stack

##@ DEV
local-deps-up: ## Поднять локальные зависимости (postgres/nats/redis) для go run
	@bash scripts/local-deps.sh up

local-deps-down: ## Снести локальные зависимости
	@bash scripts/local-deps.sh down

##@ EXTRA
test-fuzz: ## Короткие fuzz-прогоны (10s на цель; go test -fuzz работает per-пакет)
	@pkgs=$$(grep -rl 'func Fuzz' --include='*_test.go' internal/ | xargs -r -n1 dirname | sort -u); \
	[ -z "$$pkgs" ] && { echo "OK: fuzz-целей нет"; exit 0; }; \
	for p in $$pkgs; do \
		echo "→ fuzz ./$$p"; \
		go test -run='^$$' -fuzz=. -fuzztime=10s "./$$p" || exit 1; \
	done

test-bench: ## Бенчмарки
	go test -run=^$$ -bench=. -benchmem ./internal/domain/...

test-cover: ## Coverage по доменным пакетам (полный отчёт, без gate)
	go test -coverprofile=coverage.out ./internal/domain/... ./internal/platform/...
	go tool cover -func=coverage.out | tail -1

# Gate меряет юнит-тестируемое: domain-core + delivery + platform-примитивы.
# Вне scope: сгенерированное (mocks, sqlc db), integration-канон (infra/postgres),
# fx-обвязка (module/, infra/outbox, platform/events) — их держат integration/e2e/ValidateApp.
# Порог — константа гейта: ни окружение, ни командная строка его не меняют.
override COVER_FLOOR := 75
cover-check: ## Coverage-gate: юнит-профиль ≥ $(COVER_FLOOR)% (шаг make check)
	@go test -coverprofile=coverage.out ./internal/domain/... ./internal/platform/... >/dev/null
	@grep -v -e '/mocks/' -e '/infra/' -e '/module/' -e '/platform/events/' coverage.out > coverage.gate.out
	@total=$$(go tool cover -func=coverage.gate.out | tail -1 | awk '{gsub("%","",$$NF); print $$NF}'); \
	LC_ALL=C awk -v t="$$total" -v f="$(COVER_FLOOR)" 'BEGIN { \
		if (t+0 < f+0) { printf "FAIL: coverage %.1f%% < floor %s%% (scope: domain+delivery+platform без generated/infra/fx-wiring)\n", t, f; exit 1 } \
		printf "OK: coverage %.1f%% >= %s%%\n", t, f }'

tidy: ## go mod tidy
	go mod tidy
