package http

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/auth"
	"git.stit.tech/stit-core/go-backend-template/internal/platform/authgate"
	"git.stit.tech/stit-core/go-backend-template/internal/platform/authlimit"
	"git.stit.tech/stit-core/golibinfra/httpkit"
)

type Usecase interface {
	Login(ctx context.Context, in auth.LoginInput) (*auth.Session, error)
	Refresh(ctx context.Context, refreshToken string) (*auth.Session, error)
	Logout(ctx context.Context, refreshToken string) error
	ChangePassword(ctx context.Context, oldPassword, newPassword string) error
}

type Handler struct {
	uc        Usecase
	cookieCfg authgate.CookieConfig
}

func NewHandler(uc Usecase, cookieCfg authgate.CookieConfig) *Handler {
	return &Handler{uc: uc, cookieCfg: cookieCfg}
}

func Register(api huma.API, h *Handler) {
	huma.Register(api, loginErrors.Declare(httpkit.Public(huma.Operation{
		OperationID: "loginUser",
		Method:      http.MethodPost,
		Path:        "/api/auth/login",
		Tags:        []string{"auth"},
		Summary:     "Аутентификация пользователя (cookie-сессия)",
	})), h.login)

	huma.Register(api, refreshErrors.Declare(httpkit.Public(huma.Operation{
		OperationID:   "refreshToken",
		Method:        http.MethodPost,
		Path:          "/api/auth/refresh",
		Tags:          []string{"auth"},
		DefaultStatus: http.StatusNoContent,
		Summary:       "Ротация access + refresh cookie",
		Description: "Refresh-токен одноразовый: обновление гасит его и выдаёт следующий. Повтор погашенного " +
			"токена — в том числе параллельный запрос с тем же cookie — закрывает сессию целиком " +
			"(AUTH_REFRESH_REUSED, вход заново), поэтому клиент обновляет сессию последовательно.",
	})), h.refresh)

	huma.Register(api, logoutErrors.Declare(httpkit.Public(huma.Operation{
		OperationID:   "logoutUser",
		Method:        http.MethodPost,
		Path:          "/api/auth/logout",
		Tags:          []string{"auth"},
		DefaultStatus: http.StatusNoContent,
		Summary:       "Завершение сессии",
	})), h.logout)

	huma.Register(api, changePasswordErrors.Declare(authgate.Secured(huma.Operation{
		OperationID:   "changePassword",
		Method:        http.MethodPost,
		Path:          "/api/auth/change-password",
		Tags:          []string{"auth"},
		DefaultStatus: http.StatusNoContent,
		Summary:       "Смена пароля",
		Description: "Смена пароля закрывает все сессии пользователя, и текущую: ответ гасит cookie сессии, дальше — вход " +
			"новым паролем.",
	})), h.changePassword)
}

func (h *Handler) login(ctx context.Context, in *AuthLoginInput) (*AuthLoginOutput, error) {
	result, err := h.uc.Login(ctx, auth.LoginInput{
		Email:    in.Body.Email,
		Password: in.Body.Password,
	})
	if err != nil {
		return nil, authErrors.Map(err)
	}
	out := &AuthLoginOutput{
		SetCookie: authgate.BuildAuthCookies(result.AccessToken, result.RefreshToken, h.cookieCfg),
	}
	out.Body.Data.FromEntity(result.User)
	return out, nil
}

func (h *Handler) refresh(ctx context.Context, in *AuthRefreshInput) (*AuthRefreshOutput, error) {
	result, err := h.uc.Refresh(ctx, in.RefreshToken)
	if err != nil {
		return nil, authErrors.Map(err)
	}
	return &AuthRefreshOutput{
		SetCookie: authgate.BuildAuthCookies(result.AccessToken, result.RefreshToken, h.cookieCfg),
	}, nil
}

// Logout доменных ошибок не отдаёт: битый, погашенный и пустой refresh — тоже выход; ошибка — сбой хранилища.
func (h *Handler) logout(ctx context.Context, in *AuthLogoutInput) (*AuthLogoutOutput, error) {
	if err := h.uc.Logout(ctx, in.RefreshToken); err != nil {
		return nil, authErrors.Map(err)
	}
	return &AuthLogoutOutput{SetCookie: authgate.BuildExpiredAuthCookies(h.cookieCfg)}, nil
}

