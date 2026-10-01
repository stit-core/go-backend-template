package authgate

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"git.stit.tech/stit-core/golibinfra/authnkit"
	"git.stit.tech/stit-core/golibinfra/httpkit"
)

// SessionErrors — 401 входа под cookie сверх UNAUTHORIZED кита («войти заново»): истёкший access —
// «обновить сессию» через POST /api/auth/refresh.
var SessionErrors = httpkit.NewErrorMap(
	httpkit.ErrorRule{Err: authnkit.ErrTokenExpired, Status: http.StatusUnauthorized, Code: "AUTH_ACCESS_EXPIRED", Detail: "access token expired: refresh the session"},
)

// Secured — операция под cookie-сессией: Security схемы Cookie и коды 401 входа в OpenAPI.
func Secured(op huma.Operation) huma.Operation {
	op.Security = []map[string][]string{{SecuritySchemeName: {}}}
	return SessionErrors.Declare(op)
}
