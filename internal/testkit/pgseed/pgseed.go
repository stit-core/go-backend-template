package pgseed

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"git.sftbmb.ru/sftbmb-core/go-backend-template/internal/domain/user"
	"git.sftbmb.ru/sftbmb-core/go-backend-template/internal/platform/identity"
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
	Create(ctx context.Context, u *user.User) error
}

func InsertUser(t *testing.T, repo UserRepo, opts ...UserOption) *user.User {
	t.Helper()
	u := NewUser(opts...)
	if err := repo.Create(context.Background(), u); err != nil {
		t.Fatalf("pgseed: create user: %v", err)
	}
	return u
}
