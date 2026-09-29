//go:build integration

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/fx/fxtest"

	"git.stit.tech/stit-core/golibinfra/httpkit"
	"git.stit.tech/stit-core/golibinfra/pgkit"
	"git.stit.tech/stit-core/golibinfra/testkit"

	"git.stit.tech/stit-core/go-backend-template/cmd/migrate/sources"
	"git.stit.tech/stit-core/go-backend-template/internal/testkit/repopath"
)

// hookOrder — порядок хуков старта и остановки: кто добавил каждый хук (CallerName).
type hookOrder struct {
	mu          sync.Mutex
	start, stop []string
}

func (h *hookOrder) LogEvent(e fxevent.Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	switch ev := e.(type) {
	case *fxevent.OnStartExecuting:
		h.start = append(h.start, ev.CallerName)
	case *fxevent.OnStopExecuting:
		h.stop = append(h.stop, ev.CallerName)
	}
}

// TestBoot поднимает сервис набором опций main поверх Postgres и NATS в контейнерах:
// конструкторы, Invoke и хуки выполняются по-настоящему, чего fx.ValidateApp не делает.
func TestBoot(t *testing.T) {
	pg := testkit.StartPostgres(t, testkit.WithMigrations(sources.Kits...), testkit.WithMigrationsDir(repopath.Migrations()))
	nats := testkit.StartNATS(t, testkit.WithJetStream())
	unsetHTTPEnv(t)
	for k, v := range map[string]string{
		"PG_DSN": pg.DSN, "NATS_URL": nats.URL,
		"NATS_JETSTREAM_ENABLED": "true", "NATS_JS_STREAM_NAMES": "user,verification",
		"JWT_SECRET": strings.Repeat("s", 32), "HTTP_ADDR": "127.0.0.1:0", "LOG_SILENT": "true",
	} {
		t.Setenv(k, v)
	}

	hooks := &hookOrder{}
	var (
		srv        *httpkit.Server
		kitSources []pgkit.Source
	)
	app := fxtest.New(t, service,
		fx.WithLogger(func() fxevent.Logger { return hooks }), fx.Populate(&srv),
		fx.Invoke(fx.Annotate(func(s []pgkit.Source) { kitSources = s }, fx.ParamTags(`group:"`+pgkit.MigrationsGroup+`"`))))
	app.RequireStart()

	// cmd/migrate — отдельный бинарь без графа сервиса: его список обязан совпасть с kit-таблицами графа.
	require.ElementsMatch(t, sourceNames(sources.Kits), sourceNames(kitSources),
		"sources.Kits (cmd/migrate) differs from the kit tables of the service graph (group %s)", pgkit.MigrationsGroup)

	base := "http://" + srv.Addr().String()
	call := func(c *http.Client, method, path, body string) int {
		req, err := http.NewRequestWithContext(t.Context(), method, base+path, strings.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.Do(req)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		return resp.StatusCode
	}
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	user := &http.Client{Jar: jar, Timeout: 10 * time.Second}
	anon := &http.Client{Timeout: 10 * time.Second}

	const email = "boot@example.com"
	require.Equal(t, http.StatusCreated, call(user, http.MethodPost, "/api/auth/register",
		`{"email":"`+email+`","password":"password123","name":"Boot"}`), "register")
	require.Equal(t, http.StatusOK, call(user, http.MethodPost, "/api/auth/login",
		`{"email":"`+email+`","password":"password123"}`), "login")
	require.Equal(t, http.StatusOK, call(user, http.MethodGet, "/api/users/me", ""), "/me with the session cookie")
	require.Equal(t, http.StatusUnauthorized, call(anon, http.MethodGet, "/api/users/me", ""), "/me without a cookie")
	require.Equal(t, http.StatusNoContent, call(anon, http.MethodPost, "/api/auth/password-reset/request",
		`{"email":"`+email+`"}`), "password reset request")
	status := 0
	for range 6 {
		status = call(anon, http.MethodPost, "/api/auth/login", `{"email":"`+email+`","password":"wrong-password"}`)
	}
	require.Equal(t, http.StatusTooManyRequests, status, "the brute-force guard locks the account after repeated failures")

	db := pg.Pool(t)
	require.Eventually(t, func() bool {
		var n int
		err := db.QueryRow(t.Context(), `SELECT count(*) FROM email_verifications ev
			JOIN users u ON u.id = ev.user_id WHERE u.email = $1`, email).Scan(&n)
		return err == nil && n == 1
	}, 15*time.Second, 100*time.Millisecond, "user.registered goes outbox → JetStream → verification subscriber")
	var own int
	require.NoError(t, db.QueryRow(t.Context(),
		`SELECT count(*) FROM outbox_verification WHERE subject LIKE 'verification.%'`).Scan(&own))
	require.Positive(t, own, "verification publishes into its own outbox")

	humaAPI, ok := srv.API().(interface{ Config() huma.Config })
	require.True(t, ok, "the huma API exposes its Config")
	humaCfg := humaAPI.Config()
	require.NotEmpty(t, humaCfg.OpenAPIPath, "the service serves its OpenAPI spec")
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, base+humaCfg.OpenAPIPath+".json", nil)
	require.NoError(t, err)
	resp, err := anon.Do(req)
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	var live bytes.Buffer
	require.NoError(t, json.Indent(&live, body, "", "  "))
	spec, err := os.ReadFile(openAPIFile)
	require.NoError(t, err)
	require.Equal(t, string(spec), live.String()+"\n", "the running service serves openapi.json")

	// Сырой маршрут chi (Router().Get, Mount) и Hidden-операция huma обходят проверку Security и
	// openapi.json: каждый маршрут роутера — операция контракта или маршрут спецификации huma.
	var contract struct {
		Paths map[string]map[string]any `json:"paths"`
	}
	require.NoError(t, json.Unmarshal(spec, &contract))
	for _, route := range srv.Router().Routes() {
		for method := range route.Handlers {
			_, op := contract.Paths[route.Pattern][strings.ToLower(method)]
			require.True(t, op || method == http.MethodGet && humaSpecRoute(humaCfg, route.Pattern),
				"route %s %s is not in openapi.json: a raw chi route or a Hidden huma operation escapes the Security check and the API contract", method, route.Pattern)
		}
	}

	app.RequireStop()
	// Запросы в полёте держат всё, что стартовало раньше HTTP: сервер слушает последним и гасится первым.
	const httpHook = "/golibinfra/httpkit."
	require.Contains(t, hooks.start[len(hooks.start)-1], httpHook, "HTTP must start last (OnStart: %v)", hooks.start)
	require.Contains(t, hooks.stop[0], httpHook, "HTTP must stop first (OnStop: %v)", hooks.stop)
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
