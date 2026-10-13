package verification

import "time"

// Subjects stay unexported: consumers declare their own literal (rule events-and-messaging).
const (
	subjectEmailVerificationRequested = "verification.events.email_verification_requested.v2"
	subjectPasswordResetRequested     = "verification.events.password_reset_requested.v3"
)

type EmailVerificationRequested struct {
	TokenID string
	UserID  string
	At      time.Time
}

// PasswordResetRequested — на каждый запрос сброса, и без учётной записи (UserID пуст): время ответа её не
// выдаёт. Письмо шлёт подписчик verification, RequestID — id ссылки сброса.
type PasswordResetRequested struct {
	RequestID string
	UserID    string
	At        time.Time
}
