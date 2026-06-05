package postgres

import "git.sftbmb.ru/sftbmb-core/go-backend-template/internal/platform/identity"

func identityRole(s string) identity.Role {
	switch s {
	case string(identity.RoleAdmin):
		return identity.RoleAdmin
	case string(identity.RoleClient):
		return identity.RoleClient
	default:
		return identity.RoleUnknown
	}
}
