package identity_test

import (
	"context"
	"testing"

	"git.stit.tech/stit-core/go-backend-template/internal/platform/identity"
)

func TestWithFrom(t *testing.T) {
	in := identity.Identity{UserID: "u1", Role: identity.RoleAdmin}
	ctx := identity.With(context.Background(), in)

	out, ok := identity.From(ctx)
	if !ok {
		t.Fatal("ожидали identity в ctx")
	}
	if out != in {
		t.Fatalf("ожидали %v, получили %v", in, out)
	}
}

func TestFromEmpty(t *testing.T) {
	if _, ok := identity.From(context.Background()); ok {
		t.Fatal("в пустом ctx identity быть не должно")
	}
}
