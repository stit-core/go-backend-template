//go:build integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"

	"git.stit.tech/stit-core/golibinfra/httpkit"
	"git.stit.tech/stit-core/golibinfra/pgkit"
	"git.stit.tech/stit-core/golibinfra/testkit"

	"git.stit.tech/stit-core/go-backend-template/cmd/migrate/sources"
	"git.stit.tech/stit-core/go-backend-template/internal/domain/verification"
	"git.stit.tech/stit-core/go-backend-template/internal/testkit/repopath"
)

// serviceLog — что тест читает из лога сервиса: порядок хуков старта и остановки (кто добавил
// хук — caller — по событиям fx, которые logkit.FxLogger пишет уровнем DEBUG), request_id строк
// handler error (причина 5xx) и текст записей до маскирования — в нём не должно быть секретов.
type serviceLog struct {
	mu                         sync.Mutex
	start, stop, handlerErrors []string
	text                       []string
}

// Тексты событий хуков fxevent.SlogLogger, по которым serviceLog их узнаёт.
const (
	onStartExecuting = "OnStart hook executing"
	onStopExecuting  = "OnStop hook executing"
)

func (h *serviceLog) Handle(ctx context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.text = append(h.text, r.Message)
	r.Attrs(func(a slog.Attr) bool {
		h.text = append(h.text, a.String())
		return true
	})
	switch r.Message {
	case onStartExecuting:
		h.start = append(h.start, attr(r, "caller"))
	case onStopExecuting:
		h.stop = append(h.stop, attr(r, "caller"))
	case "handler error":
		h.handlerErrors = append(h.handlerErrors, httpkit.RequestIDFrom(ctx))
	}
	return nil
}

// logged5xx — причина 5xx с этим request_id дошла до лога; запись идёт из горутины сервера.
func (h *serviceLog) logged5xx(requestID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Contains(h.handlerErrors, requestID)
}

// leaked — секреты, которые встречаются в логе сервиса.
func (h *serviceLog) leaked(secrets []string) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var found []string
	for _, secret := range secrets {
		if slices.ContainsFunc(h.text, func(line string) bool { return strings.Contains(line, secret) }) {
			found = append(found, secret)
		}
	}
	return found
}

func (h *serviceLog) Enabled(context.Context, slog.Level) bool { return true }
func (h *serviceLog) WithGroup(string) slog.Handler            { return h }

func (h *serviceLog) WithAttrs(attrs []slog.Attr) slog.Handler {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, a := range attrs {
		h.text = append(h.text, a.String())
	}
	return h
}

// mailTap — токены писем сервиса: тест ищет их в логе.
type mailTap struct {
	verification.Mailer
	mu     sync.Mutex
	tokens []string
}

func (m *mailTap) SendPasswordReset(ctx context.Context, email, token string) error {
	m.add(token)
	return m.Mailer.SendPasswordReset(ctx, email, token)
}

func (m *mailTap) SendEmailVerification(ctx context.Context, email, token string) error {
	m.add(token)
	return m.Mailer.SendEmailVerification(ctx, email, token)
}

func (m *mailTap) add(token string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tokens = append(m.tokens, token)
}

