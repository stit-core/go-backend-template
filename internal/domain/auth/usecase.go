package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/fx"

	"git.stit.tech/stit-core/go-backend-template/internal/platform/identity"
	"git.stit.tech/stit-core/go-backend-template/pkg/outcome"
	"git.stit.tech/stit-core/go-backend-template/pkg/policy"
	"git.stit.tech/stit-core/golibinfra/auditkit"
	"git.stit.tech/stit-core/golibinfra/authnkit"
	"git.stit.tech/stit-core/golibinfra/clockkit"
	"git.stit.tech/stit-core/golibinfra/logkit"
	"git.stit.tech/stit-core/golibinfra/pgkit"
)

type UserReader interface {
	GetByID(ctx context.Context, id string) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
}

type PasswordVerifier interface {
	VerifyPassword(ctx context.Context, userID, plain string) (bool, error)
}

type PasswordSetter interface {
	UpdatePassword(ctx context.Context, userID, hash string, version int) error
}

type Tokens interface {
	IssueAccess(ctx context.Context, u *User) (string, error)
	IssueRefresh(ctx context.Context, userID string) (token, jti string, expiresAt time.Time, err error)
}

type TokenParser interface {
	ParseRefresh(ctx context.Context, token string) (userID, jti string, err error)
}

type SessionRepository interface {
	Insert(ctx context.Context, jti, userID string, expiresAt time.Time) error
	GetByJTI(ctx context.Context, jti string) (*RefreshSession, error)
	MarkUsed(ctx context.Context, jti, replacedByJTI string) error
}

type Authorizer interface {
	CanChangePassword(ctx context.Context, caller identity.Identity, targetID string) policy.Decision
}

// LoginGuard: Attempt учитывает попытку до проверки пароля — неудача отдельной записи не требует.
type LoginGuard interface {
	Attempt(ctx context.Context, subject, ip string) (*LockState, error)
	RecordSuccess(ctx context.Context, subject, ip string) error
}

type UsecaseDeps struct {
	fx.In

	Users          UserReader
	PasswordVerify PasswordVerifier
	PasswordSet    PasswordSetter
	Hasher         authnkit.Hasher
	Tokens         Tokens
	TokenParser    TokenParser
	Sessions       SessionRepository
	Tx             pgkit.Transactor
	Policy         Authorizer
	Guard          LoginGuard
	Audit          auditkit.Logger
	Log            logkit.Logger
	Clock          clockkit.Clock
	Metrics        *Metrics
}

type Usecase struct {
	d UsecaseDeps
}

func NewUsecase(d UsecaseDeps) *Usecase {
	d.Log = d.Log.With("domain", "auth")
	return &Usecase{d: d}
}

func (uc *Usecase) Login(ctx context.Context, in *LoginInput) (*LoginOutput, error) {
	ip := identity.IPFrom(ctx)

	lock, guardErr := uc.d.Guard.Attempt(ctx, in.Email, ip)
	if guardErr != nil {
		uc.d.Log.Error(ctx, "guard attempt", "err", guardErr)
	} else if lock != nil {
		uc.writeLoginFailedAudit(ctx, in.Email, "login_locked", "locked")
		uc.d.Metrics.RecordLogin(ctx, resultLocked)
		return nil, &LockedError{Wait: lock.Until.Sub(uc.d.Clock.Now())}
	}

	u, err := uc.d.Users.GetByEmail(ctx, in.Email)
	if err != nil {
		if !errors.Is(err, ErrUserNotFound) {
			return nil, fmt.Errorf("lookup user: %w", err)
		}
		// Холостая проверка: ответ на неизвестный email стоит столько же, сколько
		// проверка пароля — наличие учётной записи не выдаётся таймингом.
		uc.d.Hasher.VerifyDummy(ctx)
		uc.writeLoginFailedAudit(ctx, in.Email, "login_failed", "invalid_credentials")
		uc.d.Metrics.RecordLogin(ctx, resultInvalidLogin)
		return nil, ErrInvalidCredentials
	}

	if !u.CanLogin {
		uc.d.Hasher.VerifyDummy(ctx)
		uc.writeLoginFailedAudit(ctx, in.Email, "login_failed", "blocked")
		uc.d.Metrics.RecordLogin(ctx, resultBlocked)
		return nil, ErrInvalidCredentials
	}

	ok, err := uc.d.Hasher.Verify(ctx, in.Password, u.PasswordHash)
	if err != nil || !ok {
		uc.writeLoginFailedAudit(ctx, in.Email, "login_failed", "invalid_credentials")
		uc.d.Metrics.RecordLogin(ctx, resultInvalidLogin)
		return nil, ErrInvalidCredentials
	}

	if recordErr := uc.d.Guard.RecordSuccess(ctx, in.Email, ip); recordErr != nil {
		uc.d.Log.Error(ctx, "guard record success", "err", recordErr)
	}

	access, err := uc.d.Tokens.IssueAccess(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("issue access token: %w", err)
	}
	refresh, jti, expiresAt, err := uc.d.Tokens.IssueRefresh(ctx, u.ID)
	if err != nil {
		return nil, fmt.Errorf("issue refresh token: %w", err)
	}
	if err := uc.d.Sessions.Insert(ctx, jti, u.ID, expiresAt); err != nil {
		return nil, fmt.Errorf("insert refresh session: %w", err)
	}

	uc.d.Log.Info(ctx, "user logged in", "user_id", u.ID)

	if auditErr := uc.d.Audit.Log(ctx, auditkit.Entry{
		ActorID:    u.ID,
		Action:     "login",
		Resource:   "user",
		ResourceID: u.ID,
		At:         uc.d.Clock.Now(),
		IP:         ip,
		RequestID:  identity.RequestIDFrom(ctx),
	}); auditErr != nil {
		uc.d.Log.Error(ctx, "audit login", "err", auditErr)
	}
	uc.d.Metrics.RecordLogin(ctx, outcome.OK)

	return &LoginOutput{AccessToken: access, RefreshToken: refresh, User: u}, nil
}

