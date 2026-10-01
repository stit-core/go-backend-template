package authgate

import "net/http"

const (
	CookieAccess  = "access_token"
	CookieRefresh = "refresh_token"
)

type CookieConfig struct {
	Secure bool `default:"true" envconfig:"COOKIE_SECURE"`
}

func SetAuthCookies(w http.ResponseWriter, access, refresh string, cfg CookieConfig) {
	http.SetCookie(w, authCookie(CookieAccess, access, cfg))
	http.SetCookie(w, authCookie(CookieRefresh, refresh, cfg))
}

func ClearAuthCookies(w http.ResponseWriter, cfg CookieConfig) {
	http.SetCookie(w, expiredCookie(CookieAccess, cfg))
	http.SetCookie(w, expiredCookie(CookieRefresh, cfg))
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
