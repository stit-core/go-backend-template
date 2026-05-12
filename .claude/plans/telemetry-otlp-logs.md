# Telemetry: app OTLP-logs → Loki, multitenant + per-ENV

**Slug:** `telemetry-otlp-logs`
**Создан:** 2026-05-12
**Статус:** in-progress
**Последний чекпоинт:** 2026-05-12 — план approved, E1 закрыт сразу

## Контекст

Smoke-прогон 2026-05-12 показал: app не шлёт логи никуда, кроме stdout. `golibinfra/telemetry/init.go:138` создаёт `LoggerProvider` с OTLP exporter, но `golibinfra/logkit/slog_impl.go:30` пишет в `os.Stdout` через `slog.NewJSONHandler` — bridge на `LoggerProvider` отсутствует. На прод-VPS docker logging driver = default, никто stdout не собирает. В Loki за `{service=~".+"}[15m]` — пусто.

Корп-сторона **готова** для multitenant logs:
- `stit/infra/.claude/plans/observability-multitenancy.md` (done): Loki+Tempo `auth_enabled`, otel-collector routing per resource attr `tenant`, traefik tenant-auth с per-tenant bearer.
- `stit/infra/.claude/plans/grafana-multitenant.md` (done, 2026-05-10): VM/Loki/Tempo multitenant datasources с `vm_account_id`/`X-Scope-OrgID` variable.
- `corp-multi-env-onboarding` (done): `onboard-project.sh` выдаёт per-`(project, env)` tokens; Vault path `kv/projects/+/+/otel`.

Решения этой сессии:
- **App → OTLP only.** stdout JSON убираем. Локально (`OTEL_STDOUT_ONLY=true`) → OTel stdoutlog exporter (тот же OTLP pipeline, просто sink в stdout).
- **Tenant-per-env** (`<project>-<env>`). bearer/`X-Scope-OrgID` физически разделяет prod/staging/smoke. `deployment.environment.name` resource attr всё равно пишем для UI/трейсов.
- **Smoke = prod-like topology**: добавляем `otel-agent` sidecar в `scripts/smoke/compose.yml`, клонируем конфиг из `project-infra-template-compose/configs/otel-agent/config.yaml`.

## Не входит в скоуп

- Перепись метрик/трейсов exporters — они уже работают через OTLP.
- Корп-сторона `stit/infra` правки (multitenancy уже done). Возможна одна verification-задача про logs leg — выделена в **E1** как handoff document.
- Per-tenant alerting (Loki ruler) — это другая задача.
- Grafana folder RBAC / Authentik groups per tenant — out of scope (отмечено в `grafana-multitenant.md`).
- Миграция существующих stdout-логов в Loki (их нет — нечего мигрировать).

## Открытые вопросы

