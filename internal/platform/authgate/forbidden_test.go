package authgate_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/platform/authgate"
	"git.stit.tech/stit-core/go-backend-template/pkg/policy"
	"git.stit.tech/stit-core/golibinfra/httpkit"
)

func TestForbiddenRule(t *testing.T) {
	t.Parallel()
	errs := httpkit.NewErrorMap(authgate.ForbiddenRule)

	var apiErr *httpkit.APIError
	require.ErrorAs(t, errs.Map(fmt.Errorf("delete user: %w", &policy.ForbiddenError{Reason: "not self"})), &apiErr)
	require.Equal(t, http.StatusForbidden, apiErr.Status)
	require.Equal(t, "FORBIDDEN", apiErr.Code)
	require.NotContains(t, apiErr.Detail, "not self", "the policy reason stays out of the response")

	other := errors.New("db down")
	require.Equal(t, other, errs.Map(other), "an error that is not a policy denial is not forbidden")
}
