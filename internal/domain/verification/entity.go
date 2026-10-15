package verification

import "time"

// PasswordResetToken — ссылка сброса; UserVersion — версия учётной записи на момент письма (0 — ссылка без версии).
type PasswordResetToken struct {
	ID          string
	UserID      string
	UserVersion int
	ExpiresAt   time.Time
	UsedAt      *time.Time
}

// PasswordResetIssue — выпуск ссылки сброса запроса RequestID: хэш токена и версия учётной записи на момент письма.
type PasswordResetIssue struct {
	RequestID   string
	UserID      string
	UserVersion int
	TokenHash   string
	RequestedAt time.Time
	ExpiresAt   time.Time
}

type EmailVerificationToken struct {
	ID        string
	UserID    string
	ExpiresAt time.Time
	UsedAt    *time.Time
}

// CanLogin вычисляет домен user: порт переносит решение, а не сырой статус.
type User struct {
	ID            string
	Email         string
	CanLogin      bool
	EmailVerified bool
	Version       int
}
