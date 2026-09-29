//go:build integration

package main

import (
	"net/http"
	"net/http/cookiejar"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/fx/fxtest"

	"git.stit.tech/stit-core/golibinfra/httpkit"
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

// TestBoot поднимает сервис набором опций main поверх Postgres, NATS и Redis в контейнерах:
// конструкторы, Invoke и хуки выполняются по-настоящему, чего fx.ValidateApp не делает.
func TestBoot(t *testing.T) {
	pg := testkit.StartPostgres(t, testkit.WithMigrations(sources.Kits...), testkit.WithMigrationsDir(repopath.Migrations()))
	nats := testkit.StartNATS(t, testkit.WithJetStream())
	redis := testkit.StartRedis(t)
	for k, v := range map[string]string{
		"PG_DSN": pg.DSN, "NATS_URL": nats.URL, "REDIS_URL": redis.URL,
		"NATS_JETSTREAM_ENABLED": "true", "NATS_JS_STREAM_NAMES": "user,verification",
		"JWT_SECRET": strings.Repeat("s", 32), "HTTP_ADDR": "127.0.0.1:0", "LOG_SILENT": "true",
	} {
		t.Setenv(k, v)
	}

	hooks := &hookOrder{}
	var srv *httpkit.Server
	app := fxtest.New(t, service,
		fx.WithLogger(func() fxevent.Logger { return hooks }), fx.RecoverFromPanics(), fx.Populate(&srv))
	app.RequireStart()

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

	app.RequireStop()
	stops := func(kit string) int {
		i := slices.IndexFunc(hooks.stop, func(caller string) bool { return strings.Contains(caller, "/golibinfra/"+kit+".") })
		require.NotEqual(t, -1, i, "no OnStop hook of %s among %v", kit, hooks.stop)
		return i
	}
	for _, kit := range []string{"pgkit", "natskit", "cachekit"} {
		require.Less(t, stops("httpkit"), stops(kit), "HTTP stops before %s: requests in flight keep it (OnStop: %v)", kit, hooks.stop)
	}
}
