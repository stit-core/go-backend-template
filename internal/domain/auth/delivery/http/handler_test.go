package http_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/mock"

	"git.stit.tech/akadmin/go-backend-template/internal/domain/auth"
	authhttp "git.stit.tech/akadmin/go-backend-template/internal/domain/auth/delivery/http"
	"git.stit.tech/akadmin/go-backend-template/internal/domain/auth/delivery/http/mocks"
	"git.stit.tech/akadmin/golibinfra/testkit/handlertest"
)

func setup(t *testing.T, uc *mocks.MockUsecase) handlertest.API {
	t.Helper()
	return handlertest.NewAPI(t, func(api huma.API) {
		authhttp.Register(api, authhttp.NewHandler(uc))
	})
}

func TestLogin_HappyPath(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Login(mock.Anything, mock.Anything).
		Return(&auth.LoginOutput{AccessToken: "a", RefreshToken: "r", User: &auth.User{ID: "u1"}}, nil)
	api := setup(t, uc)

	resp := api.Post("/api/auth/login", strings.NewReader(`{"email":"a@b.com","password":"pass1234"}`))
	handlertest.AssertStatus(t, resp, http.StatusOK)
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

func TestRefresh_Expired(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Refresh(mock.Anything, mock.Anything).Return(nil, auth.ErrRefreshExpired)
	api := setup(t, uc)

	resp := api.Post("/api/auth/refresh", strings.NewReader(`{"refresh_token":"x"}`))
	handlertest.AssertAPIError(t, resp, http.StatusUnauthorized, "AUTH_REFRESH_EXPIRED")
}
