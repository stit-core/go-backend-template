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

// LoginGuard: Attempt учитывает попытку до проверки пароля — неудача отдельной записи не требует;
// ключ, который guard не учитывает (пустой или длиннее предела email), — ErrInvalidCredentials.
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

// Login и Refresh пишут исход в метрику ровно один раз: его возвращают login и refresh.
func (uc *Usecase) Login(ctx context.Context, in *LoginInput) (*LoginOutput, error) {
	out, result, err := uc.login(ctx, in)
	uc.d.Metrics.RecordLogin(ctx, result)
	return out, err
}

func (uc *Usecase) login(ctx context.Context, in *LoginInput) (*LoginOutput, outcome.Result, error) {
	ip := identity.IPFrom(ctx)
	lock, err := uc.d.Guard.Attempt(ctx, in.Email, ip)
	switch {
	case errors.Is(err, ErrInvalidCredentials):
		return nil, resultInvalidLogin, err
	case err != nil:
		// Fail-closed: попытка, которую guard не учёл, до пароля не доходит — иначе сбой его хранилища
		// снимал бы защиту от перебора.
		return nil, outcome.Error, fmt.Errorf("login guard: %w", err)
	case lock != nil:
		return nil, resultLocked, &LockedError{Wait: lock.Until.Sub(uc.d.Clock.Now())}
	}

	u, err := uc.d.Users.GetByEmail(ctx, in.Email)
	switch {
	case errors.Is(err, ErrUserNotFound):
		// Холостая проверка: ответ на неизвестный email стоит столько же, сколько проверка пароля, —
		// наличие учётной записи не выдаётся таймингом.
		uc.d.Hasher.VerifyDummy(ctx)
		return nil, resultInvalidLogin, ErrInvalidCredentials
	case err != nil:
		return nil, outcome.Error, fmt.Errorf("lookup user: %w", err)
	case !u.CanLogin:
		uc.d.Hasher.VerifyDummy(ctx)
		uc.auditLoginFailed(ctx, u.ID, resultBlocked)
		return nil, resultBlocked, ErrInvalidCredentials
	}
	if ok, err := uc.d.Hasher.Verify(ctx, in.Password, u.PasswordHash); err != nil || !ok {
		uc.auditLoginFailed(ctx, u.ID, resultInvalidLogin)
		return nil, resultInvalidLogin, ErrInvalidCredentials
	}

	// Пароль уже проверен: несброшенный счётчик лишь раньше заблокирует пару, если она снова ошибётся.
	if err := uc.d.Guard.RecordSuccess(ctx, in.Email, ip); err != nil {
		uc.d.Log.Error(ctx, "guard record success", "err", err)
	}

	t, err := uc.issueTokens(ctx, u)
	if err != nil {
		return nil, outcome.Error, err
	}
	if err := uc.d.Sessions.Insert(ctx, t.jti, u.ID, t.expiresAt); err != nil {
		return nil, outcome.Error, fmt.Errorf("insert refresh session: %w", err)
	}
	uc.d.Log.Info(ctx, "user logged in", "user_id", u.ID)
	uc.audit(ctx, auditkit.Entry{ActorID: u.ID, Action: "login", Resource: "user", ResourceID: u.ID})
	return &t.out, outcome.OK, nil
}

// auditLoginFailed — отказ найденной учётной записи: в аудите её ID, адреса нет.
func (uc *Usecase) auditLoginFailed(ctx context.Context, userID string, reason outcome.Result) {
	uc.audit(ctx, auditkit.Entry{
		Action: "login_failed", Resource: "user", ResourceID: userID, Metadata: map[string]any{"reason": reason.String()},
	})
}

// issued — токены сессии и строка refresh, которую пишет вызывающий: вход — сразу, Refresh — в
// транзакции ротации.
type issued struct {
	out       LoginOutput
	jti       string
	expiresAt time.Time
}

func (uc *Usecase) issueTokens(ctx context.Context, u *User) (issued, error) {
	access, err := uc.d.Tokens.IssueAccess(ctx, u)
	if err != nil {
		return issued{}, fmt.Errorf("issue access token: %w", err)
	}
	refresh, jti, expiresAt, err := uc.d.Tokens.IssueRefresh(ctx, u.ID)
	if err != nil {
		return issued{}, fmt.Errorf("issue refresh token: %w", err)
	}
	return issued{out: LoginOutput{AccessToken: access, RefreshToken: refresh, User: u}, jti: jti, expiresAt: expiresAt}, nil
}

func (uc *Usecase) Refresh(ctx context.Context, refreshToken string) (*LoginOutput, error) {
	out, result, err := uc.refresh(ctx, refreshToken)
	uc.d.Metrics.RecordRefresh(ctx, result)
	return out, err
}

