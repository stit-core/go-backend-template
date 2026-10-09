package bruteforce_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/verification/infra/bruteforce"
	"git.stit.tech/stit-core/golibinfra/bruteforcekit"
	"git.stit.tech/stit-core/golibinfra/clockkit"
	"git.stit.tech/stit-core/golibinfra/testkit/bruteforcetest"
)

const ip = "203.0.113.7"

// Запросы сброса одного пользователя с одного адреса идут подряд до лимита, дальше — отказ; у соседа
// счётчик свой.
func TestResetLimiter_Allow(t *testing.T) {
	t.Parallel()
	guard, err := bruteforcetest.NewCounting(bruteforcekit.Config{
		MaxFailures: 2, LockDuration: time.Minute, SubjectMaxFailures: bruteforcekit.SubjectBudgetOff, SubjectLockDuration: time.Minute,
		CleanupRetention: time.Minute, CleanupInterval: time.Minute, IPv6Prefix: bruteforcekit.FullIPv6Prefix,
	}, clockkit.NewFixed(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)))
	require.NoError(t, err)
	l := bruteforce.NewResetLimiter(guard)
	ctx := context.Background()

	for i, want := range []bool{true, true, false} {
		got, err := l.Allow(ctx, "u1", ip)
		require.NoError(t, err)
		require.Equal(t, want, got, "request %d", i+1)
	}
	other, err := l.Allow(ctx, "u2", ip)
	require.NoError(t, err)
	require.True(t, other, "another user has its own counter")
}

// Ключ сброса — в своём пространстве: счётчик входа по email он не трогает.
func TestResetLimiter_Subject(t *testing.T) {
	t.Parallel()
	rec := &bruteforcetest.Recorder{}

	allowed, err := bruteforce.NewResetLimiter(rec).Allow(context.Background(), "u1", ip)

	require.NoError(t, err)
	require.True(t, allowed)
	require.Equal(t, []bruteforcetest.Attempt{{Subject: "password_reset:u1", IP: ip}}, rec.Attempts())
}

func TestResetLimiter_GuardError(t *testing.T) {
	t.Parallel()

	allowed, err := bruteforce.NewResetLimiter(bruteforcetest.Permissive{}).Allow(context.Background(), "u1", "")

	require.ErrorIs(t, err, bruteforcekit.ErrInvalidKey)
	require.False(t, allowed)
}
