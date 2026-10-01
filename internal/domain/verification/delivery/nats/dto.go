package nats

import "time"

// UserRegisteredEvent — локальная схема payload'а user.events.registered.v2: archtest не пускает
// verification к домену user, контракт события verification держит сама. Payload старого subject —
// те же поля и Email, которого схема не читает.
type UserRegisteredEvent struct {
	ID string    `json:"ID"`
	At time.Time `json:"At"`
}

const (
	SubjectUserRegistered   = "user.events.registered.v2"
	subjectUserRegisteredV1 = "user.events.registered"
)
