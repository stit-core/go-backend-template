//go:build integration

// Потоки доменов идентичности сквозь HTTP: файл в .template/identity — prune удаляет его вместе с ними.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"

	"git.stit.tech/stit-core/golibinfra/httpkit"
	"git.stit.tech/stit-core/golibinfra/natskit"
	"git.stit.tech/stit-core/golibinfra/telemetry"
	"git.stit.tech/stit-core/golibinfra/testkit/metrictest"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/verification"
	"git.stit.tech/stit-core/go-backend-template/internal/platform/authgate"
)

// requestLog — что тест читает из лога сервиса: request_id строк handler error (причина 5xx) и текст
// записей до маскирования — в нём не должно быть секретов.
type requestLog struct {
	mu                  sync.Mutex
	handlerErrors, text []string
}

func (h *requestLog) Handle(ctx context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.text = append(h.text, r.Message)
	r.Attrs(func(a slog.Attr) bool {
		h.text = append(h.text, a.String())
		return true
	})
	if r.Message == "handler error" {
		h.handlerErrors = append(h.handlerErrors, httpkit.RequestIDFrom(ctx))
	}
	return nil
}

func (h *requestLog) Enabled(context.Context, slog.Level) bool { return true }
func (h *requestLog) WithGroup(string) slog.Handler            { return h }

func (h *requestLog) WithAttrs(attrs []slog.Attr) slog.Handler {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, a := range attrs {
		h.text = append(h.text, a.String())
	}
	return h
}

// logged5xx — причина 5xx с этим request_id дошла до лога; запись идёт из горутины сервера.
func (h *requestLog) logged5xx(requestID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Contains(h.handlerErrors, requestID)
}

// leaked — секреты, которые встречаются в логе сервиса.
func (h *requestLog) leaked(secrets []string) []string {
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

// teeLog — тройник лога сервиса: записи доходят до h, только если service пишет их в logkit.
func teeLog(h slog.Handler) fx.Option {
	return fx.Decorate(func(l *slog.Logger) *slog.Logger { return slog.New(slog.NewMultiHandler(l.Handler(), h)) })
}

// mailTap — токены писем сервиса: тест гасит их, как получатель письма, и ищет в логе и таблицах.
type mailTap struct {
	verification.Mailer
	mu            sync.Mutex
	reset, verify []string
}

func (m *mailTap) SendPasswordReset(ctx context.Context, email, token string) error {
	m.mu.Lock()
	m.reset = append(m.reset, token)
	m.mu.Unlock()
	return m.Mailer.SendPasswordReset(ctx, email, token)
}

func (m *mailTap) SendEmailVerification(ctx context.Context, email, token string) error {
	m.mu.Lock()
	m.verify = append(m.verify, token)
	m.mu.Unlock()
	return m.Mailer.SendEmailVerification(ctx, email, token)
}

// sent — токены писем сброса и подтверждения адреса.
func (m *mailTap) sent() ([]string, []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.reset), slices.Clone(m.verify)
}

