package user

import (
	"context"
	"fmt"

	"go.uber.org/fx"

	"git.stit.tech/stit-core/go-backend-template/internal/platform/identity"
	"git.stit.tech/stit-core/go-backend-template/pkg/outcome"
	"git.stit.tech/stit-core/go-backend-template/pkg/paging"
	"git.stit.tech/stit-core/go-backend-template/pkg/policy"
	"git.stit.tech/stit-core/golibinfra/auditkit"
	"git.stit.tech/stit-core/golibinfra/authnkit"
	"git.stit.tech/stit-core/golibinfra/clockkit"
	"git.stit.tech/stit-core/golibinfra/logkit"
	"git.stit.tech/stit-core/golibinfra/outboxkit"
	"git.stit.tech/stit-core/golibinfra/pgkit"
)

type Repository interface {
	Insert(ctx context.Context, u *User) error
	GetByID(ctx context.Context, id string) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	// Update пишет имя и статус u при версии в БД u.Version; иначе, и для удалённого, — ErrConcurrentUpdate.
	Update(ctx context.Context, u *User) (*User, error)
	UpdatePassword(ctx context.Context, id, passwordHash string, version int) (*User, error)
	// SetEmailVerified: нет пользователя или удалён — ErrNotFound.
	SetEmailVerified(ctx context.Context, id string) error
	List(ctx context.Context, filter ListFilter, page paging.Page) ([]*User, error)
	Count(ctx context.Context, filter ListFilter) (int, error)
}

type Authorizer interface {
	CanGetByID(ctx context.Context, caller identity.Identity, targetID string) policy.Decision
	CanList(ctx context.Context, caller identity.Identity) policy.Decision
	CanUpdate(ctx context.Context, caller identity.Identity, targetID string) policy.Decision
	CanChangeStatus(ctx context.Context, caller identity.Identity, targetID string) policy.Decision
	CanDelete(ctx context.Context, caller identity.Identity, targetID string) policy.Decision
}

type UsecaseDeps struct {
	fx.In

	Repo    Repository
	Hasher  authnkit.Hasher
	Policy  Authorizer
	Tx      pgkit.Transactor
	Events  outboxkit.Publisher
	Audit   auditkit.Logger
	Metrics *Metrics
	Log     logkit.Logger
	Clock   clockkit.Clock
}

type Usecase struct {
	d UsecaseDeps
}

func NewUsecase(d UsecaseDeps) *Usecase {
	d.Log = d.Log.With("domain", "user")
	return &Usecase{d: d}
}

func (uc *Usecase) Register(ctx context.Context, in *CreateInput) (u *User, err error) {
	if !policy.PasswordStrong(in.Password) {
		uc.d.Metrics.RecordRegistered(ctx, outcome.WeakPassword)
		return nil, ErrWeakPassword
	}

	hash, err := uc.d.Hasher.Hash(ctx, in.Password)
	if err != nil {
		uc.d.Metrics.RecordRegistered(ctx, outcome.Error)
		return nil, fmt.Errorf("hash password: %w", err)
	}

	newU, err := NewUser(in.Email, hash, in.Name, identity.RoleClient)
	if err != nil {
		uc.d.Metrics.RecordRegistered(ctx, outcome.Invalid)
		return nil, err
	}

	if err := uc.d.Tx.WithTx(ctx, func(txCtx context.Context) error {
		if err := uc.d.Repo.Insert(txCtx, newU); err != nil {
			return err
		}

		return uc.d.Events.Publish(txCtx, subjectUserRegistered,
			UserRegistered{ID: newU.ID, Email: newU.Email, At: newU.CreatedAt})
	}); err != nil {
		uc.d.Metrics.RecordRegistered(ctx, outcome.Error)
		return nil, err
	}

	uc.d.Metrics.RecordRegistered(ctx, outcome.OK)
	uc.d.Log.Info(ctx, "user registered", "user_id", newU.ID, "email", newU.Email)

	if auditErr := uc.d.Audit.Log(ctx, auditkit.Entry{
		ActorID:    newU.ID,
		Action:     "register",
		Resource:   "user",
		ResourceID: newU.ID,
		At:         uc.d.Clock.Now(),
		IP:         identity.IPFrom(ctx),
		RequestID:  identity.RequestIDFrom(ctx),
	}); auditErr != nil {
		uc.d.Log.Error(ctx, "audit register", "err", auditErr)
	}

	return newU, nil
}

func (uc *Usecase) GetByID(ctx context.Context, id string) (u *User, err error) {
	caller, ok := identity.From(ctx)
	if !ok {
		return nil, &policy.ForbiddenError{Reason: "no identity"}
	}
	if d := uc.d.Policy.CanGetByID(ctx, caller, id); !d.IsAllowed {
		return nil, &policy.ForbiddenError{Reason: d.Reason}
	}

	return uc.d.Repo.GetByID(ctx, id)
}

func (uc *Usecase) Me(ctx context.Context) (u *User, err error) {
	caller, ok := identity.From(ctx)
	if !ok {
		return nil, &policy.ForbiddenError{Reason: "no identity"}
	}

	return uc.d.Repo.GetByID(ctx, caller.UserID)
}

