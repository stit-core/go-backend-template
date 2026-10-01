package policy_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/pkg/policy"
)

func TestForbiddenError(t *testing.T) {
	t.Parallel()
	require.EqualError(t, &policy.ForbiddenError{}, "forbidden")
	require.EqualError(t, &policy.ForbiddenError{Reason: "admin only"}, "forbidden: admin only")
}

func TestPasswordStrong(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		plain string
		want  bool
	}{
		{"empty", "", false},
		{"seven", "1234567", false},
		{"eight", "12345678", true},
		{"eight bytes, four characters", "паро", false},
		{"eight characters", "пароль12", true},
		{"long", "a-much-longer-password", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, policy.PasswordStrong(tc.plain))
		})
	}
}
