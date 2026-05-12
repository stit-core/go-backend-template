package http

import (
	"git.stit.tech/stit/go-backend-template/internal/domain/auth"
)

type AuthUserView struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role"`
}

func (v *AuthUserView) FromEntity(u *auth.User) {
	v.ID = u.ID
	v.Email = u.Email
	v.Name = u.Name
	v.Role = string(u.Role)
}
