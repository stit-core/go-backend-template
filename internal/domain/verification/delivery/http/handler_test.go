package http_test

import (
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
	uc.EXPECT().RequestPasswordReset(mock.Anything, mock.Anything).Return(nil)
	api := setup(t, uc)

	resp := api.Post("/api/auth/password-reset/request", strings.NewReader(`{"email":"a@b.com"}`))
	handlertest.AssertStatus(t, resp, http.StatusNoContent)
}

func TestPasswordResetConfirm_InvalidToken(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().ConfirmPasswordReset(mock.Anything, mock.Anything, mock.Anything).
		Return(verification.ErrInvalidResetToken)
	api := setup(t, uc)

	resp := api.Post("/api/auth/password-reset/confirm", strings.NewReader(`{"token":"x","password":"newpass123"}`))
	handlertest.AssertAPIError(t, resp, http.StatusBadRequest, "VERIFICATION_INVALID_RESET_TOKEN")
}

func TestEmailVerify_InvalidToken(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().ConfirmEmail(mock.Anything, mock.Anything).Return(verification.ErrInvalidVerificationToken)
	api := setup(t, uc)

	resp := api.Post("/api/auth/email-verify", strings.NewReader(`{"token":"x"}`))
	handlertest.AssertAPIError(t, resp, http.StatusBadRequest, "VERIFICATION_INVALID_VERIFICATION_TOKEN")
}

func TestPasswordResetConfirm_WeakPassword(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().ConfirmPasswordReset(mock.Anything, "x", mock.Anything).Return(verification.ErrWeakPassword)
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

func resetConfirmBody(password string) string {
	return fmt.Sprintf(`{"token":"x","password":%q}`, password)
}
