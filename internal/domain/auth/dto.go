package auth

import "git.stit.tech/stit-infra/golibinfra/bruteforcekit"

type LoginInput struct {
	Email    string
	Password string
}

type LoginOutput struct {
	AccessToken  string
	RefreshToken string
	User         *User
}

// Alias (not new type) so bruteforcekit.Guard satisfies auth.LoginGuard structurally.
type LockState = bruteforcekit.LockState