func (uc *Usecase) refresh(ctx context.Context, refreshToken string) (*LoginOutput, outcome.Result, error) {
	userID, oldJTI, err := uc.d.TokenParser.ParseRefresh(ctx, refreshToken)
	switch {
	case errors.Is(err, ErrTokenExpired):
		return nil, outcome.Expired, ErrRefreshExpired
	case err != nil:
		return nil, outcome.Invalid, ErrRefreshInvalid
	}

	stored, err := uc.d.Sessions.GetByJTI(ctx, oldJTI)
	switch {
	case errors.Is(err, ErrSessionNotFound):
		return nil, outcome.Invalid, ErrRefreshInvalid
	case err != nil:
		return nil, outcome.Error, fmt.Errorf("get refresh session: %w", err)
	case stored.UserID != userID:
		return nil, outcome.Invalid, ErrRefreshInvalid
	case stored.UsedAt != nil:
		uc.audit(ctx, auditkit.Entry{ActorID: userID, Action: "refresh_reused", Resource: "refresh_session", ResourceID: oldJTI})
		return nil, resultReused, ErrRefreshReused
	case uc.d.Clock.Now().After(stored.ExpiresAt):
		return nil, outcome.Expired, ErrRefreshExpired
	}

	u, err := uc.d.Users.GetByID(ctx, userID)
	switch {
	case errors.Is(err, ErrUserNotFound):
		return nil, outcome.Invalid, ErrRefreshInvalid
	case err != nil:
		return nil, outcome.Error, fmt.Errorf("lookup user: %w", err)
	}

	t, err := uc.issueTokens(ctx, u)
	if err != nil {
		return nil, outcome.Error, err
	}
	if err := uc.d.Tx.WithTx(ctx, func(txCtx context.Context) error {
		if err := uc.d.Sessions.MarkUsed(txCtx, oldJTI, t.jti); err != nil {
			return err
		}
		return uc.d.Sessions.Insert(txCtx, t.jti, u.ID, t.expiresAt)
	}); err != nil {
		return nil, outcome.Error, fmt.Errorf("rotate refresh session: %w", err)
	}
	return &t.out, outcome.OK, nil
}

func (uc *Usecase) Logout(ctx context.Context, refreshToken string) error {
	userID, jti, parseErr := uc.d.TokenParser.ParseRefresh(ctx, refreshToken)
	if parseErr != nil {
		return nil //nolint:nilerr // logout is idempotent: a broken/expired token is already logged out
	}

	stored, err := uc.d.Sessions.GetByJTI(ctx, jti)
	switch {
	case errors.Is(err, ErrSessionNotFound):
		return nil
	case err != nil:
		return fmt.Errorf("get refresh session: %w", err)
	case stored.UserID != userID || stored.UsedAt != nil:
		return nil
	}

	if err := uc.d.Sessions.MarkUsed(ctx, jti, ""); err != nil {
		return fmt.Errorf("mark refresh session used: %w", err)
	}
	uc.audit(ctx, auditkit.Entry{ActorID: userID, Action: "logout", Resource: "user", ResourceID: userID})
	return nil
}

func (uc *Usecase) ChangePassword(ctx context.Context, userID, oldPassword, newPassword string) error {
	caller, ok := identity.From(ctx)
	if !ok {
		return &policy.ForbiddenError{Reason: "no identity"}
	}
	if d := uc.d.Policy.CanChangePassword(ctx, caller, userID); !d.IsAllowed {
		return &policy.ForbiddenError{Reason: d.Reason}
	}
	if !policy.PasswordStrong(newPassword) {
		return ErrWeakPassword
	}

	current, err := uc.d.Users.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("load user: %w", err)
	}
	// Старый пароль угадывают так же, как при входе: попытки идут в счётчик входа учётной записи.
	ip := identity.IPFrom(ctx)
	lock, err := uc.d.Guard.Attempt(ctx, current.Email, ip)
	switch {
	case err != nil:
		return fmt.Errorf("password guard: %w", err)
	case lock != nil:
		return &LockedError{Wait: lock.Until.Sub(uc.d.Clock.Now())}
	}
	verified, err := uc.d.PasswordVerify.VerifyPassword(ctx, userID, oldPassword)
	if err != nil {
		return fmt.Errorf("verify old password: %w", err)
	}
	if !verified {
		return ErrWrongOldPassword
	}
	if err := uc.d.Guard.RecordSuccess(ctx, current.Email, ip); err != nil {
		uc.d.Log.Error(ctx, "guard record success", "err", err)
	}

	hash, err := uc.d.Hasher.Hash(ctx, newPassword)
	if err != nil {
		return fmt.Errorf("hash new password: %w", err)
	}
	if err := uc.d.PasswordSet.UpdatePassword(ctx, userID, hash, current.Version); err != nil {
		return err
	}
	uc.audit(ctx, auditkit.Entry{ActorID: caller.UserID, Action: "change_password", Resource: "user", ResourceID: userID})
	return nil
}

// audit дописывает время, IP и запрос; сбой записи аудита операцию не отменяет — он уходит в лог.
func (uc *Usecase) audit(ctx context.Context, e auditkit.Entry) {
	e.At, e.IP, e.RequestID = uc.d.Clock.Now(), identity.IPFrom(ctx), identity.RequestIDFrom(ctx)
	if err := uc.d.Audit.Log(ctx, e); err != nil {
		uc.d.Log.Error(ctx, "audit "+e.Action, "err", err)
	}
}
