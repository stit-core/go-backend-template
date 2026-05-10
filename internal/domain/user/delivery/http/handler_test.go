package http_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/mock"

	"git.stit.tech/akadmin/go-backend-template/internal/domain/user"
	userhttp "git.stit.tech/akadmin/go-backend-template/internal/domain/user/delivery/http"
	"git.stit.tech/akadmin/go-backend-template/internal/domain/user/delivery/http/mocks"
	"git.stit.tech/stit/golibinfra/testkit/handlertest"
)

func setup(t *testing.T, uc *mocks.MockUsecase) handlertest.API {
	t.Helper()
	return handlertest.NewAPI(t, func(api huma.API) {
		userhttp.Register(api, userhttp.NewHandler(uc))
	})
}

func TestRegister_Created(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Register(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, in *user.Create) (*user.User, error) {
			return &user.User{ID: "u1", Email: in.Email, Name: in.Name, Status: user.StatusActive}, nil
		})
	api := setup(t, uc)

	resp := api.Post("/api/auth/register", strings.NewReader(`{"email":"a@b.com","password":"longenough","name":"Alice"}`))
	handlertest.AssertStatus(t, resp, http.StatusCreated)
}

func TestRegister_Conflict(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Register(mock.Anything, mock.Anything).Return(nil, user.ErrAlreadyExists)
	api := setup(t, uc)

	resp := api.Post("/api/auth/register", strings.NewReader(`{"email":"a@b.com","password":"longenough","name":"A"}`))
	handlertest.AssertAPIError(t, resp, http.StatusConflict, "USER_ALREADY_EXISTS")
}

func TestRegister_WeakPassword(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Register(mock.Anything, mock.Anything).Return(nil, user.ErrWeakPassword)
	api := setup(t, uc)

	resp := api.Post("/api/auth/register", strings.NewReader(`{"email":"a@b.com","password":"abcdefgh","name":"A"}`))
	handlertest.AssertAPIError(t, resp, http.StatusBadRequest, "USER_WEAK_PASSWORD")
}

func TestGetByID_Forbidden(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().GetByID(mock.Anything, mock.Anything).Return(nil, &user.ForbiddenError{Reason: "not self"})
	api := setup(t, uc)

	resp := api.Get("/api/users/u1")
	handlertest.AssertAPIError(t, resp, http.StatusForbidden, "FORBIDDEN")
}

func TestGetByID_NotFound(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().GetByID(mock.Anything, mock.Anything).Return(nil, user.ErrNotFound)
	api := setup(t, uc)

	resp := api.Get("/api/users/u1")
	handlertest.AssertAPIError(t, resp, http.StatusNotFound, "USER_NOT_FOUND")
}

func TestDelete_Forbidden(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Delete(mock.Anything, mock.Anything).Return(&user.ForbiddenError{Reason: "not admin"})
	api := setup(t, uc)

	resp := api.Delete("/api/users/u1")
	handlertest.AssertStatus(t, resp, http.StatusForbidden)
}

func TestDelete_NotFound(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Delete(mock.Anything, mock.Anything).Return(user.ErrNotFound)
	api := setup(t, uc)

	resp := api.Delete("/api/users/u1")
	handlertest.AssertStatus(t, resp, http.StatusNotFound)
}

func TestUpdate_ConcurrentUpdate(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Update(mock.Anything, mock.Anything, mock.Anything).Return(nil, user.ErrConcurrentUpdate)
	api := setup(t, uc)

	resp := api.Patch("/api/users/u1", strings.NewReader(`{"name":"new","version":1}`))
	handlertest.AssertAPIError(t, resp, http.StatusConflict, "USER_CONCURRENT_UPDATE")
}

func TestUpdate_MissingVersion(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	api := setup(t, uc)

	resp := api.Patch("/api/users/u1", strings.NewReader(`{"name":"new"}`))
	handlertest.AssertStatus(t, resp, http.StatusUnprocessableEntity)
}