// TestIdentity — потоки доменов идентичности на графе main поверх Postgres и NATS в контейнерах:
// регистрация, вход и сессия, антибрутфорс, письма через outbox и подписчика, 5xx, секреты вне лога и
// таблиц, метрики доменов.
func TestIdentity(t *testing.T) {
	pg := serviceEnv(t)

	logs, mail, metrics := &requestLog{}, &mailTap{}, metrictest.NewProvider()
	var srv *httpkit.Server
	app := fxtest.New(t, service,
		teeLog(logs),
		fx.Decorate(func(m verification.Mailer) verification.Mailer { mail.Mailer = m; return mail }),
		// Провайдер метрик telemetry — провайдер теста: метрики доменов доходят до metrics, только если
		// домены пишут в провайдер, который экспортирует сигналы.
		fx.Decorate(func(p *telemetry.Providers) *telemetry.Providers {
			tapped := *p
			tapped.MeterProvider = metrics.MeterProvider
			return &tapped
		}),
		fx.Populate(&srv))
	app.RequireStart()
	defer app.RequireStop()

	base := "http://" + srv.Addr().String()
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	user := &http.Client{Jar: jar, Timeout: 10 * time.Second}
	anon := &http.Client{Timeout: 10 * time.Second}

	const email, password, wrongPassword = "boot@example.com", "password123", "wrong-password"
	require.Equal(t, http.StatusCreated, call(t, user, http.MethodPost, base+"/api/auth/register",
		`{"email":"`+email+`","password":"`+password+`","name":"Boot"}`), "register")
	require.Equal(t, http.StatusOK, call(t, user, http.MethodPost, base+"/api/auth/login",
		`{"email":"`+email+`","password":"`+password+`"}`), "login")
	require.Equal(t, http.StatusOK, call(t, user, http.MethodGet, base+"/api/users/me", ""), "/me with the session cookie")
	require.Equal(t, http.StatusNoContent, call(t, anon, http.MethodPost, base+"/api/auth/password-reset/request",
		`{"email":"`+email+`"}`), "password reset request")
	// Ключ антибрутфорса — адрес соединения: без HTTP_TRUST_PROXY_HOPS X-Forwarded-For его не меняет, и
	// клиент, подставляющий в каждый запрос свою цепочку адресов (длиннее любого разумного числа прокси),
	// получает блокировку, как все.
	status := 0
	for i := range 6 {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, base+"/api/auth/login",
			strings.NewReader(`{"email":"`+email+`","password":"`+wrongPassword+`"}`))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Forwarded-For", forwardedChain(i, 32))
		resp, err := anon.Do(req)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		status = resp.StatusCode
	}
	require.Equal(t, http.StatusTooManyRequests, status, "the brute-force guard locks the account after repeated failures, whatever X-Forwarded-For says")

	db := pg.Pool(t)
	require.Eventually(t, func() bool {
		var n int
		err := db.QueryRow(t.Context(), `SELECT count(*) FROM email_verifications ev
			JOIN users u ON u.id = ev.user_id WHERE u.email = $1`, email).Scan(&n)
		return err == nil && n == 1
	}, 15*time.Second, 100*time.Millisecond, "user.registered goes outbox → JetStream → verification subscriber")
	require.Eventually(t, func() bool {
		var n int
		err := db.QueryRow(t.Context(), `SELECT count(*) FROM outbox_verification
			WHERE subject LIKE 'verification.%' AND published_at IS NOT NULL`).Scan(&n)
		return err == nil && n > 0
	}, 15*time.Second, 100*time.Millisecond, "verification publishes through its own outbox into its stream")

	// 5xx — без причины и с request_id, причина — в лог сервиса с тем же request_id.
	_, err = db.Exec(t.Context(), `ALTER TABLE users RENAME TO users_offline`)
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, base+"/api/users/me", nil)
	require.NoError(t, err)
	resp, err := user.Do(req)
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

	require.Eventually(t, func() bool { resets, verifies := mail.sent(); return len(resets) == 1 && len(verifies) == 1 },
		15*time.Second, 100*time.Millisecond, "the service mails the email verification and the password reset")
	resets, verifies := mail.sent()
	// Получатель гасит обе ссылки: подтверждения пишут свой аудит.
	const newPassword = "new-password123"
	require.Equal(t, http.StatusNoContent, call(t, anon, http.MethodPost, base+"/api/auth/email-verify",
		`{"token":"`+verifies[0]+`"}`), "email verification")
	require.Equal(t, http.StatusNoContent, call(t, anon, http.MethodPost, base+"/api/auth/password-reset/confirm",
		`{"token":"`+resets[0]+`","password":"`+newPassword+`"}`), "password reset confirmation")

	// Адрес, пароли, cookie сессии и токены писем не попадают в лог даже до маскирования.
	baseURL, err := url.Parse(base)
	require.NoError(t, err)
	secrets := []string{resets[0], verifies[0], email, password, wrongPassword, newPassword}
	for _, c := range jar.Cookies(baseURL) {
		secrets = append(secrets, c.Value)
	}
	require.Len(t, secrets, 8, "two mail tokens, the address, three passwords, two session cookies")
	require.Empty(t, logs.leaked(secrets), "a secret reached the service log")
	// Их нет и в таблицах, которые хранят дольше лога и читают шире: аудит и outbox.
	for _, table := range []string{"audit_log", "outbox_user", "outbox_verification"} {
		require.Empty(t, storedLeaks(t, db, table, secrets), "a secret is stored in %s", table)
	}

	snap := metrics.Collect(t)
	require.InDelta(t, 1, snap.Get(t, "biz_auth_login_attempts_total", "result", "ok").Value, 0,
		"the auth domain writes its metrics to the meter provider of telemetry")
	require.InDelta(t, 1, snap.Get(t, "biz_verification_password_reset_requested_total", "result", "ok").Value, 0,
		"the verification domain writes its metrics to the meter provider of telemetry")
}

