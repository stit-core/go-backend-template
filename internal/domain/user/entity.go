package user

import (
	"net/mail"
	"time"

	"git.stit.tech/stit-template/go-backend-template/internal/platform/identity"
)

type Status string

const (
	StatusUnknown Status = ""
	StatusActive  Status = "active"
	StatusBlocked Status = "blocked"
	StatusDeleted Status = "deleted"
)

type User struct {
	ID            string
	Email         string
	PasswordHash  string
	Name          string
	Status        Status
	Role          identity.Role
	Version       int
	EmailVerified bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func NewUser(email, passwordHash, name string, role identity.Role) (*User, error) {
	if _, err := mail.ParseAddress(email); err != nil {
		return nil, ErrInvalidEmail
	}
	if role == identity.RoleUnknown {
		role = identity.RoleClient
	}
	if role != identity.RoleAdmin && role != identity.RoleClient {
		return nil, ErrInvalidRole
	}
	return &User{
		Email:        email,
		PasswordHash: passwordHash,
		Name:         name,
		Status:       StatusActive,
		Role:         role,
		Version:      1,
	}, nil
}

func (u *User) CanLogin() bool { return u.Status == StatusActive }
