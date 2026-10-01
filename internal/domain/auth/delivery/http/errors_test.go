// White-box (package http): таблицы ошибок операций неэкспортируемые.
package http

import (
	"testing"

	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/auth"
	"git.stit.tech/stit-core/go-backend-template/pkg/policy"
	"git.stit.tech/stit-core/golibinfra/httpkit"
)

// Ошибки портов auth, которые usecase переводит сам: до HTTP не доходят (archtest сверяет со
// списком errors.go вместе с операциями ниже).
var internalErrors = []error{auth.ErrTokenExpired, auth.ErrSessionNotFound}

// Каждая ошибка, которую отдаёт usecase операции, — в таблице операции: свой code, а не 500.
func TestErrorMaps(t *testing.T) {
	t.Parallel()
	tests := []struct {
		op   string
		m    httpkit.ErrorMap
		errs []error
	}{
		{"loginUser", loginErrors, []error{auth.ErrInvalidCredentials, &auth.LockedError{}}},
		{"refreshToken", refreshErrors, []error{auth.ErrRefreshExpired, auth.ErrRefreshReused, auth.ErrInvalidCredentials}},
		{"changePassword", changePasswordErrors, []error{
			auth.ErrWeakPassword, auth.ErrInvalidCredentials, auth.ErrUserNotFound, auth.ErrConcurrentUpdate,
			&policy.ForbiddenError{Reason: "test"},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.op, func(t *testing.T) {
			t.Parallel()
			require.Empty(t, tc.m.Unmapped(tc.errs...))
			require.Equal(t, internalErrors, tc.m.Unmapped(internalErrors...), "an internal error is not part of the HTTP contract")
		})
	}
}
