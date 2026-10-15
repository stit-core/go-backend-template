package user_test

import (
	"net/mail"
	"strings"
	"testing"
	"unicode/utf8"

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
	f.Add(" ok@example.com", "h", "n", "client")
	f.Add("ok@example.com\t", "h", "n", "client")
	f.Add("ok@example.com\u00a0", "h", "n", "client")
	f.Add("\u2003ok@example.com", "h", "n", "client")
	f.Add("Bob <ok@example.com>", "h", "n", "client")
	f.Add("ok@example.com", "h", " ", "client")
	f.Add("ok@example.com", "h", strings.Repeat("я", 201), "client")

	f.Fuzz(func(t *testing.T, email, ph, name, roleStr string) {
		role := identity.Role(roleStr)
		u, err := user.NewUser(email, ph, name, role)

		addr, mailErr := mail.ParseAddress(email)
		validEmail := mailErr == nil && addr.Name == "" && addr.Address == email && strings.TrimSpace(email) == email && len(email) <= 254
		validName := strings.TrimSpace(name) != "" && utf8.RuneCountInString(name) <= 200 && !strings.ContainsRune(name, 0)
		validRole := role == identity.RoleAdmin || role == identity.RoleClient

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
			// Guard входа и поиск по адресу обрезают пробелы по краям: адрес сущности их не несёт, и у ящика
			// одна учётная запись.
			require.Equal(t, strings.TrimSpace(u.Email), u.Email)
		default:
			require.ErrorIs(t, err, user.ErrInvalidRole, "role %q", role)
			require.Nil(t, u)
		}
	})
}
