package logmail_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/verification/infra/logmail"
	appconfig "git.stit.tech/stit-core/go-backend-template/internal/platform/config"
	"git.stit.tech/stit-core/golibinfra/logkit"
	"git.stit.tech/stit-core/golibinfra/testkit/logtest"
)

func TestNew_DevOnly(t *testing.T) {
	t.Parallel()
	for _, env := range []appconfig.Env{"prod", "staging", ""} {
		_, err := logmail.New(env, logkit.Nop())
		require.ErrorIs(t, err, logmail.ErrOutsideDev, "APP_ENV=%q", env)
	}
}

// Письмо не уходит, а в лог попадает только факт: ни адреса, ни токена.
func TestMailer_LogsNoAddressOrToken(t *testing.T) {
	t.Parallel()
	const email, token = "a@b.com", "0123456789abcdef"
	log := logtest.Capture()
	m, err := logmail.New(appconfig.Dev, log)
	require.NoError(t, err)

	require.NoError(t, m.SendPasswordReset(context.Background(), email, token))
	require.NoError(t, m.SendEmailVerification(context.Background(), email, token))

	entries := log.Entries()
	require.Len(t, entries, 2)
	require.Equal(t, "password_reset", entries[0].Str("mail"))
	require.Equal(t, "email_verification", entries[1].Str("mail"))
	for _, e := range entries {
		record := fmt.Sprint(e.Msg, e.KV)
		require.NotContains(t, record, email)
		require.NotContains(t, record, token)
	}
}