func (uc *Usecase) Refresh(ctx context.Context, refreshToken string) (*LoginOutput, error) {
	userID, oldJTI, err := uc.d.TokenParser.ParseRefresh(ctx, refreshToken)
	if err != nil {
		if errors.Is(err, ErrTokenExpired) {
			uc.d.Metrics.RecordRefresh(ctx, outcome.Expired)
			return nil, ErrRefreshExpired
		}
		uc.d.Metrics.RecordRefresh(ctx, outcome.Invalid)
		return nil, ErrRefreshInvalid
	}

	session, err := uc.d.Sessions.GetByJTI(ctx, oldJTI)
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			uc.d.Metrics.RecordRefresh(ctx, outcome.Invalid)
			return nil, ErrRefreshInvalid
		}
		return nil, fmt.Errorf("get refresh session: %w", err)
	}
	if session.UserID != userID {
		uc.d.Metrics.RecordRefresh(ctx, outcome.Invalid)
		return nil, ErrRefreshInvalid
	}
	if session.UsedAt != nil {
		uc.writeRefreshReusedAudit(ctx, userID, oldJTI)
		uc.d.Metrics.RecordRefresh(ctx, resultReused)
		return nil, ErrRefreshReused
	}
	if uc.d.Clock.Now().After(session.ExpiresAt) {
		uc.d.Metrics.RecordRefresh(ctx, outcome.Expired)
		return nil, ErrRefreshExpired
	}

	u, err := uc.d.Users.GetByID(ctx, userID)
	if err != nil {
		if !errors.Is(err, ErrUserNotFound) {
			return nil, fmt.Errorf("lookup user: %w", err)
		}
		uc.d.Metrics.RecordRefresh(ctx, outcome.Invalid)
		return nil, ErrRefreshInvalid
	}

	access, err := uc.d.Tokens.IssueAccess(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("issue access token: %w", err)
	}
	newRefresh, newJTI, newExpiresAt, err := uc.d.Tokens.IssueRefresh(ctx, u.ID)
	if err != nil {
		return nil, fmt.Errorf("issue refresh token: %w", err)
	}

	if err := uc.d.Tx.WithTx(ctx, func(txCtx context.Context) error {
		if err := uc.d.Sessions.MarkUsed(txCtx, oldJTI, newJTI); err != nil {
			return err
		}
		return uc.d.Sessions.Insert(txCtx, newJTI, u.ID, newExpiresAt)
	}); err != nil {
		return nil, fmt.Errorf("rotate refresh session: %w", err)
	}
	uc.d.Metrics.RecordRefresh(ctx, outcome.OK)

	return &LoginOutput{AccessToken: access, RefreshToken: newRefresh, User: u}, nil
}

