package authgate

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
	"go.uber.org/fx"

	"git.stit.tech/stit-core/go-backend-template/internal/platform/identity"
	"git.stit.tech/stit-core/golibinfra/httpkit"
	"git.stit.tech/stit-core/golibinfra/logkit"
)

const SecuritySchemeName = "Cookie"

// userIDAttr — экстрактор поля user_id для logkit.RegisterAttr.
func userIDAttr(ctx context.Context) (any, bool) {
	id, ok := identity.From(ctx)
	return id.UserID, ok
}

func registerIdentityLogAttr() {
	logkit.RegisterAttr("user_id", userIDAttr)
}

// CookieScheme — схема Cookie в OpenAPI с extractor'ом её credentials; nil — только описание схемы.
func CookieScheme(extract httpkit.AuthExtractor) httpkit.SecuritySchemeEntry {
	return httpkit.SecuritySchemeEntry{
		Name: SecuritySchemeName,
		Scheme: &huma.SecurityScheme{
			Type:        "apiKey",
			In:          "cookie",
			Name:        CookieAccess,
			Description: "httpOnly cookie выставляется через POST /api/auth/login и автоматически прикладывается браузером.",
		},
		Extract: extract,
	}
}

// SecurityScheme — схема Cookie без extractor'а: собирает API без JWT и БД (спецификация, проверка
// маршрутов); сервис с ней не стартует — extractor схеме даёт Module.
var SecurityScheme = httpkit.ProvideSecurityScheme(func() httpkit.SecuritySchemeEntry { return CookieScheme(nil) })

var Module = fx.Module(
	"internal/platform/authgate",
	authnProviders,
	fx.Provide(NewExtractor),
	httpkit.ProvideSecurityScheme(func(e *Extractor) httpkit.SecuritySchemeEntry { return CookieScheme(e.Extract) }),
	fx.Invoke(registerIdentityLogAttr),
)
