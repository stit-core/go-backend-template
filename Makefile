.PHONY: help test lint build fmt fmt-check check gen gen-check migrate-up migrate-down e2e e2e-smoke new-domain status map search release setup doctor template-update plan-check validate-stack-docs local-deps-up local-deps-down test-int test-fuzz test-bench cover-check tidy

# Release-вычисления (image-path host/org, lockstep tag) self-contained в
# scripts/release.sh — выводятся там из `git remote get-url origin`, чтобы
# шаблон работал без правки при форке в `<client-org>/<service>`.

# Один словарь для агента: `make <verb>`. Concise-by-default; FORMAT=detailed
# или VERBOSE=1 → полный вывод (test/lint/build — см. A3). Passthrough-аргументы
# идут через A= (status/plan-check) / Q=,P= (search) / NAME= (new-domain) /
# VERSION= (release) / TO=,FROM= (template-update).

help: ## сгруппированный список команд
	@awk 'BEGIN {FS = ":.*## "} \
		/^##@ / {printf "\n\033[1m%s\033[0m\n", substr($$0, 5); next} \
		/^[a-zA-Z0-9_-]+:.*## / {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

##@ LOOP
# test/lint/build: concise-by-default (scripts/tools/* за make). FORMAT=detailed или
# VERBOSE=1 — полный вывод (у build VERBOSE=1 — сырой go build). PKG= — подмножество
# (default ./...). Без cgo test.sh идёт без -race и пишет WARN. make check эти переменные
# не читает — гейт всегда проверяет всё дерево.
DETAIL = $(if $(or $(VERBOSE),$(filter detailed,$(FORMAT))),--format=detailed)

test: ## Юнит-тесты без Docker (PKG=./x/... FORMAT=detailed)
	@bash scripts/tools/test.sh $(DETAIL) $(PKG)

test-int: ## Юнит + integration (testcontainers, нужен Docker) — шаг make check; по всему дереву — и профиль покрытия (PKG= FORMAT=)
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

# make check не запускается там, где сказал бы «clean», не проверив всё: -i глушит ошибки шагов,
# -n/-q/-t их не выполняют, подмена переменных гейта меняет шаги, GOFLAGS (из env или
# go env -w) — go test и vet, API_TEST_MAIN_ARGS — перезапуск тестов cmd/api в main.
# override — чтобы guard не выключался своей же переменной.
ifneq (,$(filter check,$(MAKECMDGOALS)))
override gate_bad := $(foreach f,i n q t,$(if $(findstring $f,$(firstword -$(MAKEFLAGS))),-$f)) \
	$(foreach v,MAKE SHELL .SHELLFLAGS GATE DETAIL API_TEST_MAIN_ARGS,$(if $(filter command% environment%,$(origin $v)),$v=…)) \
	$(if $(shell go env GOFLAGS),GOFLAGS=$(shell go env GOFLAGS))
ifneq (,$(strip $(gate_bad)))
$(error make check не запускается с $(strip $(gate_bad)) — запусти просто make check)
endif
endif

check: ## Канонический gate: fmt-check+vet+lint+migrations+test-int+build+gen-check+cover (pre-push)
	@echo "→ make fmt-check"
	@$(GATE) fmt-check
	@echo "→ go vet -tags=integration ./..."
	@go vet -tags=integration ./...
	@echo "→ make lint"
	@$(GATE) lint
	@echo "→ migrations-check"
	@bash scripts/tools/migrations-check.sh
	@echo "→ make test-int (юнит + integration, профиль покрытия)"
	@$(GATE) test-int
	@echo "→ make build"
	@$(GATE) build
	@echo "→ make gen-check"
	@$(GATE) gen-check
	@echo "→ make cover-check (профиль test-int)"
	@bash scripts/tools/cover.sh coverage.out $(COVER_FLOOR)
	@echo "✓ check clean"

##@ GEN
gen: ## Регенерация сгенерированного: sqlc, mockery, openapi.json (без запуска сервиса)
	sqlc generate
	@command -v mockery >/dev/null || { echo "FAIL: mockery не найден — запусти make setup (pinned версия)"; exit 1; }
	mockery
	go test ./cmd/api -run '^TestOpenAPISpec$$' -count=1 -args -update

gen-check: ## Проверка, что sqlc, моки и openapi.json актуальны (шаг make check)
	@sqlc diff || { echo "FAIL: sqlc diff — вывод выше (дрейф сгенерированного чинит make gen)"; exit 1; }
	@out=$$(go test ./cmd/api -run '^TestOpenAPISpec$$' -count=1 2>&1) || \
		{ echo "$$out"; echo "FAIL: TestOpenAPISpec — вывод выше (расхождение с openapi.json чинит make gen)"; exit 1; }
	@command -v mockery >/dev/null || { echo "FAIL: mockery не найден — запусти make setup (pinned версия)"; exit 1; }
	@mockery
	@git diff --exit-code -- '**/mocks/*.go' || { echo "FAIL: моки устарели — запусти make gen"; exit 1; }
	@untracked=$$(git ls-files --others --exclude-standard -- '**/mocks/*.go'); \
	if [ -n "$$untracked" ]; then \
		echo "$$untracked" | sed 's/^/  /'; \
		echo "FAIL: untracked mocks — интерфейс не в .mockery.yml был, либо мок не закоммичен"; exit 1; \
	fi
	@echo "OK: sqlc, моки и openapi.json совпадают с кодом"

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
release: ## release VERSION=vX.Y.Z — confirm→check→e2e-smoke→tag→preflight(+doctor)→push lockstep→handoff→STOP (DRY_RUN=1 — только handoff)
	@bash scripts/release.sh "$(VERSION)"

##@ SETUP
setup: ## Поставить pinned dev-tools и подключить git-хуки scripts/git-hooks (core.hooksPath)
	@bash scripts/install-tools.sh
	@git config core.hooksPath scripts/git-hooks
	@echo "✓ setup: dev-tools + git hooks (core.hooksPath=scripts/git-hooks)"

doctor: ## Fail-fast preflight: auth/env/drift
	@bash scripts/tools/doctor.sh $(A)

template-update: ## Обвязка шаблона в форк: новые теги шаблона; TO=template/vX.Y.Z — перенос (FROM= — база)
	@bash scripts/tools/template-update.sh "$(TO)" "$(FROM)"

##@ META
plan-check: ## Формат планов .claude/plans/ (A=--summary — сводка активных планов)
	@bash scripts/tools/validate-plan.sh $(A)

validate-stack-docs: ## Валидатор stack-артефактов docs/stack (8 kinds, status/frontmatter)
	@bash scripts/tools/validate-stack-doc.sh docs/stack

##@ DEV
local-deps-up: ## Поднять локальные зависимости (postgres/nats) для go run
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

# Покрытие — профиль одного прогона юнит + integration (make test-int) по internal/ и pkg/ без
# сгенерированного и internal/testkit: доля statement'ов ≥ порога и ни одной функции без исполненного
# statement'а (scripts/tools/cover.sh); make check проверяет профиль своего test-int, не гоняя тесты
# второй раз. Порог — константа гейта: ни окружение, ни командная строка его не меняют.
override COVER_FLOOR := 95
cover-check: ## Покрытие юнит + integration ≥ $(COVER_FLOOR)%, функций без тестов нет: make test-int и его профиль (FORMAT=detailed — блоки)
	@$(GATE) test-int
	@bash scripts/tools/cover.sh $(DETAIL) coverage.out $(COVER_FLOOR)

tidy: ## go mod tidy
	go mod tidy
