package auth

import "git.stit.tech/stit-core/go-backend-template/internal/platform/identity"

// CanLogin вычисляет домен user: порт переносит решение, а не сырой статус.
type User struct {
	ID           string
	Email        string
	PasswordHash string
	Name         string
	CanLogin     bool
	Role         identity.Role
	Version      int
}
