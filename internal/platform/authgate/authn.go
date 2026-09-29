package authgate

import (
	"go.uber.org/fx"

	"git.stit.tech/stit-core/golibinfra/authnkit"
)

// authnProviders — примитивы authnkit, нужные шаблону: Hasher (Argon2id) и
// HS256-JWT. authnkit.Module не подключается: он собирает ещё и TOTP, а тот
// требует обязательный TOTP_ISSUER — required-env ради неиспользуемой фичи.
var authnProviders = fx.Provide(
	authnkit.NewArgon2idHasher,
	func(h *authnkit.Argon2idHasher) authnkit.Hasher { return h },
	authnkit.NewHS256JWT,
	func(j *authnkit.HS256JWT) authnkit.JWTSigner { return j },
	func(j *authnkit.HS256JWT) authnkit.JWTParser { return j },
)