- [x] Q1: App-side OTLP vs docker driver vs filelog? → **App OTLP only**.
- [x] Q2: Env separation? → **Tenant-per-env**, `deployment.environment.name` как доп. resource attr.
- [x] Q3: Smoke topology? → **Добавить otel-agent sidecar**, конфиг из project-infra-template-compose.
- [x] Q4: Что из багфиксов в плане? → logs.sh + dup request_id + pgx encoding + inflight refactor.
- [x] Q5: Stdout-fallback при `OTEL_STDOUT_ONLY=true`? → **stdoutlog OTel exporter** (единый OTLP-pipeline, любой sink — это вопрос exporter'а). Решается в B2.
- [x] Q6: `cmd/migrate/main.go` тоже wire OTel logger? → **Да**, миграции тоже надо мониторить. Migrate — короткоживущий, требует явный `LoggerProvider.Shutdown(ctx)` на exit чтобы дофлашить пачку (иначе последние log records пропадают). Решается в C1.
- [x] Q7: Дублирование `request_id` — middleware vs `traceHandler.collectAttrs()`? → Анализ + решение **внутри C3** (без блокирующего вопроса).

## Атомы

### Phase Pre — preflight commits (независимые, могут идти параллельно)

- [x] **A1:** Коммит фиксов `logs.sh` ✓ 2026-05-12 — `fd79785 fix(observability): logs.sh resolves credential.md alias + safe trap`. **Caveat:** коммит непреднамеренно зацепил staged deletes из A2 (`cmd/worker/main.go` + `internal/worker/*` — 4 файла, −226 строк). User explicitly accepted; scope refactor продолжается в A2 без этих файлов.
  - Что делаем: фиксы уже в working tree — `credential.md` alias support + `${tmp:-}` в trap. Stage только `scripts/agent-tools/logs.sh`, commit.
  - Файлы: `scripts/agent-tools/logs.sh`
  - Verify: `git diff HEAD~1 -- scripts/agent-tools/logs.sh` показывает оба изменения; `scripts/agent-tools/logs.sh query --service backend --since 5m` — больше не FAIL по unbound и парсит `credential.md`.
  - Зависит от: —

- [ ] **A2:** Land inflight `worker → internal/jobs` + `verification/delivery/nats` refactor
  - Что делаем: завершить рефакторинг, который уже сидит в working tree (13M + 7U). Сборка зелёная (`agent-tools/build.sh` → OK). Прогон `agent-tools/test.sh` + `agent-tools/lint.sh` для подтверждения. Один-два чистых коммита (`refactor(worker): ...` + `feat(verification): nats delivery layer ...`).
  - Файлы: `cmd/api/main.go`, `cmd/worker/main.go` (D), `internal/worker/**` (D), `internal/jobs/**` (U), `internal/domain/verification/delivery/nats/**` (U), `internal/domain/verification/module/module.go`, `internal/domain/{auth,user}/delivery/http/**`, `internal/domain/user/usecase*.go`, `internal/domain/user/module/module_test.go`, `Makefile`, `go.mod`, `go.sum`.
  - Verify: `scripts/agent-tools/test.sh` PASS; `scripts/agent-tools/lint.sh` clean; `make pre-merge` локально зелёный.
  - Зависит от: —

- [ ] **A3:** Fix pgx encoding bug в auth bruteforce-guard
  - Что делаем: `unable to encode 900 into text format for text (OID 25): cannot find encode plan` стреляет на каждом login (`smoke-api` логах). Видимо, в SQL query bind кто-то пишет `int` в колонку с типом `text`. Найти место, переписать на корректный тип/binding (`pg_typeof` колонки, sqlc query, или caller). TDD: integration-test на login flow, который ловит этот error до fix'а.
  - Файлы: `internal/domain/auth/{usecase.go,policy.go}`, `internal/domain/auth/infra/postgres/queries/*.sql`, миграция (если нужно сменить тип колонки).
  - Verify: `scripts/agent-tools/test.sh ./internal/domain/auth/...` PASS; smoke-up + `curl POST /api/auth/login` — нет ERROR `guard record failure` в `docker logs smoke-api`.
  - Зависит от: —

### Phase B — golibinfra/logkit OTLP bridge

- [ ] **B1:** Заменить stdout JSON handler на OTel-bridge в `logkit`
  - Что делаем: в `golibinfra/logkit/slog_impl.go` убираем `slog.NewJSONHandler(w, ...)` writer. Вместо него — `otelslog.NewHandler("backend", otelslog.WithLoggerProvider(lp))` (из `go.opentelemetry.io/contrib/bridges/otelslog`). `traceHandler` обвязка остаётся как middleware на полученный handler (`trace_id`/`span_id` пишутся как attrs — теперь не дублируясь, потому что OTel SDK сам ставит их в record как trace context из ctx). Конструктор `logkit.New(cfg, lp)` принимает `LoggerProvider`.
  - Файлы: `golibinfra/logkit/{slog_impl.go,module.go,redact.go,logger.go}` (по необходимости), `golibinfra/go.mod` (новый dep `bridges/otelslog`).
  - Verify: `go test ./logkit/...` PASS в golibinfra; manual run в `go-backend-template` после **C1** показывает logs в Loki.
  - Зависит от: —

- [ ] **B2:** `OTEL_STDOUT_ONLY=true` режим через OTel stdoutlog exporter
  - Что делаем: в `golibinfra/telemetry/init.go` под `cfg.StdoutOnly` сейчас создаётся пустой `sdklog.NewLoggerProvider` (без processor/exporter — записи в никуда). Заменить на `stdoutlog.New()` exporter c `sdklog.NewSimpleProcessor`. Logkit как клиент `LoggerProvider`-а — без изменений. Зашиваем дефолт: для cmd-tools (migrate) — пустой Nop logger, для api — реальный.
  - Файлы: `golibinfra/telemetry/init.go`, `golibinfra/telemetry/config.go` (если есть).
  - Verify: `OTEL_STDOUT_ONLY=true go run ./cmd/api` → видим OTel log record'ы в stdout (структурированные).
  - Зависит от: B1.

- [ ] **B3:** Тесты `logkit` + миграция existing тестов
  - Что делаем: `logger_test.go`, `redact_test.go`, `bench_test.go` адаптировать под новый конструктор (передача `LoggerProvider`). Возможно — fake LP через `sdklog.NewLoggerProvider` + in-memory exporter, и assert на emitted records.
  - Файлы: `golibinfra/logkit/*_test.go`.
  - Verify: `go test ./logkit/... -race` PASS, `go test -bench=.` без регрессий >20%.
  - Зависит от: B1.

### Phase C — go-backend-template wiring

- [ ] **C1:** Wire `LoggerProvider` → `logkit` в `cmd/api/main.go` **и** `cmd/migrate/main.go`
  - Что делаем: fx graph — `telemetry.Module` уже provide'ит `*telemetry.Providers` (включая `LoggerProvider`). Добавить fx-decorator/конструктор, который вытаскивает `lp := providers.LoggerProvider` и передаёт в `logkit.New(cfg, lp)`. `logkit.Module` принимает LP как параметр fx.Provide (часть B1). **Migrate** — короткоживущий процесс (накатить N миграций → exit), но логи всё равно нужны: wire тот же `telemetry.Init` + `logkit.New`, **обязательно** вызвать `providers.Shutdown(ctx)` в defer'е после `migrate.Up()` (иначе BatchProcessor не дофлашит, последние records потеряются). В fx-mode это `app.Run()` сам делает по сигналу; в migrate скорее всего нет fx — тогда явный `defer providers.Shutdown(ctx)` после `Init`. Verify exit: вернуть тот же exitcode что `migrate` вернул, не маскировать ошибкой shutdown.
  - Файлы: `cmd/api/main.go`, `cmd/migrate/main.go`, `golibinfra/logkit/module.go` (если меняется сигнатура fx-обвязки — часть B1).
  - Verify: `make build` + `go run ./cmd/api` и `go run ./cmd/migrate up` (с env'ом `OTEL_EXPORTER_OTLP_ENDPOINT=otel.stit.tech:443 OTEL_TOKEN=... OTEL_TENANT=...`) → лог-строки от обоих процессов появляются в `loki.stit.tech` под нужным tenant'ом. `logs.sh query --service backend --since 5m | jq -r 'select(.scope=="migrate")'` показывает миграционные строки.
  - Зависит от: B1.

- [ ] **C2:** Resource attrs: `deployment.environment.name` + проверка `OTEL_TENANT=<project>-<env>` контракта
  - Что делаем: в `golibinfra/telemetry/init.go:Init` сейчас уже ставится `semconv.DeploymentEnvironmentName(cfg.Environment)` (из `attrs := ...` блок) — проверить, что app передаёт `APP_ENV` в `cfg.Environment`. Если нет — связать. Документировать в `.env.example`: `OTEL_TENANT=<project>-<env>` (а не голый `<project>`), привести пример. В `scripts/smoke/compose.yml` `OTEL_TENANT` уже из `${OTEL_TENANT}` env — credential.md должен содержать значение типа `backend-smoke` или подобное (но это не trivial — credential.md выдаёт `onboard-project.sh`, см. handoff в **E1**).
  - Файлы: `internal/platform/config/config.go` (если нужно прокинуть APP_ENV), `.env.example`, `scripts/smoke/compose.yml` (если меняем default).
  - Verify: после **C1** в Loki: `{service_namespace="<project>-<env>"} |= ""` → запись имеет лейбл `deployment_environment_name=<env>`. Корп-otel routing маршрутизирует на правильный tenant.
  - Зависит от: C1.

- [ ] **C3:** Убрать duplicate `request_id` в JSON-логах
  - Что делаем: после B1+C1 stdout JSON не используется, но HTTP-middleware и `traceHandler.collectAttrs()` могут добавлять `request_id` дважды и в OTLP records тоже. Trace: `golibinfra/logkit/slog_impl.go:traceHandler.Handle` + `golibinfra/logkit/attrs.go` + middleware в `internal/platform/authgate/` или httpkit. Убрать дублирование на стороне-вторичного-источника (предпочтительно — middleware ставит attr через ctx, traceHandler собирает из ctx; убрать прямой `slog.With("request_id", ...)` если он есть). TDD: тест в logkit, который emit'ит лог из ctx с одним `request_id`, проверяет single attr в записи.
  - Файлы: `golibinfra/logkit/{slog_impl.go,attrs.go}` или corresponding HTTP-middleware в `golibinfra/httpkit/` либо `internal/platform/authgate/`.
  - Verify: после **C1** запрос `curl http://127.0.0.1:8080/health` производит log record с **одним** `request_id` attr (`logcli query '{service_name="backend"} | json | request_id != ""' --since 5m --limit 5`).
  - Зависит от: B1, C1.

### Phase D — smoke topology

- [ ] **D1:** Добавить `otel-agent` sidecar в `scripts/smoke/compose.yml`
  - Что делаем: новый service `otel-agent` (`otel/opentelemetry-collector-contrib:0.151.0` — версия как в project-infra-template-compose). Конфиг `scripts/smoke/configs/otel-agent.yaml` — клон из `project-infra-template-compose/configs/otel-agent/config.yaml`, минус `prometheus` scraper (нет cadvisor в smoke; либо ставим — для realism). `api` service: `OTEL_EXPORTER_OTLP_ENDPOINT: otel-agent:4318`, `OTEL_EXPORTER_OTLP_INSECURE: "true"`. `otel-agent` → endpoint `https://otel.stit.tech:443` с bearer из `${STIT_OBS_TOKEN}`/`${OTEL_TENANT}` (уже резолвится в `up.sh`). `depends_on: [otel-agent]` для api.
  - Файлы: `scripts/smoke/compose.yml`, `scripts/smoke/configs/otel-agent.yaml` (новый), возможно `scripts/smoke/up.sh` (новые секреты не нужны — уже есть).
  - Verify: `bash scripts/smoke/up.sh` → 5 контейнеров healthy (api, postgres, nats, redis, otel-agent). `docker logs smoke-otel-agent` показывает успешный export к corp.
  - Зависит от: —

- [ ] **D2:** Smoke E2E verify
  - Что делаем: full reset (`scripts/smoke/down.sh && bash scripts/smoke/up.sh`), сгенерировать трафик (`curl` на 5-6 endpoints, включая validation-error и 401), подождать batch-flush (≤5s), `scripts/agent-tools/logs.sh query --service backend --since 5m --limit 30` — увидеть строки с правильными `trace_id`/`request_id`/`level`/`path`. Если 0 — debug pipeline (`logs.sh query --raw '{service_namespace=~".+"}'` для широкого матча).
  - Файлы: —
  - Verify: `logs.sh query --service backend --since 5m` returns ≥10 строк с правильными attrs.
  - Зависит от: B1, B2, C1, C2, C3, D1, E1 (last only if infra logs leg gap blocks).

### Phase E — infra handoff (для второй сессии)

- [x] **E1:** Написать verification-document в `stit/infra/.claude/plans/observability-logs-leg-verification.md` ✓ 2026-05-12 — файл создан + строка в `stit/infra/.claude/plans/roadmap.md` под Планируется
  - Что делаем: коротенький план (2-3 атома) для второй сессии в `stit/infra`: (1) проверить `configs/otel-collector/config.yaml` — есть ли `service.pipelines.logs` секция с routing connector на per-tenant `otlphttp/loki_<t>` (как сделано для metrics/traces в `observability-multitenancy.md` Phase O-3). Если нет — добавить, тестируется через push с known token. (2) Verify в Grafana: в `Loki-multitenant` datasource видны логи из smoke-VM под tenant `<project>-smoke`. (3) Если что-то в `render-tenant-auth.sh` не охватывает logs-paths — расширить. Не пишу код, только plan markdown.
  - Файлы: `/root/git/stit/infra/.claude/plans/observability-logs-leg-verification.md` (новый), `/root/git/stit/infra/.claude/plans/roadmap.md` (add line под Активные).
  - Verify: файл существует, прошёл бы checklist `plan`-skill'а (контекст, атомы, verify, exit criteria).
  - Зависит от: —

## Чекпоинты

### 2026-05-12 — план approved, E1 hand-off готов
- Approve: все 4 архитектурные развилки закрыты (App→OTLP-only, tenant-per-env, otel-agent sidecar в smoke, scope = все 4 фикса + рефакторинг).
- Открытые вопросы Q5–Q7 закрыты в plan-body.
- **E1 закрыт сразу** в этой же сессии: `stit/infra/.claude/plans/observability-logs-leg-verification.md` + roadmap-строка. Контекст для второй сессии готов.
- Следующий шаг: **Phase Pre** — A1 (logs.sh commit) первым, потом A2/A3 в любом порядке.
