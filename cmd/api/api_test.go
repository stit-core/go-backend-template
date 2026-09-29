package main

import (
	"regexp"
	"slices"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"git.stit.tech/stit-core/golibinfra/httpkit"
	"git.stit.tech/stit-core/golibinfra/logkit"

	"git.stit.tech/stit-core/go-backend-template/internal/platform/authgate"
)

// buildAPI собирает HTTP API так же, как сервис — сервер httpkit с /health и /ready, схема
// Cookie, registerHTTP, — но без БД, брокера и listen: хендлеры пустые.
func buildAPI(t *testing.T, opts ...fx.Option) huma.API {
	t.Helper()
	var api huma.API
	app := fx.New(
		fx.NopLogger,
		fx.RecoverFromPanics(),
		fx.Provide(httpkit.NewConfig, logkit.Nop),
		httpkit.Module,
		authgate.SecurityScheme,
		fx.Invoke(func(api huma.API) { registerHTTP(api, httpHandlers{}) }),
		fx.Options(opts...),
		fx.Populate(&api),
	)
	require.NoError(t, app.Err())
	return api
}

// publicOperations — операции без Security: пробы kit'а, вход и восстановление доступа.
var publicOperations = []string{
	"get-health", "get-ready",
	"registerUser", "loginUser", "refreshToken", "logoutUser",
	"requestPasswordReset", "confirmPasswordReset", "confirmEmail",
}

var pathParam = regexp.MustCompile(`\{[^}]*\}`)

// TestHTTPOperations: операция без Security публична в рантайме (SEC-03), поэтому она либо
// под Security, либо в publicOperations. Метод и путь уникальны: huma и chi молча
// перезаписывают маршрут, а дубль OperationID huma роняет паникой — она здесь и видна.
func TestHTTPOperations(t *testing.T) {
	t.Parallel()
	var ops []*huma.Operation
	record := func(c *huma.Config) {
		c.OnAddOperation = append(c.OnAddOperation, func(_ *huma.OpenAPI, op *huma.Operation) { ops = append(ops, op) })
	}
	buildAPI(t, fx.Supply(fx.Annotated{Group: httpkit.HumaConfigGroup, Target: record}))
	require.NotEmpty(t, ops)

	routes := map[string]string{}
	var public []string
	for _, op := range ops {
		route := op.Method + " " + pathParam.ReplaceAllString(op.Path, "{}")
		if prev, dup := routes[route]; dup {
			t.Errorf("operations %s and %s share the route %s", prev, op.OperationID, route)
		}
		routes[route] = op.OperationID
		switch {
		case len(op.Security) > 0:
		case slices.Contains(publicOperations, op.OperationID):
			public = append(public, op.OperationID)
		default:
			t.Errorf("operation %s has no Security and is not in publicOperations", op.OperationID)
		}
	}
	for _, id := range publicOperations {
		if !slices.Contains(public, id) {
			t.Errorf("publicOperations lists %s, but no public operation has this ID", id)
		}
	}
}
