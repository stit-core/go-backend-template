//go:build integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
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
	"git.stit.tech/stit-core/golibinfra/timerkit"

	"git.stit.tech/stit-core/go-backend-template/cmd/migrate/sources"
	"git.stit.tech/stit-core/go-backend-template/internal/platform/authgate"
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

// serviceEnv поднимает Postgres, NATS и зависимости китов графа (graphKits) в контейнерах и выставляет env сервиса:
// .env.example, направленный на них.
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
		// Argon2id с памятью кита по умолчанию (64 МиБ на хэш) под -race держит бинарь тестов на 2,2 ГБ: потоки
		// тестов от памяти хэша не зависят, а параметры по умолчанию проверяет smoke релиза на стеке.
		"ARGON2_MEMORY_KIB": "8192",
	} {
		env[k] = v
	}
	for _, kit := range graphKits() {
		maps.Copy(env, kitDeps[kit].start(t))
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
	return pg
}

// kitDep — внешняя зависимость кита для boot-теста: контейнер с env сервиса, направленным на него, и проба /ready,
// которой сервис её опрашивает.
type kitDep struct {
	probe string
	start func(t *testing.T) map[string]string
}

// kitDeps — зависимости китов по имени пакета кита golibinfra — те же, что поднимают smoke-стек и make local-deps-up
// (scripts/tools/kits.sh). Кит в графе сервиса поднимает свою в serviceEnv, а /ready сервиса опрашивает её пробу
// (TestBoot): сервису с китом правка serviceEnv не нужна.
var kitDeps = map[string]kitDep{
	"cachekit":   {probe: "cachekit", start: startRedis},
	"storagekit": {probe: "storagekit", start: startMinIO},
}

func startRedis(t *testing.T) map[string]string {
	t.Helper()
	return map[string]string{"REDIS_URL": testkit.StartRedis(t).URL}
}

// startMinIO — MinIO и env storagekit на него: бакет сервиса (STORAGE_S3_BUCKET из .env.example) сервис заводит сам.
func startMinIO(t *testing.T) map[string]string {
	t.Helper()
	s3 := testkit.StartMinIO(t)
	return map[string]string{
		"STORAGE_S3_ENDPOINT": "http://" + s3.Endpoint, "STORAGE_S3_ACCESS_KEY": s3.AccessKey,
		"STORAGE_S3_SECRET_KEY": s3.SecretKey, "STORAGE_S3_CREATE_BUCKET": "true",
	}
}

// graphKits — киты golibinfra графа сервиса с записью kitDeps: кит в графе — его конструктор конфига в kitConfigs
// (git.…/golibinfra/cachekit.NewConfig → cachekit), по какому бы имени cmd/api его ни импортировал.
func graphKits() []string {
	var kits []string
	for _, c := range kitConfigs {
		name := runtime.FuncForPC(reflect.ValueOf(c).Pointer()).Name()
		slash := strings.LastIndex(name, "/")
		kit, _, _ := strings.Cut(name[slash+1:], ".")
		if _, ok := kitDeps[kit]; ok && name[:max(slash, 0)] == golibinfraModule {
			kits = append(kits, kit)
		}
	}
	return kits
}

const golibinfraModule = "git.stit.tech/stit-core/golibinfra"

// send — код и заголовки ответа на запрос.
func send(t *testing.T, c *http.Client, method, url, body string) (int, http.Header) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, url, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	return resp.StatusCode, resp.Header
}

// spaOrigin — первый адрес HTTP_CORS_ORIGINS окружения сервиса (serviceEnv берёт его из .env.example): с него страница
// SPA шлёт изменяющие запросы с cookie сессии.
func spaOrigin(t *testing.T) string {
	t.Helper()
	origin, _, _ := strings.Cut(os.Getenv("HTTP_CORS_ORIGINS"), ",")
	origin = strings.TrimSpace(origin)
	require.NotEmpty(t, origin, "the service env has the address of the SPA in HTTP_CORS_ORIGINS")
	return origin
}

// browser — клиент страницы SPA: как браузер, ставит каждому запросу Origin, без которого изменяющий запрос с cookie
// сессии получает 403 ORIGIN_NOT_ALLOWED.
func browser(t *testing.T, jar http.CookieJar, timeout time.Duration) *http.Client {
	t.Helper()
	return &http.Client{Jar: jar, Timeout: timeout, Transport: originTransport{origin: spaOrigin(t)}}
}

