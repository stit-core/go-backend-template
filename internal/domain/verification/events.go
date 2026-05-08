package verification

import "time"

type EmailVerificationRequested struct {
	TokenID string
	UserID  string
	Email   string
	At      time.Time
}

type PasswordResetRequested struct {
	TokenID string
	UserID  string
	Email   string
	At      time.Time
}
