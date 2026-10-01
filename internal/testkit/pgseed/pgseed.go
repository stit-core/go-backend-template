package pgseed

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/user"
	"git.stit.tech/stit-core/go-backend-template/internal/platform/identity"
)

type UserOption func(*user.User)

func WithEmail(email string) UserOption      { return func(u *user.User) { u.Email = email } }
func WithName(name string) UserOption        { return func(u *user.User) { u.Name = name } }
func WithRole(role identity.Role) UserOption { return func(u *user.User) { u.Role = role } }
func WithStatus(s user.Status) UserOption    { return func(u *user.User) { u.Status = s } }

var emailSeq atomic.Int64

func NewUser(opts ...UserOption) *user.User {
	n := emailSeq.Add(1)
	u := &user.User{
		Email:        fmt.Sprintf("seed%d@b.com", n),
		PasswordHash: "hash:seed",
		Name:         fmt.Sprintf("Seed%d", n),
		Status:       user.StatusActive,
		Role:         identity.RoleClient,
	}
	for _, fn := range opts {
		fn(u)
	}
	return u
}

type UserRepo interface {
	Insert(ctx context.Context, u *user.User) error
}

func InsertUser(t *testing.T, repo UserRepo, opts ...UserOption) *user.User {
	t.Helper()
	u := NewUser(opts...)
	require.NoError(t, repo.Insert(context.Background(), u), "pgseed: insert user")
	return u
}
