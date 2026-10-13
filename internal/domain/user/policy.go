package user

import (
	"context"
	"net/mail"
	"strings"
	"unicode/utf8"

	"git.stit.tech/stit-core/go-backend-template/internal/platform/identity"
	"git.stit.tech/stit-core/go-backend-template/pkg/policy"
)

// maxEmailLen — предел адреса по RFC 5321 в байтах: длиннее не учтёт guard входа, и вход не состоится.
const maxEmailLen = 254

// validEmail — адрес ровно в той форме, в какую его разбирает mail.ParseAddress: без имени, скобок,
// комментария и пробелов по краям. Их отбрасывают guard входа и поиск по адресу — с ними у одного ящика
// было бы несколько учётных записей; регистр снимает индекс LOWER(email).
func validEmail(email string) bool {
	addr, err := mail.ParseAddress(email)
	return err == nil && addr.Name == "" && addr.Address == email && len(email) <= maxEmailLen
}

// emailKey — адрес для поиска: без пробелов по краям, как ключ guard'а входа (bruteforcekit.NormalizeKey).
func emailKey(email string) string { return strings.TrimSpace(email) }

// maxNameLen — предел имени в символах; его же держит maxLength полей name в delivery/http.
const maxNameLen = 200

// validName: NUL Postgres в TEXT не хранит — имя с ним упало бы на записи ошибкой драйвера (500).
func validName(name string) bool {
	return strings.TrimSpace(name) != "" && utf8.RuneCountInString(name) <= maxNameLen && !strings.ContainsRune(name, 0)
}

type Policy struct{}

func NewPolicy() Authorizer { return &Policy{} }

func selfOrAdmin(caller identity.Identity, targetID string) policy.Decision {
	if caller.Role == identity.RoleAdmin {
		return policy.Allow()
	}
	if caller.UserID == targetID {
		return policy.Allow()
	}
	return policy.Deny("not self, not admin")
}

func adminOnly(caller identity.Identity) policy.Decision {
	if caller.Role == identity.RoleAdmin {
		return policy.Allow()
	}
	return policy.Deny("admin only")
}

func (p *Policy) CanGetByID(_ context.Context, caller identity.Identity, targetID string) policy.Decision {
	return selfOrAdmin(caller, targetID)
}

func (p *Policy) CanList(_ context.Context, caller identity.Identity) policy.Decision {
	return adminOnly(caller)
}

func (p *Policy) CanUpdate(_ context.Context, caller identity.Identity, targetID string) policy.Decision {
	return selfOrAdmin(caller, targetID)
}

// CanChangeStatus — статус меняет только админ, и свой тоже: блокировку снимает не тот, кого
// заблокировали.
func (p *Policy) CanChangeStatus(_ context.Context, caller identity.Identity, _ string) policy.Decision {
	return adminOnly(caller)
}

func (p *Policy) CanDelete(_ context.Context, caller identity.Identity, targetID string) policy.Decision {
	return selfOrAdmin(caller, targetID)
}
