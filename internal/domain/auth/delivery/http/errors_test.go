// White-box (package http): таблицы ошибок неэкспортируемые.
package http

import (
	"testing"

	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/auth"
	"git.stit.tech/stit-core/go-backend-template/pkg/policy"
	"git.stit.tech/stit-core/golibinfra/httpkit"
)

// OpenAPI операции объявляет ошибки, которые отдаёт её usecase (список — по коду usecase). Что ни
// одна ошибка домена не уходит 500, держит таблица authErrors и archtest TestDomainErrorTables.
func TestErrorMaps(t *testing.T) {
	t.Parallel()
	tests := []struct {
		op   string
		m    httpkit.ErrorMap
		errs []error
	}{
		{"loginUser", loginErrors, []error{auth.ErrInvalidCredentials, &auth.LockedError{}}},
		{"refreshToken", refreshErrors, []error{auth.ErrRefreshExpired, auth.ErrRefreshReused, auth.ErrRefreshInvalid}},
		{"changePassword", changePasswordErrors, []error{
			auth.ErrWeakPassword, auth.ErrWrongOldPassword, &auth.LockedError{}, auth.ErrUserNotFound, auth.ErrConcurrentUpdate,
			&policy.ForbiddenError{Reason: "test"},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.op, func(t *testing.T) {
			t.Parallel()
			require.Empty(t, tc.m.Unmapped(tc.errs...))
		})
	}
}
