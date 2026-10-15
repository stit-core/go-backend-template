package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/auth"
	authhttp "git.stit.tech/stit-core/go-backend-template/internal/domain/auth/delivery/http"
	"git.stit.tech/stit-core/go-backend-template/internal/domain/auth/delivery/http/mocks"
	"git.stit.tech/stit-core/go-backend-template/internal/platform/authgate"
	"git.stit.tech/stit-core/go-backend-template/internal/platform/identity"
	"git.stit.tech/stit-core/go-backend-template/internal/testkit/identitytest"
	"git.stit.tech/stit-core/go-backend-template/pkg/policy"
	"git.stit.tech/stit-core/golibinfra/authnkit"
	"git.stit.tech/stit-core/golibinfra/testkit/handlertest"
)

// setup — API auth с cookie по COOKIE_SECURE=true, как вне dev.
func setup(t *testing.T, uc *mocks.MockUsecase) handlertest.API {
	t.Helper()
	return handlertest.NewAPI(t, func(api huma.API) {
		authhttp.Register(api, authhttp.NewHandler(uc, authgate.CookieConfig{Secure: true}))
	}, handlertest.WithSecuritySchemes(authgate.CookieScheme(identitytest.Extract)))
}

// callerIs — ctx usecase несёт identity вызывающего: по ней решает policy.
func callerIs(userID string) any {
	return mock.MatchedBy(func(ctx context.Context) bool {
		id, ok := identity.From(ctx)
		return ok && id.UserID == userID
	})
}

// cookies — cookie из Set-Cookie ответа по имени.
func cookies(resp *httptest.ResponseRecorder) map[string]*http.Cookie {
	out := map[string]*http.Cookie{}
	for _, c := range (&http.Response{Header: resp.Header()}).Cookies() {
		out[c.Name] = c
	}
	return out
}

// requireSession — ответ выдал cookie сессии с этими токенами и Secure по конфигу.
func requireSession(t *testing.T, resp *httptest.ResponseRecorder, access, refresh string) {
	t.Helper()
	got := cookies(resp)
	require.Len(t, got, 2)
	for name, value := range map[string]string{authgate.CookieAccess: access, authgate.CookieRefresh: refresh} {
		require.Contains(t, got, name)
		require.Equal(t, value, got[name].Value, name)
		require.True(t, got[name].Secure, "%s follows COOKIE_SECURE", name)
	}
}

// userView — пользователь в ответе входа глазами клиента; поле вне него — провал разбора (хэш пароля).
type userView struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role"`
}

