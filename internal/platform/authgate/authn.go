package authgate

import (
	"go.uber.org/fx"

	"git.stit.tech/stit-core/golibinfra/authnkit"
)

// authnProviders — примитивы authnkit платформы: Hasher (Argon2id) и HS256-JWT с проверкой токенов;
// подпись токенов привязывает домен, который их выдаёт. authnkit.Module не подключается: он собирает
// ещё и TOTP, а тот требует обязательный TOTP_ISSUER — required-env ради неиспользуемой фичи.
var authnProviders = fx.Provide(
	authnkit.NewArgon2idHasher,
	func(h *authnkit.Argon2idHasher) authnkit.Hasher { return h },
	authnkit.NewHS256JWT,
	func(j *authnkit.HS256JWT) authnkit.JWTParser { return j },
)
