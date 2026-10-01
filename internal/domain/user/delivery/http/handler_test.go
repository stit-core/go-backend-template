package http_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/user"
	userhttp "git.stit.tech/stit-core/go-backend-template/internal/domain/user/delivery/http"
	"git.stit.tech/stit-core/go-backend-template/internal/domain/user/delivery/http/mocks"
	"git.stit.tech/stit-core/go-backend-template/internal/platform/identity"
	"git.stit.tech/stit-core/go-backend-template/internal/testkit/identitytest"
	"git.stit.tech/stit-core/go-backend-template/pkg/paging"
	"git.stit.tech/stit-core/go-backend-template/pkg/policy"
	"git.stit.tech/stit-core/golibinfra/testkit/handlertest"
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
		RunAndReturn(func(_ context.Context, in *user.CreateInput) (*user.User, error) {
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

	resp := api.Post("/api/auth/register", strings.NewReader(registerBody(strings.Repeat("p", policy.MinPasswordLen))))
	handlertest.AssertAPIError(t, resp, http.StatusUnprocessableEntity, "USER_WEAK_PASSWORD")
	errs := handlertest.DecodeAPIError(t, resp).Errors
	require.Len(t, errs, 1)
	require.Equal(t, "body.password", errs[0].Location)
}

// Правило длины пароля — policy.MinPasswordLen; схема register отклоняет короче его до usecase.
func TestRegister_ShortPassword(t *testing.T) {
	t.Parallel()
	api := setup(t, mocks.NewMockUsecase(t))

	resp := api.Post("/api/auth/register", strings.NewReader(registerBody(strings.Repeat("p", policy.MinPasswordLen-1))))
	handlertest.AssertAPIError(t, resp, http.StatusUnprocessableEntity, "VALIDATION_FAILED")
	errs := handlertest.DecodeAPIError(t, resp).Errors
	require.Len(t, errs, 1)
	require.Equal(t, "body.password", errs[0].Location)
}

func registerBody(password string) string {
	return fmt.Sprintf(`{"email":"a@b.com","password":%q,"name":"A"}`, password)
}

func TestGetByID_Forbidden(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().GetByID(mock.Anything, mock.Anything).Return(nil, &policy.ForbiddenError{Reason: "not self"})
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
	uc.EXPECT().Delete(mock.Anything, mock.Anything).Return(&policy.ForbiddenError{Reason: "not admin"})
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

func TestMe_OK(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Me(mock.MatchedBy(func(ctx context.Context) bool {
		id, ok := identity.From(ctx)
		return ok && id.UserID == "u1" && id.Role == identity.RoleClient
	})).Return(&user.User{
		ID:        "u1",
		Email:     "a@b.com",
		Name:      "Alice",
		Status:    user.StatusActive,
		CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		UpdatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}, nil)
	api := setup(t, uc)

	resp := api.GetCtx(identitytest.Client("u1"), "/api/users/me")
	handlertest.AssertStatus(t, resp, http.StatusOK)

	var body struct {
		Data struct {
			ID        string    `json:"id"`
			Email     string    `json:"email"`
			Name      string    `json:"name"`
			Status    string    `json:"status"`
			CreatedAt time.Time `json:"created_at"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	require.Equal(t, "u1", body.Data.ID)
	require.Equal(t, "a@b.com", body.Data.Email)
	require.Equal(t, "Alice", body.Data.Name)
	require.Equal(t, "active", body.Data.Status)
	require.Equal(t, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), body.Data.CreatedAt)
}

func TestMe_ForbiddenWithoutIdentity(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Me(mock.Anything).Return(nil, &policy.ForbiddenError{Reason: "no identity"})
	api := setup(t, uc)

	resp := api.Get("/api/users/me")
	handlertest.AssertAPIError(t, resp, http.StatusForbidden, "FORBIDDEN")
}

func TestList_OK(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().List(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, filter user.ListFilter, page paging.Page) ([]*user.User, int, error) {
			require.NotNil(t, filter.Status)
			require.Equal(t, user.StatusActive, *filter.Status)
			require.Equal(t, paging.Page{Number: 2, Size: 5}, page)
			return []*user.User{
				{ID: "u1", Email: "a@b.com", Name: "Alice", Status: user.StatusActive},
				{ID: "u2", Email: "c@d.com", Name: "Bob", Status: user.StatusActive},
			}, 7, nil
		})
	api := setup(t, uc)

	resp := api.GetCtx(identitytest.Admin("admin-1"), "/api/users?page=2&size=5&status=active")
	handlertest.AssertStatus(t, resp, http.StatusOK)

	var body struct {
		Data []struct {
			ID     string `json:"id"`
			Email  string `json:"email"`
			Status string `json:"status"`
		} `json:"data"`
		Total int `json:"total"`
		Page  int `json:"page"`
		Size  int `json:"size"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	require.Equal(t, 7, body.Total)
	require.Equal(t, 2, body.Page)
	require.Equal(t, 5, body.Size)
	require.Len(t, body.Data, 2)
	require.Equal(t, "u1", body.Data[0].ID)
	require.Equal(t, "a@b.com", body.Data[0].Email)
	require.Equal(t, "active", body.Data[0].Status)
	require.Equal(t, "u2", body.Data[1].ID)
}

func TestList_InvalidStatus(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	api := setup(t, uc)

	resp := api.GetCtx(identitytest.Admin("admin-1"), "/api/users?status=deleted")
	handlertest.AssertStatus(t, resp, http.StatusUnprocessableEntity)
}

func TestUpdate_OK(t *testing.T) {
	t.Parallel()
	name, status := "New", user.StatusBlocked
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Update(mock.Anything, "u1", user.UpdateInput{Name: &name, Status: &status, Version: 3}).
		Return(&user.User{ID: "u1", Email: "a@b.com", Name: name, Status: status, Version: 4}, nil).Once()
	api := setup(t, uc)

	resp := api.PatchCtx(identitytest.Admin("a1"), "/api/users/u1", strings.NewReader(`{"name":"New","status":"blocked","version":3}`))
	handlertest.AssertStatus(t, resp, http.StatusOK)

	var body struct {
		Data struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Status  string `json:"status"`
			Version int    `json:"version"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	require.Equal(t, "u1", body.Data.ID)
	require.Equal(t, "New", body.Data.Name)
	require.Equal(t, "blocked", body.Data.Status)
	require.Equal(t, 4, body.Data.Version, "the client sends the next PATCH with this version")
}

func TestUpdate_Errors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"client changes status", &policy.ForbiddenError{Reason: "admin only"}, http.StatusForbidden, "FORBIDDEN"},
		{"no fields", user.ErrNoUpdateFields, http.StatusBadRequest, "USER_NO_UPDATE_FIELDS"},
		{"unknown id", user.ErrNotFound, http.StatusNotFound, "USER_NOT_FOUND"},
		{"stale version", user.ErrConcurrentUpdate, http.StatusConflict, "USER_CONCURRENT_UPDATE"},
		{"transition not allowed", user.ErrInvalidStatusTransition, http.StatusConflict, "USER_INVALID_STATUS_TRANSITION"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			uc := mocks.NewMockUsecase(t)
			uc.EXPECT().Update(mock.Anything, "u1", mock.Anything).Return(nil, tc.err).Once()
			api := setup(t, uc)

			resp := api.Patch("/api/users/u1", strings.NewReader(`{"status":"active","version":1}`))
			handlertest.AssertAPIError(t, resp, tc.status, tc.code)
		})
	}
}

func TestUpdate_InvalidBody(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		body     string
		location string
	}{
		{"unknown status", `{"status":"deleted","version":1}`, "body.status"},
		{"empty name", `{"name":"","version":1}`, "body.name"},
		{"no version", `{"name":"new"}`, "body"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			api := setup(t, mocks.NewMockUsecase(t))

			resp := api.Patch("/api/users/u1", strings.NewReader(tc.body))
			handlertest.AssertAPIError(t, resp, http.StatusUnprocessableEntity, "VALIDATION_FAILED")
			errs := handlertest.DecodeAPIError(t, resp).Errors
			require.Len(t, errs, 1)
			require.Equal(t, tc.location, errs[0].Location)
		})
	}
}
