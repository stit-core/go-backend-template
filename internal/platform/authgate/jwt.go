package authgate

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"git.stit.tech/stit-core/go-backend-template/internal/platform/identity"
	"git.stit.tech/stit-core/golibinfra/authnkit"
)

type Extractor struct {
	parser authnkit.JWTParser
}

func NewExtractor(parser authnkit.JWTParser) *Extractor {
	return &Extractor{parser: parser}
}

func (e *Extractor) Extract(r *http.Request) (context.Context, error) {
	token, err := tokenFromRequest(r)
	if err != nil {
		return r.Context(), err
	}

	claims, err := e.parser.Parse(token)
	if err != nil {
		return r.Context(), fmt.Errorf("authgate: parse jwt: %w", err)
	}

	sub, _ := claims["sub"].(string)
	role, _ := claims["role"].(string)

	id := identity.Identity{
		UserID: sub,
		Role:   identity.Role(role),
	}
	return identity.With(r.Context(), id), nil
}

func tokenFromRequest(r *http.Request) (string, error) {
	c, err := r.Cookie(CookieAccess)
	if err != nil || c.Value == "" {
		return "", errors.New("authgate: no auth cookie")
	}
	return c.Value, nil
}
