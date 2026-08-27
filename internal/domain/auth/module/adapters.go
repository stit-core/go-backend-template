package module

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/auth"
	"git.stit.tech/stit-core/go-backend-template/internal/domain/user"
	"git.stit.tech/stit-core/golibinfra/authnkit"
	"git.stit.tech/stit-core/golibinfra/bruteforcekit"
	"git.stit.tech/stit-core/golibinfra/clockkit"
)

type userReaderAdapter struct {
	uc *user.Usecase
}

func newUserReaderAdapter(uc *user.Usecase) auth.UserReader {
	return &userReaderAdapter{uc: uc}
}

func (a *userReaderAdapter) GetByID(ctx context.Context, id string) (*auth.User, error) {
	u, err := a.uc.LookupByID(ctx, id)
	if err != nil {
		return nil, mapUserErr(err)
	}
	return userToAuth(u), nil
}

func (a *userReaderAdapter) GetByEmail(ctx context.Context, email string) (*auth.User, error) {
	u, err := a.uc.LookupByEmail(ctx, email)
	if err != nil {
		return nil, mapUserErr(err)
	}
	return userToAuth(u), nil
}

// Ports carry consumer-domain sentinels: callers can't errors.Is against user's (depguard).
func mapUserErr(err error) error {
	if errors.Is(err, user.ErrNotFound) {
		return auth.ErrUserNotFound
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

type tokensAdapter struct {
	signer     authnkit.JWTSigner
	clock      clockkit.Clock
	accessTTL  time.Duration
	refreshTTL time.Duration
}

func newTokensAdapter(s authnkit.JWTSigner, cfg authnkit.JWTConfig, clock clockkit.Clock) auth.Tokens {
	return &tokensAdapter{signer: s, clock: clock, accessTTL: cfg.AccessTTL, refreshTTL: cfg.RefreshTTL}
}

func (a *tokensAdapter) IssueAccess(u *auth.User) (string, error) {
	claims := map[string]any{
		"sub":  u.ID,
		"role": string(u.Role),
		"typ":  "access",
	}
	return a.signer.Sign(claims, a.accessTTL)
}

func (a *tokensAdapter) IssueRefresh(userID string) (string, string, time.Time, error) {
	jti := uuid.NewString()
	expiresAt := a.clock.Now().Add(a.refreshTTL)
	token, err := a.signer.Sign(map[string]any{
		"sub": userID,
		"typ": "refresh",
		"jti": jti,
	}, a.refreshTTL)
	if err != nil {
		return "", "", time.Time{}, err
	}
	return token, jti, expiresAt, nil
}

type loginGuardAdapter struct {
	guard bruteforcekit.Guard
}

func newLoginGuardAdapter(g bruteforcekit.Guard) auth.LoginGuard {
	return &loginGuardAdapter{guard: g}
}

func (a *loginGuardAdapter) Check(ctx context.Context, subject, ip string) (*auth.LockState, error) {
	lock, err := a.guard.Check(ctx, subject, ip)
	if err != nil || lock == nil {
		return nil, err
	}
	return &auth.LockState{Until: lock.Until}, nil
}

func (a *loginGuardAdapter) RecordFailure(ctx context.Context, subject, ip string) error {
	return a.guard.RecordFailure(ctx, subject, ip)
}

func (a *loginGuardAdapter) RecordSuccess(ctx context.Context, subject, ip string) error {
	return a.guard.RecordSuccess(ctx, subject, ip)
}

type tokenParserAdapter struct {
	parser authnkit.JWTParser
}

func newTokenParserAdapter(p authnkit.JWTParser) auth.TokenParser {
	return &tokenParserAdapter{parser: p}
}

func (a *tokenParserAdapter) ParseRefresh(token string) (string, string, error) {
	claims, err := a.parser.Parse(token)
	if err != nil {
		if errors.Is(err, authnkit.ErrTokenExpired) {
			return "", "", auth.ErrTokenExpired
		}
		return "", "", fmt.Errorf("parse refresh token: %w", err)
	}
	typ, _ := claims["typ"].(string)
	if typ != "refresh" {
		return "", "", fmt.Errorf("token type is not refresh")
	}
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return "", "", fmt.Errorf("missing sub in refresh token")
	}
	jti, _ := claims["jti"].(string)
	if jti == "" {
		return "", "", fmt.Errorf("missing jti in refresh token")
	}
	return sub, jti, nil
}
