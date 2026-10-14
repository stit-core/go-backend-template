package bruteforce_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/verification"
	"git.stit.tech/stit-core/go-backend-template/internal/domain/verification/infra/bruteforce"
	"git.stit.tech/stit-core/golibinfra/bruteforcekit"
	"git.stit.tech/stit-core/golibinfra/clockkit"
	"git.stit.tech/stit-core/golibinfra/testkit/bruteforcetest"
)

const ip = "203.0.113.7"

// twoRequests — лимитер на настоящем счётчике guard'а: два запроса пары (субъект, IP), третий — отказ.
func twoRequests(t *testing.T) verification.ResetLimiter {
	t.Helper()
	guard, err := bruteforcetest.NewCounting(bruteforcekit.Config{
		MaxFailures: 2, LockDuration: time.Minute, SubjectMaxFailures: bruteforcekit.SubjectBudgetOff, SubjectLockDuration: time.Minute,
		CleanupRetention: time.Minute, CleanupInterval: time.Minute, IPv6Prefix: bruteforcekit.FullIPv6Prefix,
	}, clockkit.NewFixed(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)))
	require.NoError(t, err)
	return bruteforce.NewResetLimiter(guard)
}

// Запросы сброса одного пользователя с одного адреса идут подряд до лимита, дальше — отказ; у соседа
// счётчик свой.
func TestResetLimiter_Allow(t *testing.T) {
	t.Parallel()
	l := twoRequests(t)
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

// Адрес без учётной записи — субъект лимита: его формы с пробелами по краям и в другом регистре делят один
// счётчик, как формы адреса учётной записи — её счётчик. Иначе по работе запроса видно, есть ли учётная запись.
func TestResetLimiter_AddressForms(t *testing.T) {
	t.Parallel()
	l := twoRequests(t)
	ctx := context.Background()

	for _, form := range []string{" a@b.com", "\tA@B.com "} {
		allowed, err := l.Allow(ctx, form, ip)
		require.NoError(t, err)
		require.True(t, allowed, "%q", form)
	}
	allowed, err := l.Allow(ctx, "a@b.com", ip)
	require.NoError(t, err)
	require.False(t, allowed, "the forms of the address spent its requests")
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
