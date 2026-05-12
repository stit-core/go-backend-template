package authgate

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"git.stit.tech/stit/go-backend-template/internal/platform/identity"
	"git.stit.tech/stit/golibinfra/authnkit"
)

type Extractor struct {
	parser authnkit.JWTParser
}

func NewExtractor(parser authnkit.JWTParser) *Extractor {
	return &Extractor{parser: parser}
}

const bearerPrefix = "Bearer "

func (e *Extractor) Extract(r *http.Request) (context.Context, error) {
	raw := strings.TrimSpace(r.Header.Get("Authorization"))
	if raw == "" {
		return r.Context(), errors.New("authgate: empty Authorization header")
	}
	if !strings.HasPrefix(raw, bearerPrefix) {
		return r.Context(), errors.New("authgate: Authorization must start with \"Bearer \"")
	}
	token := strings.TrimPrefix(raw, bearerPrefix)

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
