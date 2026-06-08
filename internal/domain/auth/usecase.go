package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/fx"

	"git.stit.tech/stit-core/go-backend-template/internal/platform/identity"
	"git.stit.tech/stit-core/go-backend-template/pkg/policy"
	"git.stit.tech/stit-core/golibinfra/auditkit"
	"git.stit.tech/stit-core/golibinfra/authnkit"
	"git.stit.tech/stit-core/golibinfra/clockkit"
	"git.stit.tech/stit-core/golibinfra/httpkit"
	"git.stit.tech/stit-core/golibinfra/logkit"
	"git.stit.tech/stit-core/golibinfra/pgkit"
	"git.stit.tech/stit-core/golibinfra/telemetry"
)

const minPasswordLen = 8

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
	IssueAccess(u *User) (string, error)
	IssueRefresh(userID string) (token, jti string, expiresAt time.Time, err error)
}

type TokenParser interface {
	ParseRefresh(token string) (userID, jti string, err error)
}

type SessionRepository interface {
	Insert(ctx context.Context, jti, userID string, expiresAt time.Time) error
	GetByJTI(ctx context.Context, jti string) (*RefreshSession, error)
	MarkUsed(ctx context.Context, jti, replacedByJTI string) error
}

type Authorizer interface {
	CanChangePassword(ctx context.Context, caller identity.Identity, targetID string) policy.Decision
}

type LoginGuard interface {
	Check(ctx context.Context, subject, ip string) (*LockState, error)
	RecordFailure(ctx context.Context, subject, ip string) error
	RecordSuccess(ctx context.Context, subject, ip string) error
}

type ForbiddenError struct{ Reason string }

func (e *ForbiddenError) Error() string {
	if e.Reason == "" {
		return "forbidden"
	}
	return "forbidden: " + e.Reason
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
	users    UserReader
	pwVer    PasswordVerifier
	pwSet    PasswordSetter
	hasher   authnkit.Hasher
	tokens   Tokens
	tparser  TokenParser
	sessions SessionRepository
	tx       pgkit.Transactor
	policy   Authorizer
	guard    LoginGuard
	audit    auditkit.Logger
	log      logkit.Logger
	clock    clockkit.Clock
	metrics  *Metrics
}

func NewUsecase(d UsecaseDeps) *Usecase {
	return &Usecase{
		users:    d.Users,
		pwVer:    d.PasswordVerify,
		pwSet:    d.PasswordSet,
		hasher:   d.Hasher,
		tokens:   d.Tokens,
		tparser:  d.TokenParser,
		sessions: d.Sessions,
		tx:       d.Tx,
		policy:   d.Policy,
		guard:    d.Guard,
		audit:    d.Audit,
		log:      d.Log.With("domain", "auth"),
		clock:    d.Clock,
		metrics:  d.Metrics,
	}
}

