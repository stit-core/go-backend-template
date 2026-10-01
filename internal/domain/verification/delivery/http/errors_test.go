// White-box (package http): таблицы ошибок операций неэкспортируемые.
package http

import (
	"testing"

	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/verification"
	"git.stit.tech/stit-core/golibinfra/httpkit"
)

// Ошибки домена, которые usecase переводит сам: отсутствие пользователя — анти-перебор у запроса
// сброса и недействительный токен у подтверждений (archtest сверяет со списком errors.go).
var internalErrors = []error{verification.ErrUserNotFound}

// Каждая ошибка, которую отдаёт usecase операции, — в таблице операции: свой code, а не 500.
func TestErrorMaps(t *testing.T) {
	t.Parallel()
	tests := []struct {
		op   string
		m    httpkit.ErrorMap
		errs []error
	}{
		{"confirmPasswordReset", confirmPasswordResetErrors, []error{
			verification.ErrWeakPassword, verification.ErrInvalidResetToken, verification.ErrTokenUsed, verification.ErrConcurrentUpdate,
		}},
		{"confirmEmail", confirmEmailErrors, []error{verification.ErrInvalidVerificationToken, verification.ErrTokenUsed}},
	}
	for _, tc := range tests {
		t.Run(tc.op, func(t *testing.T) {
			t.Parallel()
			require.Empty(t, tc.m.Unmapped(tc.errs...))
			require.Equal(t, internalErrors, tc.m.Unmapped(internalErrors...), "an internal error is not part of the HTTP contract")
		})
	}
}
