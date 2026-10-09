package events_test

import (
	"context"
	"encoding/json"
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

// repo — хранилище outbox теста: помнит subject'ы вставок.
type repo struct{ inserted []string }

func (r *repo) Insert(_ context.Context, subject string, _ json.RawMessage, _ int) error {
	r.inserted = append(r.inserted, subject)
	return nil
}

func (r *repo) ListUnpublished(context.Context, int, outboxkit.PublishMode) ([]outboxkit.Message, error) {
	return nil, nil
}
func (r *repo) MarkPublished(context.Context, []outboxkit.MessageRef) error { return nil }
func (r *repo) RecordFailures(context.Context, []outboxkit.Failure) error   { return nil }

// factory — хранилища outbox теста по таблице.
type factory struct{ repos map[string]*repo }

func (f factory) Repository(table string) (outboxkit.Repository, error) {
	f.repos[table] = &repo{}
	return f.repos[table], nil
}

func (f factory) Migrations(table string) pgkit.Source { return pgkit.Source{Name: table} }

// kit — зависимости outboxkit.Instance: хранилища теста, без шины и транзакций.
func kit(f factory) fx.Option {
	return fx.Options(timerkit.Module, fx.Provide(
		func() outboxkit.RepositoryFactory { return f },
		func() natskit.StreamBus { return nil },
		func() pgkit.Transactor { return nil },
		outboxkit.NewConfig,
		logkit.Nop,
	))
}

// Publisher домена виден только модулю, в который включён OutboxFor, и пишет в таблицу домена только
// события под `<name>.>`.
func TestOutboxFor(t *testing.T) {
	t.Parallel()
	f := factory{repos: map[string]*repo{}}
	var publisher outboxkit.Publisher
	app := fx.New(fx.NopLogger, kit(f),
		fx.Module("order", events.OutboxFor("order", "outbox_order"), fx.Invoke(func(p outboxkit.Publisher) { publisher = p })))
	require.NoError(t, app.Err())

	require.NoError(t, publisher.Publish(context.Background(), "order.events.created", map[string]string{"id": "o1"}))
	require.Error(t, publisher.Publish(context.Background(), "user.events.created", map[string]string{"id": "u1"}),
		"a subject outside order.> stays out of the outbox of order")
	require.Equal(t, []string{"order.events.created"}, f.repos["outbox_order"].inserted)

	// Чужой модуль Publisher домена не получает: fx.Private.
	err := fx.ValidateApp(fx.NopLogger, kit(factory{repos: map[string]*repo{}}),
		fx.Module("order", events.OutboxFor("order", "outbox_order")), fx.Invoke(func(outboxkit.Publisher) {}))
	require.ErrorContains(t, err, "missing type: outboxkit.Publisher")
}
