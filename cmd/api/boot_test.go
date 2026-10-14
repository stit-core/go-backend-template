//go:build integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/fx/fxtest"

	"git.stit.tech/stit-core/golibinfra/httpkit"
	"git.stit.tech/stit-core/golibinfra/pgkit"
	"git.stit.tech/stit-core/golibinfra/telemetry"
	"git.stit.tech/stit-core/golibinfra/testkit"
	"git.stit.tech/stit-core/golibinfra/testkit/metrictest"

	"git.stit.tech/stit-core/go-backend-template/cmd/migrate/sources"
	"git.stit.tech/stit-core/go-backend-template/internal/testkit/repopath"
)

// hookLog — порядок хуков старта и остановки сервиса: кто добавил хук (caller) — по событиям fx.
type hookLog struct {
	mu          sync.Mutex
	start, stop []string
}

// Тексты событий хуков fxevent.SlogLogger, по которым hookLog их узнаёт.
const (
	onStartExecuting = "OnStart hook executing"
	onStopExecuting  = "OnStop hook executing"
)

func (h *hookLog) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	switch r.Message {
	case onStartExecuting:
		h.start = append(h.start, attr(r, "caller"))
	case onStopExecuting:
		h.stop = append(h.stop, attr(r, "caller"))
	}
	return nil
}

func (h *hookLog) Enabled(context.Context, slog.Level) bool { return true }
func (h *hookLog) WithAttrs([]slog.Attr) slog.Handler       { return h }
func (h *hookLog) WithGroup(string) slog.Handler            { return h }

func attr(r slog.Record, key string) string {
	var v string
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == key {
			v = a.Value.String()
		}
		return v == ""
	})
	return v
}

// hookEvents отдаёт события fx обработчику h. Логгер событий service (logkit.FxLogger) берёт sink из
// logkit.Config, а не из графа, и декоратор *slog.Logger его не видит; опция подменяет его — что service
// его ставит, держит TestMainLogsStartFailure.
func hookEvents(h slog.Handler) fx.Option {
	return fx.WithLogger(func() fxevent.Logger { return &fxevent.SlogLogger{Logger: slog.New(h)} })
}