func (uc *Usecase) Logout(ctx context.Context, refreshToken string) error {
	userID, jti, parseErr := uc.d.TokenParser.ParseRefresh(ctx, refreshToken)
	if parseErr != nil {
		return nil //nolint:nilerr // logout is idempotent: a broken/expired token is already logged out
	}

	session, err := uc.d.Sessions.GetByJTI(ctx, jti)
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			return nil
		}
		return fmt.Errorf("get refresh session: %w", err)
	}
	if session.UserID != userID || session.UsedAt != nil {
		return nil
	}

	if err := uc.d.Sessions.MarkUsed(ctx, jti, ""); err != nil {
		return fmt.Errorf("mark refresh session used: %w", err)
	}

	if auditErr := uc.d.Audit.Log(ctx, auditkit.Entry{
		ActorID:    userID,
		Action:     "logout",
		Resource:   "user",
		ResourceID: userID,
		At:         uc.d.Clock.Now(),
		IP:         identity.IPFrom(ctx),
		RequestID:  identity.RequestIDFrom(ctx),
	}); auditErr != nil {
		uc.d.Log.Error(ctx, "audit logout", "err", auditErr)
	}
	uc.d.Metrics.RecordLogout(ctx)
	return nil
}

func (uc *Usecase) ChangePassword(ctx context.Context, userID, oldPassword, newPassword string) error {
	caller, ok := identity.From(ctx)
	if !ok {
		uc.d.Metrics.RecordPasswordChange(ctx, outcome.Forbidden)
		return &policy.ForbiddenError{Reason: "no identity"}
	}
	if d := uc.d.Policy.CanChangePassword(ctx, caller, userID); !d.IsAllowed {
		uc.d.Metrics.RecordPasswordChange(ctx, outcome.Forbidden)
		return &policy.ForbiddenError{Reason: d.Reason}
	}

	if !policy.PasswordStrong(newPassword) {
		uc.d.Metrics.RecordPasswordChange(ctx, outcome.WeakPassword)
		return ErrWeakPassword
	}

	verified, err := uc.d.PasswordVerify.VerifyPassword(ctx, userID, oldPassword)
	if err != nil {
		return fmt.Errorf("verify old password: %w", err)
	}
	if !verified {
		uc.d.Metrics.RecordPasswordChange(ctx, resultInvalidOldPassword)
		return ErrWrongOldPassword
	}

	current, err := uc.d.Users.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("load user: %w", err)
	}

	hash, err := uc.d.Hasher.Hash(ctx, newPassword)
	if err != nil {
		return fmt.Errorf("hash new password: %w", err)
	}
	if err := uc.d.PasswordSet.UpdatePassword(ctx, userID, hash, current.Version); err != nil {
		return err
	}

	if auditErr := uc.d.Audit.Log(ctx, auditkit.Entry{
		ActorID:    caller.UserID,
		Action:     "change_password",
		Resource:   "user",
		ResourceID: userID,
		At:         uc.d.Clock.Now(),
		IP:         identity.IPFrom(ctx),
		RequestID:  identity.RequestIDFrom(ctx),
	}); auditErr != nil {
		uc.d.Log.Error(ctx, "audit change_password", "err", auditErr)
	}
	uc.d.Metrics.RecordPasswordChange(ctx, outcome.OK)

	return nil
}

func (uc *Usecase) writeLoginFailedAudit(ctx context.Context, email, action, reason string) {
	entry := auditkit.Entry{
		Action:     action,
		Resource:   "user",
		ResourceID: email,
		At:         uc.d.Clock.Now(),
		IP:         identity.IPFrom(ctx),
		RequestID:  identity.RequestIDFrom(ctx),
		Metadata:   map[string]any{"reason": reason},
	}
	if auditErr := uc.d.Audit.Log(ctx, entry); auditErr != nil {
		uc.d.Log.Error(ctx, "audit "+action, "err", auditErr)
	}
}

func (uc *Usecase) writeRefreshReusedAudit(ctx context.Context, userID, jti string) {
	entry := auditkit.Entry{
		ActorID:    userID,
		Action:     "refresh_reused",
		Resource:   "refresh_session",
		ResourceID: jti,
		At:         uc.d.Clock.Now(),
		IP:         identity.IPFrom(ctx),
		RequestID:  identity.RequestIDFrom(ctx),
	}
	if auditErr := uc.d.Audit.Log(ctx, entry); auditErr != nil {
		uc.d.Log.Error(ctx, "audit refresh_reused", "err", auditErr)
	}
}
