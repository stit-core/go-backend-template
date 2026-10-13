package nats_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx/fxtest"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/user"
	usermocks "git.stit.tech/stit-core/go-backend-template/internal/domain/user/mocks"
	"git.stit.tech/stit-core/go-backend-template/internal/domain/verification"
	verifnats "git.stit.tech/stit-core/go-backend-template/internal/domain/verification/delivery/nats"
	"git.stit.tech/stit-core/go-backend-template/internal/domain/verification/delivery/nats/mocks"
	verifmocks "git.stit.tech/stit-core/go-backend-template/internal/domain/verification/mocks"
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
	bus := natstest.New(natstest.WithStream("user"), natstest.WithStream("verification"))
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

// Контракт своего события: что публикует запрос сброса verification, то её подписка принимает и разбирает
// своей схемой — тот же subject, тот же ID запроса, учётная запись и время.
func TestPasswordResetRequestedContract(t *testing.T) {
	t.Parallel()
	users := verifmocks.NewMockUserReader(t)
	users.EXPECT().GetByEmail(mock.Anything, "a@b.com").Return(&verification.User{ID: "u1", Email: "a@b.com", CanLogin: true}, nil).Once()
	limiter := verifmocks.NewMockResetLimiter(t)
	limiter.EXPECT().Allow(mock.Anything, "u1", mock.Anything).Return(true, nil).Once()
	published := &outboxtest.Recorder{}
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	request := verification.NewUsecase(verification.UsecaseDeps{
		Users: users, Limiter: limiter, Tx: pgtest.Identity{}, Events: published, Log: logkit.Nop(), Clock: clockkit.NewFixed(at),
	})
	require.NoError(t, request.RequestPasswordReset(context.Background(), "a@b.com"))
	events := published.All()
	require.Len(t, events, 1)
	var sent struct{ RequestID string }
	require.NoError(t, json.Unmarshal(events[0].Payload, &sent))

	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().IssuePasswordReset(mock.Anything, sent.RequestID, "u1", at).Return(nil).Once()
	bus := natstest.New(natstest.WithStream("user"), natstest.WithStream("verification"))
	lc := fxtest.NewLifecycle(t)
	verifnats.Register(lc, bus, verifnats.NewHandler(uc, logkit.Nop()))
	lc.RequireStart()
	t.Cleanup(lc.RequireStop)

	require.NoError(t, bus.Deliver(context.Background(), events[0].Subject, events[0].Payload), events[0].Subject)
}
