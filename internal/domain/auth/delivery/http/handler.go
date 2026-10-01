package http

import (
	"context"
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
	huma.Register(api, loginErrors.Declare(huma.Operation{
		OperationID: "loginUser",
		Method:      http.MethodPost,
		Path:        "/api/auth/login",
		Tags:        []string{"auth"},
		Summary:     "Аутентификация пользователя (cookie-сессия)",
	}), h.login)

	huma.Register(api, refreshErrors.Declare(huma.Operation{
		OperationID:   "refreshToken",
		Method:        http.MethodPost,
		Path:          "/api/auth/refresh",
		Tags:          []string{"auth"},
		DefaultStatus: http.StatusNoContent,
		Summary:       "Ротация access + refresh cookie",
	}), h.refresh)

	huma.Register(api, huma.Operation{
		OperationID:   "logoutUser",
		Method:        http.MethodPost,
		Path:          "/api/auth/logout",
		Tags:          []string{"auth"},
		DefaultStatus: http.StatusNoContent,
		Summary:       "Завершение сессии",
	}, h.logout)

	huma.Register(api, changePasswordErrors.Declare(authgate.Secured(huma.Operation{
		OperationID:   "changePassword",
		Method:        http.MethodPost,
		Path:          "/api/auth/change-password",
		Tags:          []string{"auth"},
		DefaultStatus: http.StatusNoContent,
		Summary:       "Смена пароля",
	})), h.changePassword)
}

func (h *Handler) login(ctx context.Context, in *AuthLoginInput) (*AuthLoginOutput, error) {
	result, err := h.uc.Login(ctx, &auth.LoginInput{
		Email:    in.Body.Email,
		Password: in.Body.Password,
	})
	if err != nil {
		return nil, loginErrors.Map(err)
	}
	out := &AuthLoginOutput{
		SetCookie: authgate.BuildAuthCookies(result.AccessToken, result.RefreshToken, h.cookieCfg),
	}
	out.Body.Data.FromEntity(result.User)
	return out, nil
}

func (h *Handler) refresh(ctx context.Context, in *AuthRefreshInput) (*AuthRefreshOutput, error) {
	if in.RefreshCookie.Value == "" {
		return nil, refreshErrors.Map(auth.ErrRefreshExpired)
	}
	result, err := h.uc.Refresh(ctx, in.RefreshCookie.Value)
	if err != nil {
		return nil, refreshErrors.Map(err)
	}
	return &AuthRefreshOutput{
		SetCookie: authgate.BuildAuthCookies(result.AccessToken, result.RefreshToken, h.cookieCfg),
	}, nil
}

// Logout доменных ошибок не отдаёт: битый или погашенный refresh — тоже выход; ошибка — сбой хранилища.
func (h *Handler) logout(ctx context.Context, in *AuthLogoutInput) (*AuthLogoutOutput, error) {
	if in.RefreshCookie.Value != "" {
		if err := h.uc.Logout(ctx, in.RefreshCookie.Value); err != nil {
			return nil, err
		}
	}
	return &AuthLogoutOutput{SetCookie: authgate.BuildExpiredAuthCookies(h.cookieCfg)}, nil
}

func (h *Handler) changePassword(ctx context.Context, in *AuthChangePasswordInput) (*struct{}, error) {
	if err := h.uc.ChangePassword(ctx, in.Body.UserID, in.Body.OldPassword, in.Body.NewPassword); err != nil {
		return nil, changePasswordErrors.Map(err)
	}
	return nil, nil
}

// Таблица операции — правила того, что отдаёт её usecase (полноту держит тест). 401 входа и refresh —
// «войти заново»; неверный старый пароль у смены — ошибка поля, а не 401: сессия цела.
var (
	loginErrors = httpkit.NewErrorMap(
		httpkit.ErrorRule{Err: auth.ErrInvalidCredentials, Status: http.StatusUnauthorized, Code: "AUTH_INVALID_CREDENTIALS", Detail: "invalid email or password"},
		httpkit.ErrorRule{Err: auth.ErrAccountLocked, Status: http.StatusTooManyRequests, Code: "AUTH_ACCOUNT_LOCKED", Detail: "account temporarily locked"},
	)
	refreshErrors = httpkit.NewErrorMap(
		httpkit.ErrorRule{Err: auth.ErrRefreshExpired, Status: http.StatusUnauthorized, Code: "AUTH_REFRESH_EXPIRED", Detail: "refresh token expired or missing: log in again"},
		httpkit.ErrorRule{Err: auth.ErrRefreshReused, Status: http.StatusUnauthorized, Code: "AUTH_REFRESH_REUSED", Detail: "refresh token reused: log in again"},
		httpkit.ErrorRule{Err: auth.ErrInvalidCredentials, Status: http.StatusUnauthorized, Code: "AUTH_REFRESH_INVALID", Detail: "refresh token invalid: log in again"},
	)
	changePasswordErrors = httpkit.NewErrorMap(
		httpkit.ErrorRule{Err: auth.ErrWeakPassword, Status: http.StatusUnprocessableEntity, Code: "AUTH_WEAK_PASSWORD", Detail: "password too weak", Field: "body.new_password"},
		httpkit.ErrorRule{Err: auth.ErrInvalidCredentials, Status: http.StatusUnprocessableEntity, Code: "AUTH_INVALID_OLD_PASSWORD", Detail: "old password does not match", Field: "body.old_password"},
		httpkit.ErrorRule{Err: auth.ErrUserNotFound, Status: http.StatusNotFound, Code: "AUTH_USER_NOT_FOUND", Detail: "user not found"},
		httpkit.ErrorRule{Err: auth.ErrConcurrentUpdate, Status: http.StatusConflict, Code: "AUTH_CONCURRENT_UPDATE", Detail: "user changed concurrently, retry"},
		authgate.ForbiddenRule,
	)
)
