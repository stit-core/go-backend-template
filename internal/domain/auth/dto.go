package auth

import "time"

type LoginInput struct {
	Email    string
	Password string
}

type LoginOutput struct {
	AccessToken  string
	RefreshToken string
	User         *User
}

// LockState — блокировка входа guard'ом; RetryAfter — её остаток по часам хранилища guard'а.
type LockState struct {
	RetryAfter time.Duration
}
