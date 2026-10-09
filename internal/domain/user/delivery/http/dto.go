package http

import (
	"git.stit.tech/stit-core/go-backend-template/internal/domain/user"
	"git.stit.tech/stit-core/go-backend-template/pkg/paging"
)

type UserRegisterInput struct {
	Body struct {
		Email    string `json:"email"    format:"email"   required:"true"`
		Password string `json:"password" maxLength:"1024" minLength:"8"   required:"true"`
		Name     string `json:"name"     minLength:"1"    required:"true"`
	}
}

type UserRegisterOutput struct {
	Body struct {
		Data UserView `json:"data"`
	}
}

type UserGetInput struct {
	ID string `path:"id"`
}

type UserGetOutput struct {
	Body struct {
		Data UserView `json:"data"`
	}
}

type UserMeOutput struct {
	Body struct {
		Data UserView `json:"data"`
	}
}

type UserListInput struct {
	paging.Page
	Status string `enum:"active,blocked" query:"status"`
}

type UserListOutput struct {
	Body struct {
		Data  []UserView `json:"data"`
		Total int        `json:"total"`
		Page  int        `json:"page"`
		Size  int        `json:"size"`
	}
}

type UserUpdateInput struct {
	ID   string `path:"id"`
	Body struct {
		Name    *string      `json:"name,omitempty"   minLength:"1"`
		Status  *user.Status `json:"status,omitempty" enum:"active,blocked"`
		Version int          `json:"version"          required:"true"`
	}
}

type UserUpdateOutput struct {
	Body struct {
		Data UserView `json:"data"`
	}
}

type UserDeleteInput struct {
	ID string `path:"id"`
}
