package module

import (
	"context"
	"errors"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/user"
	"git.stit.tech/stit-core/go-backend-template/internal/domain/verification"
)

// userAdapter — порты verification к домену user (UserReader, PasswordSetter, EmailVerificationSetter).
type userAdapter struct {
	users *user.Trusted
}

func newUserAdapter(u *user.Trusted) *userAdapter { return &userAdapter{users: u} }

func (a *userAdapter) GetByID(ctx context.Context, id string) (*verification.User, error) {
	u, err := a.users.LookupByID(ctx, id)
	if err != nil {
		return nil, mapUserErr(err)
	}
	return userToVerification(u), nil
}

func (a *userAdapter) GetByEmail(ctx context.Context, email string) (*verification.User, error) {
	u, err := a.users.LookupByEmail(ctx, email)
	if err != nil {
		return nil, mapUserErr(err)
	}
	return userToVerification(u), nil
}

func (a *userAdapter) HashPassword(ctx context.Context, plain string) (string, error) {
	hash, err := a.users.HashPassword(ctx, plain)
	return hash, mapUserErr(err)
}

func (a *userAdapter) UpdatePassword(ctx context.Context, userID, hash string, version int) error {
	return mapUserErr(a.users.UpdatePassword(ctx, userID, hash, version))
}

func (a *userAdapter) MarkEmailVerified(ctx context.Context, userID string) error {
	return mapUserErr(a.users.MarkEmailVerified(ctx, userID))
}

// mapUserErr переводит каждую ошибку user.Trusted в ошибку verification: sentinel'ов user потребитель
// не видит.
func mapUserErr(err error) error {
	switch {
	case errors.Is(err, user.ErrNotFound):
		return verification.ErrUserNotFound
	case errors.Is(err, user.ErrConcurrentUpdate):
		return verification.ErrConcurrentUpdate
	case errors.Is(err, user.ErrWeakPassword):
		return verification.ErrWeakPassword
	case errors.Is(err, user.ErrPasswordTooLong):
		return verification.ErrPasswordTooLong
	}
	return err
}

func userToVerification(u *user.User) *verification.User {
	return &verification.User{
		ID:            u.ID,
		Email:         u.Email,
		CanLogin:      u.CanLogin(),
		EmailVerified: u.EmailVerified,
		Version:       u.Version,
	}
}
