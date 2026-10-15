package http

import (
	"time"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/user"
)

type UserView struct {
	ID            string    `json:"id"             format:"uuid"`
	Email         string    `json:"email"`
	EmailVerified bool      `json:"email_verified"`
	Name          string    `json:"name"`
	Role          string    `json:"role"           enum:"admin,client"`
	Status        string    `json:"status"         enum:"active,blocked"`
	Version       int       `json:"version"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (v *UserView) FromEntity(u *user.User) {
	v.ID = u.ID
	v.Email = u.Email
	v.EmailVerified = u.EmailVerified
	v.Name = u.Name
	v.Role = string(u.Role)
	v.Status = string(u.Status)
	v.Version = u.Version
	v.CreatedAt = u.CreatedAt
	v.UpdatedAt = u.UpdatedAt
}