// forwardedChain — цепочка X-Forwarded-For запроса n из hops своих записей IPv4 (IPv6 одной /64 guard
// считает одним адресом): при доверии к любому числу прокси до hops ключ антибрутфорса менялся бы от
// запроса к запросу.
func forwardedChain(n, hops int) string {
	entries := make([]string, hops)
	for i := range entries {
		entries[i] = fmt.Sprintf("198.18.%d.%d", n+1, i+1)
	}
	return strings.Join(entries, ", ")
}

// Стрим домена-публикатора — условие старта: без него строки его outbox копились бы, не получая PubAck.
func TestIdentity_PublisherStreamRequired(t *testing.T) {
	serviceEnv(t)
	t.Setenv("NATS_JS_STREAM_NAMES", "user")
	app := fx.New(service)
	require.NoError(t, app.Err())

	err := app.Start(t.Context())
	require.ErrorIs(t, err, natskit.ErrNoStream)
	require.ErrorContains(t, err, "instance verification")
}

// storedLeaks — секреты, которые встречаются в строках таблицы table (строка целиком, как текст).
func storedLeaks(t *testing.T, db *pgxpool.Pool, table string, secrets []string) []string {
	t.Helper()
	rows, err := db.Query(t.Context(), "SELECT t::text FROM "+table+" t")
	require.NoError(t, err)
	stored, err := pgx.CollectRows(rows, pgx.RowTo[string])
	require.NoError(t, err)
	require.NotEmpty(t, stored, "%s is empty: nothing to check", table)
	var found []string
	for _, secret := range secrets {
		if slices.ContainsFunc(stored, func(row string) bool { return strings.Contains(row, secret) }) {
			found = append(found, secret)
		}
	}
	return found
}

// sessionAPI — клиент без cookie jar: cookie сессии тест держит сам и предъявляет их повторно.
type sessionAPI struct {
	t      *testing.T
	base   string
	client *http.Client
}

// call шлёт запрос с cookie (nil — без них) и отдаёт статус, тело и выданные cookie.
func (a sessionAPI) call(method, path, body string, cookies map[string]string) (int, []byte, map[string]string) {
	a.t.Helper()
	req, err := http.NewRequestWithContext(a.t.Context(), method, a.base+path, strings.NewReader(body))
	require.NoError(a.t, err)
	req.Header.Set("Content-Type", "application/json")
	for name, value := range cookies {
		req.AddCookie(&http.Cookie{Name: name, Value: value})
	}
	resp, err := a.client.Do(req)
	require.NoError(a.t, err)
	data, err := io.ReadAll(resp.Body)
	require.NoError(a.t, err)
	require.NoError(a.t, resp.Body.Close())
	got := map[string]string{}
	for _, c := range resp.Cookies() {
		got[c.Name] = c.Value
	}
	return resp.StatusCode, data, got
}

