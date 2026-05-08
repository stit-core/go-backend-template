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

type User struct {
	ID      string
	Email   string
	Active  bool
	Version int
}
