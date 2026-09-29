package authgate

import (
	"context"
	"log/slog"

	"github.com/danielgtaylor/huma/v2"
	"go.uber.org/fx"

	"git.stit.tech/stit-core/go-backend-template/internal/platform/identity"
	"git.stit.tech/stit-core/golibinfra/httpkit"
	"git.stit.tech/stit-core/golibinfra/logkit"
)

const SecuritySchemeName = "Cookie"

// userIDAttr — экстрактор поля user_id для logkit.RegisterAttr.
//
//nolint:forbidigo // slog.Value — тип контракта logkit.Attr, не прямое логирование
func userIDAttr(ctx context.Context) (slog.Value, bool) {
	id, ok := identity.From(ctx)
	if !ok {
		return slog.Value{}, false
	}
	return slog.StringValue(id.UserID), true
}

func registerIdentityLogAttr() {
	logkit.RegisterAttr("user_id", userIDAttr)
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

// SecurityScheme — схема Cookie в OpenAPI; входит в Module, а без него собирает API без JWT и БД.
var SecurityScheme = fx.Provide(
	fx.Annotate(newCookieSecurityScheme, fx.ResultTags(`group:"`+httpkit.SecuritySchemeGroup+`"`)),
)

var Module = fx.Module(
	"authgate",
	authnProviders,
	SecurityScheme,
	fx.Provide(
		NewExtractor,

		func(e *Extractor) httpkit.AuthExtractor { return e.Extract },
		fx.Annotate(newClientIPMiddleware, fx.ResultTags(`group:"`+httpkit.MiddlewareGroup+`"`)),
	),
	fx.Invoke(registerIdentityLogAttr),
)
