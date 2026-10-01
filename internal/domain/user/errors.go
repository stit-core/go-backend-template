package user

import "errors"

var (
	ErrNotFound                = errors.New("user: not found")
	ErrAlreadyExists           = errors.New("user: already exists")
	ErrInvalidEmail            = errors.New("user: invalid email")
	ErrInvalidRole             = errors.New("user: invalid role")
	ErrWeakPassword            = errors.New("user: password too weak")
	ErrConcurrentUpdate        = errors.New("user: concurrent update")
	ErrNoUpdateFields          = errors.New("user: no update fields")
	ErrInvalidStatusTransition = errors.New("user: invalid status transition")
)
