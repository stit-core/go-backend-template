package http_test

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/verification"
	verifhttp "git.stit.tech/stit-core/go-backend-template/internal/domain/verification/delivery/http"
	"git.stit.tech/stit-core/go-backend-template/internal/domain/verification/delivery/http/mocks"
	"git.stit.tech/stit-core/go-backend-template/pkg/policy"
	"git.stit.tech/stit-core/golibinfra/authnkit"
	"git.stit.tech/stit-core/golibinfra/testkit/handlertest"
)

func setup(t *testing.T, uc *mocks.MockUsecase) handlertest.API {
	t.Helper()
	return handlertest.NewAPI(t, func(api huma.API) {
		verifhttp.Register(api, verifhttp.NewHandler(uc))
	})
}

func TestPasswordResetRequest_NoContent(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().RequestPasswordReset(mock.Anything, "a@b.com").Return(nil).Once()
	api := setup(t, uc)

	resp := api.Post("/api/auth/password-reset/request", strings.NewReader(`{"email":"a@b.com"}`))
	handlertest.AssertStatus(t, resp, http.StatusNoContent)
	require.Empty(t, resp.Body.String())
}

// Сброс — только для адреса: не адрес схема отклоняет до usecase, и лимит запросов его не ключует.
func TestPasswordResetRequest_NotAnAddress(t *testing.T) {
	t.Parallel()
	api := setup(t, mocks.NewMockUsecase(t))

	resp := api.Post("/api/auth/password-reset/request", strings.NewReader(`{"email":"not an address"}`))
	handlertest.AssertAPIError(t, resp, http.StatusUnprocessableEntity, "VALIDATION_FAILED")
	errs := handlertest.DecodeAPIError(t, resp).Errors
	require.Len(t, errs, 1)
	require.Equal(t, "body.email", errs[0].Location)
}

// Сбой поиска учётной записи от адреса не зависит и уходит ответом 500, а не тихим 204.
func TestPasswordResetRequest_Failure(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().RequestPasswordReset(mock.Anything, "a@b.com").Return(errors.New("lookup user: db down")).Once()
	api := setup(t, uc)

	resp := api.Post("/api/auth/password-reset/request", strings.NewReader(`{"email":"a@b.com"}`))
	handlertest.AssertAPIError(t, resp, http.StatusInternalServerError, "INTERNAL")
}

func TestPasswordResetConfirm_NoContent(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().ConfirmPasswordReset(mock.Anything, "tok", "newpass123").Return(nil).Once()
	api := setup(t, uc)

	resp := api.Post("/api/auth/password-reset/confirm", strings.NewReader(`{"token":"tok","password":"newpass123"}`))
	handlertest.AssertStatus(t, resp, http.StatusNoContent)
	require.Empty(t, resp.Body.String())
}

func TestEmailVerify_NoContent(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().ConfirmEmail(mock.Anything, "tok").Return(nil).Once()
	api := setup(t, uc)

	resp := api.Post("/api/auth/email-verify", strings.NewReader(`{"token":"tok"}`))
	handlertest.AssertStatus(t, resp, http.StatusNoContent)
	require.Empty(t, resp.Body.String())
}

func TestPasswordResetConfirm_InvalidToken(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().ConfirmPasswordReset(mock.Anything, "x", "newpass123").Return(verification.ErrInvalidResetToken).Once()
	api := setup(t, uc)

	resp := api.Post("/api/auth/password-reset/confirm", strings.NewReader(`{"token":"x","password":"newpass123"}`))
	handlertest.AssertAPIError(t, resp, http.StatusBadRequest, "VERIFICATION_INVALID_RESET_TOKEN")
}

func TestPasswordResetConfirm_TokenUsed(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().ConfirmPasswordReset(mock.Anything, "x", "newpass123").Return(verification.ErrTokenUsed).Once()
	api := setup(t, uc)

	resp := api.Post("/api/auth/password-reset/confirm", strings.NewReader(`{"token":"x","password":"newpass123"}`))
	handlertest.AssertAPIError(t, resp, http.StatusConflict, "VERIFICATION_TOKEN_USED")
}