func data(t *testing.T, resp *httptest.ResponseRecorder) userView {
	t.Helper()
	var body struct {
		Data json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	dec := json.NewDecoder(bytes.NewReader(body.Data))
	dec.DisallowUnknownFields()
	var v userView
	require.NoError(t, dec.Decode(&v))
	return v
}

func TestLogin_HappyPath(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Login(mock.Anything, &auth.LoginInput{Email: "a@b.com", Password: "pass1234"}).Return(&auth.LoginOutput{
		AccessToken: "access-token", RefreshToken: "refresh-token",
		User: &auth.User{ID: "u1", Email: "a@b.com", PasswordHash: "hash:pass1234", Name: "Alice", CanLogin: true, Role: identity.RoleClient, Version: 3},
	}, nil).Once()
	api := setup(t, uc)

	resp := api.Post("/api/auth/login", strings.NewReader(`{"email":"a@b.com","password":"pass1234"}`))
	handlertest.AssertStatus(t, resp, http.StatusOK)
	require.Equal(t, userView{ID: "u1", Email: "a@b.com", Name: "Alice", Role: "client"}, data(t, resp), "the user goes in the data envelope")
	requireSession(t, resp, "access-token", "refresh-token")
}

func TestLogin_InvalidCredentials(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Login(mock.Anything, mock.Anything).Return(nil, auth.ErrInvalidCredentials).Once()
	api := setup(t, uc)

	resp := api.Post("/api/auth/login", strings.NewReader(`{"email":"a@b.com","password":"x"}`))
	handlertest.AssertAPIError(t, resp, http.StatusUnauthorized, "AUTH_INVALID_CREDENTIALS")
}

// Блокировка входа говорит, когда повторить: Retry-After — из ожидания блокировки.
func TestLogin_AccountLocked(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Login(mock.Anything, mock.Anything).Return(nil, &auth.LockedError{Wait: 90 * time.Second}).Once()
	api := setup(t, uc)

	resp := api.Post("/api/auth/login", strings.NewReader(`{"email":"a@b.com","password":"x"}`))
	handlertest.AssertAPIError(t, resp, http.StatusTooManyRequests, "AUTH_ACCOUNT_LOCKED")
	require.Equal(t, "90", resp.Header().Get("Retry-After"))
}

func TestRefresh_HappyPath(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Refresh(mock.Anything, "old-refresh").
		Return(&auth.LoginOutput{AccessToken: "new-access", RefreshToken: "new-refresh", User: &auth.User{ID: "u1"}}, nil).Once()
	api := setup(t, uc)

	resp := api.Do(http.MethodPost, "/api/auth/refresh", "Cookie: refresh_token=old-refresh")
	handlertest.AssertStatus(t, resp, http.StatusNoContent)
	require.Empty(t, resp.Body.String())
	requireSession(t, resp, "new-access", "new-refresh")
}

// Без cookie решает usecase, как о любом неизвестном токене.
func TestRefresh_NoCookie(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Refresh(mock.Anything, "").Return(nil, auth.ErrRefreshInvalid).Once()
	api := setup(t, uc)

	resp := api.Post("/api/auth/refresh", strings.NewReader(""))
	handlertest.AssertAPIError(t, resp, http.StatusUnauthorized, "AUTH_REFRESH_INVALID")
}

func TestRefresh_Expired(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Refresh(mock.Anything, "expired").Return(nil, auth.ErrRefreshExpired).Once()
	api := setup(t, uc)

	resp := api.Do(http.MethodPost, "/api/auth/refresh", "Cookie: refresh_token=expired")
	handlertest.AssertAPIError(t, resp, http.StatusUnauthorized, "AUTH_REFRESH_EXPIRED")
}

// Битый refresh — «войти заново», а не 500.
func TestRefresh_Invalid(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Refresh(mock.Anything, "broken").Return(nil, auth.ErrRefreshInvalid).Once()
	api := setup(t, uc)

	resp := api.Do(http.MethodPost, "/api/auth/refresh", "Cookie: refresh_token=broken")
	handlertest.AssertAPIError(t, resp, http.StatusUnauthorized, "AUTH_REFRESH_INVALID")
}

func TestRefresh_Reused(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Refresh(mock.Anything, "reused").Return(nil, auth.ErrRefreshReused).Once()
	api := setup(t, uc)

	resp := api.Do(http.MethodPost, "/api/auth/refresh", "Cookie: refresh_token=reused")
	handlertest.AssertAPIError(t, resp, http.StatusUnauthorized, "AUTH_REFRESH_REUSED")
}

// requireLoggedOut — ответ стирает обе cookie сессии в браузере.
func requireLoggedOut(t *testing.T, resp *httptest.ResponseRecorder) {
	t.Helper()
	got := cookies(resp)
	require.Len(t, got, 2)
	for _, name := range []string{authgate.CookieAccess, authgate.CookieRefresh} {
		require.Contains(t, got, name)
		require.Empty(t, got[name].Value, name)
		require.Negative(t, got[name].MaxAge, "%s is dropped by the browser", name)
		require.True(t, got[name].Secure, "%s follows COOKIE_SECURE", name)
	}
}

func TestLogout_WithCookie(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Logout(mock.Anything, "refresh-token").Return(nil).Once()
	api := setup(t, uc)

	resp := api.Do(http.MethodPost, "/api/auth/logout", "Cookie: refresh_token=refresh-token")
	handlertest.AssertStatus(t, resp, http.StatusNoContent)
	requireLoggedOut(t, resp)
}

// Выход без cookie — тоже выход: usecase не зовётся (строгий мок), cookie стираются.
func TestLogout_NoCookie(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Logout(mock.Anything, "").Return(nil).Once()
	api := setup(t, uc)

	resp := api.Post("/api/auth/logout", strings.NewReader(""))
	handlertest.AssertStatus(t, resp, http.StatusNoContent)
	requireLoggedOut(t, resp)
}

// 5xx — без причины: клиенту код и request_id, причина — в лог сервера с тем же request_id.
func TestLogout_StoreFailure(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Logout(mock.Anything, "refresh-token").Return(errors.New("revoke session: connection refused")).Once()
	api := setup(t, uc)

	resp := api.Do(http.MethodPost, "/api/auth/logout", "Cookie: refresh_token=refresh-token")
	handlertest.AssertAPIError(t, resp, http.StatusInternalServerError, "INTERNAL")
	require.Empty(t, handlertest.DecodeAPIError(t, resp).Detail)
	require.NotContains(t, resp.Body.String(), "connection refused")
}

func TestChangePassword_HappyPath(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().ChangePassword(callerIs("u1"), "u1", "old-password", "new-password1").Return(nil).Once()
	api := setup(t, uc)

	resp := api.PostCtx(identitytest.Client("u1"), "/api/auth/change-password", strings.NewReader(changePasswordBody("new-password1")))
	handlertest.AssertStatus(t, resp, http.StatusNoContent)
	require.Empty(t, resp.Body.String())
	requireLoggedOut(t, resp)
}

// Операция под Security: запрос без сессии отвечает 401 до usecase — строгий мок без ожиданий.
func TestChangePassword_Unauthenticated(t *testing.T) {
	t.Parallel()
	api := setup(t, mocks.NewMockUsecase(t))

	resp := api.Post("/api/auth/change-password", strings.NewReader(changePasswordBody("new-password1")))
	handlertest.AssertAPIError(t, resp, http.StatusUnauthorized, "UNAUTHORIZED")
}

func TestChangePassword_Errors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"not self, not admin", &policy.ForbiddenError{Reason: "not self"}, http.StatusForbidden, "FORBIDDEN"},
		{"unknown user", auth.ErrUserNotFound, http.StatusNotFound, "AUTH_USER_NOT_FOUND"},
		{"concurrent update", auth.ErrConcurrentUpdate, http.StatusConflict, "AUTH_CONCURRENT_UPDATE"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			uc := mocks.NewMockUsecase(t)
			uc.EXPECT().ChangePassword(callerIs("u1"), "u1", "old-password", mock.Anything).Return(tc.err).Once()
			api := setup(t, uc)

			resp := api.PostCtx(identitytest.Client("u1"), "/api/auth/change-password",
				strings.NewReader(changePasswordBody(strings.Repeat("p", policy.MinPasswordLen))))
			handlertest.AssertAPIError(t, resp, tc.status, tc.code)
		})
	}
}

