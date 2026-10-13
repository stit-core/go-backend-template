package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/user"
	userhttp "git.stit.tech/stit-core/go-backend-template/internal/domain/user/delivery/http"
	"git.stit.tech/stit-core/go-backend-template/internal/domain/user/delivery/http/mocks"
	"git.stit.tech/stit-core/go-backend-template/internal/platform/authgate"
	"git.stit.tech/stit-core/go-backend-template/internal/platform/identity"
	"git.stit.tech/stit-core/go-backend-template/internal/testkit/identitytest"
	"git.stit.tech/stit-core/go-backend-template/pkg/paging"
	"git.stit.tech/stit-core/go-backend-template/pkg/policy"
	"git.stit.tech/stit-core/golibinfra/authnkit"
	"git.stit.tech/stit-core/golibinfra/ptrkit"
	"git.stit.tech/stit-core/golibinfra/testkit/handlertest"
)

func setup(t *testing.T, uc *mocks.MockUsecase) handlertest.API {
	t.Helper()
	return handlertest.NewAPI(t, func(api huma.API) {
		userhttp.Register(api, userhttp.NewHandler(uc))
	}, handlertest.WithSecuritySchemes(authgate.CookieScheme(identitytest.Extract)))
}

// callerIs — ctx usecase несёт identity вызывающего: по ней решает policy.
func callerIs(userID string) any {
	return mock.MatchedBy(func(ctx context.Context) bool {
		id, ok := identity.From(ctx)
		return ok && id.UserID == userID
	})
}

var createdAt = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func alice() *user.User {
	return &user.User{
		ID: "u1", Email: "a@b.com", PasswordHash: "hash:secret", Name: "Alice", Status: user.StatusActive,
		Version: 3, CreatedAt: createdAt, UpdatedAt: createdAt.Add(time.Hour),
	}
}

// view — пользователь в ответе глазами клиента: теги свои, а не из кода.
type view struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

var aliceView = view{
	ID: "u1", Email: "a@b.com", Name: "Alice", Status: "active", Version: 3, CreatedAt: createdAt, UpdatedAt: createdAt.Add(time.Hour),
}

