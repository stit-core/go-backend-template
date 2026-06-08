package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/verification"
	"git.stit.tech/stit-core/golibinfra/httpkit"
)

type Usecase interface {
	RequestPasswordReset(ctx context.Context, email string) error
	ConfirmPasswordReset(ctx context.Context, token, newPassword string) error
	ConfirmEmail(ctx context.Context, token string) error
}

type Handler struct {
	uc Usecase
}

func NewHandler(uc Usecase) *Handler { return &Handler{uc: uc} }

func Register(api huma.API, h *Handler) {
	huma.Register(api, huma.Operation{
		OperationID:   "requestPasswordReset",
		Method:        http.MethodPost,
		Path:          "/api/auth/password-reset/request",
		Tags:          []string{"auth"},
		DefaultStatus: http.StatusNoContent,
		Summary:       "Запрос сброса пароля",
	}, h.requestPasswordReset)

	huma.Register(api, huma.Operation{
		OperationID:   "confirmPasswordReset",
		Method:        http.MethodPost,
		Path:          "/api/auth/password-reset/confirm",
		Tags:          []string{"auth"},
		DefaultStatus: http.StatusNoContent,
		Summary:       "Подтверждение сброса пароля",
	}, h.confirmPasswordReset)

	huma.Register(api, huma.Operation{
		OperationID:   "confirmEmail",
		Method:        http.MethodPost,
		Path:          "/api/auth/email-verify",
		Tags:          []string{"auth"},
		DefaultStatus: http.StatusNoContent,
		Summary:       "Подтверждение email",
	}, h.confirmEmail)
}

func (h *Handler) requestPasswordReset(ctx context.Context, in *PasswordResetRequestInput) (*struct{}, error) {
	if err := h.uc.RequestPasswordReset(ctx, in.Body.Email); err != nil {
		return nil, mapErr(err)
	}
	return nil, nil
}

func (h *Handler) confirmPasswordReset(ctx context.Context, in *PasswordResetConfirmInput) (*struct{}, error) {
	if err := h.uc.ConfirmPasswordReset(ctx, in.Body.Token, in.Body.Password); err != nil {
		return nil, mapErr(err)
	}
	return nil, nil
}

func (h *Handler) confirmEmail(ctx context.Context, in *EmailVerifyInput) (*struct{}, error) {
	if err := h.uc.ConfirmEmail(ctx, in.Body.Token); err != nil {
		return nil, mapErr(err)
	}
	return nil, nil
}

func mapErr(err error) error {
	return httpkit.MapErr(
		err,
		httpkit.ErrorMapping{
			Match:  func(e error) bool { return errors.Is(e, verification.ErrInvalidResetToken) },
			Code:   "VERIFICATION_INVALID_RESET_TOKEN",
			Status: http.StatusBadRequest,
			Detail: "invalid or expired reset token",
		},
		httpkit.ErrorMapping{
			Match:  func(e error) bool { return errors.Is(e, verification.ErrInvalidVerificationToken) },
			Code:   "VERIFICATION_INVALID_VERIFICATION_TOKEN",
			Status: http.StatusBadRequest,
			Detail: "invalid or expired verification token",
		},
		httpkit.ErrorMapping{
			Match:  func(e error) bool { return errors.Is(e, verification.ErrWeakPassword) },
			Code:   "VERIFICATION_WEAK_PASSWORD",
			Status: http.StatusBadRequest,
			Detail: "password too weak",
		},
	)
}
