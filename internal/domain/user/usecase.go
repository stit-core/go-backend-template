package user

import (
	"context"
	"fmt"

	"go.uber.org/fx"

	"git.stit.tech/akadmin/go-backend-template/internal/platform/identity"
	"git.stit.tech/akadmin/go-backend-template/pkg/policy"
	"git.stit.tech/stit/golibinfra/auditkit"
	"git.stit.tech/stit/golibinfra/authnkit"
	"git.stit.tech/stit/golibinfra/clockkit"
	"git.stit.tech/stit/golibinfra/httpkit"
	"git.stit.tech/stit/golibinfra/logkit"
	"git.stit.tech/stit/golibinfra/outboxkit"
	"git.stit.tech/stit/golibinfra/pgkit"
	"git.stit.tech/stit/golibinfra/telemetry"
)

const minPasswordLen = 8

type Repository interface {
	Create(ctx context.Context, u *User) error
	GetByID(ctx context.Context, id string) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	UpdateProfile(ctx context.Context, id, name string, version int) (*User, error)
	UpdateStatus(ctx context.Context, id string, status Status, version int) (*User, error)
	UpdatePassword(ctx context.Context, id, passwordHash string, version int) (*User, error)
	SetEmailVerified(ctx context.Context, id string) error
	List(ctx context.Context, filter ListFilter, pagination ListPagination) ([]*User, error)
	Count(ctx context.Context, filter ListFilter) (int, error)
}

type Authorizer interface {
	CanGetByID(ctx context.Context, caller identity.Identity, targetID string) policy.Decision
	CanList(ctx context.Context, caller identity.Identity) policy.Decision
	CanUpdate(ctx context.Context, caller identity.Identity, targetID string) policy.Decision
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
	repo    Repository
	hasher  authnkit.Hasher
	policy  Authorizer
	tx      pgkit.Transactor
	events  outboxkit.Publisher
	audit   auditkit.Logger
	metrics *Metrics
	log     logkit.Logger
	clock   clockkit.Clock
}

func NewUsecase(d UsecaseDeps) *Usecase {
	return &Usecase{
		repo:    d.Repo,
		hasher:  d.Hasher,
		policy:  d.Policy,
		tx:      d.Tx,
		events:  d.Events,
		audit:   d.Audit,
		metrics: d.Metrics,
		log:     d.Log.With("domain", "user"),
		clock:   d.Clock,
	}
}

