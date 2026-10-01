package module

import (
	"context"
	"errors"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/auth"
	"git.stit.tech/stit-core/go-backend-template/internal/domain/user"
)

// userAdapter — порты auth к домену user (UserReader, PasswordVerifier, PasswordSetter).
type userAdapter struct {
	users *user.Trusted
}

func newUserAdapter(u *user.Trusted) *userAdapter { return &userAdapter{users: u} }

func (a *userAdapter) GetByID(ctx context.Context, id string) (*auth.User, error) {
	u, err := a.users.LookupByID(ctx, id)
	if err != nil {
		return nil, mapUserErr(err)
	}
	return userToAuth(u), nil
}

func (a *userAdapter) GetByEmail(ctx context.Context, email string) (*auth.User, error) {
	u, err := a.users.LookupByEmail(ctx, email)
	if err != nil {
		return nil, mapUserErr(err)
	}
	return userToAuth(u), nil
}

func (a *userAdapter) VerifyPassword(ctx context.Context, userID, plain string) (bool, error) {
	ok, err := a.users.VerifyPassword(ctx, userID, plain)
	return ok, mapUserErr(err)
}

func (a *userAdapter) UpdatePassword(ctx context.Context, userID, hash string, version int) error {
	return mapUserErr(a.users.UpdatePassword(ctx, userID, hash, version))
}

// mapUserErr переводит каждую ошибку user.Trusted в ошибку auth: sentinel'ов user потребитель не видит.
func mapUserErr(err error) error {
	switch {
	case errors.Is(err, user.ErrNotFound):
		return auth.ErrUserNotFound
	case errors.Is(err, user.ErrConcurrentUpdate):
		return auth.ErrConcurrentUpdate
	}
	return err
}

func userToAuth(u *user.User) *auth.User {
	return &auth.User{
		ID:           u.ID,
		Email:        u.Email,
		PasswordHash: u.PasswordHash,
		Name:         u.Name,
		CanLogin:     u.CanLogin(),
		Role:         u.Role,
		Version:      u.Version,
	}
}
