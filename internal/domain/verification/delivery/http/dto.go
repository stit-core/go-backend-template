package http

type VerificationPasswordResetRequestInput struct {
	Body struct {
		Email string `json:"email" required:"true"`
	}
}

type VerificationPasswordResetConfirmInput struct {
	Body struct {
		Token    string `json:"token"    required:"true"`
		Password string `json:"password" maxLength:"1024" minLength:"8" required:"true"`
	}
}

type VerificationEmailVerifyInput struct {
	Body struct {
		Token string `json:"token" required:"true"`
	}
}
