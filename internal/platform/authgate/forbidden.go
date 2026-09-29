package authgate

import (
	"errors"
	"net/http"

	"git.stit.tech/stit-core/go-backend-template/pkg/policy"
	"git.stit.tech/stit-core/golibinfra/httpkit"
)

// ForbiddenRule — отказ policy → 403 FORBIDDEN: общее правило таблицы ошибок каждого домена.
var ForbiddenRule = httpkit.ErrorRule{Match: isForbidden, Status: http.StatusForbidden, Code: "FORBIDDEN", Detail: "forbidden"}

func isForbidden(err error) bool {
	var fe *policy.ForbiddenError
	return errors.As(err, &fe)
}
