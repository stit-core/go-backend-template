package user_test

import (
	"net/mail"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/user"
	"git.stit.tech/stit-core/go-backend-template/internal/platform/identity"
)

func FuzzNewUser(f *testing.F) {
	f.Add("ok@example.com", "hash", "Name", "admin")
	f.Add("", "", "", "")
	f.Add("not-an-email", "h", "n", "client")
	f.Add("user+tag@e.co", "h", "юзер", "weird-role")
	f.Add("ok@example.com", "h", "a\x00b", "client")

	f.Fuzz(func(t *testing.T, email, ph, name, roleStr string) {
		role := identity.Role(roleStr)
		u, err := user.NewUser(email, ph, name, role)

		_, mailErr := mail.ParseAddress(email)
		validEmail := mailErr == nil && len(email) <= 254
		validName := !strings.ContainsRune(name, 0)
		validRole := role == identity.RoleAdmin || role == identity.RoleClient || role == identity.RoleUnknown

		switch {
		case !validEmail:
			require.ErrorIs(t, err, user.ErrInvalidEmail, "email %q", email)
			require.Nil(t, u)
		case !validName:
			require.ErrorIs(t, err, user.ErrInvalidName, "name %q", name)
			require.Nil(t, u)
		case validRole:
			require.NoError(t, err, "email %q role %q", email, role)
			require.Equal(t, email, u.Email)
		}
	})
}
