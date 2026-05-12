package user_test

import (
	"errors"
	"testing"

	"git.stit.tech/stit/go-backend-template/internal/domain/user"
	"git.stit.tech/stit/go-backend-template/internal/platform/identity"
)

func TestNewUser(t *testing.T) {
	tests := []struct {
		name    string
		email   string
		hash    string
		uname   string
		role    identity.Role
		wantErr error
	}{
		{"happy client", "a@b.com", "h", "Alice", identity.RoleClient, nil},
		{"happy admin", "a@b.com", "h", "Alice", identity.RoleAdmin, nil},
		{"default role", "a@b.com", "h", "Alice", identity.RoleUnknown, nil},
		{"bad email", "not-an-email", "h", "A", identity.RoleClient, user.ErrInvalidEmail},
		{"bad role", "a@b.com", "h", "A", identity.Role("hacker"), user.ErrInvalidRole},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			u, err := user.NewUser(tc.email, tc.hash, tc.uname, tc.role)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("ожидали %v, получили %v", tc.wantErr, err)
			}
			if err == nil {
				wantRole := tc.role
				if wantRole == identity.RoleUnknown {
					wantRole = identity.RoleClient
				}
				if u.Role != wantRole {
					t.Fatalf("ожидали role=%s, получили %s", wantRole, u.Role)
				}
			}
		})
	}
}

func TestUserCanLogin(t *testing.T) {
	cases := map[user.Status]bool{
		user.StatusActive:  true,
		user.StatusBlocked: false,
		user.StatusUnknown: false,
	}
	for s, want := range cases {
		u := &user.User{Status: s}
		if got := u.CanLogin(); got != want {
			t.Fatalf("status=%s: ожидали %v, получили %v", s, want, got)
		}
	}
}
