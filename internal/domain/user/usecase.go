package user

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/fx"

	"git.stit.tech/stit-core/go-backend-template/internal/platform/identity"
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
	// GetByIDForShare — GetByID под блокировкой FOR SHARE: до конца транзакции из ctx строку не изменят,
	// а читает он её после коммита изменения, которое уже идёт.
	GetByIDForShare(ctx context.Context, id string) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	// GetByEmailForUpdate — GetByEmail под блокировкой строки до конца транзакции из ctx.
	GetByEmailForUpdate(ctx context.Context, email string) (*User, error)
	// Update пишет имя и статус u при версии в БД u.Version; иначе, и для удалённого, — ErrConcurrentUpdate.
	Update(ctx context.Context, u *User) (*User, error)
	// Delete удаляет мягко и без версии: уже удалённого или несуществующего нет — ErrNotFound, так что
	// повтор и гонка с другой записью — не конфликт.
	Delete(ctx context.Context, id string) error
	UpdatePassword(ctx context.Context, id, passwordHash string, version int) (*User, error)
	// Promote делает учётную запись admin с паролем passwordHash при версии version; иначе — ErrConcurrentUpdate.
	Promote(ctx context.Context, id, passwordHash string, version int) (*User, error)
	// SetEmailVerified: нет пользователя или удалён — ErrNotFound.
	SetEmailVerified(ctx context.Context, id string) error
	List(ctx context.Context, filter ListFilter, page paging.Page) ([]*User, error)
	Count(ctx context.Context, filter ListFilter) (int, error)
}

// Sessions — сессии пользователя (authnkit/session): RevokeAll закрывает все и участвует в транзакции из ctx.
type Sessions interface {
	RevokeAll(ctx context.Context, userID string) error
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

	Repo     Repository
	Hasher   authnkit.Hasher
	Policy   Authorizer
	Tx       pgkit.Transactor
	Events   outboxkit.Publisher
	Sessions Sessions
	Audit    auditkit.Logger
	Log      logkit.Logger
	Clock    clockkit.Clock
}

type Usecase struct {
	d UsecaseDeps
}

func NewUsecase(d UsecaseDeps) *Usecase {
	d.Log = d.Log.With("domain", "user")
	return &Usecase{d: d}
}

func (uc *Usecase) Register(ctx context.Context, in *CreateInput) (*User, error) {
	hash, err := hashPassword(ctx, uc.d.Hasher, in.Password)
	if err != nil {
		return nil, err
	}
	u, err := NewUser(in.Email, hash, in.Name, identity.RoleClient)
	if err != nil {
		return nil, err
	}

	if err := uc.d.Tx.WithTx(ctx, func(txCtx context.Context) error {
		if err := uc.d.Repo.Insert(txCtx, u); err != nil {
			return err
		}
		return uc.d.Events.Publish(txCtx, subjectUserRegistered, UserRegistered{ID: u.ID, At: uc.d.Clock.Now()})
	}); err != nil {
		return nil, err
	}

	uc.d.Log.Info(ctx, "user registered", "user_id", u.ID)
	uc.audit(ctx, auditkit.Entry{ActorID: u.ID, Action: "register", Resource: "user", ResourceID: u.ID})
	return u, nil
}

func (uc *Usecase) GetByID(ctx context.Context, id string) (*User, error) {
	caller, ok := identity.From(ctx)
	if !ok {
		return nil, &policy.ForbiddenError{Reason: "no identity"}
	}
	if d := uc.d.Policy.CanGetByID(ctx, caller, id); !d.IsAllowed {
		return nil, &policy.ForbiddenError{Reason: d.Reason}
	}
	return uc.d.Repo.GetByID(ctx, id)
}

func (uc *Usecase) Me(ctx context.Context) (*User, error) {
	caller, ok := identity.From(ctx)
	if !ok {
		return nil, &policy.ForbiddenError{Reason: "no identity"}
	}
	return uc.d.Repo.GetByID(ctx, caller.UserID)
}

func (uc *Usecase) List(ctx context.Context, filter ListFilter, page paging.Page) ([]*User, int, error) {
	caller, ok := identity.From(ctx)
	if !ok {
		return nil, 0, &policy.ForbiddenError{Reason: "no identity"}
	}
	if d := uc.d.Policy.CanList(ctx, caller); !d.IsAllowed {
		return nil, 0, &policy.ForbiddenError{Reason: d.Reason}
	}

	total, err := uc.d.Repo.Count(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("count users: %w", err)
	}
	list, err := uc.d.Repo.List(ctx, filter, page.Normalize())
	if err != nil {
		return nil, 0, fmt.Errorf("list users: %w", err)
	}
	return list, total, nil
}

