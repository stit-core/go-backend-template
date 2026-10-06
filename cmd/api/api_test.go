package main

import (
	"context"
	"encoding/json"
	"flag"
	"net/http"
	"os"
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
func buildAPI(t *testing.T, opts ...fx.Option) *fx.App {
	t.Helper()
	app := fx.New(
		fx.NopLogger,
		fx.RecoverFromPanics(),
		fx.Provide(httpkit.NewConfig, logkit.Nop),
		httpkit.Module,
		authgate.SecurityScheme,
		fx.Invoke(func(api huma.API) { registerHTTP(api, httpHandlers{}) }),
		fx.Options(opts...),
	)
	require.NoError(t, app.Err())
	return app
}

var update = flag.Bool("update", false, "rewrite openapi.json from the registered operations (make gen)")

// openAPIFile — контракт API в корне репо: одна спецификация на сервис.
const openAPIFile = "../../openapi.json"

// TestOpenAPISpec: openapi.json пишет make gen (-update) из тех же регистраций, что у сервиса,
// при конфиге по умолчанию; без флага тест ловит расхождение файла с кодом.
func TestOpenAPISpec(t *testing.T) {
	unsetHTTPEnv(t)
	var api huma.API
	buildAPI(t, fx.Populate(&api))
	spec, err := json.MarshalIndent(api.OpenAPI(), "", "  ")
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

// httpkitPaths — пробы, которые регистрирует сам httpkit; пути операций доменов — под /api/.
var httpkitPaths = []string{"/health", "/ready"}

// TestHTTPOperations: API сервиса проходит проверку старта httpkit (Server.CheckRoutes) — у каждой
// операции Security или пометка httpkit.Public, METHOD+path не повторяются, — пути операций доменов
// живут под /api/, а операция под Security объявляет коды 401 сессии: её регистрирует authgate.Secured.
func TestHTTPOperations(t *testing.T) {
	t.Parallel()
	var (
		ops []*huma.Operation
		srv *httpkit.Server
	)
	record := func(c *huma.Config) {
		c.OnAddOperation = append(c.OnAddOperation, func(_ *huma.OpenAPI, op *huma.Operation) { ops = append(ops, op) })
	}
	buildAPI(t, fx.Supply(httpkit.HumaConfigs{record}), fx.Populate(&srv))
	require.NoError(t, srv.CheckRoutes())

	secured := 0
	for _, op := range ops {
		if !strings.HasPrefix(op.Path, "/api/") && !slices.Contains(httpkitPaths, op.Path) {
			t.Errorf("operation %s at %s: domain operations live under /api/", op.OperationID, op.Path)
		}
		if len(op.Security) == 0 {
			continue
		}
		secured++
		if resp := op.Responses["401"]; resp == nil || !strings.Contains(resp.Description, "AUTH_ACCESS_EXPIRED") {
			t.Errorf("operation %s is under Security without the session 401 codes in OpenAPI: register it through authgate.Secured", op.OperationID)
		}
	}
	require.NotZero(t, secured, "the API has operations under Security")
}

// Операция без Security и без пометки httpkit.Public — ошибка старта сервиса: открытой операцию делает
// только явная пометка.
func TestHTTPOperations_UnmarkedFailsStart(t *testing.T) {
	t.Parallel()
	app := buildAPI(t, fx.Invoke(func(api huma.API) {
		huma.Register(api, huma.Operation{OperationID: "getUnmarked", Method: http.MethodGet, Path: "/api/unmarked"},
			func(context.Context, *struct{}) (*struct{}, error) { return nil, nil })
	}))

	err := app.Start(t.Context())
	require.ErrorIs(t, err, httpkit.ErrRoutes)
	require.ErrorContains(t, err, "getUnmarked has no Security and is not httpkit.Public")
}
