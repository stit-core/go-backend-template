package http_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/mock"

	"git.stit.tech/stit-infra/go-backend-template/internal/domain/verification"
	verifhttp "git.stit.tech/stit-infra/go-backend-template/internal/domain/verification/delivery/http"
	"git.stit.tech/stit-infra/go-backend-template/internal/domain/verification/delivery/http/mocks"
	"git.stit.tech/stit-infra/golibinfra/testkit/handlertest"
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
