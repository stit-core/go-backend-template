// White-box (package http): таблицы ошибок неэкспортируемые.
package http

import (
	"testing"

	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/user"
	"git.stit.tech/stit-core/go-backend-template/pkg/policy"
	"git.stit.tech/stit-core/golibinfra/httpkit"
)

// OpenAPI операции объявляет ошибки, которые отдаёт её usecase (список — по коду usecase). Что ни
// одна ошибка домена не уходит 500, держит таблица userErrors и archtest TestDomainErrorTables.
func TestErrorMaps(t *testing.T) {
	t.Parallel()
	forbidden := &policy.ForbiddenError{Reason: "test"}
	tests := []struct {
		op   string
		m    httpkit.ErrorMap
		errs []error
	}{
		{"registerUser", registerErrors, []error{user.ErrWeakPassword, user.ErrInvalidEmail, user.ErrInvalidName, user.ErrAlreadyExists}},
		{"getMe", meErrors, []error{user.ErrNotFound, forbidden}},
		{"listUsers", listErrors, []error{forbidden}},
		{"getUser", getErrors, []error{user.ErrNotFound, forbidden}},
		{"updateUser", updateErrors, []error{user.ErrNotFound, user.ErrNoUpdateFields, user.ErrInvalidName, user.ErrConcurrentUpdate, user.ErrInvalidStatusTransition, forbidden}},
		{"deleteUser", deleteErrors, []error{user.ErrNotFound, forbidden}},
	}
	for _, tc := range tests {
		t.Run(tc.op, func(t *testing.T) {
			t.Parallel()
			require.Empty(t, tc.m.Unmapped(tc.errs...))
		})
	}
}
