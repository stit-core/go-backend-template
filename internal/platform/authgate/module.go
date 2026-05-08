package authgate

import (
	"context"

	"go.uber.org/fx"

	"git.stit.tech/akadmin/go-backend-template/internal/platform/identity"
	"git.stit.tech/akadmin/golibinfra/httpkit"
	"git.stit.tech/akadmin/golibinfra/logkit"
)

func registerIdentityLogAttr() {
	logkit.RegisterAttr(func(ctx context.Context) (string, any, bool) {
		id, ok := identity.From(ctx)
		if !ok {
			return "", nil, false
		}
		return "user_id", id.UserID, true
	})
}

var Module = fx.Module(
	"authgate",
	fx.Provide(
		NewExtractor,

		func(e *Extractor) httpkit.AuthExtractor { return e.Extract },
	),
	fx.Invoke(registerIdentityLogAttr),
)
