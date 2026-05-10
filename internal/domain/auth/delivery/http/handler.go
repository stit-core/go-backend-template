package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"git.stit.tech/akadmin/go-backend-template/internal/domain/auth"
	"git.stit.tech/stit/golibinfra/httpkit"
)

type Usecase interface {
	Login(ctx context.Context, in *auth.LoginInput) (*auth.LoginOutput, error)
	Refresh(ctx context.Context, refreshToken string) (*auth.LoginOutput, error)
	Logout(ctx context.Context, refreshToken string) error
	ChangePassword(ctx context.Context, userID, oldPassword, newPassword string) error
}

type Handler struct {
	uc Usecase
}

func NewHandler(uc Usecase) *Handler { return &Handler{uc: uc} }

func Register(api huma.API, h *Handler) {
	huma.Register(api, huma.Operation{
		OperationID: "loginUser",
		Method:      http.MethodPost,
		Path:        "/api/auth/login",
		Tags:        []string{"auth"},
		Summary:     "Аутентификация пользователя",
	}, h.login)

	huma.Register(api, huma.Operation{
		OperationID: "refreshToken",
		Method:      http.MethodPost,
		Path:        "/api/auth/refresh",
		Tags:        []string{"auth"},
		Summary:     "Обновление токенов",
	}, h.refresh)

	huma.Register(api, huma.Operation{
		OperationID:   "logoutUser",
		Method:        http.MethodPost,
		Path:          "/api/auth/logout",
		Tags:          []string{"auth"},
		DefaultStatus: http.StatusNoContent,
		Summary:       "Завершение сессии",
	}, h.logout)

	huma.Register(api, huma.Operation{
		OperationID:   "changePassword",
		Method:        http.MethodPost,
		Path:          "/api/auth/change-password",
		Tags:          []string{"auth"},
		Security:      []map[string][]string{{"Bearer": {}}},
		DefaultStatus: http.StatusNoContent,
		Summary:       "Смена пароля",
	}, h.changePassword)
}

func (h *Handler) login(ctx context.Context, in *LoginInput) (*LoginOutput, error) {
	result, err := h.uc.Login(ctx, &auth.LoginInput{
		Email:    in.Body.Email,
		Password: in.Body.Password,
	})
	if err != nil {
		return nil, mapErr(err)
	}
	out := &LoginOutput{}
	out.Body.AccessToken = result.AccessToken
	out.Body.RefreshToken = result.RefreshToken
	out.Body.User.FromEntity(result.User)
	return out, nil
}

func (h *Handler) refresh(ctx context.Context, in *RefreshInput) (*RefreshOutput, error) {
	result, err := h.uc.Refresh(ctx, in.Body.RefreshToken)
	if err != nil {
		return nil, mapErr(err)
	}
	out := &RefreshOutput{}
	out.Body.AccessToken = result.AccessToken
	out.Body.RefreshToken = result.RefreshToken
	out.Body.User.FromEntity(result.User)
	return out, nil
}

func (h *Handler) logout(ctx context.Context, in *LogoutInput) (*struct{}, error) {
	if err := h.uc.Logout(ctx, in.Body.RefreshToken); err != nil {
		return nil, mapErr(err)
	}
	return nil, nil
}

func (h *Handler) changePassword(ctx context.Context, in *ChangePasswordInput) (*struct{}, error) {
	if err := h.uc.ChangePassword(ctx, in.Body.UserID, in.Body.OldPassword, in.Body.NewPassword); err != nil {
		return nil, mapErr(err)
	}
	return nil, nil
}

func mapErr(err error) error {
	return httpkit.MapErr(
		err,
		httpkit.ErrorMapping{
			Match:  func(e error) bool { return errors.Is(e, auth.ErrInvalidCredentials) },
			Code:   "AUTH_INVALID_CREDENTIALS",
			Status: http.StatusUnauthorized,
			Detail: "invalid email or password",
		},
		httpkit.ErrorMapping{
			Match:  func(e error) bool { return errors.Is(e, auth.ErrRefreshExpired) },
			Code:   "AUTH_REFRESH_EXPIRED",
			Status: http.StatusUnauthorized,
			Detail: "refresh token expired",
		},
		httpkit.ErrorMapping{
			Match:  func(e error) bool { return errors.Is(e, auth.ErrRefreshReused) },
			Code:   "AUTH_REFRESH_REUSED",
			Status: http.StatusUnauthorized,
			Detail: "refresh token reused",
		},
		httpkit.ErrorMapping{
			Match:  func(e error) bool { return errors.Is(e, auth.ErrAccountLocked) },
			Code:   "AUTH_ACCOUNT_LOCKED",
			Status: http.StatusTooManyRequests,
			Detail: "account temporarily locked",
		},
		httpkit.ErrorMapping{
			Match:  func(e error) bool { return errors.Is(e, auth.ErrWeakPassword) },
			Code:   "AUTH_WEAK_PASSWORD",
			Status: http.StatusBadRequest,
			Detail: "password too weak",
		},
		httpkit.ErrorMapping{
			Match: func(e error) bool {
				var fe *auth.ForbiddenError
				return errors.As(e, &fe)
			},
			Code:   "FORBIDDEN",
			Status: http.StatusForbidden,
			Detail: "forbidden",
		},
	)
}
