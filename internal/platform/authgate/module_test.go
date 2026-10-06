package authgate_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"git.stit.tech/stit-core/go-backend-template/internal/platform/authgate"
	"git.stit.tech/stit-core/go-backend-template/internal/platform/identity"
	"git.stit.tech/stit-core/golibinfra/logkit"
)

// Запись лога несёт user_id личности из ctx, запись без личности — нет: поле регистрирует модуль
// authgate, logkit берёт его из ctx каждой записи — так строки одного запроса связаны с пользователем.
func TestModule_LogsUserID(t *testing.T) {
	t.Parallel()
	require.NoError(t, fx.New(authgate.Module, fx.NopLogger).Err())
	var out bytes.Buffer
	log, err := logkit.New(logkit.Config{Level: "info"}, logkit.WithWriter(&out))
	require.NoError(t, err)

	log.Info(identity.With(context.Background(), identity.Identity{UserID: "u1", Role: identity.RoleClient}), "with identity")
	log.Info(context.Background(), "anonymous")

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	require.Len(t, lines, 2)
	records := make([]map[string]any, len(lines))
	for i, line := range lines {
		require.NoError(t, json.Unmarshal([]byte(line), &records[i]))
	}
	require.Equal(t, "u1", records[0]["user_id"])
	require.NotContains(t, records[1], "user_id")
}
