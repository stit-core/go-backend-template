package auth

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/fx"

	"git.stit.tech/stit-core/go-backend-template/internal/platform/identity"
	"git.stit.tech/stit-core/go-backend-template/pkg/outcome"
	"git.stit.tech/stit-core/go-backend-template/pkg/policy"
	"git.stit.tech/stit-core/golibinfra/auditkit"
	"git.stit.tech/stit-core/golibinfra/authnkit"
	"git.stit.tech/stit-core/golibinfra/authnkit/session"
	"git.stit.tech/stit-core/golibinfra/clockkit"
	"git.stit.tech/stit-core/golibinfra/logkit"
	"git.stit.tech/stit-core/golibinfra/pgkit"
)

type UserReader interface {
	GetByID(ctx context.Context, id string) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	// LockByID — GetByID под блокировкой строки до конца транзакции из ctx: смена пароля, роли или статуса
	// пишет эту строку до закрытия сессий и ждёт транзакцию читателя, а идущую он дожидается сам.
	LockByID(ctx context.Context, id string) (*User, error)
}

// PasswordSetter: HashPassword — хэш нового пароля по правилу продукта (ErrWeakPassword, ErrPasswordTooLong),
// UpdatePassword пишет его при версии учётной записи и закрывает все её сессии.
type PasswordSetter interface {
	HashPassword(ctx context.Context, plain string) (string, error)
	UpdatePassword(ctx context.Context, userID, hash string, version int) error
}

type Tokens interface {
	IssueAccess(ctx context.Context, u *User) (string, error)
}

// LoginGuard: Attempt учитывает попытку до проверки пароля — неудача отдельной записи не требует;
// ключ, который guard не учитывает (пустой, с NUL или не в UTF-8), — ErrInvalidCredentials.
type LoginGuard interface {
	Attempt(ctx context.Context, subject, ip string) (*LockState, error)
	RecordSuccess(ctx context.Context, subject, ip string) error
}

type UsecaseDeps struct {
	fx.In

	Users       UserReader
	PasswordSet PasswordSetter
	Hasher      authnkit.Hasher
	Tokens      Tokens
	Sessions    session.Store
	Tx          pgkit.Transactor
	Guard       LoginGuard
	Audit       auditkit.Logger
	Log         logkit.Logger
	Clock       clockkit.Clock
	Metrics     *Metrics
}

type Usecase struct {
	d UsecaseDeps
}

func NewUsecase(d UsecaseDeps) *Usecase {
	d.Log = d.Log.With("domain", "auth")
	return &Usecase{d: d}
}

// Login и Refresh пишут исход в метрику ровно один раз: его возвращают login и refresh.
func (uc *Usecase) Login(ctx context.Context, in LoginInput) (*Session, error) {
	out, result, err := uc.login(ctx, in)
	uc.d.Metrics.RecordLogin(ctx, result)
	return out, err
}

func (uc *Usecase) login(ctx context.Context, in LoginInput) (*Session, outcome.Result, error) {
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
		return nil, resultLocked, &LockedError{Wait: lock.RetryAfter}
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
	ok, err := uc.passwordMatches(ctx, in.Password, u.PasswordHash)
	switch {
	case err != nil:
		return nil, outcome.Error, err
	case !ok:
		uc.auditLoginFailed(ctx, u.ID, resultInvalidLogin)
		return nil, resultInvalidLogin, ErrInvalidCredentials
	}

	// Пароль уже проверен: несброшенный счётчик лишь раньше заблокирует пару, если она снова ошибётся.
	if err := uc.d.Guard.RecordSuccess(ctx, in.Email, ip); err != nil {
		uc.d.Log.Error(ctx, "guard record success", "error", err)
	}

	access, err := uc.d.Tokens.IssueAccess(ctx, u)
	if err != nil {
		return nil, outcome.Error, fmt.Errorf("issue access token: %w", err)
	}
	refresh, err := uc.openSession(ctx, u)
	switch {
	case errors.Is(err, ErrInvalidCredentials):
		uc.auditLoginFailed(ctx, u.ID, resultInvalidLogin)
		return nil, resultInvalidLogin, err
	case err != nil:
		return nil, outcome.Error, err
	}
	uc.d.Log.Info(ctx, "user logged in", "user_id", u.ID)
	uc.audit(ctx, auditkit.Entry{ActorID: u.ID, Action: "login", Resource: "user", ResourceID: u.ID})
	return &Session{AccessToken: access, RefreshToken: refresh.Value, User: u}, outcome.OK, nil
}

// openSession открывает сессию под строкой пользователя, которую держит её транзакция: смена пароля, роли
// или статуса, успевшая между проверкой пароля и открытием, отказывает во входе (ErrInvalidCredentials),
// а начатая позже ждёт коммита и закрывает сессию своим RevokeAll.
func (uc *Usecase) openSession(ctx context.Context, checked *User) (session.Token, error) {
	var refresh session.Token
	err := uc.d.Tx.WithTx(ctx, func(txCtx context.Context) error {
		current, err := uc.d.Users.LockByID(txCtx, checked.ID)
		switch {
		case errors.Is(err, ErrUserNotFound):
			return ErrInvalidCredentials
		case err != nil:
			return fmt.Errorf("lock user: %w", err)
		case !current.CanLogin || current.PasswordHash != checked.PasswordHash || current.Role != checked.Role:
			return ErrInvalidCredentials
		}
		if refresh, err = uc.d.Sessions.Issue(txCtx, checked.ID); err != nil {
			return fmt.Errorf("open session: %w", err)
		}
		return nil
	})
	return refresh, err
}

