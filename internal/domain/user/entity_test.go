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

func TestUser_ChangeStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		from, to user.Status
		wantErr  error
		want     user.Status
	}{
		{"block active", user.StatusActive, user.StatusBlocked, nil, user.StatusBlocked},
		{"unblock blocked", user.StatusBlocked, user.StatusActive, nil, user.StatusActive},
		{"active stays active", user.StatusActive, user.StatusActive, nil, user.StatusActive},
		{"blocked stays blocked", user.StatusBlocked, user.StatusBlocked, nil, user.StatusBlocked},
		{"delete goes through Delete", user.StatusActive, user.StatusDeleted, user.ErrInvalidStatusTransition, user.StatusActive},
		{"deleted is final", user.StatusDeleted, user.StatusActive, user.ErrInvalidStatusTransition, user.StatusDeleted},
		{"deleted stays out of ChangeStatus", user.StatusDeleted, user.StatusDeleted, user.ErrInvalidStatusTransition, user.StatusDeleted},
		{"unknown target", user.StatusActive, user.Status("archived"), user.ErrInvalidStatusTransition, user.StatusActive},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			u := &user.User{Status: tc.from}
			require.ErrorIs(t, u.ChangeStatus(tc.to), tc.wantErr)
			require.Equal(t, tc.want, u.Status)
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