func (uc *Usecase) Login(ctx context.Context, in *LoginInput) (out *LoginOutput, err error) {
	ctx, end := telemetry.Span(ctx, "auth.usecase.login")
	defer end(&err)

	ip := identity.IPFrom(ctx)

	lock, guardErr := uc.guard.Check(ctx, in.Email, ip)
	if guardErr != nil {
		uc.log.Error(ctx, "guard check", "err", guardErr)
	} else if lock != nil {
		uc.writeLoginFailedAudit(ctx, in.Email, "login_locked", "locked")
		uc.metrics.RecordLogin(ctx, "locked")
		return nil, ErrAccountLocked
	}

	u, err := uc.users.GetByEmail(ctx, in.Email)
	if err != nil {
		if recordErr := uc.guard.RecordFailure(ctx, in.Email, ip); recordErr != nil {
			uc.log.Error(ctx, "guard record failure", "err", recordErr)
		}
		uc.writeLoginFailedAudit(ctx, in.Email, "login_failed", "invalid_credentials")
		uc.metrics.RecordLogin(ctx, "invalid_credentials")
		return nil, ErrInvalidCredentials
	}

	if u.Status != "active" {
		if recordErr := uc.guard.RecordFailure(ctx, in.Email, ip); recordErr != nil {
			uc.log.Error(ctx, "guard record failure", "err", recordErr)
		}
		uc.writeLoginFailedAudit(ctx, in.Email, "login_failed", "blocked")
		uc.metrics.RecordLogin(ctx, "blocked")
		return nil, ErrInvalidCredentials
	}

	ok, err := uc.hasher.Verify(in.Password, u.PasswordHash)
	if err != nil || !ok {
		if recordErr := uc.guard.RecordFailure(ctx, in.Email, ip); recordErr != nil {
			uc.log.Error(ctx, "guard record failure", "err", recordErr)
		}
		uc.writeLoginFailedAudit(ctx, in.Email, "login_failed", "invalid_credentials")
		uc.metrics.RecordLogin(ctx, "invalid_credentials")
		return nil, ErrInvalidCredentials
	}

	if recordErr := uc.guard.RecordSuccess(ctx, in.Email, ip); recordErr != nil {
		uc.log.Error(ctx, "guard record success", "err", recordErr)
	}

	access, err := uc.tokens.IssueAccess(u)
	if err != nil {
		return nil, fmt.Errorf("issue access token: %w", err)
	}
	refresh, jti, expiresAt, err := uc.tokens.IssueRefresh(u.ID)
	if err != nil {
		return nil, fmt.Errorf("issue refresh token: %w", err)
	}
	if err := uc.sessions.Insert(ctx, jti, u.ID, expiresAt); err != nil {
		return nil, fmt.Errorf("insert refresh session: %w", err)
	}

	uc.log.Info(ctx, "user logged in", "user_id", u.ID)

	if auditErr := uc.audit.Log(ctx, auditkit.Entry{
		ActorID:    u.ID,
		Action:     "login",
		Resource:   "user",
		ResourceID: u.ID,
		At:         uc.clock.Now(),
		IP:         ip,
		RequestID:  httpkit.RequestIDFrom(ctx),
	}); auditErr != nil {
		uc.log.Error(ctx, "audit login", "err", auditErr)
	}
	uc.metrics.RecordLogin(ctx, "ok")

	return &LoginOutput{AccessToken: access, RefreshToken: refresh, User: u}, nil
}

func (uc *Usecase) Refresh(ctx context.Context, refreshToken string) (out *LoginOutput, err error) {
	ctx, end := telemetry.Span(ctx, "auth.usecase.refresh")
	defer end(&err)

	userID, oldJTI, err := uc.tparser.ParseRefresh(refreshToken)
	if err != nil {
		if errors.Is(err, ErrTokenExpired) {
			uc.metrics.RecordRefresh(ctx, "expired")
			return nil, ErrRefreshExpired
		}
		uc.metrics.RecordRefresh(ctx, "invalid")
		return nil, ErrInvalidCredentials
	}

	session, err := uc.sessions.GetByJTI(ctx, oldJTI)
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			uc.metrics.RecordRefresh(ctx, "invalid")
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("get refresh session: %w", err)
	}
	if session.UserID != userID {
		uc.metrics.RecordRefresh(ctx, "invalid")
		return nil, ErrInvalidCredentials
	}
	if session.UsedAt != nil {
		uc.writeRefreshReusedAudit(ctx, userID, oldJTI)
		uc.metrics.RecordRefresh(ctx, "reused")
		return nil, ErrRefreshReused
	}
	if uc.clock.Now().After(session.ExpiresAt) {
		uc.metrics.RecordRefresh(ctx, "expired")
		return nil, ErrRefreshExpired
	}

	u, err := uc.users.GetByID(ctx, userID)
	if err != nil {
		uc.metrics.RecordRefresh(ctx, "invalid")
		return nil, ErrInvalidCredentials
	}

	access, err := uc.tokens.IssueAccess(u)
	if err != nil {
		return nil, fmt.Errorf("issue access token: %w", err)
	}
	newRefresh, newJTI, newExpiresAt, err := uc.tokens.IssueRefresh(u.ID)
	if err != nil {
		return nil, fmt.Errorf("issue refresh token: %w", err)
	}

	if err := uc.tx.WithTx(ctx, func(txCtx context.Context) error {
		if err := uc.sessions.MarkUsed(txCtx, oldJTI, newJTI); err != nil {
			return err
		}
		return uc.sessions.Insert(txCtx, newJTI, u.ID, newExpiresAt)
	}); err != nil {
		return nil, fmt.Errorf("rotate refresh session: %w", err)
	}
	uc.metrics.RecordRefresh(ctx, "ok")

	return &LoginOutput{AccessToken: access, RefreshToken: newRefresh, User: u}, nil
}

