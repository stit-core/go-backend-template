// Package bruteforce — порт LoginGuard домена auth на bruteforcekit.Guard.
package bruteforce

import (
	"context"
	"errors"
	"fmt"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/auth"
	"git.stit.tech/stit-core/golibinfra/bruteforcekit"
)

type loginGuard struct {
	guard bruteforcekit.Guard
}

func NewLoginGuard(g bruteforcekit.Guard) auth.LoginGuard {
	return &loginGuard{guard: g}
}

func (g *loginGuard) Attempt(ctx context.Context, subject, ip string) (*auth.LockState, error) {
	lock, err := g.guard.Attempt(ctx, subject, ip)
	switch {
	case errors.Is(err, bruteforcekit.ErrInvalidKey):
		return nil, fmt.Errorf("%w: %w", auth.ErrInvalidCredentials, err)
	case err != nil || lock == nil:
		return nil, err
	}
	return &auth.LockState{RetryAfter: lock.RetryAfter}, nil
}

func (g *loginGuard) RecordSuccess(ctx context.Context, subject, ip string) error {
	return g.guard.RecordSuccess(ctx, subject, ip)
}
