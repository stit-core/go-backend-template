// White-box (package nats): обработчик onUserRegistered неэкспортируемый.
package nats

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx/fxtest"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/verification/delivery/nats/mocks"
	"git.stit.tech/stit-core/golibinfra/logkit"
	"git.stit.tech/stit-core/golibinfra/natskit"
	"git.stit.tech/stit-core/golibinfra/testkit/logtest"
	"git.stit.tech/stit-core/golibinfra/testkit/natstest"
)

func testEvent() UserRegisteredEvent {
	return UserRegisteredEvent{
		ID: "3f1d2c4b-5a6e-4f70-8b91-0c2d3e4f5a6b",
		At: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
}

func TestOnUserRegistered(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		ucErr error
	}{
		{name: "valid payload issues verification", ucErr: nil},
		{name: "usecase error propagates for redelivery", ucErr: errors.New("issue failed")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			e := testEvent()
			uc := mocks.NewMockUsecase(t)
			uc.EXPECT().IssueEmailVerification(mock.Anything, e.ID).Return(tc.ucErr).Once()

			log := logtest.CaptureAt(slog.LevelWarn)
			err := NewHandler(uc, log).onUserRegistered(context.Background(), e)

			if tc.ucErr != nil {
				require.ErrorIs(t, err, tc.ucErr)
				require.ErrorContains(t, err, e.ID, "the broker's log names the user by ID")
				require.Empty(t, log.Entries(), "natskit logs the error with the redelivery: the handler only returns it")
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestOnUserRegistered_DuplicateDelivery(t *testing.T) {
	t.Parallel()

	e := testEvent()
	uc := mocks.NewMockUsecase(t)
	// Dedup lives in the usecase (the schema records the sent email): on
	// redelivery the handler must call it again, not short-circuit.
	uc.EXPECT().IssueEmailVerification(mock.Anything, e.ID).Return(nil).Times(2)

	h := NewHandler(uc, logkit.Nop())
	require.NoError(t, h.onUserRegistered(context.Background(), e))
	require.NoError(t, h.onUserRegistered(context.Background(), e))
}

// streamSubs — шина natstest, которая помнит durable каждой stream-подписки.
type streamSubs struct {
	*natstest.Recorder
	durables map[string]string
}

func (b *streamSubs) SubscribeStream(ctx context.Context, stream, subject, durable string, h natskit.Handler) (natskit.Subscription, error) {
	b.durables[subject] = durable
	return b.Recorder.SubscribeStream(ctx, stream, subject, durable, h)
}

// Событие до перехода на .v2 (с Email) дочитывает прежний durable — с места, где остановился прежний
// консьюмер, — тем же обработчиком: строка outbox, записанная до выкатки, и недоставленное письма не теряют.
func TestRegister(t *testing.T) {
	t.Parallel()
	bus := &streamSubs{Recorder: natstest.New(natstest.WithStream("user")), durables: map[string]string{}}
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().IssueEmailVerification(mock.Anything, "u-v2").Return(nil).Once()
	uc.EXPECT().IssueEmailVerification(mock.Anything, "u-v1").Return(nil).Once()
	lc := fxtest.NewLifecycle(t)
	Register(lc, bus, NewHandler(uc, logkit.Nop()))
	lc.RequireStart()
	t.Cleanup(lc.RequireStop)

	require.Equal(t, map[string]string{
		"user.events.registered.v2": "verification-user-registered-v2",
		"user.events.registered":    "verification-user-registered",
	}, bus.durables)
	ctx := context.Background()
	require.NoError(t, bus.Deliver(ctx, "user.events.registered.v2", []byte(`{"ID":"u-v2","At":"2026-01-02T03:04:05Z"}`)))
	require.NoError(t, bus.Deliver(ctx, "user.events.registered", []byte(`{"ID":"u-v1","Email":"a@b.com","At":"2026-01-02T03:04:05Z"}`)))
}
