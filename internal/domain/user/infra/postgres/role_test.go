// White-box (package postgres): перевод роли из строки БД — неэкспортируемый.
package postgres

import (
	"testing"

	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/platform/identity"
)

// Роль из БД, которой код не знает, — RoleUnknown: прав admin она не даёт.
func TestIdentityRole(t *testing.T) {
	t.Parallel()
	for stored, want := range map[string]identity.Role{
		"admin": identity.RoleAdmin, "client": identity.RoleClient, "owner": identity.RoleUnknown, "": identity.RoleUnknown,
	} {
		require.Equal(t, want, identityRole(stored), stored)
	}
}