// serviceEnv поднимает Postgres и NATS в контейнерах и выставляет env сервиса: .env.example, направленный на
// них.
func serviceEnv(t *testing.T) *testkit.PostgresFixture {
	t.Helper()
	pg := testkit.StartPostgres(t, testkit.WithMigrations(sources.Kits...), testkit.WithMigrationsDir(repopath.Migrations()))
	nats := testkit.StartNATS(t, testkit.WithJetStream())
	unsetHTTPEnv(t)
	env := envExample(t)
	for k, v := range map[string]string{
		"PG_DSN": pg.DSN, "NATS_URL": nats.URL, "HTTP_ADDR": "127.0.0.1:0", "LOG_SILENT": "true",
		// Спецификацию сервис отдаёт только с HTTP_DOCS_ENABLED.
		"HTTP_DOCS_ENABLED": "true",
	} {
		env[k] = v
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
	return pg
}

// call шлёт запрос и отдаёт статус ответа.
func call(t *testing.T, c *http.Client, method, url, body string) int {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, url, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	return resp.StatusCode
}

// TestBoot поднимает сервис набором опций main поверх Postgres и NATS в контейнерах: конструкторы, Invoke и
// хуки выполняются по-настоящему, чего fx.ValidateApp не делает. Проверки — про сервис целиком, без знания
// его доменов; потоки доменов идентичности — TestIdentity.
func TestBoot(t *testing.T) {
	serviceEnv(t)

	hooks := &hookLog{}
	var (
		srv         *httpkit.Server
		kitSources  []pgkit.Source
		telemetryMP *sdkmetric.MeterProvider
	)
	app := fxtest.New(t, service,
		hookEvents(hooks),
		fx.Decorate(func(p *telemetry.Providers) *telemetry.Providers { telemetryMP = p.MeterProvider; return p }),
		fx.Populate(&srv),
		fx.Invoke(fx.Annotate(func(s []pgkit.Source) { kitSources = s }, fx.ParamTags(`group:"`+pgkit.MigrationsGroup+`"`))))
	app.RequireStart()
	require.Equal(t, stopTimeout, app.StopTimeout(), "the stop hooks get the budget checkStopBudget checks the env against, not the fx default")
	require.Same(t, telemetryMP, otel.GetMeterProvider(),
		"telemetry.Module sets its meter provider as the global one: without it every metric of the kits is a no-op")

	// cmd/migrate — отдельный бинарь без графа сервиса: его список обязан совпасть с kit-таблицами графа.
	require.ElementsMatch(t, sourceNames(sources.Kits), sourceNames(kitSources),
		"sources.Kits (cmd/migrate) differs from the kit tables of the service graph (group %s)", pgkit.MigrationsGroup)

	require.NoError(t, httpkit.CheckHealth(t.Context(), srv.Addr().String()), "api healthcheck gets 200 from /health of the service")

	base := "http://" + srv.Addr().String()
	anon := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, base+"/ready", nil)
	require.NoError(t, err)
	resp, err := anon.Do(req)
	require.NoError(t, err)
	var ready struct {
		Probes []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"probes"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&ready))
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusOK, resp.StatusCode, "/ready of the started service")
	probes := map[string]string{}
	for _, p := range ready.Probes {
		probes[p.Name] = p.Status
	}
	require.Equal(t, map[string]string{"pgkit": "ok", "natskit": "ok"}, probes,
		"/ready polls every dependency of the service: without its probe an instance that lost it stays in rotation")

	humaAPI, ok := srv.API().(interface{ Config() huma.Config })
	require.True(t, ok, "the huma API exposes its Config")
	humaCfg := humaAPI.Config()
	require.NotEmpty(t, humaCfg.OpenAPIPath, "the service serves its OpenAPI spec")
	req, err = http.NewRequestWithContext(t.Context(), http.MethodGet, base+humaCfg.OpenAPIPath+".json", nil)
	require.NoError(t, err)
	resp, err = anon.Do(req)
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	var live bytes.Buffer
	require.NoError(t, json.Indent(&live, body, "", "  "))
	spec, err := os.ReadFile(openAPIFile)
	require.NoError(t, err)
	require.Equal(t, string(spec), live.String()+"\n", "the running service serves openapi.json")

	var contract struct {
		Paths map[string]map[string]struct {
			Security []map[string][]string `json:"security"`
		} `json:"paths"`
	}
	require.NoError(t, json.Unmarshal(spec, &contract))
	// Операция под Security без cookie — 401: аутентификация стоит на каждой, а не на одной.
	secured := 0
	for path, ops := range contract.Paths {
		for method, op := range ops {
			if len(op.Security) > 0 {
				secured++
				require.Equal(t, http.StatusUnauthorized, call(t, anon, strings.ToUpper(method), base+pathParam.ReplaceAllString(path, "00000000-0000-0000-0000-000000000001"), "{}"),
					"%s %s without a cookie", method, path)
			}
		}
	}
	require.NotZero(t, secured, "the contract has operations under Security")
	// Сырой маршрут chi (Router().Get, Mount) и Hidden-операция huma обходят проверку Security и
	// openapi.json: каждый маршрут роутера — операция контракта или маршрут спецификации huma.
	for _, route := range srv.Router().Routes() {
		for method := range route.Handlers {
			_, op := contract.Paths[route.Pattern][strings.ToLower(method)]
			require.True(t, op || method == http.MethodGet && humaSpecRoute(humaCfg, route.Pattern),
				"route %s %s is not in openapi.json: a raw chi route or a Hidden huma operation escapes the Security check and the API contract", method, route.Pattern)
		}
	}

	app.RequireStop()
	const noHookEvents = "no %q events of the service hooks: fx renamed this fxevent.SlogLogger message"
	require.NotEmpty(t, hooks.start, noHookEvents, onStartExecuting)
	require.NotEmpty(t, hooks.stop, noHookEvents, onStopExecuting)
	// Запросы в полёте держат всё, что стартовало раньше HTTP: сервер слушает последним и гасится первым.
	const httpServe = "git.stit.tech/stit-core/golibinfra/httpkit.serve"
	require.Equal(t, httpServe, hooks.start[len(hooks.start)-1], "HTTP must start last (OnStart: %v)", hooks.start)
	require.Equal(t, httpServe, hooks.stop[0], "HTTP must stop first (OnStop: %v)", hooks.stop)
}

