package user

import "time"

// Subjects stay unexported: consumers declare their own literal (rule events-and-messaging).
const (
	subjectUserRegistered = "user.events.registered"
	subjectUserUpdated    = "user.events.updated"
	subjectUserDeleted    = "user.events.deleted"
)

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
