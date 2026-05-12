package nats

import "time"

// UserRegisteredEvent — локальная схема user.events.registered payload'а.
// Anti-corruption layer: depguard запрещает verification импортить user, и
// верификация владеет своей версией интеграционного контракта. При изменении
// схемы у user — оба домена должны мигрировать (новый subject + co-existence).
type UserRegisteredEvent struct {
	ID    string    `json:"ID"`
	Email string    `json:"Email"`
	At    time.Time `json:"At"`
}

const SubjectUserRegistered = "user.events.registered"
