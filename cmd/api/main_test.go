package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"git.stit.tech/stit-core/golibinfra/httpkit"
	"git.stit.tech/stit-core/golibinfra/natskit"

	appconfig "git.stit.tech/stit-core/go-backend-template/internal/platform/config"
	"git.stit.tech/stit-core/go-backend-template/internal/testkit/allowlist"
)

// mainArgsEnv — аргументы процесса api: тестовый бинарь, перезапущенный с ней тестом, вместо тестов
// запускает main — так тест проверяет разбор подкоманд в main.
const mainArgsEnv = "API_TEST_MAIN_ARGS"

// TestMain отличает перезапуск по пустой командной строке: go test всегда передаёт бинарю флаги -test.*,
// и переменная, унаследованная самим прогоном, тестов не выключает.
func TestMain(m *testing.M) {
	if args, ok := os.LookupEnv(mainArgsEnv); ok && len(os.Args) == 1 {
		os.Args = append([]string{"api"}, strings.Fields(args)...)
		main()
		return
	}
	os.Exit(m.Run())
}

// Отказ старта пишет логгер событий сервиса (logkit.FxLogger): JSON-записи ERROR в stdout с причиной, а не
// текст консольного логгера fx в stderr. Тест запускает свой бинарь как `api` без окружения (TestMain зовёт
// main): первым падает конфиг без обязательной переменной.
func TestMainLogsStartFailure(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0])
	cmd.Env = []string{mainArgsEnv + "="}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	err := cmd.Run()
	require.NotNil(t, cmd.ProcessState, "api did not run: %v", err)
	require.Equal(t, 1, cmd.ProcessState.ExitCode(), "exit code of api without env, stderr:\n%s", stderr.String())
	require.NotContains(t, stderr.String(), "[Fx]", "start failure goes to the service log, not to the console logger of fx")
	var failed bool
	for line := range bytes.SplitSeq(bytes.TrimSpace(stdout.Bytes()), []byte("\n")) {
		var rec struct{ Level, Msg, Error string }
		require.NoError(t, json.Unmarshal(line, &rec), "service log line is not JSON: %s", line)
		require.Equal(t, "ERROR", rec.Level, "service log line: %s", line)
		failed = failed || rec.Msg == "start failed" && strings.Contains(rec.Error, "is required")
	}
	require.True(t, failed, "the service log names the required env behind the failed start:\n%s", stdout.String())
}

// Граф fx должен быть разрешим (все зависимости предоставлены), включая per-domain
// outbox (fx.Private в доменных модулях) и durable JetStream subscriber поверх
// natskit.StreamBus. ValidateApp проверяет wiring без запуска lifecycle и без
// обращения к pg/nats.
func TestFxGraphValidates(t *testing.T) {
	require.NoError(t, fx.ValidateApp(service, fx.NopLogger), "fx graph invalid")
}

// TestFxGraphHasNoContext: компонент графа живёт дольше любого ctx — свой ctx он получает от хука
// или запроса, а не из графа. Видит ctx, доступный корню: именованный и fx.Private — нет.
func TestFxGraphHasNoContext(t *testing.T) {
	t.Parallel()
	err := fx.ValidateApp(service, fx.NopLogger, fx.Invoke(func(context.Context) {}))
	require.ErrorContains(t, err, "missing type: context.Context", "the fx graph provides a context.Context to its root")
}

// envAllowlist — required-переменные конфигов, которых нет в .env.example.
var envAllowlist = allowlist.List{}

var envLine = regexp.MustCompile(`(?m)^([A-Z][A-Z0-9_]*)=(.*)$`)

// envExample — переменные .env.example: минимум, с которым сервис стартует (boot-тест берёт env из него).
func envExample(t *testing.T) map[string]string {
	t.Helper()
	data, err := os.ReadFile("../../.env.example")
	require.NoError(t, err)
	env := map[string]string{}
	for _, m := range envLine.FindAllStringSubmatch(string(data), -1) {
		env[m[1]] = strings.TrimSpace(m[2])
	}
	return env
}

// TestEnvExampleHasRequired: .env.example — минимум, с которым сервис стартует; required-
// переменная конфига, которой там нет, роняет запуск по примеру.
func TestEnvExampleHasRequired(t *testing.T) {
	t.Parallel()
	example := envExample(t)
	configs := []reflect.Type{reflect.TypeFor[appconfig.App]()}
	for _, ctor := range kitConfigs {
		configs = append(configs, reflect.TypeOf(ctor).Out(0))
	}
	var missing []string
	for _, c := range configs {
		for _, key := range requiredEnv(c) {
			if _, ok := example[key]; !ok && !slices.Contains(missing, key) {
				missing = append(missing, key)
			}
		}
	}
	envAllowlist.Check(t, missing, "a required env of the service configs is missing from .env.example")
}

// requiredEnv — переменные с required:"true" без default в конфиге typ, включая вложенные.
func requiredEnv(typ reflect.Type) []string {
	var keys []string
	for i := range typ.NumField() {
		f := typ.Field(i)
		switch {
		case !f.IsExported():
		case f.Type.Kind() == reflect.Struct && f.Tag.Get("envconfig") == "":
			keys = append(keys, requiredEnv(f.Type)...)
		case f.Tag.Get("required") == "true" && f.Tag.Get("default") == "":
			keys = append(keys, f.Tag.Get("envconfig"))
		}
	}
	return keys
}

// Остановка из env укладывается в бюджет fx.StopTimeout: HTTP — в ⅔, дренаж NATS — в треть.
func TestCheckStopBudget(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name              string
		drain, http, nats time.Duration
		wantErr           string // "" — помещается
	}{
		{name: "kit defaults", http: 10 * time.Second, nats: 10 * time.Second},
		{name: "behind a load balancer", drain: 5 * time.Second, http: 10 * time.Second, nats: 10 * time.Second},
		{name: "http over two thirds", drain: 15 * time.Second, http: 10 * time.Second, nats: 10 * time.Second, wantErr: "HTTP_SHUTDOWN_DRAIN_DELAY + HTTP_SHUTDOWN_TIMEOUT = 25s"},
		{name: "nats drain over a third", http: 10 * time.Second, nats: 11 * time.Second, wantErr: "NATS_DRAIN_TIMEOUT = 11s"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := checkStopBudget(httpkit.Config{ShutdownDrainDelay: tc.drain, ShutdownTimeout: tc.http}, natskit.Config{DrainTimeout: tc.nats})
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, errStopBudget)
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}
