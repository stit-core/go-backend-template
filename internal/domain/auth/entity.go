package auth

import (
	"time"

	"git.stit.tech/stit-core/go-backend-template/internal/platform/identity"
)

// CanLogin is computed by the user domain (port carries decisions, not raw status enums).
type User struct {
	ID           string
	Email        string
	PasswordHash string
	Name         string
	CanLogin     bool
	Role         identity.Role
	Version      int
}

type RefreshSession struct {
	JTI           string
	UserID        string
	IssuedAt      time.Time
	ExpiresAt     time.Time
	UsedAt        *time.Time
	ReplacedByJTI string
}
