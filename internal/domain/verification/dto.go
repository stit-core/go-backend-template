package verification

type ConfirmPasswordResetInput struct {
	Token       string
	NewPassword string
}

type ConfirmEmailInput struct {
	Token string
}
