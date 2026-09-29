package auth

import (
	"context"

	"git.stit.tech/stit-core/go-backend-template/internal/platform/identity"
	"git.stit.tech/stit-core/go-backend-template/pkg/policy"
)

const minPasswordLen = 8

func PasswordStrengthOK(plain string) bool { return len(plain) >= minPasswordLen }

type Policy struct{}

func NewPolicy() Authorizer { return &Policy{} }

func (p *Policy) CanChangePassword(_ context.Context, caller identity.Identity, targetID string) policy.Decision {
	if caller.Role == identity.RoleAdmin {
		return policy.Allow()
	}
	if caller.UserID == targetID {
		return policy.Allow()
	}
	return policy.Deny("not self, not admin")
}
