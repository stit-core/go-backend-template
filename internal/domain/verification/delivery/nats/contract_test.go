package nats_test

import (
	"context"
	"encoding/json"
	"slices"
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
	"git.stit.tech/stit-core/golibinfra/natskit"
	"git.stit.tech/stit-core/golibinfra/testkit/audittest"
	"git.stit.tech/stit-core/golibinfra/testkit/authntest"
	"git.stit.tech/stit-core/golibinfra/testkit/natstest"
	"git.stit.tech/stit-core/golibinfra/testkit/outboxtest"
	"git.stit.tech/stit-core/golibinfra/testkit/pgtest"
)

// Контракт подписок verification с поставщиками: событие, которое публикует настоящий usecase поставщика, — в
// JSON, как его отдаёт outbox (outboxkit.Bus), — доставляется подпискам Register и разбирается их схемой: тот же
// subject, те же поля. Подписка, которой здесь не досталось события поставщика, — отказ: её схему ничто не сверило.
func TestContracts(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	events := slices.Concat(userRegistered(t, uc), passwordResetRequested(t, uc))
	bus := natstest.New(natstest.WithStream("user"), natstest.WithStream("verification"))
	lc := fxtest.NewLifecycle(t)
	verifnats.Register(lc, bus, verifnats.NewHandler(uc, logkit.Nop()))
	lc.RequireStart()
	t.Cleanup(lc.RequireStop)

	for _, e := range events {
		require.NoError(t, bus.Deliver(context.Background(), e.Subject, e.Payload), e.Subject)
	}
	for _, filter := range bus.Subscriptions() {
		require.True(t, slices.ContainsFunc(events, func(e outboxtest.Event) bool { return natskit.SubjectMatches(filter, e.Subject) }),
			"subscription %s gets no event of a provider usecase here: nothing checks its schema", filter)
	}
}

// userRegistered — событие регистрации user; подписка verification выпускает подтверждение адреса по ID из него.
func userRegistered(t *testing.T, uc *mocks.MockUsecase) []outboxtest.Event {
	t.Helper()
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
	uc.EXPECT().IssueEmailVerification(mock.Anything, "u1").Return(nil).Once()
	return published.All()
}

// passwordResetRequested — своё событие запроса сброса; подписка verification выпускает ссылку по ID запроса,
// учётной записи и времени из него.
func passwordResetRequested(t *testing.T, uc *mocks.MockUsecase) []outboxtest.Event {
	t.Helper()
	users := verifmocks.NewMockUserReader(t)
	users.EXPECT().GetByEmail(mock.Anything, "a@b.com").Return(&verification.User{ID: "u1", Email: "a@b.com", CanLogin: true}, nil).Once()
	published := &outboxtest.Recorder{}
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	request := verification.NewUsecase(verification.UsecaseDeps{
		Users: users, Tx: pgtest.Identity{}, Events: published, Log: logkit.Nop(), Clock: clockkit.NewFixed(at),
	})
	require.NoError(t, request.RequestPasswordReset(context.Background(), "a@b.com"))
	events := published.All()
	require.Len(t, events, 1)
	var sent struct{ RequestID string }
	require.NoError(t, json.Unmarshal(events[0].Payload, &sent))
	uc.EXPECT().IssuePasswordReset(mock.Anything, sent.RequestID, "u1", at).Return(nil).Once()
	return events
}
