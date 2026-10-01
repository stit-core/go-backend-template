package user

import (
	"context"

	"go.uber.org/fx"

	"git.stit.tech/stit-core/golibinfra/authnkit"
)

type TrustedDeps struct {
	fx.In

	Repo   Repository
	Hasher authnkit.Hasher
}

// Trusted — операции user для других доменов, без policy: решение принял вызывающий, и берут их
// только адаптеры module/ потребителей (archtest).
type Trusted struct {
	d TrustedDeps
}

// TrustedErrors — ошибки, которые отдают методы Trusted (кроме сбоев инфраструктуры): Lookup*,
// VerifyPassword, MarkEmailVerified — ErrNotFound (нет или удалён); UpdatePassword — ErrConcurrentUpdate
// (версия устарела или удалён). Адаптер потребителя переводит каждую, тест и archtest держат список.
var TrustedErrors = []error{ErrNotFound, ErrConcurrentUpdate}

func NewTrusted(d TrustedDeps) *Trusted { return &Trusted{d: d} }

func (t *Trusted) LookupByID(ctx context.Context, id string) (*User, error) {
	return t.d.Repo.GetByID(ctx, id)
}

func (t *Trusted) LookupByEmail(ctx context.Context, email string) (*User, error) {
	return t.d.Repo.GetByEmail(ctx, email)
}

func (t *Trusted) VerifyPassword(ctx context.Context, userID, plain string) (bool, error) {
	u, err := t.d.Repo.GetByID(ctx, userID)
	if err != nil {
		return false, err
	}
	return t.d.Hasher.Verify(ctx, plain, u.PasswordHash)
}

func (t *Trusted) UpdatePassword(ctx context.Context, userID, hash string, version int) error {
	_, err := t.d.Repo.UpdatePassword(ctx, userID, hash, version)
	return err
}

func (t *Trusted) MarkEmailVerified(ctx context.Context, userID string) error {
	return t.d.Repo.SetEmailVerified(ctx, userID)
}
