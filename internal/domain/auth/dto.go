package auth

import "time"

type LoginInput struct {
	Email    string
	Password string
}

type LoginOutput struct {
	AccessToken  string
	RefreshToken string
	User         *User
}

type LockState struct {
	Until time.Time
}
