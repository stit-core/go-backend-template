package auth

import "time"

type LoggedIn struct {
	UserID string
	At     time.Time
}
