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
	"git.stit.tech/stit-core/golibinfra/authnkit"
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

// Модуль отдаёт Hasher платформы (Argon2id): пароль сверяется со своим хешем и не сверяется с чужим.
func TestModule_Hasher(t *testing.T) {
	t.Parallel()
	var hasher authnkit.Hasher
	app := fx.New(fx.NopLogger, authgate.Module,
		fx.Supply(authnkit.Argon2idConfig{Time: 1, Memory: 8 * 1024, Threads: 1, KeyLen: 32, SaltLen: 16}),
		fx.Populate(&hasher))
	require.NoError(t, app.Err())

	hash, err := hasher.Hash(context.Background(), "password123")
	require.NoError(t, err)
	ok, err := hasher.Verify(context.Background(), "password123", hash)
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = hasher.Verify(context.Background(), "wrong-password", hash)
	require.NoError(t, err)
	require.False(t, ok)
}
