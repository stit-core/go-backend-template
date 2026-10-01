// Package jwt — порты Tokens и TokenParser домена auth на HS256-JWT authnkit.
package jwt

import (
	"context"
	"errors"
	"fmt"
	"time"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/auth"
	"git.stit.tech/stit-core/golibinfra/authnkit"
	"git.stit.tech/stit-core/golibinfra/clockkit"
	"git.stit.tech/stit-core/golibinfra/idkit"
)

type tokens struct {
	signer     authnkit.JWTSigner
	clock      clockkit.Clock
	refreshTTL time.Duration
}

// NewTokens: TTL и claim typ проставляет кит (SignAccess/SignRefresh); refreshTTL нужен здесь только
// для expires_at строки refresh_sessions.
func NewTokens(s authnkit.JWTSigner, cfg authnkit.JWTConfig, clock clockkit.Clock) auth.Tokens {
	ttl := cfg.RefreshTTL
	if ttl <= 0 {
		ttl = authnkit.DefaultRefreshTTL // нулевой JWT_REFRESH_TTL = дефолт кита
	}
	return &tokens{signer: s, clock: clock, refreshTTL: ttl}
}

func (t *tokens) IssueAccess(ctx context.Context, u *auth.User) (string, error) {
	return t.signer.SignAccess(ctx, map[string]any{
		"sub":  u.ID,
		"role": string(u.Role),
	})
}

func (t *tokens) IssueRefresh(ctx context.Context, userID string) (string, string, time.Time, error) {
	jti := idkit.NewV7()
	expiresAt := t.clock.Now().Add(t.refreshTTL)
	token, err := t.signer.SignRefresh(ctx, map[string]any{
		"sub": userID,
		"jti": jti,
	})
	if err != nil {
		return "", "", time.Time{}, err
	}
	return token, jti, expiresAt, nil
}

type tokenParser struct {
	parser authnkit.JWTParser
}

func NewTokenParser(p authnkit.JWTParser) auth.TokenParser {
	return &tokenParser{parser: p}
}

// ParseRefresh: тип токена (typ=refresh) проверяет кит — access-токен в refresh-контуре даёт
// authnkit.ErrWrongTokenType.
func (p *tokenParser) ParseRefresh(ctx context.Context, token string) (string, string, error) {
	claims, err := p.parser.ParseRefresh(ctx, token)
	if err != nil {
		if errors.Is(err, authnkit.ErrTokenExpired) {
			return "", "", auth.ErrTokenExpired
		}
		return "", "", fmt.Errorf("parse refresh token: %w", err)
	}
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return "", "", errors.New("missing sub in refresh token")
	}
	jti, _ := claims["jti"].(string)
	if jti == "" {
		return "", "", errors.New("missing jti in refresh token")
	}
	return sub, jti, nil
}