func TestPasswordResetConfirm_ConcurrentUpdate(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().ConfirmPasswordReset(mock.Anything, "x", "newpass123").Return(verification.ErrConcurrentUpdate).Once()
	api := setup(t, uc)

	resp := api.Post("/api/auth/password-reset/confirm", strings.NewReader(`{"token":"x","password":"newpass123"}`))
	handlertest.AssertAPIError(t, resp, http.StatusConflict, "VERIFICATION_CONCURRENT_UPDATE")
}

func TestEmailVerify_TokenUsed(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().ConfirmEmail(mock.Anything, "x").Return(verification.ErrTokenUsed).Once()
	api := setup(t, uc)

	resp := api.Post("/api/auth/email-verify", strings.NewReader(`{"token":"x"}`))
	handlertest.AssertAPIError(t, resp, http.StatusConflict, "VERIFICATION_TOKEN_USED")
}

func TestEmailVerify_InvalidToken(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().ConfirmEmail(mock.Anything, "x").Return(verification.ErrInvalidVerificationToken).Once()
	api := setup(t, uc)

	resp := api.Post("/api/auth/email-verify", strings.NewReader(`{"token":"x"}`))
	handlertest.AssertAPIError(t, resp, http.StatusBadRequest, "VERIFICATION_INVALID_VERIFICATION_TOKEN")
}

func TestPasswordResetConfirm_WeakPassword(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().ConfirmPasswordReset(mock.Anything, "x", strings.Repeat("p", policy.MinPasswordLen)).Return(verification.ErrWeakPassword).Once()
	api := setup(t, uc)

	resp := api.Post("/api/auth/password-reset/confirm", strings.NewReader(resetConfirmBody(strings.Repeat("p", policy.MinPasswordLen))))
	handlertest.AssertAPIError(t, resp, http.StatusUnprocessableEntity, "VERIFICATION_WEAK_PASSWORD")
	errs := handlertest.DecodeAPIError(t, resp).Errors
	require.Len(t, errs, 1)
	require.Equal(t, "body.password", errs[0].Location)
}

// Правило длины пароля — policy.MinPasswordLen; схема сброса отклоняет короче его до usecase.
func TestPasswordResetConfirm_ShortPassword(t *testing.T) {
	t.Parallel()
	api := setup(t, mocks.NewMockUsecase(t))

	resp := api.Post("/api/auth/password-reset/confirm", strings.NewReader(resetConfirmBody(strings.Repeat("p", policy.MinPasswordLen-1))))
	handlertest.AssertAPIError(t, resp, http.StatusUnprocessableEntity, "VALIDATION_FAILED")
	errs := handlertest.DecodeAPIError(t, resp).Errors
	require.Len(t, errs, 1)
	require.Equal(t, "body.password", errs[0].Location)
}

// Предел хэшера — в байтах, maxLength схемы — в символах: 1024 символа не из ASCII доходят до usecase, и
// его отказ — ошибка поля; 1025 символов схема отклоняет сама.
func TestPasswordResetConfirm_PasswordTooLong(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("я", authnkit.MaxPasswordLen)
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().ConfirmPasswordReset(mock.Anything, "x", long).Return(verification.ErrPasswordTooLong).Once()
	api := setup(t, uc)

	resp := api.Post("/api/auth/password-reset/confirm", strings.NewReader(resetConfirmBody(long)))
	handlertest.AssertAPIError(t, resp, http.StatusUnprocessableEntity, "VERIFICATION_PASSWORD_TOO_LONG")
	errs := handlertest.DecodeAPIError(t, resp).Errors
	require.Len(t, errs, 1)
	require.Equal(t, "body.password", errs[0].Location)

	resp = api.Post("/api/auth/password-reset/confirm", strings.NewReader(resetConfirmBody(long+"я")))
	handlertest.AssertAPIError(t, resp, http.StatusUnprocessableEntity, "VALIDATION_FAILED")
}

func resetConfirmBody(password string) string {
	return fmt.Sprintf(`{"token":"x","password":%q}`, password)
}
