package http

import (
	"time"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/user"
)

type UserView struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (v *UserView) FromEntity(u *user.User) {
	v.ID = u.ID
	v.Email = u.Email
	v.Name = u.Name
	v.Status = string(u.Status)
	v.Version = u.Version
	v.CreatedAt = u.CreatedAt
	v.UpdatedAt = u.UpdatedAt
}