// Неверный старый пароль — ошибка поля, а не 401: сессия цела, входить заново не нужно.
func TestChangePassword_WrongOldPassword(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().ChangePassword(mock.Anything, "u1", "old-password", mock.Anything).Return(auth.ErrWrongOldPassword).Once()
	api := setup(t, uc)

	resp := api.PostCtx(identitytest.Client("u1"), "/api/auth/change-password",
		strings.NewReader(changePasswordBody(strings.Repeat("p", policy.MinPasswordLen))))
	handlertest.AssertAPIError(t, resp, http.StatusUnprocessableEntity, "AUTH_INVALID_OLD_PASSWORD")
	errs := handlertest.DecodeAPIError(t, resp).Errors
	require.Len(t, errs, 1)
	require.Equal(t, "body.old_password", errs[0].Location)
}

func TestChangePassword_WeakPassword(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().ChangePassword(mock.Anything, "u1", "old-password", mock.Anything).Return(auth.ErrWeakPassword).Once()
	api := setup(t, uc)

	resp := api.PostCtx(identitytest.Client("u1"), "/api/auth/change-password",
		strings.NewReader(changePasswordBody(strings.Repeat("p", policy.MinPasswordLen))))
	handlertest.AssertAPIError(t, resp, http.StatusUnprocessableEntity, "AUTH_WEAK_PASSWORD")
	errs := handlertest.DecodeAPIError(t, resp).Errors
	require.Len(t, errs, 1)
	require.Equal(t, "body.new_password", errs[0].Location)
}