// Update — все поля входа одной записью или ни одного: статус меняет только админ, переход — сущность;
// у учётной записи, которую не пускают, той же транзакцией закрываются все сессии.
func (uc *Usecase) Update(ctx context.Context, id string, in UpdateInput) (*User, error) {
	caller, ok := identity.From(ctx)
	if !ok {
		return nil, &policy.ForbiddenError{Reason: "no identity"}
	}
	if d := uc.d.Policy.CanUpdate(ctx, caller, id); !d.IsAllowed {
		return nil, &policy.ForbiddenError{Reason: d.Reason}
	}
	if in.Status != nil {
		if d := uc.d.Policy.CanChangeStatus(ctx, caller, id); !d.IsAllowed {
			return nil, &policy.ForbiddenError{Reason: d.Reason}
		}
	}
	if in.Name == nil && in.Status == nil {
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
			if err := u.Rename(*in.Name); err != nil {
				return err
			}
		}
		if in.Status != nil {
			if err := u.ChangeStatus(*in.Status); err != nil {
				return err
			}
		}
		if updated, err = uc.d.Repo.Update(txCtx, u); err != nil {
			return err
		}
		if !u.CanLogin() {
			if err := uc.d.Sessions.RevokeAll(txCtx, u.ID); err != nil {
				return err
			}
		}
		return uc.d.Events.Publish(txCtx, subjectUserUpdated, UserUpdated{ID: updated.ID, At: uc.d.Clock.Now()})
	}); err != nil {
		return nil, err
	}

	uc.d.Log.Info(ctx, "user updated", "user_id", updated.ID)
	uc.audit(ctx, auditkit.Entry{ActorID: caller.UserID, Action: "update", Resource: "user", ResourceID: updated.ID})
	return updated, nil
}

// Delete закрывает все сессии удалённого той же транзакцией.
func (uc *Usecase) Delete(ctx context.Context, id string) error {
	caller, ok := identity.From(ctx)
	if !ok {
		return &policy.ForbiddenError{Reason: "no identity"}
	}
	if d := uc.d.Policy.CanDelete(ctx, caller, id); !d.IsAllowed {
		return &policy.ForbiddenError{Reason: d.Reason}
	}

	if err := uc.d.Tx.WithTx(ctx, func(txCtx context.Context) error {
		if err := uc.d.Repo.Delete(txCtx, id); err != nil {
			return err
		}
		if err := uc.d.Sessions.RevokeAll(txCtx, id); err != nil {
			return err
		}
		return uc.d.Events.Publish(txCtx, subjectUserDeleted, UserDeleted{ID: id, At: uc.d.Clock.Now()})
	}); err != nil {
		return err
	}

	uc.d.Log.Info(ctx, "user deleted", "user_id", id)
	uc.audit(ctx, auditkit.Entry{ActorID: caller.UserID, Action: "delete", Resource: "user", ResourceID: id})
	return nil
}

// EnsureAdmin — вход оператора cmd/seed-admin, без policy: решает тот, кто держит доступ к БД. Учётная запись адреса
// становится admin с этим паролем, нет такой — заводится с подтверждённым адресом; прежние сессии закрываются.
func (uc *Usecase) EnsureAdmin(ctx context.Context, in CreateInput) (*User, error) {
	hash, err := hashPassword(ctx, uc.d.Hasher, in.Password)
	if err != nil {
		return nil, err
	}
	admin, err := NewUser(in.Email, hash, in.Name, identity.RoleAdmin)
	if err != nil {
		return nil, err
	}
	admin.EmailVerified = true
	if err := uc.d.Tx.WithTx(ctx, func(txCtx context.Context) error {
		existing, err := uc.d.Repo.GetByEmailForUpdate(txCtx, admin.Email)
		switch {
		case errors.Is(err, ErrNotFound):
			err = uc.d.Repo.Insert(txCtx, admin)
		case err == nil:
			admin, err = uc.d.Repo.Promote(txCtx, existing.ID, hash, existing.Version)
		}
		if err != nil {
			return err
		}
		return uc.d.Sessions.RevokeAll(txCtx, admin.ID)
	}); err != nil {
		return nil, err
	}

	uc.d.Log.Info(ctx, "admin seeded", "user_id", admin.ID)
	uc.audit(ctx, auditkit.Entry{Action: "seed_admin", Resource: "user", ResourceID: admin.ID})
	return admin, nil
}

// hashPassword — правило пароля продукта и хэш: пароль длиннее предела хэшера — ошибка поля, а не сбой.
func hashPassword(ctx context.Context, h authnkit.Hasher, plain string) (string, error) {
	if !policy.PasswordStrong(plain) {
		return "", ErrWeakPassword
	}
	hash, err := h.Hash(ctx, plain)
	switch {
	case errors.Is(err, authnkit.ErrPasswordTooLong):
		return "", ErrPasswordTooLong
	case err != nil:
		return "", fmt.Errorf("hash password: %w", err)
	}
	return hash, nil
}

// audit дописывает время (IP клиента и ID запроса auditkit берёт из ctx); сбой записи аудита операцию не
// отменяет — он уходит в лог.
func (uc *Usecase) audit(ctx context.Context, e auditkit.Entry) {
	e.At = uc.d.Clock.Now()
	if err := uc.d.Audit.Log(ctx, e); err != nil {
		uc.d.Log.Error(ctx, "audit "+e.Action, "error", err)
	}
}