// data разбирает поле data ответа; поле вне T — провал: ответ не отдаёт лишнего (хэш пароля).
func data[T any](t *testing.T, resp *httptest.ResponseRecorder) T {
	t.Helper()
	var body struct {
		Data json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	dec := json.NewDecoder(bytes.NewReader(body.Data))
	dec.DisallowUnknownFields()
	var v T
	require.NoError(t, dec.Decode(&v))
	return v
}

func TestRegister_Created(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Register(mock.Anything, &user.CreateInput{Email: "a@b.com", Password: "longenough", Name: "Alice"}).
		Return(alice(), nil).Once()
	api := setup(t, uc)

	resp := api.Post("/api/auth/register", strings.NewReader(`{"email":"a@b.com","password":"longenough","name":"Alice"}`))
	handlertest.AssertStatus(t, resp, http.StatusCreated)
	require.Equal(t, aliceView, data[view](t, resp))
}

func TestRegister_Conflict(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Register(mock.Anything, mock.Anything).Return(nil, user.ErrAlreadyExists).Once()
	api := setup(t, uc)

	resp := api.Post("/api/auth/register", strings.NewReader(`{"email":"a@b.com","password":"longenough","name":"A"}`))
	handlertest.AssertAPIError(t, resp, http.StatusConflict, "USER_ALREADY_EXISTS")
}

func TestRegister_WeakPassword(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Register(mock.Anything, mock.Anything).Return(nil, user.ErrWeakPassword).Once()
	api := setup(t, uc)

	resp := api.Post("/api/auth/register", strings.NewReader(registerBody(strings.Repeat("p", policy.MinPasswordLen))))
	handlertest.AssertAPIError(t, resp, http.StatusUnprocessableEntity, "USER_WEAK_PASSWORD")
	errs := handlertest.DecodeAPIError(t, resp).Errors
	require.Len(t, errs, 1)
	require.Equal(t, "body.password", errs[0].Location)
}

// Домен проверяет email строже формата схемы: его отказ — та же форма ошибки поля.
func TestRegister_InvalidEmail(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Register(mock.Anything, mock.Anything).Return(nil, user.ErrInvalidEmail).Once()
	api := setup(t, uc)

	resp := api.Post("/api/auth/register", strings.NewReader(registerBody(strings.Repeat("p", policy.MinPasswordLen))))
	handlertest.AssertAPIError(t, resp, http.StatusUnprocessableEntity, "USER_INVALID_EMAIL")
	errs := handlertest.DecodeAPIError(t, resp).Errors
	require.Len(t, errs, 1)
	require.Equal(t, "body.email", errs[0].Location)
}

// Имя с NUL отвергает домен: та же форма ошибки поля, что у валидации схемы.
func TestRegister_InvalidName(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Register(mock.Anything, mock.Anything).Return(nil, user.ErrInvalidName).Once()
	api := setup(t, uc)

	resp := api.Post("/api/auth/register", strings.NewReader(registerBody(strings.Repeat("p", policy.MinPasswordLen))))
	handlertest.AssertAPIError(t, resp, http.StatusUnprocessableEntity, "USER_INVALID_NAME")
	errs := handlertest.DecodeAPIError(t, resp).Errors
	require.Len(t, errs, 1)
	require.Equal(t, "body.name", errs[0].Location)
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

// Предел хэшера — в байтах, maxLength схемы — в символах: 1024 символа не из ASCII доходят до usecase, и
// его отказ — ошибка поля; 1025 символов схема отклоняет сама.
func TestRegister_PasswordTooLong(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("я", authnkit.MaxPasswordLen)
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Register(mock.Anything, &user.CreateInput{Email: "a@b.com", Password: long, Name: "A"}).Return(nil, user.ErrPasswordTooLong).Once()
	api := setup(t, uc)

	resp := api.Post("/api/auth/register", strings.NewReader(registerBody(long)))
	handlertest.AssertAPIError(t, resp, http.StatusUnprocessableEntity, "USER_PASSWORD_TOO_LONG")
	errs := handlertest.DecodeAPIError(t, resp).Errors
	require.Len(t, errs, 1)
	require.Equal(t, "body.password", errs[0].Location)

	resp = api.Post("/api/auth/register", strings.NewReader(registerBody(long+"я")))
	handlertest.AssertAPIError(t, resp, http.StatusUnprocessableEntity, "VALIDATION_FAILED")
}

// Предел имени — 200 символов (maxLength считает символы): длиннее схема отклоняет до usecase.
func TestRegister_NameTooLong(t *testing.T) {
	t.Parallel()
	limit := strings.Repeat("я", 200)
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Register(mock.Anything, &user.CreateInput{Email: "a@b.com", Password: "longenough", Name: limit}).Return(alice(), nil).Once()
	api := setup(t, uc)
	body := func(name string) *strings.Reader {
		return strings.NewReader(fmt.Sprintf(`{"email":"a@b.com","password":"longenough","name":%q}`, name))
	}

	handlertest.AssertStatus(t, api.Post("/api/auth/register", body(limit)), http.StatusCreated)
	resp := api.Post("/api/auth/register", body(limit+"я"))
	handlertest.AssertAPIError(t, resp, http.StatusUnprocessableEntity, "VALIDATION_FAILED")
	errs := handlertest.DecodeAPIError(t, resp).Errors
	require.Len(t, errs, 1)
	require.Equal(t, "body.name", errs[0].Location)
}

func registerBody(password string) string {
	return fmt.Sprintf(`{"email":"a@b.com","password":%q,"name":"A"}`, password)
}

func TestGetByID_OK(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().GetByID(callerIs("a1"), "u1").Return(alice(), nil).Once()
	api := setup(t, uc)

	resp := api.GetCtx(identitytest.Admin("a1"), "/api/users/u1")
	handlertest.AssertStatus(t, resp, http.StatusOK)
	require.Equal(t, aliceView, data[view](t, resp))
}

func TestGetByID_Forbidden(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().GetByID(callerIs("u2"), "u1").Return(nil, &policy.ForbiddenError{Reason: "not self"}).Once()
	api := setup(t, uc)

	resp := api.GetCtx(identitytest.Client("u2"), "/api/users/u1")
	handlertest.AssertAPIError(t, resp, http.StatusForbidden, "FORBIDDEN")
}

func TestGetByID_NotFound(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().GetByID(callerIs("a1"), "u1").Return(nil, user.ErrNotFound).Once()
	api := setup(t, uc)

	resp := api.GetCtx(identitytest.Admin("a1"), "/api/users/u1")
	handlertest.AssertAPIError(t, resp, http.StatusNotFound, "USER_NOT_FOUND")
}

func TestDelete_NoContent(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Delete(callerIs("u1"), "u1").Return(nil).Once()
	api := setup(t, uc)

	resp := api.DeleteCtx(identitytest.Client("u1"), "/api/users/u1")
	handlertest.AssertStatus(t, resp, http.StatusNoContent)
	require.Empty(t, resp.Body.String())
}

func TestDelete_Errors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		caller string
		ctx    context.Context
		err    error
		status int
		code   string
	}{
		{"not self, not admin", "u2", identitytest.Client("u2"), &policy.ForbiddenError{Reason: "not admin"}, http.StatusForbidden, "FORBIDDEN"},
		{"unknown or already deleted", "a1", identitytest.Admin("a1"), user.ErrNotFound, http.StatusNotFound, "USER_NOT_FOUND"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			uc := mocks.NewMockUsecase(t)
			uc.EXPECT().Delete(callerIs(tc.caller), "u1").Return(tc.err).Once()
			api := setup(t, uc)

			resp := api.DeleteCtx(tc.ctx, "/api/users/u1")
			handlertest.AssertAPIError(t, resp, tc.status, tc.code)
		})
	}
}

