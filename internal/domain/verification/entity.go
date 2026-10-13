package verification

import "time"

type PasswordResetToken struct {
	ID        string
	UserID    string
	ExpiresAt time.Time
	UsedAt    *time.Time
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
