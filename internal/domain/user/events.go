package user

import "time"

type UserRegistered struct {
	ID    string
	Email string
	At    time.Time
}

type UserUpdated struct {
	ID string
	At time.Time
}

type UserDeleted struct {
	ID string
	At time.Time
}
