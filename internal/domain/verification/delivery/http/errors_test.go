// White-box (package http): таблицы ошибок неэкспортируемые.
package http

import (
	"testing"

	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/verification"
	"git.stit.tech/stit-core/golibinfra/httpkit"
)

// OpenAPI операции объявляет ошибки, которые отдаёт её usecase (список — по коду usecase). Что ни
// одна ошибка домена не уходит 500, держит таблица verificationErrors и archtest TestDomainErrorTables.
func TestErrorMaps(t *testing.T) {
	t.Parallel()
	tests := []struct {
		op   string
		m    httpkit.ErrorMap
		errs []error
	}{
		{"confirmPasswordReset", confirmPasswordResetErrors, []error{
			verification.ErrWeakPassword, verification.ErrPasswordTooLong, verification.ErrInvalidResetToken, verification.ErrTokenUsed,
		}},
		{"confirmEmail", confirmEmailErrors, []error{verification.ErrInvalidVerificationToken, verification.ErrTokenUsed}},
	}
	for _, tc := range tests {
		t.Run(tc.op, func(t *testing.T) {
			t.Parallel()
			require.Empty(t, tc.m.Unmapped(tc.errs...))
		})
	}
}