var pathParam = regexp.MustCompile(`\{[^}]*\}`)

// Остановка из env, которая не помещается в бюджет fx.StopTimeout, — ошибка старта: checkStopBudget стоит в
// графе service, а не только объявлен.
func TestBoot_StopBudgetRequired(t *testing.T) {
	serviceEnv(t)
	t.Setenv("HTTP_SHUTDOWN_DRAIN_DELAY", "15s")

	require.ErrorIs(t, fx.New(service).Err(), errStopBudget)
}

// Строгий лимит клиента на /api/auth/* стоит в графе service: поток одного клиента получает 429, а не
// место в общей очереди Argon2id процесса; клиент — IP из цепочки прокси, прочие пути лимит не делят.
func TestBoot_AuthRateLimit(t *testing.T) {
	serviceEnv(t)
	t.Setenv("HTTP_TRUST_PROXY_HOPS", "1")
	t.Setenv("HTTP_AUTH_RATE_LIMIT_RPS", "1")
	t.Setenv("HTTP_AUTH_RATE_LIMIT_BURST", "2")
	var srv *httpkit.Server
	app := fxtest.New(t, service, fx.Populate(&srv))
	app.RequireStart()
	defer app.RequireStop()
	client := &http.Client{Timeout: 10 * time.Second}
	send := func(method, path, ip string) (int, string) {
		req, err := http.NewRequestWithContext(t.Context(), method, "http://"+srv.Addr().String()+path, http.NoBody)
		require.NoError(t, err)
		req.Header.Set("X-Forwarded-For", ip)
		resp, err := client.Do(req)
		require.NoError(t, err)
		var body struct {
			Code string `json:"code"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		require.NoError(t, resp.Body.Close())
		return resp.StatusCode, body.Code
	}

	for range 2 {
		status, code := send(http.MethodPost, "/api/auth/refresh", "198.51.100.1")
		require.NotEqual(t, http.StatusTooManyRequests, status, "within the burst: %s", code)
	}
	status, code := send(http.MethodPost, "/api/auth/refresh", "198.51.100.1")
	require.Equal(t, http.StatusTooManyRequests, status)
	require.Equal(t, "RATE_LIMITED", code)
	status, code = send(http.MethodPost, "/api/auth/refresh", "198.51.100.2")
	require.NotEqual(t, http.StatusTooManyRequests, status, "another client has its own bucket: %s", code)
	status, code = send(http.MethodGet, "/health", "198.51.100.1")
	require.Equal(t, http.StatusOK, status, "a path outside /api/auth/ is not limited: %s", code)
}

// Метрики доменов — под scope app провайдера telemetry: по нему их отличают от метрик китов.
func TestBoot_DomainMeterScope(t *testing.T) {
	serviceEnv(t)
	metrics := metrictest.NewProvider()
	var meter metric.Meter
	app := fx.New(service,
		fx.Decorate(func(p *telemetry.Providers) *telemetry.Providers {
			tapped := *p
			tapped.MeterProvider = metrics.MeterProvider
			return &tapped
		}),
		fx.Populate(&meter))
	require.NoError(t, app.Err())

	counter, err := meter.Int64Counter("probe_total")
	require.NoError(t, err)
	counter.Add(t.Context(), 1)
	require.Equal(t, "app", metrics.Collect(t).Get(t, "probe_total").Scope)
}

func sourceNames(srcs []pgkit.Source) []string {
	names := make([]string, len(srcs))
	for i, s := range srcs {
		names[i] = s.Name
	}
	return names
}

// humaSpecRoute — маршрут спецификации или документации, который huma вешает на роутер сам по
// своему конфигу: OpenAPIPath с расширением, DocsPath, SchemasPath/{schema}.
func humaSpecRoute(cfg huma.Config, route string) bool {
	if cfg.DocsPath != "" && route == cfg.DocsPath || cfg.SchemasPath != "" && route == cfg.SchemasPath+"/{schema}" {
		return true
	}
	suffix, ok := strings.CutPrefix(route, cfg.OpenAPIPath)
	return cfg.OpenAPIPath != "" && ok && slices.Contains([]string{".json", ".yaml", "-3.0.json", "-3.0.yaml"}, suffix)
}
