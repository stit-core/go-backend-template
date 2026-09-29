package http

import "net/http"

type LoginInput struct {
	Body struct {
		Email    string `json:"email"    required:"true"`
		Password string `json:"password" required:"true"`
	}
}

type LoginOutput struct {
	SetCookie []http.Cookie `header:"Set-Cookie"`
	Body      struct {
		User AuthUserView `json:"user"`
	}
}

type RefreshInput struct {
	RefreshCookie http.Cookie `cookie:"refresh_token"`
}

type RefreshOutput struct {
	SetCookie []http.Cookie `header:"Set-Cookie"`
}

type LogoutInput struct {
	RefreshCookie http.Cookie `cookie:"refresh_token"`
}

type LogoutOutput struct {
	SetCookie []http.Cookie `header:"Set-Cookie"`
}

type ChangePasswordInput struct {
	Body struct {
		UserID      string `json:"user_id"      required:"true"`
		OldPassword string `json:"old_password" required:"true"`
		NewPassword string `json:"new_password" required:"true"`
	}
}