// Правило длины пароля — policy.MinPasswordLen; схема смены пароля отклоняет короче его до usecase.
func TestChangePassword_ShortPassword(t *testing.T) {
	t.Parallel()
	api := setup(t, mocks.NewMockUsecase(t))

	resp := api.PostCtx(identitytest.Client("u1"), "/api/auth/change-password",
		strings.NewReader(changePasswordBody(strings.Repeat("p", policy.MinPasswordLen-1))))
	handlertest.AssertAPIError(t, resp, http.StatusUnprocessableEntity, "VALIDATION_FAILED")
	errs := handlertest.DecodeAPIError(t, resp).Errors
	require.Len(t, errs, 1)
	require.Equal(t, "body.new_password", errs[0].Location)
}

// Предел хэшера — в байтах, maxLength схемы — в символах: 1024 символа не из ASCII доходят до usecase, и
// его отказ — ошибка поля; 1025 символов схема отклоняет сама — и в новом, и в старом пароле.
func TestChangePassword_PasswordTooLong(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("я", authnkit.MaxPasswordLen)
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().ChangePassword(mock.Anything, "u1", "old-password", long).Return(auth.ErrPasswordTooLong).Once()
	api := setup(t, uc)

	resp := api.PostCtx(identitytest.Client("u1"), "/api/auth/change-password", strings.NewReader(changePasswordBody(long)))
	handlertest.AssertAPIError(t, resp, http.StatusUnprocessableEntity, "AUTH_PASSWORD_TOO_LONG")
	errs := handlertest.DecodeAPIError(t, resp).Errors
	require.Len(t, errs, 1)
	require.Equal(t, "body.new_password", errs[0].Location)

	for field, body := range map[string]string{
		"body.new_password": changePasswordBody(long + "я"),
		"body.old_password": fmt.Sprintf(`{"user_id":"u1","old_password":%q,"new_password":"new-password"}`, long+"я"),
	} {
		resp = api.PostCtx(identitytest.Client("u1"), "/api/auth/change-password", strings.NewReader(body))
		handlertest.AssertAPIError(t, resp, http.StatusUnprocessableEntity, "VALIDATION_FAILED")
		errs = handlertest.DecodeAPIError(t, resp).Errors
		require.Len(t, errs, 1)
		require.Equal(t, field, errs[0].Location)
	}
}

// Пароль длиннее maxLength схемы до входа не доходит: попытка не тратит хэширование.
func TestLogin_PasswordOverSchemaLimit(t *testing.T) {
	t.Parallel()
	api := setup(t, mocks.NewMockUsecase(t))

	body := fmt.Sprintf(`{"email":"a@b.com","password":%q}`, strings.Repeat("я", authnkit.MaxPasswordLen+1))
	resp := api.Post("/api/auth/login", strings.NewReader(body))
	handlertest.AssertAPIError(t, resp, http.StatusUnprocessableEntity, "VALIDATION_FAILED")
}

func changePasswordBody(newPassword string) string {
	return fmt.Sprintf(`{"user_id":"u1","old_password":"old-password","new_password":%q}`, newPassword)
}
