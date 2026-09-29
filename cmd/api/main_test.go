package main

import (
	"os"
	"reflect"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	appconfig "git.stit.tech/stit-core/go-backend-template/internal/platform/config"
)

// Граф fx должен быть разрешим (все зависимости предоставлены), включая per-domain
// outbox (fx.Private в доменных модулях) и durable JetStream subscriber поверх
// natskit.StreamBus. ValidateApp проверяет wiring без запуска lifecycle и без
// обращения к pg/nats.
func TestFxGraphValidates(t *testing.T) {
	require.NoError(t, fx.ValidateApp(service, fx.NopLogger), "fx graph invalid")
}

// envAllowlist — required-переменные конфигов, которых нет в .env.example: переменная → ID
// находки; опустошает fix-reference-code.
var envAllowlist = map[string]string{"JWT_SECRET": "SEC-10"}

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
	missing := map[string]string{}
	for _, c := range configs {
		for _, key := range requiredEnv(c) {
			if !example[key] {
				missing[key] = envAllowlist[key]
			}
		}
	}
	require.Equal(t, envAllowlist, missing,
		"required env of the service configs belongs in .env.example; envAllowlist lists exactly what is still missing")
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
