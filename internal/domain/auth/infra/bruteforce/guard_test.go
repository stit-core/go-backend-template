package bruteforce_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/auth"
	authbruteforce "git.stit.tech/stit-core/go-backend-template/internal/domain/auth/infra/bruteforce"
	"git.stit.tech/stit-core/golibinfra/testkit/bruteforcetest"
)

func TestLoginGuard_Attempt(t *testing.T) {
	t.Parallel()
	until := time.Date(2026, 1, 1, 12, 15, 0, 0, time.UTC)

	lock, err := authbruteforce.NewLoginGuard(bruteforcetest.Permissive{}).Attempt(context.Background(), "a@b.com", "10.0.0.1")
	require.NoError(t, err)
	require.Nil(t, lock, "an allowed attempt carries no lock")

	lock, err = authbruteforce.NewLoginGuard(bruteforcetest.Locked{Until: until}).Attempt(context.Background(), "a@b.com", "10.0.0.1")
	require.NoError(t, err)
	require.Equal(t, &auth.LockState{Until: until}, lock)

	_, err = authbruteforce.NewLoginGuard(bruteforcetest.Permissive{}).Attempt(context.Background(), "a@b.com", "")
	require.Error(t, err, "the guard's error reaches the usecase")
}

func TestLoginGuard_RecordSuccess(t *testing.T) {
	t.Parallel()
	rec := &bruteforcetest.Recorder{}
	require.NoError(t, authbruteforce.NewLoginGuard(rec).RecordSuccess(context.Background(), "a@b.com", "10.0.0.1"))
	require.Equal(t, []bruteforcetest.Attempt{{Subject: "a@b.com", IP: "10.0.0.1"}}, rec.Successes())
}