func (uc *Usecase) Register(ctx context.Context, in *Create) (u *User, err error) {
	ctx, end := telemetry.Span(ctx, "user.usecase.register")
	defer end(&err)

	if len(in.Password) < minPasswordLen {
		return nil, ErrWeakPassword
	}

	hash, err := uc.hasher.Hash(in.Password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	newU, err := NewUser(in.Email, hash, in.Name, identity.RoleClient)
	if err != nil {
		return nil, err
	}

	if err := uc.tx.WithTx(ctx, func(txCtx context.Context) error {
		if err := uc.repo.Create(txCtx, newU); err != nil {
			return err
		}

		return uc.events.Publish(txCtx, "user.events.registered",
			UserRegistered{ID: newU.ID, Email: newU.Email, At: newU.CreatedAt})
	}); err != nil {
		return nil, err
	}

	uc.metrics.Registered.Add(ctx, 1)
	uc.log.Info(ctx, "user registered", "user_id", newU.ID, "email", newU.Email)

	if auditErr := uc.audit.Log(ctx, auditkit.Entry{
		ActorID:    newU.ID,
		Action:     "register",
		Resource:   "user",
		ResourceID: newU.ID,
		At:         uc.clock.Now(),
		IP:         identity.IPFrom(ctx),
		RequestID:  httpkit.RequestIDFrom(ctx),
	}); auditErr != nil {
		uc.log.Error(ctx, "audit register", "err", auditErr)
	}

	return newU, nil
}

func (uc *Usecase) GetByID(ctx context.Context, id string) (u *User, err error) {
	ctx, end := telemetry.Span(ctx, "user.usecase.get_by_id")
	defer end(&err)

	caller, ok := identity.From(ctx)
	if !ok {
		return nil, &ForbiddenError{Reason: "no identity"}
	}
	if d := uc.policy.CanGetByID(ctx, caller, id); !d.IsAllowed {
		return nil, &ForbiddenError{Reason: d.Reason}
	}

	return uc.repo.GetByID(ctx, id)
}

func (uc *Usecase) Me(ctx context.Context) (u *User, err error) {
	ctx, end := telemetry.Span(ctx, "user.usecase.me")
	defer end(&err)

	caller, ok := identity.From(ctx)
	if !ok {
		return nil, &ForbiddenError{Reason: "no identity"}
	}

	return uc.repo.GetByID(ctx, caller.UserID)
}

func (uc *Usecase) List(ctx context.Context, filter ListFilter, pg ListPagination) (users []*User, total int, err error) {
	ctx, end := telemetry.Span(ctx, "user.usecase.list")
	defer end(&err)

	caller, ok := identity.From(ctx)
	if !ok {
		return nil, 0, &ForbiddenError{Reason: "no identity"}
	}
	if d := uc.policy.CanList(ctx, caller); !d.IsAllowed {
		return nil, 0, &ForbiddenError{Reason: d.Reason}
	}

	if pg.Size <= 0 {
		pg.Size = 20
	}
	if pg.Size > 100 {
		pg.Size = 100
	}
	if pg.Page < 1 {
		pg.Page = 1
	}

	total, err = uc.repo.Count(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("count users: %w", err)
	}

	list, err := uc.repo.List(ctx, filter, pg)
	if err != nil {
		return nil, 0, fmt.Errorf("list users: %w", err)
	}

	return list, total, nil
}

func (uc *Usecase) Update(ctx context.Context, id string, in Update) (u *User, err error) {
	ctx, end := telemetry.Span(ctx, "user.usecase.update")
	defer end(&err)

	caller, ok := identity.From(ctx)
	if !ok {
		return nil, &ForbiddenError{Reason: "no identity"}
	}
	if d := uc.policy.CanUpdate(ctx, caller, id); !d.IsAllowed {
		return nil, &ForbiddenError{Reason: d.Reason}
	}

	if in.Name == nil && in.Status == nil {
		return nil, ErrNoUpdateFields
	}

	if err := uc.tx.WithTx(ctx, func(txCtx context.Context) error {
		var updated *User
		var updErr error
		switch {
		case in.Name != nil:
			updated, updErr = uc.repo.UpdateProfile(txCtx, id, *in.Name, in.Version)
		case in.Status != nil:
			updated, updErr = uc.repo.UpdateStatus(txCtx, id, *in.Status, in.Version)
		}
		if updErr != nil {
			return updErr
		}
		u = updated

		return uc.events.Publish(txCtx, "user.events.updated",
			UserUpdated{ID: updated.ID, At: updated.UpdatedAt})
	}); err != nil {
		return nil, err
	}

	uc.log.Info(ctx, "user updated", "user_id", u.ID)

	if auditErr := uc.audit.Log(ctx, auditkit.Entry{
		ActorID:    caller.UserID,
		Action:     "update",
		Resource:   "user",
		ResourceID: u.ID,
		At:         uc.clock.Now(),
		IP:         identity.IPFrom(ctx),
		RequestID:  httpkit.RequestIDFrom(ctx),
	}); auditErr != nil {
		uc.log.Error(ctx, "audit update", "err", auditErr)
	}

	return u, nil
}

func (uc *Usecase) Delete(ctx context.Context, id string) (err error) {
	ctx, end := telemetry.Span(ctx, "user.usecase.delete")
	defer end(&err)

	caller, ok := identity.From(ctx)
	if !ok {
		return &ForbiddenError{Reason: "no identity"}
	}
	if d := uc.policy.CanDelete(ctx, caller, id); !d.IsAllowed {
		return &ForbiddenError{Reason: d.Reason}
	}

	if err := uc.tx.WithTx(ctx, func(txCtx context.Context) error {
		current, err := uc.repo.GetByID(txCtx, id)
		if err != nil {
			return err
		}
		if current.Status == StatusDeleted {
			return ErrNotFound
		}
		if _, err := uc.repo.UpdateStatus(txCtx, id, StatusDeleted, current.Version); err != nil {
			return err
		}
		return uc.events.Publish(txCtx, "user.events.deleted",
			UserDeleted{ID: id, At: uc.clock.Now()})
	}); err != nil {
		return err
	}

	uc.log.Info(ctx, "user deleted", "user_id", id)

	if auditErr := uc.audit.Log(ctx, auditkit.Entry{
		ActorID:    caller.UserID,
		Action:     "delete",
		Resource:   "user",
		ResourceID: id,
		At:         uc.clock.Now(),
		IP:         identity.IPFrom(ctx),
		RequestID:  httpkit.RequestIDFrom(ctx),
	}); auditErr != nil {
		uc.log.Error(ctx, "audit delete", "err", auditErr)
	}

	return nil
}

func (uc *Usecase) LookupByID(ctx context.Context, id string) (*User, error) {
	return uc.repo.GetByID(ctx, id)
}

func (uc *Usecase) LookupByEmail(ctx context.Context, email string) (*User, error) {
	return uc.repo.GetByEmail(ctx, email)
}

func (uc *Usecase) UpdatePassword(ctx context.Context, userID, hash string, version int) error {
	_, err := uc.repo.UpdatePassword(ctx, userID, hash, version)
	return err
}

func (uc *Usecase) VerifyPassword(ctx context.Context, userID, plain string) (bool, error) {
	u, err := uc.repo.GetByID(ctx, userID)
	if err != nil {
		return false, err
	}
	return uc.hasher.Verify(plain, u.PasswordHash)
}

func (uc *Usecase) MarkEmailVerified(ctx context.Context, userID string) error {
	return uc.repo.SetEmailVerified(ctx, userID)
}
