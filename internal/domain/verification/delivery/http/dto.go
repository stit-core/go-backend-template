package http

type PasswordResetRequestInput struct {
	Body struct {
		Email string `json:"email" required:"true"`
	}
}

type PasswordResetConfirmInput struct {
	Body struct {
		Token    string `json:"token"    required:"true"`
		Password string `json:"password" required:"true"`
	}
}

type EmailVerifyInput struct {
	Body struct {
		Token string `json:"token" required:"true"`
	}
}
