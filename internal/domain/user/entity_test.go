package user_test

import (
	"strings"
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
		{"letter case is kept", "Bob@Example.com", "h", "A", identity.RoleClient, nil},
		// Адрес — только в форме, которую разбирает mail.ParseAddress: иначе один ящик получил бы несколько
		// учётных записей, а guard входа считает их одним субъектом.
		{"space before the address", " a@b.com", "h", "A", identity.RoleClient, user.ErrInvalidEmail},
		{"space after the address", "a@b.com ", "h", "A", identity.RoleClient, user.ErrInvalidEmail},
		{"tab before the address", "\ta@b.com", "h", "A", identity.RoleClient, user.ErrInvalidEmail},
		{"display name", "Bob <bob@example.com>", "h", "A", identity.RoleClient, user.ErrInvalidEmail},
		{"angle brackets", "<bob@example.com>", "h", "A", identity.RoleClient, user.ErrInvalidEmail},
		{"comment", "bob@example.com (comment)", "h", "A", identity.RoleClient, user.ErrInvalidEmail},
		{"email of 254 bytes", strings.Repeat("a", 64) + "@" + strings.Repeat("b", 185) + ".com", "h", "A", identity.RoleClient, nil},
		{"email of 255 bytes in 131 characters", strings.Repeat("я", 124) + "a@b.com", "h", "A", identity.RoleClient, user.ErrInvalidEmail},
		{"bad role", "a@b.com", "h", "A", identity.Role("hacker"), user.ErrInvalidRole},
		{"name with NUL", "a@b.com", "h", "A\x00B", identity.RoleClient, user.ErrInvalidName},
		// Предел имени — в символах: 200 букв не из ASCII — 400 байт и ещё имя.
		{"name of 200 characters", "a@b.com", "h", strings.Repeat("я", 200), identity.RoleClient, nil},
		{"name of 201 characters", "a@b.com", "h", strings.Repeat("я", 201), identity.RoleClient, user.ErrInvalidName},
		{"empty name", "a@b.com", "h", "", identity.RoleClient, user.ErrInvalidName},
		{"blank name", "a@b.com", "h", " \t ", identity.RoleClient, user.ErrInvalidName},
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

// Имя пишется в TEXT: NUL Postgres не хранит — отказ поля, а не ошибка драйвера на записи; предел и
// пустое имя — те же, что у NewUser.
func TestUser_Rename(t *testing.T) {
	t.Parallel()
	u := &user.User{Name: "Alice"}
	require.NoError(t, u.Rename("Алиса"))
	require.Equal(t, "Алиса", u.Name)
	require.NoError(t, u.Rename(strings.Repeat("я", 200)))

	for _, name := range []string{"A\x00B", strings.Repeat("я", 201), "  "} {
		require.ErrorIs(t, u.Rename(name), user.ErrInvalidName, "name %q", name)
		require.Equal(t, strings.Repeat("я", 200), u.Name, "a rejected name leaves the user as is")
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
