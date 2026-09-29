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

// Ports carry consumer-domain sentinels: callers can't errors.Is against user's (archtest).
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
	refreshTTL time.Duration
}

// newTokensAdapter: TTL и claim typ проставляет кит (SignAccess/SignRefresh);
// refreshTTL нужен здесь только для expires_at строки refresh_sessions.
func newTokensAdapter(s authnkit.JWTSigner, cfg authnkit.JWTConfig, clock clockkit.Clock) auth.Tokens {
	ttl := cfg.RefreshTTL
	if ttl <= 0 {
		ttl = authnkit.DefaultRefreshTTL // нулевой JWT_REFRESH_TTL = дефолт кита
	}
	return &tokensAdapter{signer: s, clock: clock, refreshTTL: ttl}
}

func (a *tokensAdapter) IssueAccess(ctx context.Context, u *auth.User) (string, error) {
	return a.signer.SignAccess(ctx, map[string]any{
		"sub":  u.ID,
		"role": string(u.Role),
	})
}

func (a *tokensAdapter) IssueRefresh(ctx context.Context, userID string) (string, string, time.Time, error) {
	jti := uuid.NewString()
	expiresAt := a.clock.Now().Add(a.refreshTTL)
	token, err := a.signer.SignRefresh(ctx, map[string]any{
		"sub": userID,
		"jti": jti,
	})
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
	// Состояние блокировки после неудачи домену не нужно: следующий Login
	// увидит её через Check.
	_, err := a.guard.RecordFailure(ctx, subject, ip)
	return err
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

// ParseRefresh: тип токена (typ=refresh) проверяет кит — access-токен в
// refresh-контуре даёт authnkit.ErrWrongTokenType.
func (a *tokenParserAdapter) ParseRefresh(ctx context.Context, token string) (string, string, error) {
	claims, err := a.parser.ParseRefresh(ctx, token)
	if err != nil {
		if errors.Is(err, authnkit.ErrTokenExpired) {
			return "", "", auth.ErrTokenExpired
		}
		return "", "", fmt.Errorf("parse refresh token: %w", err)
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
