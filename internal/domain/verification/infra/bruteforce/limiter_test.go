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

// twoRequests — лимитер на настоящем счётчике guard'а: два запроса пары (адрес, IP), третий — отказ.
func twoRequests(t *testing.T) verification.ResetLimiter {
	t.Helper()
	guard, err := bruteforcetest.NewCounting(bruteforcekit.Config{
		MaxFailures: 2, LockDuration: time.Minute, SubjectMaxFailures: bruteforcekit.SubjectBudgetOff, SubjectLockDuration: time.Minute,
		CleanupRetention: time.Minute, CleanupInterval: time.Minute, IPv6Prefix: bruteforcekit.FullIPv6Prefix,
	}, clockkit.NewFixed(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)))
	require.NoError(t, err)
	return bruteforce.NewResetLimiter(guard)
}

// Запросы сброса на один адрес с одного IP идут подряд до лимита, дальше — отказ; у соседнего адреса
// счётчик свой.
func TestResetLimiter_Allow(t *testing.T) {
	t.Parallel()
	l := twoRequests(t)
	ctx := context.Background()

	for i, want := range []bool{true, true, false} {
		got, err := l.Allow(ctx, "a@b.com", ip)
		require.NoError(t, err)
		require.Equal(t, want, got, "request %d", i+1)
	}
	other, err := l.Allow(ctx, "c@d.com", ip)
	require.NoError(t, err)
	require.True(t, other, "another address has its own counter")
}

// Формы адреса — пробелы по краям и регистр — делят его счётчик: иначе лимит обходили бы формой адреса.
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

	allowed, err := bruteforce.NewResetLimiter(rec).Allow(context.Background(), "a@b.com", ip)

	require.NoError(t, err)
	require.True(t, allowed)
	require.Equal(t, []bruteforcetest.Attempt{{Subject: "password_reset:a@b.com", IP: ip}}, rec.Attempts())
}

func TestResetLimiter_GuardError(t *testing.T) {
	t.Parallel()

	allowed, err := bruteforce.NewResetLimiter(bruteforcetest.Permissive{}).Allow(context.Background(), "a@b.com", "")

	require.ErrorIs(t, err, bruteforcekit.ErrInvalidKey)
	require.False(t, allowed)
}
