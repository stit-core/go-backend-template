package user

import (
	"time"

	"git.stit.tech/stit-core/go-backend-template/internal/platform/identity"
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
	if !validEmail(email) {
		return nil, ErrInvalidEmail
	}
	if !validName(name) {
		return nil, ErrInvalidName
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

func (u *User) Rename(name string) error {
	if !validName(name) {
		return ErrInvalidName
	}
	u.Name = name
	return nil
}

// ChangeStatus — переход active ⇄ blocked; тот же статус — без перехода. В deleted переводит только
// удаление (usecase Delete): у него своё событие и аудит.
func (u *User) ChangeStatus(to Status) error {
	switch {
	case to == u.Status && to != StatusDeleted:
		return nil
	case u.Status == StatusActive && to == StatusBlocked, u.Status == StatusBlocked && to == StatusActive:
		u.Status = to
		return nil
	}
	return ErrInvalidStatusTransition
}
