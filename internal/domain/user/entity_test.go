package user_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/user"
	"git.stit.tech/stit-core/go-backend-template/internal/platform/identity"
)

func TestNewUser(t *testing.T) {
	t.Parallel()
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
			t.Parallel()
			u, err := user.NewUser(tc.email, tc.hash, tc.uname, tc.role)
			require.ErrorIs(t, err, tc.wantErr)
			if err == nil {
				wantRole := tc.role
				if wantRole == identity.RoleUnknown {
					wantRole = identity.RoleClient
				}
				require.Equal(t, wantRole, u.Role)
			}
		})
	}
}

func TestUserCanLogin(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		status user.Status
		want   bool
	}{
		{"active", user.StatusActive, true},
		{"blocked", user.StatusBlocked, false},
		{"unknown", user.StatusUnknown, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			u := &user.User{Status: tc.status}
			require.Equal(t, tc.want, u.CanLogin())
		})
	}
}
