package user_test

import (
	"net/mail"
	"testing"

	"git.stit.tech/akadmin/go-backend-template/internal/domain/user"
	"git.stit.tech/akadmin/go-backend-template/internal/platform/identity"
)

func FuzzNewUser(f *testing.F) {
	f.Add("ok@example.com", "hash", "Name", "admin")
	f.Add("", "", "", "")
	f.Add("not-an-email", "h", "n", "client")
	f.Add("user+tag@e.co", "h", "юзер", "weird-role")

	f.Fuzz(func(t *testing.T, email, ph, name, roleStr string) {
		role := identity.Role(roleStr)
		u, err := user.NewUser(email, ph, name, role)

		_, mailErr := mail.ParseAddress(email)
		validEmail := mailErr == nil
		validRole := role == identity.RoleAdmin || role == identity.RoleClient || role == identity.RoleUnknown

		if validEmail && validRole {
			if err != nil {
				t.Fatalf("expected nil err for email=%q role=%q, got %v", email, role, err)
			}
			if u == nil {
				t.Fatal("expected non-nil user for valid input")
			}
			if u.Email != email {
				t.Errorf("email mismatch: got %q want %q", u.Email, email)
			}
		}
		if !validEmail {
			if err == nil {
				t.Fatalf("expected err for invalid email %q", email)
			}
			if u != nil {
				t.Fatal("expected nil user for invalid email")
			}
		}
	})
}
