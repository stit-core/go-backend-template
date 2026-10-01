package main

import (
	"encoding/json"
	"flag"
	"os"
	"regexp"
	"slices"
	"strings"
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

var update = flag.Bool("update", false, "rewrite openapi.json from the registered operations (make gen)")

// openAPIFile — контракт API в корне репо: одна спецификация на сервис.
const openAPIFile = "../../openapi.json"

// TestOpenAPISpec: openapi.json пишет make gen (-update) из тех же регистраций, что у сервиса,
// при конфиге по умолчанию; без флага тест ловит расхождение файла с кодом.
func TestOpenAPISpec(t *testing.T) {
	unsetHTTPEnv(t)
	spec, err := json.MarshalIndent(buildAPI(t).OpenAPI(), "", "  ")
	require.NoError(t, err)
	spec = append(spec, '\n')
	if *update {
		require.NoError(t, os.WriteFile(openAPIFile, spec, 0o644))
		return
	}
	file, err := os.ReadFile(openAPIFile)
	require.NoError(t, err)
	require.Equal(t, string(file), string(spec), "openapi.json is out of date: run make gen")
}

// unsetHTTPEnv убирает HTTP_* из окружения на время теста: спецификация — при конфиге по умолчанию,
// а не при том, что выставлено в шелле.
func unsetHTTPEnv(t *testing.T) {
	t.Helper()
	for _, kv := range os.Environ() {
		if key, _, _ := strings.Cut(kv, "="); strings.HasPrefix(key, "HTTP_") {
			t.Setenv(key, "") // вернёт прежнее значение после теста
			require.NoError(t, os.Unsetenv(key))
		}
	}
}

// publicOperations — операции без Security: пробы kit'а, вход и восстановление доступа.
var publicOperations = []string{
	"get-health", "get-ready",
	"registerUser", "loginUser", "refreshToken", "logoutUser",
	"requestPasswordReset", "confirmPasswordReset", "confirmEmail",
}

var pathParam = regexp.MustCompile(`\{[^}]*\}`)

// TestHTTPOperations: операция без Security публична в рантайме, поэтому она либо
// под Security, либо в publicOperations. Метод и путь уникальны: huma и chi молча
// перезаписывают маршрут, а дубль OperationID huma роняет паникой — она здесь и видна.
func TestHTTPOperations(t *testing.T) {
	t.Parallel()
	var ops []*huma.Operation
	record := func(c *huma.Config) {
		c.OnAddOperation = append(c.OnAddOperation, func(_ *huma.OpenAPI, op *huma.Operation) { ops = append(ops, op) })
	}
	buildAPI(t, fx.Supply(httpkit.HumaConfigs{record}))
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
			if resp := op.Responses["401"]; resp == nil || !strings.Contains(resp.Description, "AUTH_ACCESS_EXPIRED") {
				t.Errorf("operation %s is under Security without the session 401 codes in OpenAPI: register it through authgate.Secured", op.OperationID)
			}
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