// register заводит пользователя и отдаёт его ID.
func (a sessionAPI) register(email string) string {
	a.t.Helper()
	status, body, _ := a.call(http.MethodPost, "/api/auth/register", `{"email":"`+email+`","password":"`+sessionPassword+`","name":"S"}`, nil)
	require.Equal(a.t, http.StatusCreated, status, "register %s: %s", email, body)
	var created struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(a.t, json.Unmarshal(body, &created))
	return created.Data.ID
}

// login открывает сессию и отдаёт её cookie.
func (a sessionAPI) login(email, password string) map[string]string {
	a.t.Helper()
	status, body, cookies := a.call(http.MethodPost, "/api/auth/login", `{"email":"`+email+`","password":"`+password+`"}`, nil)
	require.Equal(a.t, http.StatusOK, status, "login %s: %s", email, body)
	require.Len(a.t, cookies[authgate.CookieRefresh], 64, "the refresh token is an opaque authnkit token")
	return cookies
}

// refresh обновляет сессию cookie и отдаёт статус и новые cookie.
func (a sessionAPI) refresh(cookies map[string]string) (int, map[string]string) {
	a.t.Helper()
	status, _, got := a.call(http.MethodPost, "/api/auth/refresh", "", map[string]string{authgate.CookieRefresh: cookies[authgate.CookieRefresh]})
	return status, got
}

// refreshCode — refresh одним cookie из своей горутины (без require): "" — 204, иначе code ответа-ошибки.
func refreshCode(t *testing.T, api sessionAPI, refresh string) (string, error) {
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, api.base+"/api/auth/refresh", http.NoBody)
	if err != nil {
		return "", err
	}
	req.AddCookie(&http.Cookie{Name: authgate.CookieRefresh, Value: refresh})
	resp, err := api.client.Do(req)
	if err != nil {
		return "", err
	}
	body, err := io.ReadAll(resp.Body)
	if err := errors.Join(err, resp.Body.Close()); err != nil {
		return "", err
	}
	if resp.StatusCode == http.StatusNoContent {
		return "", nil
	}
	var apiErr httpkit.APIError
	if err := json.Unmarshal(body, &apiErr); err != nil {
		return "", err
	}
	return apiErr.Code, nil
}

const sessionPassword = "password123"

// errorCode — code ответа-ошибки.
func errorCode(t *testing.T, body []byte) string {
	t.Helper()
	var apiErr httpkit.APIError
	require.NoError(t, json.Unmarshal(body, &apiErr), "error body: %s", body)
	return apiErr.Code
}

// openSessions — незакрытые сессии пользователя в хранилище кита.
func openSessions(t *testing.T, db *pgxpool.Pool, userID string) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRow(t.Context(), `SELECT count(*) FROM authn_sessions WHERE subject = $1 AND revoked_at IS NULL`, userID).Scan(&n))
	return n
}

