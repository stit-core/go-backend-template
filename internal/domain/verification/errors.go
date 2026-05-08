package verification

import "errors"

var (
	ErrInvalidResetToken        = errors.New("verification: invalid or expired reset token")
	ErrInvalidVerificationToken = errors.New("verification: invalid or expired verification token")
	ErrWeakPassword             = errors.New("verification: password too weak")
	ErrUserNotFound             = errors.New("verification: user not found")
)
