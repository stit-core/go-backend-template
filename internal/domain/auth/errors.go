package auth

import "errors"

var (
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
	ErrRefreshExpired     = errors.New("auth: refresh token expired")
	ErrRefreshReused      = errors.New("auth: refresh token reused")
	ErrAccountLocked      = errors.New("auth: account temporarily locked")
	ErrTokenExpired       = errors.New("auth: token expired")
	ErrWeakPassword       = errors.New("auth: password too weak")
	ErrSessionNotFound    = errors.New("auth: refresh session not found")
	ErrUserNotFound       = errors.New("auth: user not found")
	ErrConcurrentUpdate   = errors.New("auth: concurrent update")
)
