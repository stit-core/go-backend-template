package authgate

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
	"go.uber.org/fx"

	"git.sftbmb.ru/sftbmb-core/go-backend-template/internal/platform/identity"
	"git.sftbmb.ru/sftbmb-core/golibinfra/httpkit"
	"git.sftbmb.ru/sftbmb-core/golibinfra/logkit"
)

const SecuritySchemeName = "Cookie"

func registerIdentityLogAttr() {
	logkit.RegisterAttr(func(ctx context.Context) (string, any, bool) {
		id, ok := identity.From(ctx)
		if !ok {
			return "", nil, false
		}
		return "user_id", id.UserID, true
	})
}

func newCookieSecurityScheme() httpkit.SecuritySchemeEntry {
	return httpkit.SecuritySchemeEntry{
		Name: SecuritySchemeName,
		Scheme: &huma.SecurityScheme{
			Type:        "apiKey",
			In:          "cookie",
			Name:        CookieAccess,
			Description: "httpOnly cookie выставляется через POST /api/auth/login и автоматически прикладывается браузером.",
		},
	}
}

var Module = fx.Module(
	"authgate",
	fx.Provide(
		NewExtractor,

		func(e *Extractor) httpkit.AuthExtractor { return e.Extract },
		fx.Annotate(newCookieSecurityScheme, fx.ResultTags(`group:"httpkit.security_scheme"`)),
	),
	fx.Invoke(registerIdentityLogAttr),
)
