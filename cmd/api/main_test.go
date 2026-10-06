package main

import (
	"context"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

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

// TestEnvExampleHasRequired: .env.example — минимум, с которым сервис стартует; required-
// переменная конфига, которой там нет, роняет запуск по примеру.
func TestEnvExampleHasRequired(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../../.env.example")
	require.NoError(t, err)
	example := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^([A-Z][A-Z0-9_]*)=`).FindAllStringSubmatch(string(data), -1) {
		example[m[1]] = true
	}
	configs := []reflect.Type{reflect.TypeFor[appconfig.App]()}
	for _, ctor := range kitConfigs {
		configs = append(configs, reflect.TypeOf(ctor).Out(0))
	}
	var missing []string
	for _, c := range configs {
		for _, key := range requiredEnv(c) {
			if !example[key] && !slices.Contains(missing, key) {
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
