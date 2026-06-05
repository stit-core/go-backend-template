package auth

import (
	"time"

	"git.sftbmb.ru/sftbmb-core/go-backend-template/internal/platform/identity"
)

type User struct {
	ID           string
	Email        string
	PasswordHash string
	Name         string
	Status       string
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
