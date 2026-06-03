package http

import "git.stit.tech/stit-template/go-backend-template/internal/domain/user"

type RegisterInput struct {
	Body struct {
		Email    string `json:"email"    format:"email" required:"true"`
		Password string `json:"password" minLength:"8"  required:"true"`
		Name     string `json:"name"     minLength:"1"  required:"true"`
	}
}

func (i *RegisterInput) ToEntity() *user.Create {
	return &user.Create{
		Email:    i.Body.Email,
		Password: i.Body.Password,
		Name:     i.Body.Name,
	}
}

type RegisterOutput struct {
	Body struct {
		Data UserView `json:"data"`
	}
}

type GetUserInput struct {
	ID string `path:"id"`
}

type GetUserOutput struct {
	Body struct {
		Data UserView `json:"data"`
	}
}

type MeOutput struct {
	Body struct {
		Data UserView `json:"data"`
	}
}

type ListInput struct {
	Page   int    `default:"1"           minimum:"1"    query:"page"`
	Size   int    `default:"20"          maximum:"100"  minimum:"1"  query:"size"`
	Status string `enum:"active,blocked" query:"status"`
}

type ListOutput struct {
	Body struct {
		Data  []UserView `json:"data"`
		Total int        `json:"total"`
		Page  int        `json:"page"`
		Size  int        `json:"size"`
	}
}

type UpdateInput struct {
	ID   string `path:"id"`
	Body struct {
		Name    *string      `json:"name,omitempty"`
		Status  *user.Status `json:"status,omitempty"`
		Version int          `json:"version"          required:"true"`
	}
}

type UpdateOutput struct {
	Body struct {
		Data UserView `json:"data"`
	}
}

type DeleteInput struct {
	ID string `path:"id"`
}
