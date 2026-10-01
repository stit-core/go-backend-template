package module

import (
	"context"
	"errors"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/user"
	"git.stit.tech/stit-core/go-backend-template/internal/domain/verification"
	"git.stit.tech/stit-core/golibinfra/auditkit"
	"git.stit.tech/stit-core/golibinfra/clockkit"
	"git.stit.tech/stit-core/golibinfra/logkit"
)

// userAdapter — порты verification к домену user (UserReader, PasswordSetter, EmailVerificationSetter).
type userAdapter struct {
	users *user.Trusted
}

func newUserAdapter(u *user.Trusted) *userAdapter { return &userAdapter{users: u} }

func (a *userAdapter) GetByID(ctx context.Context, id string) (*verification.User, error) {
	u, err := a.users.LookupByID(ctx, id)
	if err != nil {
		return nil, mapUserErr(err)
	}
	return userToVerification(u), nil
}

func (a *userAdapter) GetByEmail(ctx context.Context, email string) (*verification.User, error) {
	u, err := a.users.LookupByEmail(ctx, email)
	if err != nil {
		return nil, mapUserErr(err)
	}
	return userToVerification(u), nil
}

func (a *userAdapter) UpdatePassword(ctx context.Context, userID, hash string, version int) error {
	return mapUserErr(a.users.UpdatePassword(ctx, userID, hash, version))
}

func (a *userAdapter) MarkEmailVerified(ctx context.Context, userID string) error {
	return mapUserErr(a.users.MarkEmailVerified(ctx, userID))
}

// mapUserErr переводит каждую ошибку user.Trusted в ошибку verification: sentinel'ов user потребитель
// не видит.
func mapUserErr(err error) error {
	switch {
	case errors.Is(err, user.ErrNotFound):
		return verification.ErrUserNotFound
	case errors.Is(err, user.ErrConcurrentUpdate):
		return verification.ErrConcurrentUpdate
	}
	return err
}

func userToVerification(u *user.User) *verification.User {
	return &verification.User{
		ID:       u.ID,
		Email:    u.Email,
		CanLogin: u.CanLogin(),
		Version:  u.Version,
	}
}

type logMailerAdapter struct {
	log   logkit.Logger
	audit auditkit.Logger
	clock clockkit.Clock
}

func newLogMailerAdapter(log logkit.Logger, auditLog auditkit.Logger, clock clockkit.Clock) verification.Mailer {
	return &logMailerAdapter{log: log.With("component", "mailer"), audit: auditLog, clock: clock}
}

func (m *logMailerAdapter) SendPasswordReset(ctx context.Context, email, token string) error {
	m.log.Info(ctx, "would send password reset link", "email", email, "token", token)
	return m.audit.Log(ctx, auditkit.Entry{
		Action:   "mail_sent",
		Resource: "password_reset",
		At:       m.clock.Now(),
		Metadata: map[string]any{"email": email},
	})
}

func (m *logMailerAdapter) SendEmailVerification(ctx context.Context, email, token string) error {
	m.log.Info(ctx, "would send email verification link", "email", email, "token", token)
	return m.audit.Log(ctx, auditkit.Entry{
		Action:   "mail_sent",
		Resource: "email_verification",
		At:       m.clock.Now(),
		Metadata: map[string]any{"email": email},
	})
}
