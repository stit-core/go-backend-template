package http

import "net/http"

type AuthLoginInput struct {
	Body struct {
		Email    string `json:"email"    required:"true"`
		Password string `json:"password" maxLength:"1024" required:"true"`
	}
}

type AuthLoginOutput struct {
	SetCookie []http.Cookie `header:"Set-Cookie"`
	Body      struct {
		Data AuthUserView `json:"data"`
	}
}

type AuthRefreshInput struct {
	RefreshToken string `cookie:"refresh_token"`
}

type AuthRefreshOutput struct {
	SetCookie []http.Cookie `header:"Set-Cookie"`
}

type AuthLogoutInput struct {
	RefreshToken string `cookie:"refresh_token"`
}

type AuthLogoutOutput struct {
	SetCookie []http.Cookie `header:"Set-Cookie"`
}

type AuthChangePasswordInput struct {
	Body struct {
		UserID      string `json:"user_id"      required:"true"`
		OldPassword string `json:"old_password" maxLength:"1024" required:"true"`
		NewPassword string `json:"new_password" maxLength:"1024" minLength:"8"   required:"true"`
	}
}

type AuthChangePasswordOutput struct {
	SetCookie []http.Cookie `header:"Set-Cookie"`
}
