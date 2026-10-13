package nats

import "time"

// UserRegisteredEvent — локальная схема payload'а user.events.registered.v2: archtest не пускает
// verification к домену user, контракт события verification держит сама.
type UserRegisteredEvent struct {
	ID string    `json:"ID"`
	At time.Time `json:"At"`
}

// PasswordResetRequestedEvent — payload своего события домена verification.events.password_reset_requested.v3:
// UserID пуст у адреса без учётной записи.
type PasswordResetRequestedEvent struct {
	RequestID string    `json:"RequestID"`
	UserID    string    `json:"UserID"`
	At        time.Time `json:"At"`
}

const (
	SubjectUserRegistered         = "user.events.registered.v2"
	SubjectPasswordResetRequested = "verification.events.password_reset_requested.v3"
)
