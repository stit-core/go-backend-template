package user

import "errors"

var (
	ErrNotFound                = errors.New("user: not found")
	ErrAlreadyExists           = errors.New("user: already exists")
	ErrInvalidEmail            = errors.New("user: invalid email")
	ErrInvalidName             = errors.New("user: invalid name")
	ErrInvalidRole             = errors.New("user: invalid role")
	ErrWeakPassword            = errors.New("user: password too weak")
	ErrPasswordTooLong         = errors.New("user: password too long")
	ErrConcurrentUpdate        = errors.New("user: concurrent update")
	ErrNoUpdateFields          = errors.New("user: no update fields")
	ErrInvalidStatusTransition = errors.New("user: invalid status transition")
	ErrBusy                    = errors.New("user: password hasher busy")
)