func (m *mailTap) sent() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.tokens)
}

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
		// dev: лог-mailer и cookie без Secure — клиент теста ходит по http.
		"APP_ENV": "dev", "COOKIE_SECURE": "false", "OTEL_SDK_DISABLED": "true",
	} {
		t.Setenv(k, v)
	}

	logs, mail := &serviceLog{}, &mailTap{}
	var (
		srv        *httpkit.Server
		kitSources []pgkit.Source
	)
	app := fxtest.New(t, service,
		// Тройник лога сервиса: события fx доходят до logs, только если service пишет их в logkit.
		fx.Decorate(func(l *slog.Logger) *slog.Logger { return slog.New(slog.NewMultiHandler(l.Handler(), logs)) }),
		fx.Decorate(func(m verification.Mailer) verification.Mailer { mail.Mailer = m; return mail }),
		fx.Populate(&srv),
		fx.Invoke(fx.Annotate(func(s []pgkit.Source) { kitSources = s }, fx.ParamTags(`group:"`+pgkit.MigrationsGroup+`"`))))
	app.RequireStart()
	require.IsType(t, &sdkmetric.MeterProvider{}, otel.GetMeterProvider(),
		"telemetry.Module sets the global meter provider: without it every metric of the service is a no-op")

	// cmd/migrate — отдельный бинарь без графа сервиса: его список обязан совпасть с kit-таблицами графа.
	require.ElementsMatch(t, sourceNames(sources.Kits), sourceNames(kitSources),
		"sources.Kits (cmd/migrate) differs from the kit tables of the service graph (group %s)", pgkit.MigrationsGroup)

	require.NoError(t, checkHealth(t.Context(), srv.Addr().String()), "api healthcheck gets 200 from /health of the service")

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

	const email, password, wrongPassword = "boot@example.com", "password123", "wrong-password"
	require.Equal(t, http.StatusCreated, call(user, http.MethodPost, "/api/auth/register",
		`{"email":"`+email+`","password":"`+password+`","name":"Boot"}`), "register")
	require.Equal(t, http.StatusOK, call(user, http.MethodPost, "/api/auth/login",
		`{"email":"`+email+`","password":"`+password+`"}`), "login")
	require.Equal(t, http.StatusOK, call(user, http.MethodGet, "/api/users/me", ""), "/me with the session cookie")
	require.Equal(t, http.StatusNoContent, call(anon, http.MethodPost, "/api/auth/password-reset/request",
		`{"email":"`+email+`"}`), "password reset request")
	status := 0
	for range 6 {
		status = call(anon, http.MethodPost, "/api/auth/login", `{"email":"`+email+`","password":"`+wrongPassword+`"}`)
	}
	require.Equal(t, http.StatusTooManyRequests, status, "the brute-force guard locks the account after repeated failures")

	db := pg.Pool(t)
	require.Eventually(t, func() bool {
		var n int
		err := db.QueryRow(t.Context(), `SELECT count(*) FROM email_verifications ev
			JOIN users u ON u.id = ev.user_id WHERE u.email = $1`, email).Scan(&n)
		return err == nil && n == 1
	}, 15*time.Second, 100*time.Millisecond, "user.registered goes outbox → JetStream → verification subscriber")
	// Стрим публикатора старт не проверяет: без него строки outbox не получают PubAck.
	require.Eventually(t, func() bool {
		var n int
		err := db.QueryRow(t.Context(), `SELECT count(*) FROM outbox_verification
			WHERE subject LIKE 'verification.%' AND published_at IS NOT NULL`).Scan(&n)
		return err == nil && n > 0
	}, 15*time.Second, 100*time.Millisecond, "verification publishes through its own outbox into its stream")

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

	var contract struct {
		Paths map[string]map[string]struct {
			Security []map[string][]string `json:"security"`
		} `json:"paths"`
	}
	require.NoError(t, json.Unmarshal(spec, &contract))
	// Операция под Security без cookie — 401: аутентификация стоит на каждой, а не на одной /me.
	secured := 0
	for path, ops := range contract.Paths {
		for method, op := range ops {
			if len(op.Security) > 0 {
				secured++
				require.Equal(t, http.StatusUnauthorized, call(anon, strings.ToUpper(method), pathParam.ReplaceAllString(path, "00000000-0000-0000-0000-000000000001"), "{}"),
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

	// 5xx — без причины и с request_id, причина — в лог сервиса с тем же request_id.
	_, err = db.Exec(t.Context(), `ALTER TABLE users RENAME TO users_offline`)
	require.NoError(t, err)
	req, err = http.NewRequestWithContext(t.Context(), http.MethodGet, base+"/api/users/me", nil)
	require.NoError(t, err)
	resp, err = user.Do(req)
	require.NoError(t, err)
	var internal httpkit.APIError
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&internal))
	require.NoError(t, resp.Body.Close())
	_, err = db.Exec(t.Context(), `ALTER TABLE users_offline RENAME TO users`)
	require.NoError(t, err)
	require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	require.Equal(t, "INTERNAL", internal.Code)
	require.Empty(t, internal.Detail, "a 5xx carries no cause")
	require.NotEmpty(t, internal.RequestID)
	require.True(t, logs.logged5xx(internal.RequestID), "the cause is in the service log under the request_id of the response")

	// Адрес, пароли, cookie сессии и токены писем не попадают в лог даже до маскирования.
	require.Eventually(t, func() bool { return len(mail.sent()) == 2 }, 15*time.Second, 100*time.Millisecond,
		"the service mails the email verification and the password reset")
	baseURL, err := url.Parse(base)
	require.NoError(t, err)
	secrets := append(mail.sent(), email, password, wrongPassword)
	for _, c := range jar.Cookies(baseURL) {
		secrets = append(secrets, c.Value)
	}
	require.Len(t, secrets, 7, "two mail tokens, the address, two passwords, two session cookies")
	require.Empty(t, logs.leaked(secrets), "a secret reached the service log")

	app.RequireStop()
	const noHookEvents = "no %q events in the service log: service does not set logkit.FxLogger, or fx renamed this fxevent.SlogLogger message"
	require.NotEmpty(t, logs.start, noHookEvents, onStartExecuting)
	require.NotEmpty(t, logs.stop, noHookEvents, onStopExecuting)
	// Запросы в полёте держат всё, что стартовало раньше HTTP: сервер слушает последним и гасится первым.
	const httpServe = "git.stit.tech/stit-core/golibinfra/httpkit.serve"
	require.Equal(t, httpServe, logs.start[len(logs.start)-1], "HTTP must start last (OnStart: %v)", logs.start)
	require.Equal(t, httpServe, logs.stop[0], "HTTP must stop first (OnStop: %v)", logs.stop)
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
