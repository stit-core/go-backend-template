package http

import (
	"git.stit.tech/akadmin/go-backend-template/internal/domain/auth"
)

type View struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role"`
}

func (v *View) FromEntity(u *auth.User) {
	v.ID = u.ID
	v.Email = u.Email
	v.Name = u.Name
	v.Role = string(u.Role)
}
