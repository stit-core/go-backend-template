package auth

import "git.stit.tech/stit-core/go-backend-template/internal/platform/identity"

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
