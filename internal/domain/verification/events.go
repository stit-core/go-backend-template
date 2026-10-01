package verification

import "time"

// Subjects stay unexported: consumers declare their own literal (rule events-and-messaging).
const (
	subjectEmailVerificationRequested = "verification.events.email_verification_requested.v2"
	subjectPasswordResetRequested     = "verification.events.password_reset_requested.v2"
)

type EmailVerificationRequested struct {
	TokenID string
	UserID  string
	At      time.Time
}

type PasswordResetRequested struct {
	TokenID string
	UserID  string
	At      time.Time
}
