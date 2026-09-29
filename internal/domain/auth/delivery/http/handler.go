package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/auth"
	"git.stit.tech/stit-core/go-backend-template/internal/platform/authgate"
	"git.stit.tech/stit-core/golibinfra/httpkit"
)

type Usecase interface {
	Login(ctx context.Context, in *auth.LoginInput) (*auth.LoginOutput, error)
	Refresh(ctx context.Context, refreshToken string) (*auth.LoginOutput, error)
	Logout(ctx context.Context, refreshToken string) error
	ChangePassword(ctx context.Context, userID, oldPassword, newPassword string) error
}

type Handler struct {
	uc        Usecase
	cookieCfg authgate.CookieConfig
}

func NewHandler(uc Usecase, cookieCfg authgate.CookieConfig) *Handler {
	return &Handler{uc: uc, cookieCfg: cookieCfg}
}

func Register(api huma.API, h *Handler) {
	huma.Register(api, huma.Operation{
		OperationID: "loginUser",
		Method:      http.MethodPost,
		Path:        "/api/auth/login",
		Tags:        []string{"auth"},
		Summary:     "Аутентификация пользователя (cookie-сессия)",
	}, h.login)

	huma.Register(api, huma.Operation{
		OperationID:   "refreshToken",
		Method:        http.MethodPost,
		Path:          "/api/auth/refresh",
		Tags:          []string{"auth"},
		DefaultStatus: http.StatusNoContent,
		Summary:       "Ротация access + refresh cookie",
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
		Security:      []map[string][]string{{"Cookie": {}}},
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
		return nil, authErrors.Map(err)
	}
	out := &LoginOutput{
		SetCookie: authgate.BuildAuthCookies(result.AccessToken, result.RefreshToken, h.cookieCfg),
	}
	out.Body.User.FromEntity(result.User)
	return out, nil
}

func (h *Handler) refresh(ctx context.Context, in *RefreshInput) (*RefreshOutput, error) {
	if in.RefreshCookie.Value == "" {
		return nil, refreshErrors.Map(auth.ErrRefreshExpired)
	}
	result, err := h.uc.Refresh(ctx, in.RefreshCookie.Value)
	if err != nil {
		return nil, refreshErrors.Map(err)
	}
	return &RefreshOutput{
		SetCookie: authgate.BuildAuthCookies(result.AccessToken, result.RefreshToken, h.cookieCfg),
	}, nil
}

func (h *Handler) logout(ctx context.Context, in *LogoutInput) (*LogoutOutput, error) {
	if in.RefreshCookie.Value != "" {
		if err := h.uc.Logout(ctx, in.RefreshCookie.Value); err != nil {
			// Битый/просроченный/reused refresh — всё равно очищаем cookies и отвечаем 204
			// (logout идемпотентен с точки зрения клиента).
			if !errors.Is(err, auth.ErrRefreshExpired) && !errors.Is(err, auth.ErrRefreshReused) {
				return nil, authErrors.Map(err)
			}
		}
	}
	return &LogoutOutput{SetCookie: authgate.BuildExpiredAuthCookies(h.cookieCfg)}, nil
}

func (h *Handler) changePassword(ctx context.Context, in *ChangePasswordInput) (*struct{}, error) {
	if err := h.uc.ChangePassword(ctx, in.Body.UserID, in.Body.OldPassword, in.Body.NewPassword); err != nil {
		return nil, authErrors.Map(err)
	}
	return nil, nil
}

var authErrors = httpkit.NewErrorMap(
	httpkit.ErrorRule{Err: auth.ErrInvalidCredentials, Status: http.StatusUnauthorized, Code: "AUTH_INVALID_CREDENTIALS", Detail: "invalid email or password"},
	httpkit.ErrorRule{Err: auth.ErrRefreshExpired, Status: http.StatusUnauthorized, Code: "AUTH_REFRESH_EXPIRED", Detail: "refresh token expired"},
	httpkit.ErrorRule{Err: auth.ErrRefreshReused, Status: http.StatusUnauthorized, Code: "AUTH_REFRESH_REUSED", Detail: "refresh token reused"},
	httpkit.ErrorRule{Err: auth.ErrAccountLocked, Status: http.StatusTooManyRequests, Code: "AUTH_ACCOUNT_LOCKED", Detail: "account temporarily locked"},
	httpkit.ErrorRule{Err: auth.ErrWeakPassword, Status: http.StatusUnprocessableEntity, Code: "AUTH_WEAK_PASSWORD", Detail: "password too weak", Field: "body.new_password"},
	authgate.ForbiddenRule,
)

var refreshErrors = httpkit.NewErrorMap(
	httpkit.ErrorRule{Err: auth.ErrRefreshExpired, Status: http.StatusUnauthorized, Code: "AUTH_REFRESH_EXPIRED", Detail: "refresh token expired or missing"},
	httpkit.ErrorRule{Err: auth.ErrRefreshReused, Status: http.StatusUnauthorized, Code: "AUTH_REFRESH_REUSED", Detail: "refresh token reused"},
)
