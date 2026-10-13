package nats

import "time"

// UserRegisteredEvent — локальная схема payload'а user.events.registered.v2: archtest не пускает
// verification к домену user, контракт события verification держит сама.
type UserRegisteredEvent struct {
	ID string    `json:"ID"`
	At time.Time `json:"At"`
}

const SubjectUserRegistered = "user.events.registered.v2"