func (h *Handler) changePassword(ctx context.Context, in *AuthChangePasswordInput) (*AuthChangePasswordOutput, error) {
	if err := h.uc.ChangePassword(ctx, in.Body.OldPassword, in.Body.NewPassword); err != nil {
		return nil, authErrors.Map(err)
	}
	return &AuthChangePasswordOutput{SetCookie: authgate.BuildExpiredAuthCookies(h.cookieCfg)}, nil
}

// authErrors переводит все ошибки домена (archtest), таблица операции объявляет коды своего usecase; неверный старый
// пароль — ошибка поля (422), а не 401: сессия цела.
var (
	invalidCredentialsRule = httpkit.ErrorRule{Err: auth.ErrInvalidCredentials, Status: http.StatusUnauthorized, Code: "AUTH_INVALID_CREDENTIALS", Detail: "invalid email or password"}
	accountLockedRule      = httpkit.ErrorRule{Err: auth.ErrAccountLocked, Status: http.StatusTooManyRequests, Code: "AUTH_ACCOUNT_LOCKED", Detail: "account temporarily locked"}
	refreshExpiredRule     = httpkit.ErrorRule{Err: auth.ErrRefreshExpired, Status: http.StatusUnauthorized, Code: "AUTH_REFRESH_EXPIRED", Detail: "refresh token expired: log in again"}
	refreshReusedRule      = httpkit.ErrorRule{Err: auth.ErrRefreshReused, Status: http.StatusUnauthorized, Code: "AUTH_REFRESH_REUSED", Detail: "refresh token reused: log in again"}
	refreshInvalidRule     = httpkit.ErrorRule{Err: auth.ErrRefreshInvalid, Status: http.StatusUnauthorized, Code: "AUTH_REFRESH_INVALID", Detail: "refresh token invalid or missing: log in again"}
	weakPasswordRule       = httpkit.ErrorRule{Err: auth.ErrWeakPassword, Status: http.StatusUnprocessableEntity, Code: "AUTH_WEAK_PASSWORD", Detail: "password too weak", Field: "body.new_password"}
	passwordTooLongRule    = httpkit.ErrorRule{Err: auth.ErrPasswordTooLong, Status: http.StatusUnprocessableEntity, Code: "AUTH_PASSWORD_TOO_LONG", Detail: "password too long", Field: "body.new_password"}
	wrongOldPasswordRule   = httpkit.ErrorRule{Err: auth.ErrWrongOldPassword, Status: http.StatusUnprocessableEntity, Code: "AUTH_INVALID_OLD_PASSWORD", Detail: "old password does not match", Field: "body.old_password"}
	userNotFoundRule       = httpkit.ErrorRule{Err: auth.ErrUserNotFound, Status: http.StatusNotFound, Code: "AUTH_USER_NOT_FOUND", Detail: "user not found"}
	concurrentUpdateRule   = httpkit.ErrorRule{Err: auth.ErrConcurrentUpdate, Status: http.StatusConflict, Code: "AUTH_CONCURRENT_UPDATE", Detail: "user changed concurrently, retry"}

	authErrors = httpkit.NewErrorMap(invalidCredentialsRule, accountLockedRule, refreshExpiredRule, refreshReusedRule,
		refreshInvalidRule, weakPasswordRule, passwordTooLongRule, wrongOldPasswordRule, userNotFoundRule, concurrentUpdateRule,
		authgate.ForbiddenRule)

	loginErrors          = httpkit.NewErrorMap(invalidCredentialsRule, accountLockedRule, authlimit.Rule)
	refreshErrors        = httpkit.NewErrorMap(refreshExpiredRule, refreshReusedRule, refreshInvalidRule, authlimit.Rule)
	logoutErrors         = httpkit.NewErrorMap(authlimit.Rule)
	changePasswordErrors = httpkit.NewErrorMap(weakPasswordRule, passwordTooLongRule, wrongOldPasswordRule, accountLockedRule,
		userNotFoundRule, concurrentUpdateRule, authgate.ForbiddenRule, authlimit.Rule)
)