func TestMe_OK(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Me(callerIs("u1")).Return(alice(), nil).Once()
	api := setup(t, uc)

	resp := api.GetCtx(identitytest.Client("u1"), "/api/users/me")
	handlertest.AssertStatus(t, resp, http.StatusOK)
	require.Equal(t, aliceView, data[view](t, resp))
}

// Операция под Security: запрос без сессии отвечает 401 до usecase — строгий мок без ожиданий.
func TestMe_Unauthenticated(t *testing.T) {
	t.Parallel()
	api := setup(t, mocks.NewMockUsecase(t))

	resp := api.Get("/api/users/me")
	handlertest.AssertAPIError(t, resp, http.StatusUnauthorized, "UNAUTHORIZED")
}

func TestList_OK(t *testing.T) {
	t.Parallel()
	bob := &user.User{ID: "u2", Email: "c@d.com", Name: "Bob", Status: user.StatusActive, Version: 1, CreatedAt: createdAt, UpdatedAt: createdAt}
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().List(callerIs("a1"), user.ListFilter{Status: ptrkit.Of(user.StatusActive)}, paging.Page{Number: 2, Size: 5}).
		Return([]*user.User{alice(), bob}, 7, nil).Once()
	api := setup(t, uc)

	resp := api.GetCtx(identitytest.Admin("a1"), "/api/users?page=2&size=5&status=active")
	handlertest.AssertStatus(t, resp, http.StatusOK)
	require.Equal(t, []view{aliceView, {ID: "u2", Email: "c@d.com", Name: "Bob", Status: "active", Version: 1, CreatedAt: createdAt, UpdatedAt: createdAt}},
		data[[]view](t, resp))
	var page struct {
		Total int `json:"total"`
		Page  int `json:"page"`
		Size  int `json:"size"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &page))
	require.Equal(t, 7, page.Total)
	require.Equal(t, 2, page.Page)
	require.Equal(t, 5, page.Size)
}

// Без query — первая страница по умолчанию и без фильтра статуса.
func TestList_Defaults(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().List(mock.Anything, user.ListFilter{}, paging.Page{Number: 1, Size: paging.DefaultSize}).Return(nil, 0, nil).Once()
	api := setup(t, uc)

	resp := api.GetCtx(identitytest.Admin("a1"), "/api/users")
	handlertest.AssertStatus(t, resp, http.StatusOK)
	require.Empty(t, data[[]view](t, resp))
}

// Список — только admin: отказ policy — 403.
func TestList_Forbidden(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().List(callerIs("u1"), user.ListFilter{}, paging.Page{Number: 1, Size: paging.DefaultSize}).
		Return(nil, 0, &policy.ForbiddenError{Reason: "admin only"}).Once()
	api := setup(t, uc)

	resp := api.GetCtx(identitytest.Client("u1"), "/api/users")
	handlertest.AssertAPIError(t, resp, http.StatusForbidden, "FORBIDDEN")
}

func TestList_InvalidStatus(t *testing.T) {
	t.Parallel()
	api := setup(t, mocks.NewMockUsecase(t))

	resp := api.GetCtx(identitytest.Admin("a1"), "/api/users?status=deleted")
	handlertest.AssertAPIError(t, resp, http.StatusUnprocessableEntity, "VALIDATION_FAILED")
}

func TestUpdate_OK(t *testing.T) {
	t.Parallel()
	renamed := alice()
	renamed.Name, renamed.Status, renamed.Version = "New", user.StatusBlocked, 4
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Update(callerIs("a1"), "u1", user.UpdateInput{Name: ptrkit.Of("New"), Status: ptrkit.Of(user.StatusBlocked), Version: 3}).
		Return(renamed, nil).Once()
	api := setup(t, uc)

	resp := api.PatchCtx(identitytest.Admin("a1"), "/api/users/u1", strings.NewReader(`{"name":"New","status":"blocked","version":3}`))
	handlertest.AssertStatus(t, resp, http.StatusOK)
	want := aliceView
	want.Name, want.Status, want.Version = "New", "blocked", 4
	require.Equal(t, want, data[view](t, resp), "the client sends the next PATCH with this version")
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
		{"name with NUL", user.ErrInvalidName, http.StatusUnprocessableEntity, "USER_INVALID_NAME"},
		{"unknown id", user.ErrNotFound, http.StatusNotFound, "USER_NOT_FOUND"},
		{"stale version", user.ErrConcurrentUpdate, http.StatusConflict, "USER_CONCURRENT_UPDATE"},
		{"transition not allowed", user.ErrInvalidStatusTransition, http.StatusConflict, "USER_INVALID_STATUS_TRANSITION"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			uc := mocks.NewMockUsecase(t)
			uc.EXPECT().Update(callerIs("a1"), "u1", mock.Anything).Return(nil, tc.err).Once()
			api := setup(t, uc)

			resp := api.PatchCtx(identitytest.Admin("a1"), "/api/users/u1", strings.NewReader(`{"status":"active","version":1}`))
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
		{"name over 200 characters", fmt.Sprintf(`{"name":%q,"version":1}`, strings.Repeat("я", 201)), "body.name"},
		{"no version", `{"name":"new"}`, "body"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			api := setup(t, mocks.NewMockUsecase(t))

			resp := api.PatchCtx(identitytest.Admin("a1"), "/api/users/u1", strings.NewReader(tc.body))
			handlertest.AssertAPIError(t, resp, http.StatusUnprocessableEntity, "VALIDATION_FAILED")
			errs := handlertest.DecodeAPIError(t, resp).Errors
			require.Len(t, errs, 1)
			require.Equal(t, tc.location, errs[0].Location)
		})
	}
}
