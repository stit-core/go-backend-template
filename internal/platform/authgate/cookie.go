package authgate

import "net/http"

const (
	CookieAccess  = "access_token"
	CookieRefresh = "refresh_token"
)

type CookieConfig struct {
	Secure bool `default:"true" envconfig:"COOKIE_SECURE"`
}

func BuildAuthCookies(access, refresh string, cfg CookieConfig) []http.Cookie {
	return []http.Cookie{*authCookie(CookieAccess, access, cfg), *authCookie(CookieRefresh, refresh, cfg)}
}

func BuildExpiredAuthCookies(cfg CookieConfig) []http.Cookie {
	return []http.Cookie{*expiredCookie(CookieAccess, cfg), *expiredCookie(CookieRefresh, cfg)}
}

func authCookie(name, value string, cfg CookieConfig) *http.Cookie {
	return &http.Cookie{ //nolint:gosec // Secure — COOKIE_SECURE (по умолчанию true), false вне dev не пускает config.Guard
		Name:     name,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   cfg.Secure,
	}
}

func expiredCookie(name string, cfg CookieConfig) *http.Cookie {
	return &http.Cookie{ //nolint:gosec // Secure — COOKIE_SECURE (по умолчанию true), false вне dev не пускает config.Guard
		Name:     name,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   cfg.Secure,
		MaxAge:   -1,
	}
}