func (uc *Usecase) List(ctx context.Context, filter ListFilter, page paging.Page) (users []*User, total int, err error) {
	caller, ok := identity.From(ctx)
	if !ok {
		return nil, 0, &policy.ForbiddenError{Reason: "no identity"}
	}
	if d := uc.d.Policy.CanList(ctx, caller); !d.IsAllowed {
		return nil, 0, &policy.ForbiddenError{Reason: d.Reason}
	}

	total, err = uc.d.Repo.Count(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("count users: %w", err)
	}

	list, err := uc.d.Repo.List(ctx, filter, page.Normalize())
	if err != nil {
		return nil, 0, fmt.Errorf("list users: %w", err)
	}

	return list, total, nil
}

// Update — все поля входа одной записью или ни одного: статус меняет только админ, переход — сущность.
func (uc *Usecase) Update(ctx context.Context, id string, in UpdateInput) (*User, error) {
	caller, ok := identity.From(ctx)
	if !ok {
		uc.d.Metrics.RecordUpdated(ctx, outcome.Forbidden)
		return nil, &policy.ForbiddenError{Reason: "no identity"}
	}
	if d := uc.d.Policy.CanUpdate(ctx, caller, id); !d.IsAllowed {
		uc.d.Metrics.RecordUpdated(ctx, outcome.Forbidden)
		return nil, &policy.ForbiddenError{Reason: d.Reason}
	}
	if in.Status != nil {
		if d := uc.d.Policy.CanChangeStatus(ctx, caller, id); !d.IsAllowed {
			uc.d.Metrics.RecordUpdated(ctx, outcome.Forbidden)
			return nil, &policy.ForbiddenError{Reason: d.Reason}
		}
	}

	if in.Name == nil && in.Status == nil {
		uc.d.Metrics.RecordUpdated(ctx, outcome.Invalid)
		return nil, ErrNoUpdateFields
	}

	var updated *User
	if err := uc.d.Tx.WithTx(ctx, func(txCtx context.Context) error {
		u, err := uc.d.Repo.GetByID(txCtx, id)
		if err != nil {
			return err
		}
		if u.Version != in.Version {
			return ErrConcurrentUpdate
		}
		if in.Name != nil {
			u.Name = *in.Name
		}
		if in.Status != nil {
			if err := u.ChangeStatus(*in.Status); err != nil {
				return err
			}
		}
		if updated, err = uc.d.Repo.Update(txCtx, u); err != nil {
			return err
		}

		return uc.d.Events.Publish(txCtx, subjectUserUpdated,
			UserUpdated{ID: updated.ID, At: updated.UpdatedAt})
	}); err != nil {
		uc.d.Metrics.RecordUpdated(ctx, outcome.Error)
		return nil, err
	}

	uc.d.Metrics.RecordUpdated(ctx, outcome.OK)
	uc.d.Log.Info(ctx, "user updated", "user_id", updated.ID)

	if auditErr := uc.d.Audit.Log(ctx, auditkit.Entry{
		ActorID:    caller.UserID,
		Action:     "update",
		Resource:   "user",
		ResourceID: updated.ID,
		At:         uc.d.Clock.Now(),
		IP:         identity.IPFrom(ctx),
		RequestID:  identity.RequestIDFrom(ctx),
	}); auditErr != nil {
		uc.d.Log.Error(ctx, "audit update", "err", auditErr)
	}

	return updated, nil
}

func (uc *Usecase) Delete(ctx context.Context, id string) error {
	caller, ok := identity.From(ctx)
	if !ok {
		uc.d.Metrics.RecordDeleted(ctx, outcome.Forbidden)
		return &policy.ForbiddenError{Reason: "no identity"}
	}
	if d := uc.d.Policy.CanDelete(ctx, caller, id); !d.IsAllowed {
		uc.d.Metrics.RecordDeleted(ctx, outcome.Forbidden)
		return &policy.ForbiddenError{Reason: d.Reason}
	}

	if err := uc.d.Tx.WithTx(ctx, func(txCtx context.Context) error {
		u, err := uc.d.Repo.GetByID(txCtx, id)
		if err != nil {
			return err
		}
		if err := u.Delete(); err != nil {
			return err
		}
		if _, err := uc.d.Repo.Update(txCtx, u); err != nil {
			return err
		}
		return uc.d.Events.Publish(txCtx, subjectUserDeleted,
			UserDeleted{ID: id, At: uc.d.Clock.Now()})
	}); err != nil {
		uc.d.Metrics.RecordDeleted(ctx, outcome.Error)
		return err
	}

	uc.d.Metrics.RecordDeleted(ctx, outcome.OK)
	uc.d.Log.Info(ctx, "user deleted", "user_id", id)

	if auditErr := uc.d.Audit.Log(ctx, auditkit.Entry{
		ActorID:    caller.UserID,
		Action:     "delete",
		Resource:   "user",
		ResourceID: id,
		At:         uc.d.Clock.Now(),
		IP:         identity.IPFrom(ctx),
		RequestID:  identity.RequestIDFrom(ctx),
	}); auditErr != nil {
		uc.d.Log.Error(ctx, "audit delete", "err", auditErr)
	}

	return nil
}
