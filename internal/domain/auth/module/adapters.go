package module

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"git.sftbmb.ru/sftbmb-core/go-backend-template/internal/domain/auth"
	"git.sftbmb.ru/sftbmb-core/go-backend-template/internal/domain/user"
	"git.sftbmb.ru/sftbmb-core/golibinfra/authnkit"
)

type userReaderAdapter struct {
	uc *user.Usecase
}

func (a *userReaderAdapter) GetByID(ctx context.Context, id string) (*auth.User, error) {
	u, err := a.uc.LookupByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return userToAuth(u), nil
}

func (a *userReaderAdapter) GetByEmail(ctx context.Context, email string) (*auth.User, error) {
	u, err := a.uc.LookupByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	return userToAuth(u), nil
}

func userToAuth(u *user.User) *auth.User {
	return &auth.User{
		ID:           u.ID,
		Email:        u.Email,
		PasswordHash: u.PasswordHash,
		Name:         u.Name,
		Status:       string(u.Status),
		Role:         u.Role,
		Version:      u.Version,
	}
}

type tokensAdapter struct {
	signer     authnkit.JWTSigner
	accessTTL  time.Duration
	refreshTTL time.Duration
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
	expiresAt := time.Now().Add(a.refreshTTL)
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

type tokenParserAdapter struct {
	parser authnkit.JWTParser
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
