// White-box (package http): таблицы ошибок операций неэкспортируемые.
package http

import (
	"testing"

	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/user"
	"git.stit.tech/stit-core/go-backend-template/pkg/policy"
	"git.stit.tech/stit-core/golibinfra/httpkit"
)

// Ошибки домена, которые до HTTP не доходят (archtest сверяет со списком errors.go вместе с
// операциями ниже).
var internalErrors = []error{user.ErrInvalidRole}

// Каждая ошибка, которую отдаёт usecase операции, — в таблице операции: свой code, а не 500.
func TestErrorMaps(t *testing.T) {
	t.Parallel()
	forbidden := &policy.ForbiddenError{Reason: "test"}
	tests := []struct {
		op   string
		m    httpkit.ErrorMap
		errs []error
	}{
		{"registerUser", registerErrors, []error{user.ErrWeakPassword, user.ErrInvalidEmail, user.ErrAlreadyExists}},
		{"getMe", meErrors, []error{user.ErrNotFound, forbidden}},
		{"listUsers", listErrors, []error{forbidden}},
		{"getUser", getErrors, []error{user.ErrNotFound, forbidden}},
		{"updateUser", updateErrors, []error{user.ErrNotFound, user.ErrNoUpdateFields, user.ErrConcurrentUpdate, user.ErrInvalidStatusTransition, forbidden}},
		{"deleteUser", deleteErrors, []error{user.ErrNotFound, forbidden}},
	}
	for _, tc := range tests {
		t.Run(tc.op, func(t *testing.T) {
			t.Parallel()
			require.Empty(t, tc.m.Unmapped(tc.errs...))
			require.Equal(t, internalErrors, tc.m.Unmapped(internalErrors...), "an internal error is not part of the HTTP contract")
		})
	}
}
