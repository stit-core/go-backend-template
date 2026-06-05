package auth

import (
	"context"

	"git.sftbmb.ru/sftbmb-core/go-backend-template/internal/platform/identity"
	"git.sftbmb.ru/sftbmb-core/go-backend-template/pkg/policy"
)

type Policy struct{}

func NewPolicy() *Policy { return &Policy{} }

func (p *Policy) CanChangePassword(_ context.Context, caller identity.Identity, targetID string) policy.Decision {
	if caller.Role == identity.RoleAdmin {
		return policy.Allow()
	}
	if caller.UserID == targetID {
		return policy.Allow()
	}
	return policy.Deny("not self, not admin")
}
