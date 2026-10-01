// Package logmail — verification.Mailer для dev: письмо не уходит, в лог — только факт; адрес и токен
// несёт лишь настоящий канал доставки.
package logmail

import (
	"context"
	"errors"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/verification"
	appconfig "git.stit.tech/stit-core/go-backend-template/internal/platform/config"
	"git.stit.tech/stit-core/golibinfra/logkit"
)

// ErrOutsideDev — вне dev письма обязаны уходить: сервису нужен настоящий verification.Mailer.
var ErrOutsideDev = errors.New("logmail: the log mailer runs in APP_ENV=dev only: wire a real verification.Mailer")

type mailer struct {
	log logkit.Logger
}

func New(env appconfig.Env, log logkit.Logger) (verification.Mailer, error) {
	if env != appconfig.Dev {
		return nil, ErrOutsideDev
	}
	return &mailer{log: log.With("component", "logmail")}, nil
}

func (m *mailer) SendPasswordReset(ctx context.Context, _, _ string) error {
	m.log.Info(ctx, "mail not sent: dev log mailer", "mail", "password_reset")
	return nil
}

func (m *mailer) SendEmailVerification(ctx context.Context, _, _ string) error {
	m.log.Info(ctx, "mail not sent: dev log mailer", "mail", "email_verification")
	return nil
}