// TestSessions — сессии refresh сквозь HTTP на графе сервиса и хранилище кита: ротация, повтор токена
// закрывает сессию, выход закрывает свою, а смена и сброс пароля, блокировка и удаление — все сессии
// пользователя.
func TestSessions(t *testing.T) {
	pg := serviceEnv(t)
	mail := &mailTap{}
	var srv *httpkit.Server
	app := fxtest.New(t, service,
		fx.Decorate(func(m verification.Mailer) verification.Mailer { mail.Mailer = m; return mail }),
		fx.Populate(&srv))
	app.RequireStart()
	defer app.RequireStop()
	api := sessionAPI{t: t, base: "http://" + srv.Addr().String(), client: &http.Client{Timeout: 10 * time.Second}}
	db := pg.Pool(t)

	const alice = "alice@example.com"
	aliceID := api.register(alice)

	t.Run("refresh rotates the token, a reuse closes the session", func(t *testing.T) {
		first := api.login(alice, sessionPassword)
		status, rotated := api.refresh(first)
		require.Equal(t, http.StatusNoContent, status)
		require.NotEqual(t, first[authgate.CookieRefresh], rotated[authgate.CookieRefresh])

		status, _ = api.refresh(first)
		require.Equal(t, http.StatusUnauthorized, status, "a reused token")
		status, _ = api.refresh(rotated)
		require.Equal(t, http.StatusUnauthorized, status, "the reuse closed the session with its next token")
		var reused int
		require.NoError(t, db.QueryRow(t.Context(), `SELECT count(*) FROM audit_log WHERE action = 'refresh_reused' AND actor_id = $1`, aliceID).Scan(&reused))
		require.Equal(t, 1, reused, "the reuse is in the audit of the account")
	})

	// Один ящик — одна учётная запись: адрес с пробелом по краям регистрация отвергает, а вход находит по нему
	// ту же учётную запись — guard входа тоже считает их одним субъектом.
	t.Run("a padded address is the same mailbox", func(t *testing.T) {
		for _, padded := range []string{" " + alice, alice + " ", "\t" + alice} {
			body, err := json.Marshal(map[string]string{"email": padded, "password": sessionPassword, "name": "Twin"})
			require.NoError(t, err)
			status, resp, _ := api.call(http.MethodPost, "/api/auth/register", string(body), nil)
			require.Equal(t, http.StatusUnprocessableEntity, status, "register %q: %s", padded, resp)
			require.Equal(t, "USER_INVALID_EMAIL", errorCode(t, resp))
		}
		var accounts int
		require.NoError(t, db.QueryRow(t.Context(), `SELECT count(*) FROM users WHERE lower(btrim(email, E' \t')) = $1`, alice).Scan(&accounts))
		require.Equal(t, 1, accounts)

		before := openSessions(t, db, aliceID)
		api.login(" "+alice+" ", sessionPassword)
		require.Equal(t, before+1, openSessions(t, db, aliceID), "the padded address logs into the same account")
	})

	t.Run("logout closes its session", func(t *testing.T) {
		other := api.login(alice, sessionPassword)
		current := api.login(alice, sessionPassword)
		status, _, _ := api.call(http.MethodPost, "/api/auth/logout", "", current)
		require.Equal(t, http.StatusNoContent, status)

		status, _ = api.refresh(current)
		require.Equal(t, http.StatusUnauthorized, status)
		status, _ = api.refresh(other)
		require.Equal(t, http.StatusNoContent, status, "the other session lives on")

		status, _, _ = api.call(http.MethodPost, "/api/auth/logout", "", current)
		require.Equal(t, http.StatusNoContent, status, "a closed session logs out as well")
		var logouts, owned int
		require.NoError(t, db.QueryRow(t.Context(),
			`SELECT count(*), count(*) FILTER (WHERE actor_id = $1 AND resource_id = $1) FROM audit_log WHERE action = 'logout'`, aliceID).Scan(&logouts, &owned))
		require.Equal(t, 1, logouts, "a logout that closed no session leaves no audit record")
		require.Equal(t, 1, owned, "the audit names the owner of the session the logout closed")
	})

	// Погашение токена атомарно: из параллельных обновлений одним cookie успешно одно, остальные — повтор, и
	// сессия закрыта (поэтому клиент обновляет сессию последовательно).
	t.Run("parallel refreshes with one token: one wins, the rest are reuse", func(t *testing.T) {
		const erin = "erin@example.com"
		erinID := api.register(erin)
		c := api.login(erin, sessionPassword)
		const n = 8
		codes, errs := make([]string, n), make([]error, n)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := range n {
			wg.Go(func() {
				<-start
				codes[i], errs[i] = refreshCode(t, api, c[authgate.CookieRefresh])
			})
		}
		close(start)
		wg.Wait()
		require.NoError(t, errors.Join(errs...))
		won, reused := 0, 0
		for _, code := range codes {
			switch code {
			case "":
				won++
			case "AUTH_REFRESH_REUSED":
				reused++
			}
		}
		require.Equal(t, 1, won, "codes: %v", codes)
		require.Equal(t, n-1, reused, "codes: %v", codes)
		require.Zero(t, openSessions(t, db, erinID), "the reuse closed the session, the winner's token included")
	})

	// Пароль и закрытие сессий — одна транзакция: user закрывает их своим портом Sessions на хранилище кита,
	// и сбой закрытия откатывает смену — прежний пароль и сессия живы.
	const newPassword = "new-password123"
	t.Run("a failed revocation rolls back the password change", func(t *testing.T) {
		const frank = "frank@example.com"
		frankID := api.register(frank)
		c := api.login(frank, sessionPassword)
		_, err := db.Exec(t.Context(), `ALTER TABLE authn_sessions RENAME TO authn_sessions_offline`)
		require.NoError(t, err)
		// Имя возвращает и упавший тест: иначе следующие подтесты упали бы без таблицы сессий.
		restore := func() {
			_, err := db.Exec(context.Background(), `ALTER TABLE IF EXISTS authn_sessions_offline RENAME TO authn_sessions`)
			require.NoError(t, err)
		}
		t.Cleanup(restore)
		status, _, _ := api.call(http.MethodPost, "/api/auth/change-password",
			`{"user_id":"`+frankID+`","old_password":"`+sessionPassword+`","new_password":"`+newPassword+`"}`, c)
		restore()
		require.Equal(t, http.StatusInternalServerError, status)

		require.Equal(t, 1, openSessions(t, db, frankID), "the session outlives the failed change")
		status, _ = api.refresh(c)
		require.Equal(t, http.StatusNoContent, status)
		api.login(frank, sessionPassword)
	})

	// NUL Postgres в TEXT не хранит: guard такой адрес ключом не берёт, и вход с ним — неверные учётные
	// данные, а не сбой записи попытки.
	t.Run("login with NUL in the address is a wrong login", func(t *testing.T) {
		status, body, _ := api.call(http.MethodPost, "/api/auth/login", `{"email":"a\u0000@example.com","password":"`+sessionPassword+`"}`, nil)
		require.Equal(t, http.StatusUnauthorized, status, "login: %s", body)
		require.Equal(t, "AUTH_INVALID_CREDENTIALS", errorCode(t, body))
	})

	// Предел пароля у хэшера — в байтах, maxLength схемы — в символах: 1024 символа не из ASCII схему
	// проходят и получают отказ ввода, а не сбой; на входе и в старом пароле такой пароль — неверный.
	overLimit := strings.Repeat("я", 1024)
	t.Run("a password over the hasher limit is an input error", func(t *testing.T) {
		status, body, _ := api.call(http.MethodPost, "/api/auth/register", `{"email":"long@example.com","password":"`+overLimit+`","name":"L"}`, nil)
		require.Equal(t, http.StatusUnprocessableEntity, status, "register: %s", body)
		require.Equal(t, "USER_PASSWORD_TOO_LONG", errorCode(t, body))
		status, body, _ = api.call(http.MethodPost, "/api/auth/register", `{"email":"long@example.com","password":"`+overLimit+`я","name":"L"}`, nil)
		require.Equal(t, http.StatusUnprocessableEntity, status, "register over maxLength: %s", body)
		require.Equal(t, "VALIDATION_FAILED", errorCode(t, body), "the schema refuses 1025 characters before the usecase")

		c := api.login(alice, sessionPassword)
		before := openSessions(t, db, aliceID)
		change := func(oldPassword, newPassword string) (int, string) {
			status, body, _ := api.call(http.MethodPost, "/api/auth/change-password",
				`{"user_id":"`+aliceID+`","old_password":"`+oldPassword+`","new_password":"`+newPassword+`"}`, c)
			return status, errorCode(t, body)
		}
		status, code := change(sessionPassword, overLimit)
		require.Equal(t, http.StatusUnprocessableEntity, status)
		require.Equal(t, "AUTH_PASSWORD_TOO_LONG", code)
		status, code = change(overLimit, newPassword)
		require.Equal(t, http.StatusUnprocessableEntity, status)
		require.Equal(t, "AUTH_INVALID_OLD_PASSWORD", code, "an old password over the limit is a wrong one")

		status, body, _ = api.call(http.MethodPost, "/api/auth/login", `{"email":"`+alice+`","password":"`+overLimit+`"}`, nil)
		require.Equal(t, http.StatusUnauthorized, status, "login: %s", body)
		require.Equal(t, before, openSessions(t, db, aliceID), "the refusals neither closed nor opened a session")
	})

	t.Run("password change closes every session", func(t *testing.T) {
		first, second := api.login(alice, sessionPassword), api.login(alice, sessionPassword)
		status, body, _ := api.call(http.MethodPost, "/api/auth/change-password",
			`{"user_id":"`+aliceID+`","old_password":"`+sessionPassword+`","new_password":"`+newPassword+`"}`, first)
		require.Equal(t, http.StatusNoContent, status, "change password: %s", body)

		for _, c := range []map[string]string{first, second} {
			status, _ = api.refresh(c)
			require.Equal(t, http.StatusUnauthorized, status)
		}
		require.Zero(t, openSessions(t, db, aliceID))
	})

	t.Run("password reset closes every session", func(t *testing.T) {
		c := api.login(alice, newPassword)
		status, _, _ := api.call(http.MethodPost, "/api/auth/password-reset/request", `{"email":"`+alice+`"}`, nil)
		require.Equal(t, http.StatusNoContent, status)
		resets, _ := mail.sent()
		require.Len(t, resets, 1, "the reset is mailed")
		status, body, _ := api.call(http.MethodPost, "/api/auth/password-reset/confirm", `{"token":"`+resets[0]+`","password":"`+overLimit+`"}`, nil)
		require.Equal(t, http.StatusUnprocessableEntity, status, "confirm reset over the hasher limit: %s", body)
		require.Equal(t, "VERIFICATION_PASSWORD_TOO_LONG", errorCode(t, body))
		status, body, _ = api.call(http.MethodPost, "/api/auth/password-reset/confirm", `{"token":"`+resets[0]+`","password":"reset-password123"}`, nil)
		require.Equal(t, http.StatusNoContent, status, "the refused password did not spend the token: %s", body)

		status, _ = api.refresh(c)
		require.Equal(t, http.StatusUnauthorized, status)
		require.Zero(t, openSessions(t, db, aliceID))
	})

	t.Run("blocking closes every session, the blocked does not log in", func(t *testing.T) {
		const admin, carol = "admin@example.com", "carol@example.com"
		adminID := api.register(admin)
		_, err := db.Exec(t.Context(), `UPDATE users SET role = 'admin' WHERE id = $1`, adminID)
		require.NoError(t, err)
		adminCookies := api.login(admin, sessionPassword)
		carolID := api.register(carol)
		c := api.login(carol, sessionPassword)
		var version int
		require.NoError(t, db.QueryRow(t.Context(), `SELECT version FROM users WHERE id = $1`, carolID).Scan(&version))

		status, body, _ := api.call(http.MethodPatch, "/api/users/"+carolID, `{"status":"blocked","version":`+strconv.Itoa(version)+`}`, adminCookies)
		require.Equal(t, http.StatusOK, status, "block: %s", body)
		status, _ = api.refresh(c)
		require.Equal(t, http.StatusUnauthorized, status)
		require.Zero(t, openSessions(t, db, carolID))
		status, _, _ = api.call(http.MethodPost, "/api/auth/login", `{"email":"`+carol+`","password":"`+sessionPassword+`"}`, nil)
		require.Equal(t, http.StatusUnauthorized, status, "the blocked does not log in")
	})

	t.Run("deletion closes every session", func(t *testing.T) {
		const dave = "dave@example.com"
		daveID := api.register(dave)
		c := api.login(dave, sessionPassword)

		status, body, _ := api.call(http.MethodDelete, "/api/users/"+daveID, "", c)
		require.Equal(t, http.StatusNoContent, status, "delete: %s", body)
		status, _ = api.refresh(c)
		require.Equal(t, http.StatusUnauthorized, status)
		require.Zero(t, openSessions(t, db, daveID))
	})
}

