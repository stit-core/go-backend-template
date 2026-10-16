package http

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/verification"
	"git.stit.tech/stit-core/go-backend-template/internal/platform/authlimit"
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
	huma.Register(api, requestPasswordResetErrors.Declare(httpkit.Public(huma.Operation{
		OperationID:   "requestPasswordReset",
		Method:        http.MethodPost,
		Path:          "/api/auth/password-reset/request",
		Tags:          []string{"auth"},
		DefaultStatus: http.StatusNoContent,
		Summary:       "Запрос сброса пароля",
	})), h.requestPasswordReset)

	huma.Register(api, confirmPasswordResetErrors.Declare(httpkit.Public(huma.Operation{
		OperationID:   "confirmPasswordReset",
		Method:        http.MethodPost,
		Path:          "/api/auth/password-reset/confirm",
		Tags:          []string{"auth"},
		DefaultStatus: http.StatusNoContent,
		Summary:       "Подтверждение сброса пароля",
	})), h.confirmPasswordReset)

	huma.Register(api, confirmEmailErrors.Declare(httpkit.Public(huma.Operation{
		OperationID:   "confirmEmail",
		Method:        http.MethodPost,
		Path:          "/api/auth/email-verify",
		Tags:          []string{"auth"},
		DefaultStatus: http.StatusNoContent,
		Summary:       "Подтверждение email",
	})), h.confirmEmail)
}

// requestPasswordReset: доменных ошибок usecase не отдаёт — отсутствие учётной записи неотличимо от
// успеха.
func (h *Handler) requestPasswordReset(ctx context.Context, in *VerificationPasswordResetRequestInput) (*struct{}, error) {
	if err := h.uc.RequestPasswordReset(ctx, in.Body.Email); err != nil {
		return nil, verificationErrors.Map(err)
	}
	return nil, nil
}

func (h *Handler) confirmPasswordReset(ctx context.Context, in *VerificationPasswordResetConfirmInput) (*struct{}, error) {
	if err := h.uc.ConfirmPasswordReset(ctx, in.Body.Token, in.Body.Password); err != nil {
		return nil, verificationErrors.Map(err)
	}
	return nil, nil
}

func (h *Handler) confirmEmail(ctx context.Context, in *VerificationEmailVerifyInput) (*struct{}, error) {
	if err := h.uc.ConfirmEmail(ctx, in.Body.Token); err != nil {
		return nil, verificationErrors.Map(err)
	}
	return nil, nil
}

// Правило каждой ошибки домена — одно. verificationErrors переводит в хендлерах их все (ни одна не
// уходит 500 — archtest), таблица операции объявляет в OpenAPI те, что отдаёт её usecase.
var (
	weakPasswordRule     = httpkit.ErrorRule{Err: verification.ErrWeakPassword, Status: http.StatusUnprocessableEntity, Code: "VERIFICATION_WEAK_PASSWORD", Detail: "password too weak", Field: "body.password"}
	passwordTooLongRule  = httpkit.ErrorRule{Err: verification.ErrPasswordTooLong, Status: http.StatusUnprocessableEntity, Code: "VERIFICATION_PASSWORD_TOO_LONG", Detail: "password too long", Field: "body.password"}
	invalidResetRule     = httpkit.ErrorRule{Err: verification.ErrInvalidResetToken, Status: http.StatusBadRequest, Code: "VERIFICATION_INVALID_RESET_TOKEN", Detail: "invalid or expired reset token"}
	invalidVerifyRule    = httpkit.ErrorRule{Err: verification.ErrInvalidVerificationToken, Status: http.StatusBadRequest, Code: "VERIFICATION_INVALID_VERIFICATION_TOKEN", Detail: "invalid or expired verification token"}
	tokenUsedRule        = httpkit.ErrorRule{Err: verification.ErrTokenUsed, Status: http.StatusConflict, Code: "VERIFICATION_TOKEN_USED", Detail: "token already used"}
	concurrentUpdateRule = httpkit.ErrorRule{Err: verification.ErrConcurrentUpdate, Status: http.StatusConflict, Code: "VERIFICATION_CONCURRENT_UPDATE", Detail: "user changed concurrently, retry"}
	userNotFoundRule     = httpkit.ErrorRule{Err: verification.ErrUserNotFound, Status: http.StatusNotFound, Code: "VERIFICATION_USER_NOT_FOUND", Detail: "user not found"}
	busyRule             = httpkit.ErrorRule{Err: verification.ErrBusy, Status: http.StatusServiceUnavailable, Code: "VERIFICATION_BUSY", Detail: "password hasher busy, retry later"}

	verificationErrors = httpkit.NewErrorMap(weakPasswordRule, passwordTooLongRule, invalidResetRule, invalidVerifyRule, tokenUsedRule,
		concurrentUpdateRule, userNotFoundRule, busyRule)

	requestPasswordResetErrors = httpkit.NewErrorMap(authlimit.Rule)
	confirmPasswordResetErrors = httpkit.NewErrorMap(weakPasswordRule, passwordTooLongRule, invalidResetRule, tokenUsedRule, busyRule, authlimit.Rule)
	confirmEmailErrors         = httpkit.NewErrorMap(invalidVerifyRule, tokenUsedRule, authlimit.Rule)
)
