package auth

import "git.stit.tech/stit/golibinfra/bruteforcekit"

type LoginInput struct {
	Email    string
	Password string
}

type LoginOutput struct {
	AccessToken  string
	RefreshToken string
	User         *User
}

// LockState — алиас на bruteforcekit.LockState. Один тип на оба пакета,
// чтобы bruteforcekit.Guard удовлетворял auth.LoginGuard структурно
// без adapter-обёртки.
type LockState = bruteforcekit.LockState
