package events_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"git.stit.tech/stit-core/go-backend-template/internal/platform/events"
	"git.stit.tech/stit-core/golibinfra/logkit"
	"git.stit.tech/stit-core/golibinfra/natskit"
	"git.stit.tech/stit-core/golibinfra/outboxkit"
	"git.stit.tech/stit-core/golibinfra/pgkit"
	"git.stit.tech/stit-core/golibinfra/timerkit"
)

// Publisher домена виден только модулю, в который включён OutboxFor: чужой модуль публиковать в его
// outbox не может.
func TestOutboxFor(t *testing.T) {
	t.Parallel()
	// ValidateApp конструкторов не вызывает: графу нужны только типы зависимостей outboxkit.Instance.
	kit := fx.Options(timerkit.Module, fx.Provide(
		func() outboxkit.RepositoryFactory { return nil },
		func() natskit.StreamBus { return nil },
		func() pgkit.Transactor { return nil },
		func() outboxkit.Config { return outboxkit.Config{} },
		logkit.Nop,
	))
	publish := fx.Invoke(func(outboxkit.Publisher) {})

	require.NoError(t, fx.ValidateApp(kit, fx.Module("order", events.OutboxFor("order", "outbox_order"), publish)))
	require.ErrorContains(t, fx.ValidateApp(kit, fx.Module("order", events.OutboxFor("order", "outbox_order")), publish),
		"missing type: outboxkit.Publisher")
}
