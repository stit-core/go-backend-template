package user

import "time"

// Subject не экспортируется: потребитель держит свою константу (rule events-and-messaging).
const (
	subjectUserRegistered = "user.events.registered.v2"
	subjectUserUpdated    = "user.events.updated"
	subjectUserDeleted    = "user.events.deleted"
)

type UserRegistered struct {
	ID string
	At time.Time
}

type UserUpdated struct {
	ID string
	At time.Time
}

type UserDeleted struct {
	ID string
	At time.Time
}
