// Package jwt — порт Tokens домена auth на HS256-JWT authnkit.
package jwt

import (
	"context"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/auth"
	"git.stit.tech/stit-core/golibinfra/authnkit"
)

type tokens struct {
	signer authnkit.JWTSigner
}

// NewTokens: TTL, iss, aud и claim typ проставляет кит (SignAccess).
func NewTokens(s authnkit.JWTSigner) auth.Tokens {
	return &tokens{signer: s}
}

func (t *tokens) IssueAccess(ctx context.Context, u *auth.User) (string, error) {
	return t.signer.SignAccess(ctx, map[string]any{
		"sub":  u.ID,
		"role": string(u.Role),
	})
}
