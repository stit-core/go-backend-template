package http_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"git.sftbmb.ru/sftbmb-core/go-backend-template/internal/domain/auth"
	authhttp "git.sftbmb.ru/sftbmb-core/go-backend-template/internal/domain/auth/delivery/http"
	"git.sftbmb.ru/sftbmb-core/go-backend-template/internal/domain/auth/delivery/http/mocks"
	"git.sftbmb.ru/sftbmb-core/go-backend-template/internal/platform/authgate"
	"git.sftbmb.ru/sftbmb-core/golibinfra/testkit/handlertest"
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
		Return(&auth.LoginOutput{AccessToken: "a", RefreshToken: "r", User: &auth.User{ID: "u1"}}, nil)
	api := setup(t, uc)

	resp := api.Post("/api/auth/login", strings.NewReader(`{"email":"a@b.com","password":"pass1234"}`))
	handlertest.AssertStatus(t, resp, http.StatusOK)
	assert.ElementsMatch(t, []string{authgate.CookieAccess, authgate.CookieRefresh}, setCookieNames(resp.Header()))
	assert.Contains(t, resp.Body.String(), `"id":"u1"`)
}

func TestLogin_InvalidCredentials(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Login(mock.Anything, mock.Anything).Return(nil, auth.ErrInvalidCredentials)
	api := setup(t, uc)

	resp := api.Post("/api/auth/login", strings.NewReader(`{"email":"a@b.com","password":"x"}`))
	handlertest.AssertAPIError(t, resp, http.StatusUnauthorized, "AUTH_INVALID_CREDENTIALS")
}

func TestLogin_AccountLocked(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Login(mock.Anything, mock.Anything).Return(nil, auth.ErrAccountLocked)
	api := setup(t, uc)

	resp := api.Post("/api/auth/login", strings.NewReader(`{"email":"a@b.com","password":"x"}`))
	handlertest.AssertStatus(t, resp, http.StatusTooManyRequests)
}

func TestRefresh_HappyPath(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Refresh(mock.Anything, "old.refresh.jwt").
		Return(&auth.LoginOutput{AccessToken: "new.a", RefreshToken: "new.r", User: &auth.User{ID: "u1"}}, nil)
	api := setup(t, uc)

	resp := api.Do(http.MethodPost, "/api/auth/refresh", "Cookie: refresh_token=old.refresh.jwt")
	handlertest.AssertStatus(t, resp, http.StatusNoContent)
	assert.ElementsMatch(t, []string{authgate.CookieAccess, authgate.CookieRefresh}, setCookieNames(resp.Header()))
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
	assert.Contains(t, cookies, authgate.CookieAccess)
	assert.Contains(t, cookies, authgate.CookieRefresh)
}

func TestLogout_NoCookie(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t) // uc.Logout не должен вызываться
	api := setup(t, uc)

	resp := api.Post("/api/auth/logout", strings.NewReader(""))
	handlertest.AssertStatus(t, resp, http.StatusNoContent)
}

func TestLogout_ExpiredTokenStillClears(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Logout(mock.Anything, mock.Anything).Return(auth.ErrRefreshExpired)
	api := setup(t, uc)

	resp := api.Do(http.MethodPost, "/api/auth/logout", "Cookie: refresh_token=expired.jwt")
	handlertest.AssertStatus(t, resp, http.StatusNoContent)
}