func (uc *Usecase) Logout(ctx context.Context, refreshToken string) (err error) {
	ctx, end := telemetry.Span(ctx, "auth.usecase.logout")
	defer end(&err)

	userID, jti, parseErr := uc.tparser.ParseRefresh(refreshToken)
	if parseErr != nil {
		return nil
	}

	session, err := uc.sessions.GetByJTI(ctx, jti)
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			return nil
		}
		return fmt.Errorf("get refresh session: %w", err)
	}
	if session.UserID != userID || session.UsedAt != nil {
		return nil
	}

	if err := uc.sessions.MarkUsed(ctx, jti, ""); err != nil {
		return fmt.Errorf("mark refresh session used: %w", err)
	}

	if auditErr := uc.audit.Log(ctx, auditkit.Entry{
		ActorID:    userID,
		Action:     "logout",
		Resource:   "user",
		ResourceID: userID,
		At:         uc.clock.Now(),
		IP:         identity.IPFrom(ctx),
		RequestID:  httpkit.RequestIDFrom(ctx),
	}); auditErr != nil {
		uc.log.Error(ctx, "audit logout", "err", auditErr)
	}
	uc.metrics.RecordLogout(ctx)
	return nil
}

func (uc *Usecase) ChangePassword(ctx context.Context, userID, oldPassword, newPassword string) (err error) {
	ctx, end := telemetry.Span(ctx, "auth.usecase.change_password")
	defer end(&err)

	caller, ok := identity.From(ctx)
	if !ok {
		uc.metrics.RecordPasswordChange(ctx, "forbidden")
		return &ForbiddenError{Reason: "no identity"}
	}
	if d := uc.policy.CanChangePassword(ctx, caller, userID); !d.IsAllowed {
		uc.metrics.RecordPasswordChange(ctx, "forbidden")
		return &ForbiddenError{Reason: d.Reason}
	}

	if len(newPassword) < minPasswordLen {
		uc.metrics.RecordPasswordChange(ctx, "weak")
		return ErrWeakPassword
	}

	ok, err = uc.pwVer.VerifyPassword(ctx, userID, oldPassword)
	if err != nil {
		return fmt.Errorf("verify old password: %w", err)
	}
	if !ok {
		uc.metrics.RecordPasswordChange(ctx, "invalid_old")
		return ErrInvalidCredentials
	}

	current, err := uc.users.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("load user: %w", err)
	}

	hash, err := uc.hasher.Hash(newPassword)
	if err != nil {
		return fmt.Errorf("hash new password: %w", err)
	}
	if err := uc.pwSet.UpdatePassword(ctx, userID, hash, current.Version); err != nil {
		return err
	}

	if auditErr := uc.audit.Log(ctx, auditkit.Entry{
		ActorID:    caller.UserID,
		Action:     "change_password",
		Resource:   "user",
		ResourceID: userID,
		At:         uc.clock.Now(),
		IP:         identity.IPFrom(ctx),
		RequestID:  httpkit.RequestIDFrom(ctx),
	}); auditErr != nil {
		uc.log.Error(ctx, "audit change_password", "err", auditErr)
	}
	uc.metrics.RecordPasswordChange(ctx, "ok")

	return nil
}

func (uc *Usecase) writeLoginFailedAudit(ctx context.Context, email, action, reason string) {
	entry := auditkit.Entry{
		Action:     action,
		Resource:   "user",
		ResourceID: email,
		At:         uc.clock.Now(),
		IP:         identity.IPFrom(ctx),
		RequestID:  httpkit.RequestIDFrom(ctx),
		Metadata:   map[string]any{"reason": reason},
	}
	if auditErr := uc.audit.Log(ctx, entry); auditErr != nil {
		uc.log.Error(ctx, "audit "+action, "err", auditErr)
	}
}

func (uc *Usecase) writeRefreshReusedAudit(ctx context.Context, userID, jti string) {
	entry := auditkit.Entry{
		ActorID:    userID,
		Action:     "refresh_reused",
		Resource:   "refresh_session",
		ResourceID: jti,
		At:         uc.clock.Now(),
		IP:         identity.IPFrom(ctx),
		RequestID:  httpkit.RequestIDFrom(ctx),
	}
	if auditErr := uc.audit.Log(ctx, entry); auditErr != nil {
		uc.log.Error(ctx, "audit refresh_reused", "err", auditErr)
	}
}
