package user

import (
	"context"

	"git.stit.tech/stit/go-backend-template/internal/platform/identity"
	"git.stit.tech/stit/go-backend-template/pkg/policy"
)

type Policy struct{}

func NewPolicy() *Policy { return &Policy{} }

func selfOrAdmin(caller identity.Identity, targetID string) policy.Decision {
	if caller.Role == identity.RoleAdmin {
		return policy.Allow()
	}
	if caller.UserID == targetID {
		return policy.Allow()
	}
	return policy.Deny("not self, not admin")
}

func (p *Policy) CanGetByID(_ context.Context, caller identity.Identity, targetID string) policy.Decision {
	return selfOrAdmin(caller, targetID)
}

func (p *Policy) CanList(_ context.Context, caller identity.Identity) policy.Decision {
	if caller.Role == identity.RoleAdmin {
		return policy.Allow()
	}
	return policy.Deny("admin only")
}

func (p *Policy) CanUpdate(_ context.Context, caller identity.Identity, targetID string) policy.Decision {
	return selfOrAdmin(caller, targetID)
}

func (p *Policy) CanDelete(_ context.Context, caller identity.Identity, targetID string) policy.Decision {
	return selfOrAdmin(caller, targetID)
}
