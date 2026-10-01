package http_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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
	"git.stit.tech/stit-core/golibinfra/testkit/handlertest"
)

func setup(t *testing.T, uc *mocks.MockUsecase) handlertest.API {
	t.Helper()
	return handlertest.NewAPI(t, func(api huma.API) {
		authhttp.Register(api, authhttp.NewHandler(uc, authgate.CookieConfig{Secure: false}))
	})
}

func setCookieNames(resp http.Header) []string {
	out := []string{}
	for _, line := range resp.Values("Set-Cookie") {
		if i := strings.Index(line, "="); i > 0 {
			out = append(out, line[:i])
		}
	}
	return out
}

func TestLogin_HappyPath(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Login(mock.Anything, mock.Anything).
		Return(&auth.LoginOutput{AccessToken: "a", RefreshToken: "r", User: &auth.User{ID: "u1", Role: identity.RoleClient}}, nil)
	api := setup(t, uc)

	resp := api.Post("/api/auth/login", strings.NewReader(`{"email":"a@b.com","password":"pass1234"}`))
	handlertest.AssertStatus(t, resp, http.StatusOK)
	require.ElementsMatch(t, []string{authgate.CookieAccess, authgate.CookieRefresh}, setCookieNames(resp.Header()))
	var body struct {
		Data struct {
			ID   string `json:"id"`
			Role string `json:"role"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	require.Equal(t, "u1", body.Data.ID, "the user goes in the data envelope")
	require.Equal(t, "client", body.Data.Role)
}

func TestLogin_InvalidCredentials(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Login(mock.Anything, mock.Anything).Return(nil, auth.ErrInvalidCredentials)
	api := setup(t, uc)

	resp := api.Post("/api/auth/login", strings.NewReader(`{"email":"a@b.com","password":"x"}`))
	handlertest.AssertAPIError(t, resp, http.StatusUnauthorized, "AUTH_INVALID_CREDENTIALS")
}

// Блокировка входа говорит, когда повторить: Retry-After — из ожидания блокировки.
func TestLogin_AccountLocked(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Login(mock.Anything, mock.Anything).Return(nil, &auth.LockedError{Wait: 90 * time.Second})
	api := setup(t, uc)

	resp := api.Post("/api/auth/login", strings.NewReader(`{"email":"a@b.com","password":"x"}`))
	handlertest.AssertAPIError(t, resp, http.StatusTooManyRequests, "AUTH_ACCOUNT_LOCKED")
	require.Equal(t, "90", resp.Header().Get("Retry-After"))
}

func TestRefresh_HappyPath(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Refresh(mock.Anything, "old.refresh.jwt").
		Return(&auth.LoginOutput{AccessToken: "new.a", RefreshToken: "new.r", User: &auth.User{ID: "u1"}}, nil)
	api := setup(t, uc)

	resp := api.Do(http.MethodPost, "/api/auth/refresh", "Cookie: refresh_token=old.refresh.jwt")
	handlertest.AssertStatus(t, resp, http.StatusNoContent)
	require.ElementsMatch(t, []string{authgate.CookieAccess, authgate.CookieRefresh}, setCookieNames(resp.Header()))
}

func TestRefresh_NoCookie(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t) // uc.Refresh не должен вызываться
	api := setup(t, uc)

	resp := api.Post("/api/auth/refresh", strings.NewReader(""))
	handlertest.AssertAPIError(t, resp, http.StatusUnauthorized, "AUTH_REFRESH_EXPIRED")
}

func TestRefresh_Expired(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Refresh(mock.Anything, mock.Anything).Return(nil, auth.ErrRefreshExpired)
	api := setup(t, uc)

	resp := api.Do(http.MethodPost, "/api/auth/refresh", "Cookie: refresh_token=expired.jwt")
	handlertest.AssertAPIError(t, resp, http.StatusUnauthorized, "AUTH_REFRESH_EXPIRED")
}

// Битый refresh — «войти заново», а не 500.
func TestRefresh_Invalid(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Refresh(mock.Anything, "broken").Return(nil, auth.ErrInvalidCredentials)
	api := setup(t, uc)

	resp := api.Do(http.MethodPost, "/api/auth/refresh", "Cookie: refresh_token=broken")
	handlertest.AssertAPIError(t, resp, http.StatusUnauthorized, "AUTH_REFRESH_INVALID")
}

func TestRefresh_Reused(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Refresh(mock.Anything, mock.Anything).Return(nil, auth.ErrRefreshReused)
	api := setup(t, uc)

	resp := api.Do(http.MethodPost, "/api/auth/refresh", "Cookie: refresh_token=reused.jwt")
	handlertest.AssertAPIError(t, resp, http.StatusUnauthorized, "AUTH_REFRESH_REUSED")
}

func TestLogout_WithCookie(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Logout(mock.Anything, "refresh.jwt").Return(nil)
	api := setup(t, uc)

	resp := api.Do(http.MethodPost, "/api/auth/logout", "Cookie: refresh_token=refresh.jwt")
	handlertest.AssertStatus(t, resp, http.StatusNoContent)

	cookies := setCookieNames(resp.Header())
	require.Contains(t, cookies, authgate.CookieAccess)
	require.Contains(t, cookies, authgate.CookieRefresh)
}

func TestLogout_NoCookie(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t) // uc.Logout не должен вызываться
	api := setup(t, uc)

	resp := api.Post("/api/auth/logout", strings.NewReader(""))
	handlertest.AssertStatus(t, resp, http.StatusNoContent)
}

// 5xx — без причины: клиенту код и request_id, причина — в лог сервера с тем же request_id.
func TestLogout_StoreFailure(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Logout(mock.Anything, "refresh.jwt").Return(errors.New("get refresh session: connection refused"))
	api := setup(t, uc)

	resp := api.Do(http.MethodPost, "/api/auth/logout", "Cookie: refresh_token=refresh.jwt")
	handlertest.AssertAPIError(t, resp, http.StatusInternalServerError, "INTERNAL")
	require.Empty(t, handlertest.DecodeAPIError(t, resp).Detail)
	require.NotContains(t, resp.Body.String(), "connection refused")
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
			uc.EXPECT().ChangePassword(mock.Anything, "u1", "old-password", mock.Anything).Return(tc.err).Once()
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
	uc.EXPECT().ChangePassword(mock.Anything, "u1", "old-password", mock.Anything).Return(auth.ErrInvalidCredentials).Once()
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
	uc.EXPECT().ChangePassword(mock.Anything, "u1", "old-password", mock.Anything).Return(auth.ErrWeakPassword)
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

func changePasswordBody(newPassword string) string {
	return fmt.Sprintf(`{"user_id":"u1","old_password":"old-password","new_password":%q}`, newPassword)
}
