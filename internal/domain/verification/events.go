package verification

import "time"

// Subjects stay unexported: consumers declare their own literal (rule events-and-messaging).
const (
	subjectEmailVerificationRequested = "verification.events.email_verification_requested"
	subjectPasswordResetRequested     = "verification.events.password_reset_requested"
)

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