// loginCookie — вход из своей горутины (без require): статус и выданный refresh-cookie.
func loginCookie(ctx context.Context, api sessionAPI, email, password string) (int, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, api.base+"/api/auth/login",
		strings.NewReader(`{"email":"`+email+`","password":"`+password+`"}`))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := api.client.Do(req)
	if err != nil {
		return 0, "", err
	}
	_, err = io.Copy(io.Discard, resp.Body)
	if err := errors.Join(err, resp.Body.Close()); err != nil {
		return 0, "", err
	}
	for _, c := range resp.Cookies() {
		if c.Name == authgate.CookieRefresh {
			return resp.StatusCode, c.Value, nil
		}
	}
	return resp.StatusCode, "", nil
}

// Вход старым паролем, который идёт в момент смены пароля, сессии не получает: проверка пароля долгая
// (Argon2id), и сессия, открытая после RevokeAll смены, пережила бы её. Входы крутятся до и после смены:
// гонка случилась (вход начался до ответа смены и кончился после него), 5xx нет, ни один вход, ответивший
// 200 после смены, не держит живую сессию, и открытых сессий пользователя после смены нет.
func TestSessions_LoginDuringPasswordChange(t *testing.T) {
	pg := serviceEnv(t)
	// Все запросы теста — с одного IP: лимитер httpkit не должен глушить гонку.
	t.Setenv("HTTP_RATE_LIMIT_RPS", "100000")
	t.Setenv("HTTP_RATE_LIMIT_BURST", "100000")
	var srv *httpkit.Server
	app := fxtest.New(t, service, fx.Populate(&srv))
	app.RequireStart()
	defer app.RequireStop()
	api := sessionAPI{t: t, base: "http://" + srv.Addr().String(), client: &http.Client{Timeout: 30 * time.Second}}

	const email, newPassword = "race@example.com", "new-password123"
	id := api.register(email)
	victim := api.login(email, sessionPassword)

	var (
		stop, changed atomic.Bool
		straddled     atomic.Int64 // входы, начатые до ответа смены и законченные после него
		mu            sync.Mutex
		late          []string // refresh-cookie входов старым паролем, ответивших 200 после смены
		errs          []error
		wg            sync.WaitGroup
	)
	for range 4 {
		wg.Go(func() {
			for !stop.Load() {
				before := !changed.Load()
				status, refresh, err := loginCookie(t.Context(), api, email, sessionPassword)
				after := changed.Load()
				if before && after {
					straddled.Add(1)
				}
				mu.Lock()
				switch {
				case err != nil:
					errs = append(errs, err)
				case status >= http.StatusInternalServerError:
					errs = append(errs, fmt.Errorf("login answered %d", status))
				case status == http.StatusOK && after:
					late = append(late, refresh)
				}
				mu.Unlock()
			}
		})
	}
	time.Sleep(500 * time.Millisecond)
	status, body, _ := api.call(http.MethodPost, "/api/auth/change-password",
		`{"user_id":"`+id+`","old_password":"`+sessionPassword+`","new_password":"`+newPassword+`"}`, victim)
	changed.Store(true)
	require.Equal(t, http.StatusNoContent, status, "change password: %s", body)
	time.Sleep(1500 * time.Millisecond)
	stop.Store(true)
	wg.Wait()

	require.NoError(t, errors.Join(errs...))
	require.Positive(t, straddled.Load(), "the race happened: old-password logins in flight across the change")
	require.Zero(t, openSessions(t, pg.Pool(t), id), "no session of the old password outlives the change")
	for _, refresh := range late {
		code, err := refreshCode(t, api, refresh)
		require.NoError(t, err)
		require.NotEmpty(t, code, "an old-password login in flight during the change keeps a live session")
	}
}
