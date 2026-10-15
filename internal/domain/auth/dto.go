package auth

import "time"

type LoginInput struct {
	Email    string
	Password string
}

// Session — итог входа и обновления сессии: токены cookie и пользователь.
type Session struct {
	AccessToken  string
	RefreshToken string
	User         *User
}

// LockState — блокировка входа guard'ом; RetryAfter — её остаток по часам хранилища guard'а.
type LockState struct {
	RetryAfter time.Duration
}
