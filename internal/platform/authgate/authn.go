package authgate

import (
	"go.uber.org/fx"

	"git.stit.tech/stit-core/golibinfra/authnkit"
)

// authnProviders — примитивы authnkit, нужные шаблону: Hasher (Argon2id) и
// HS256-JWT. authnkit.Module не подключается: он собирает ещё и TOTP, а тот
// требует обязательный TOTP_ISSUER — required-env ради неиспользуемой фичи.
var authnProviders = fx.Provide(
	fx.Annotate(authnkit.NewArgon2idHasher, fx.As(new(authnkit.Hasher))),
	fx.Annotate(newJWT, fx.As(new(authnkit.JWTSigner)), fx.As(new(authnkit.JWTParser))),
)

// newJWT — обёртка: fx.Annotate не аннотирует вариадические конструкторы.
func newJWT(cfg authnkit.JWTConfig) (*authnkit.HS256JWT, error) {
	return authnkit.NewHS256JWT(cfg)
}
