package bruteforce_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/auth"
	authbruteforce "git.stit.tech/stit-core/go-backend-template/internal/domain/auth/infra/bruteforce"
	"git.stit.tech/stit-core/golibinfra/bruteforcekit"
	"git.stit.tech/stit-core/golibinfra/pgkit"
	"git.stit.tech/stit-core/golibinfra/testkit/bruteforcetest"
)

func TestLoginGuard_Attempt(t *testing.T) {
	t.Parallel()
	const wait = 15 * time.Minute

	rec := &bruteforcetest.Recorder{}
	lock, err := authbruteforce.NewLoginGuard(rec).Attempt(context.Background(), "a@b.com", "10.0.0.1")
	require.NoError(t, err)
	require.Nil(t, lock, "an allowed attempt carries no lock")
	require.Equal(t, []bruteforcetest.Attempt{{Subject: "a@b.com", IP: "10.0.0.1"}}, rec.Attempts(), "the attempt is counted with the key of the client")

	lock, err = authbruteforce.NewLoginGuard(bruteforcetest.Locked{Until: time.Now().Add(wait)}).Attempt(context.Background(), "a@b.com", "10.0.0.1")
	require.NoError(t, err)
	require.NotNil(t, lock)
	require.InDelta(t, wait, lock.RetryAfter, float64(time.Minute), "the wait is the one the guard reports")

	_, err = authbruteforce.NewLoginGuard(bruteforcetest.Permissive{}).Attempt(pgkit.WithTestTx(context.Background()), "a@b.com", "10.0.0.1")
	require.ErrorIs(t, err, bruteforcekit.ErrInTx, "the guard's failure reaches the usecase as is")
	require.NotErrorIs(t, err, auth.ErrInvalidCredentials)
}

// Ключ, который guard не учитывает, — не вход: неверные учётные данные, а не сбой.
func TestLoginGuard_UnkeyableAttempt(t *testing.T) {
	t.Parallel()
	for name, subject := range map[string]string{"empty email": " ", "email with NUL": "a\x00@b.com"} {
		_, err := authbruteforce.NewLoginGuard(bruteforcetest.Permissive{}).Attempt(context.Background(), subject, "10.0.0.1")
		require.ErrorIs(t, err, auth.ErrInvalidCredentials, name)
		require.ErrorIs(t, err, bruteforcekit.ErrInvalidKey, name)
	}
}

func TestLoginGuard_RecordSuccess(t *testing.T) {
	t.Parallel()
	rec := &bruteforcetest.Recorder{}
	require.NoError(t, authbruteforce.NewLoginGuard(rec).RecordSuccess(context.Background(), "a@b.com", "10.0.0.1"))
	require.Equal(t, []bruteforcetest.Attempt{{Subject: "a@b.com", IP: "10.0.0.1"}}, rec.Successes())
}
