//go:build integration

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"

	"git.stit.tech/stit-core/golibinfra/httpkit"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/verification"
	"git.stit.tech/stit-core/go-backend-template/internal/platform/authgate"
)

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

	t.Run("logout closes its session", func(t *testing.T) {
		other := api.login(alice, sessionPassword)
		current := api.login(alice, sessionPassword)
		status, _, _ := api.call(http.MethodPost, "/api/auth/logout", "", current)
		require.Equal(t, http.StatusNoContent, status)

		status, _ = api.refresh(current)
		require.Equal(t, http.StatusUnauthorized, status)
		status, _ = api.refresh(other)
		require.Equal(t, http.StatusNoContent, status, "the other session lives on")
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
		status, _, _ := api.call(http.MethodPost, "/api/auth/change-password",
			`{"user_id":"`+frankID+`","old_password":"`+sessionPassword+`","new_password":"`+newPassword+`"}`, c)
		_, err = db.Exec(t.Context(), `ALTER TABLE authn_sessions_offline RENAME TO authn_sessions`)
		require.NoError(t, err)
		require.Equal(t, http.StatusInternalServerError, status)

		require.Equal(t, 1, openSessions(t, db, frankID), "the session outlives the failed change")
		status, _ = api.refresh(c)
		require.Equal(t, http.StatusNoContent, status)
		api.login(frank, sessionPassword)
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
		status, body, _ := api.call(http.MethodPost, "/api/auth/password-reset/confirm", `{"token":"`+resets[0]+`","password":"reset-password123"}`, nil)
		require.Equal(t, http.StatusNoContent, status, "confirm reset: %s", body)

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