type originTransport struct{ origin string }

func (o originTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Origin", o.origin)
	return http.DefaultTransport.RoundTrip(r)
}

// Изменяющий запрос с cookie сессии сервис пропускает только с Origin из HTTP_CORS_ORIGINS, и отказ — до маршрута:
// страница соседнего адреса того же сайта шлёт cookie same-site, SameSite=Strict её не останавливает. Путь без
// маршрута показывает, что проверка не зависит от доменов: с адресом SPA запрос доходит до маршрутизатора (404).
func TestBoot_SessionOrigin(t *testing.T) {
	serviceEnv(t)
	var srv *httpkit.Server
	app := fxtest.New(t, service, fx.Populate(&srv))
	app.RequireStart()
	defer app.RequireStop()
	client := &http.Client{Timeout: 10 * time.Second}
	post := func(origin, cookie string) (int, string) {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://"+srv.Addr().String()+"/api/no-such-route", http.NoBody)
		require.NoError(t, err)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: cookie, Value: "any"})
		}
		resp, err := client.Do(req)
		require.NoError(t, err)
		var body struct {
			Code string `json:"code"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		require.NoError(t, resp.Body.Close())
		return resp.StatusCode, body.Code
	}

	for _, tc := range []struct{ name, origin, cookie string }{
		{name: "access without origin", cookie: authgate.CookieAccess},
		{name: "refresh without origin", cookie: authgate.CookieRefresh},
		{name: "access from a sibling page", origin: "https://evil.example.com", cookie: authgate.CookieAccess},
		{name: "refresh from a sibling page", origin: "https://evil.example.com", cookie: authgate.CookieRefresh},
	} {
		status, code := post(tc.origin, tc.cookie)
		require.Equal(t, http.StatusForbidden, status, tc.name)
		require.Equal(t, "ORIGIN_NOT_ALLOWED", code, tc.name)
	}
	status, code := post(spaOrigin(t), authgate.CookieRefresh)
	require.Equal(t, http.StatusNotFound, status, "the page of the SPA reaches the router: %s", code)
	status, code = post("", "")
	require.Equal(t, http.StatusNotFound, status, "a request without a session cookie needs no Origin: %s", code)
}

// TestBoot поднимает сервис набором опций main поверх Postgres, NATS и зависимостей китов графа в контейнерах:
// конструкторы, Invoke и хуки выполняются по-настоящему, чего fx.ValidateApp не делает. Проверки — про сервис целиком,
// без знания его доменов; потоки доменов идентичности — TestIdentity.
func TestBoot(t *testing.T) {
	serviceEnv(t)

	hooks := &hookLog{}
	var (
		srv         *httpkit.Server
		kitSources  []pgkit.Source
		jobs        []string
		telemetryMP *sdkmetric.MeterProvider
	)
	app := fxtest.New(t, service,
		hookEvents(hooks),
		fx.Decorate(func(p *telemetry.Providers) *telemetry.Providers { telemetryMP = p.MeterProvider; return p }),
		fx.Populate(&srv),
		fx.Invoke(fx.Annotate(func(s []pgkit.Source) { kitSources = s }, fx.ParamTags(`group:"`+pgkit.MigrationsGroup+`"`))),
		fx.Invoke(fx.Annotate(func(js []timerkit.Job) {
			for _, j := range js {
				jobs = append(jobs, j.Name)
			}
		}, fx.ParamTags(`group:"`+timerkit.JobsGroup+`"`))))
	app.RequireStart()
	require.Contains(t, jobs, "auditkit.retention", "the service keeps audit_log for a term, not forever")
	// Outbox домена-публикатора в cmd/migrate — и дренаж outbox в графе: без него события остаются в таблице.
	for _, src := range sources.Kits {
		if name, ok := strings.CutPrefix(src.Name, "outbox_"); ok {
			require.Contains(t, jobs, "outboxkit.publisher."+name, "the outbox %s publishes its events", src.Name)
		}
	}
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
	want := map[string]string{"pgkit": "ok", "natskit": "ok"}
	for _, kit := range graphKits() {
		want[kitDeps[kit].probe] = "ok"
	}
	require.Equal(t, want, probes,
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
	// Операция под Security без cookie — 401: аутентификация стоит на каждой, а не на одной; ответ операции
	// домена, и отказ тоже, — вне кэшей.
	secured := 0
	for path, ops := range contract.Paths {
		for method, op := range ops {
			if len(op.Security) > 0 {
				secured++
				status, header := send(t, anon, strings.ToUpper(method), base+pathParam.ReplaceAllString(path, "00000000-0000-0000-0000-000000000001"), "{}")
				require.Equal(t, http.StatusUnauthorized, status, "%s %s without a cookie", method, path)
				require.Equal(t, "no-store", header.Get("Cache-Control"), "%s %s: a response of a domain operation stays out of caches", method, path)
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
	from := func(method, path, ip string) (int, string, string) {
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
		return resp.StatusCode, body.Code, resp.Header.Get("Cache-Control")
	}

	for range 2 {
		status, code, _ := from(http.MethodPost, "/api/auth/refresh", "198.51.100.1")
		require.NotEqual(t, http.StatusTooManyRequests, status, "within the burst: %s", code)
	}
	status, code, cache := from(http.MethodPost, "/api/auth/refresh", "198.51.100.1")
	require.Equal(t, http.StatusTooManyRequests, status)
	require.Equal(t, "RATE_LIMITED", code)
	require.Equal(t, "no-store", cache, "the refusal of the limit stays out of caches too")
	status, code, _ = from(http.MethodPost, "/api/auth/refresh", "198.51.100.2")
	require.NotEqual(t, http.StatusTooManyRequests, status, "another client has its own bucket: %s", code)
	status, code, _ = from(http.MethodGet, "/health", "198.51.100.1")
	require.Equal(t, http.StatusOK, status, "a path outside /api/auth/ is not limited: %s", code)
}

// Задача домена (timerkit.ProvideJob в модуле-владельце) идёт на графе main: её гоняет timerkit.Module сервиса, а не
// модуль домена.
func TestBoot_Jobs(t *testing.T) {
	serviceEnv(t)
	var ran atomic.Bool
	app := fxtest.New(t, service, timerkit.ProvideJob(func() timerkit.Job {
		return timerkit.Job{Name: "probe.job", Interval: time.Hour, RunOnStart: true, Run: func(context.Context) error {
			ran.Store(true)
			return nil
		}}
	}))
	app.RequireStart()
	defer app.RequireStop()
	require.Eventually(t, ran.Load, 10*time.Second, 10*time.Millisecond, "the job of a domain module runs on the service graph")
}

// Ключ лимита клиента httpkit — адрес соединения: без HTTP_TRUST_PROXY_HOPS X-Forwarded-For его не меняет, и
// клиент, подставляющий в каждый запрос свою цепочку адресов (длиннее любого разумного числа прокси), упирается в
// лимит, как все.
func TestBoot_RateLimitKey(t *testing.T) {
	serviceEnv(t)
	t.Setenv("HTTP_RATE_LIMIT_RPS", "1")
	t.Setenv("HTTP_RATE_LIMIT_BURST", "2")
	var srv *httpkit.Server
	app := fxtest.New(t, service, fx.Populate(&srv))
	app.RequireStart()
	defer app.RequireStop()
	client := &http.Client{Timeout: 10 * time.Second}
	limited := 0
	// Запас 2 и квота в секунду: все десять запросов прошли бы, только растянувшись на восемь секунд.
	for n := range 10 {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+srv.Addr().String()+"/health", http.NoBody)
		require.NoError(t, err)
		req.Header.Set("X-Forwarded-For", forwardedChain(n, 32))
		resp, err := client.Do(req)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		if resp.StatusCode == http.StatusTooManyRequests {
			limited++
		}
	}
	require.Positive(t, limited, "the client limit holds whatever X-Forwarded-For says")
}

// forwardedChain — цепочка X-Forwarded-For запроса n из hops своих записей IPv4 (IPv6 одной /64 guard входа считает
// одним адресом): при доверии к любому числу прокси до hops ключ лимита клиента и антибрутфорса менялся бы от запроса
// к запросу.
func forwardedChain(n, hops int) string {
	entries := make([]string, hops)
	for i := range entries {
		entries[i] = fmt.Sprintf("198.18.%d.%d", n+1, i+1)
	}
	return strings.Join(entries, ", ")
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
