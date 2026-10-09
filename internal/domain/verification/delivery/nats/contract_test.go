package nats_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx/fxtest"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/user"
	usermocks "git.stit.tech/stit-core/go-backend-template/internal/domain/user/mocks"
	verifnats "git.stit.tech/stit-core/go-backend-template/internal/domain/verification/delivery/nats"
	"git.stit.tech/stit-core/go-backend-template/internal/domain/verification/delivery/nats/mocks"
	"git.stit.tech/stit-core/golibinfra/clockkit"
	"git.stit.tech/stit-core/golibinfra/logkit"
	"git.stit.tech/stit-core/golibinfra/testkit/audittest"
	"git.stit.tech/stit-core/golibinfra/testkit/authntest"
	"git.stit.tech/stit-core/golibinfra/testkit/natstest"
	"git.stit.tech/stit-core/golibinfra/testkit/outboxtest"
	"git.stit.tech/stit-core/golibinfra/testkit/pgtest"
)

// Контракт события регистрации между поставщиком и потребителем: что публикует регистрация user, то
// подписка verification принимает и разбирает своей схемой — тот же subject, тот же ID. Outbox отдаёт
// payload брокеру в JSON (outboxkit.Bus) — так его и доставляет тест.
func TestUserRegisteredContract(t *testing.T) {
	t.Parallel()
	repo := usermocks.NewMockRepository(t)
	repo.EXPECT().Insert(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, u *user.User) error {
		u.ID = "u1"
		return nil
	}).Once()
	published := &outboxtest.Recorder{}
	registration := user.NewUsecase(user.UsecaseDeps{
		Repo:   repo,
		Hasher: &authntest.DeterministicHasher{},
		Policy: user.NewPolicy(),
		Tx:     pgtest.Identity{},
		Events: published,
		Audit:  &audittest.Recorder{},
		Log:    logkit.Nop(),
		Clock:  clockkit.NewFixed(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)),
	})
	_, err := registration.Register(context.Background(), &user.CreateInput{Email: "a@b.com", Password: "longenough", Name: "Alice"})
	require.NoError(t, err)

	verification := mocks.NewMockUsecase(t)
	verification.EXPECT().IssueEmailVerification(mock.Anything, "u1").Return(nil).Once()
	bus := natstest.New(natstest.WithStream("user"))
	lc := fxtest.NewLifecycle(t)
	verifnats.Register(lc, bus, verifnats.NewHandler(verification, logkit.Nop()))
	lc.RequireStart()
	t.Cleanup(lc.RequireStop)

	events := published.All()
	require.NotEmpty(t, events)
	for _, e := range events {
		require.NoError(t, bus.Deliver(context.Background(), e.Subject, e.Payload), e.Subject)
	}
}