// auditLoginFailed — отказ найденной учётной записи: в аудите её ID, адреса нет.
func (uc *Usecase) auditLoginFailed(ctx context.Context, userID string, reason outcome.Result) {
	uc.audit(ctx, auditkit.Entry{
		Action: "login_failed", Resource: "user", ResourceID: userID, Metadata: map[string]any{"reason": reason.String()},
	})
}

func (uc *Usecase) Refresh(ctx context.Context, refreshToken string) (*Session, error) {
	out, result, err := uc.refresh(ctx, refreshToken)
	uc.d.Metrics.RecordRefresh(ctx, result)
	return out, err
}

// refresh: чья сессия, известно только после Rotate — пускают ли пользователя, проверяется после него.
func (uc *Usecase) refresh(ctx context.Context, refreshToken string) (*Session, outcome.Result, error) {
	next, err := uc.d.Sessions.Rotate(ctx, refreshToken)
	var reused *session.ReusedError
	switch {
	case errors.As(err, &reused):
		uc.audit(ctx, auditkit.Entry{ActorID: reused.Subject, Action: "refresh_reused", Resource: "session", ResourceID: reused.SessionID})
		return nil, resultReused, ErrRefreshReused
	case errors.Is(err, session.ErrExpired):
		return nil, outcome.Expired, ErrRefreshExpired
	case errors.Is(err, session.ErrInvalid):
		return nil, outcome.Invalid, ErrRefreshInvalid
	case err != nil:
		return nil, outcome.Error, fmt.Errorf("rotate session: %w", err)
	}

	u, err := uc.d.Users.GetByID(ctx, next.Subject)
	switch {
	case err != nil && !errors.Is(err, ErrUserNotFound):
		return nil, outcome.Error, fmt.Errorf("lookup user: %w", err)
	case err != nil || !u.CanLogin:
		// Удалённый или заблокированный сессию не продлевает: токен Rotate клиенту не уходит, сессия закрыта.
		if _, err := uc.d.Sessions.Revoke(ctx, next.Value); err != nil {
			uc.d.Log.Error(ctx, "revoke session", "user_id", next.Subject, "error", err)
		}
		return nil, outcome.Invalid, ErrRefreshInvalid
	}
	access, err := uc.d.Tokens.IssueAccess(ctx, u)
	if err != nil {
		return nil, outcome.Error, fmt.Errorf("issue access token: %w", err)
	}
	return &Session{AccessToken: access, RefreshToken: next.Value, User: u}, outcome.OK, nil
}

// Logout: неизвестный токен и закрытая раньше сессия — тоже выход; аудит — у сессии, которую закрыл
// этот вызов: её владельца отдаёт хранилище.
func (uc *Usecase) Logout(ctx context.Context, refreshToken string) error {
	subject, err := uc.d.Sessions.Revoke(ctx, refreshToken)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	if subject != "" {
		uc.d.Log.Info(ctx, "user logged out", "user_id", subject)
		uc.audit(ctx, auditkit.Entry{ActorID: subject, Action: "logout", Resource: "user", ResourceID: subject})
	}
	return nil
}

// ChangePassword меняет пароль вызывающего: старый пароль угадывают так же, как при входе, — попытки идут в счётчик
// входа учётной записи; все её сессии закрывает запись пароля (PasswordSetter.UpdatePassword).
func (uc *Usecase) ChangePassword(ctx context.Context, oldPassword, newPassword string) error {
	caller, ok := identity.From(ctx)
	if !ok {
		return &policy.ForbiddenError{Reason: "no identity"}
	}
	current, err := uc.d.Users.GetByID(ctx, caller.UserID)
	if err != nil {
		return fmt.Errorf("load user: %w", err)
	}
	ip := identity.IPFrom(ctx)
	lock, err := uc.d.Guard.Attempt(ctx, current.Email, ip)
	switch {
	case err != nil:
		return fmt.Errorf("password guard: %w", err)
	case lock != nil:
		return &LockedError{Wait: lock.RetryAfter}
	}
	matches, err := uc.passwordMatches(ctx, oldPassword, current.PasswordHash)
	switch {
	case err != nil:
		return err
	case !matches:
		return ErrWrongOldPassword
	}
	if err := uc.d.Guard.RecordSuccess(ctx, current.Email, ip); err != nil {
		uc.d.Log.Error(ctx, "guard record success", "error", err)
	}

	hash, err := uc.d.PasswordSet.HashPassword(ctx, newPassword)
	if err != nil {
		return err
	}
	if err := uc.d.PasswordSet.UpdatePassword(ctx, current.ID, hash, current.Version); err != nil {
		return err
	}
	uc.d.Log.Info(ctx, "password changed", "user_id", current.ID)
	uc.audit(ctx, auditkit.Entry{ActorID: current.ID, Action: "change_password", Resource: "user", ResourceID: current.ID})
	return nil
}

// passwordMatches: пароль длиннее предела хэшера ни с одним хэшем не совпадает — неверный (холостой хэш Verify
// посчитал сам), прочий сбой хэшера — ошибка, а не неверный пароль.
func (uc *Usecase) passwordMatches(ctx context.Context, plain, hash string) (bool, error) {
	ok, err := uc.d.Hasher.Verify(ctx, plain, hash)
	if err != nil && !errors.Is(err, authnkit.ErrPasswordTooLong) {
		return false, fmt.Errorf("verify password: %w", err)
	}
	return ok && err == nil, nil
}

// audit дописывает время (IP клиента и ID запроса auditkit берёт из ctx); сбой записи аудита операцию не
// отменяет — он уходит в лог.
func (uc *Usecase) audit(ctx context.Context, e auditkit.Entry) {
	e.At = uc.d.Clock.Now()
	if err := uc.d.Audit.Log(ctx, e); err != nil {
		uc.d.Log.Error(ctx, "audit "+e.Action, "error", err)
	}
}
