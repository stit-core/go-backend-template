package http

type LoginInput struct {
	Body struct {
		Email    string `json:"email"    required:"true"`
		Password string `json:"password" required:"true"`
	}
}

type LoginOutput struct {
	Body struct {
		AccessToken  string       `json:"access_token"`
		RefreshToken string       `json:"refresh_token"`
		User         AuthUserView `json:"user"`
	}
}

type RefreshInput struct {
	Body struct {
		RefreshToken string `json:"refresh_token" required:"true"`
	}
}

type RefreshOutput struct {
	Body struct {
		AccessToken  string       `json:"access_token"`
		RefreshToken string       `json:"refresh_token"`
		User         AuthUserView `json:"user"`
	}
}

type LogoutInput struct {
	Body struct {
		RefreshToken string `json:"refresh_token" required:"true"`
	}
}

type ChangePasswordInput struct {
	Body struct {
		UserID      string `json:"user_id"      required:"true"`
		OldPassword string `json:"old_password" required:"true"`
		NewPassword string `json:"new_password" minLength:"8"   required:"true"`
	}
}
