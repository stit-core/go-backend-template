package verification

import "errors"

var (
	ErrInvalidResetToken        = errors.New("verification: invalid or expired reset token")
	ErrInvalidVerificationToken = errors.New("verification: invalid or expired verification token")
	ErrTokenUsed                = errors.New("verification: token already used")
	ErrWeakPassword             = errors.New("verification: password too weak")
	ErrPasswordTooLong          = errors.New("verification: password too long")
	ErrUserNotFound             = errors.New("verification: user not found")
	ErrConcurrentUpdate         = errors.New("verification: concurrent update")
	ErrBusy                     = errors.New("verification: password hasher busy")
)
