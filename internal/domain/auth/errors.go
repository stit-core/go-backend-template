package auth

import (
	"errors"
	"time"
)

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

// LockedError — вход заблокирован guard'ом (errors.Is — ErrAccountLocked); Wait — до снятия
// блокировки, его отдаёт заголовок Retry-After.
type LockedError struct{ Wait time.Duration }

func (e *LockedError) Error() string             { return ErrAccountLocked.Error() }
func (e *LockedError) Unwrap() error             { return ErrAccountLocked }
func (e *LockedError) RetryAfter() time.Duration { return e.Wait }
