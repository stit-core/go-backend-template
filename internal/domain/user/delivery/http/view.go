package http

import (
	"time"

	"git.stit.tech/akadmin/go-backend-template/internal/domain/user"
)

type View struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (v *View) FromEntity(u *user.User) {
	v.ID = u.ID
	v.Email = u.Email
	v.Name = u.Name
	v.Status = string(u.Status)
	v.CreatedAt = u.CreatedAt
	v.UpdatedAt = u.UpdatedAt
}
